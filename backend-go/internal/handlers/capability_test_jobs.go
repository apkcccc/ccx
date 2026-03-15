package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/BenedictKing/ccx/internal/config"
	"github.com/gin-gonic/gin"
)

type CapabilityJobStatus string

type CapabilityProtocolStatus string

type CapabilityModelStatus string

const (
	CapabilityJobStatusQueued    CapabilityJobStatus = "queued"
	CapabilityJobStatusRunning   CapabilityJobStatus = "running"
	CapabilityJobStatusCompleted CapabilityJobStatus = "completed"
	CapabilityJobStatusFailed    CapabilityJobStatus = "failed"
	CapabilityJobStatusCancelled CapabilityJobStatus = "cancelled"
)

const (
	CapabilityProtocolStatusQueued    CapabilityProtocolStatus = "queued"
	CapabilityProtocolStatusRunning   CapabilityProtocolStatus = "running"
	CapabilityProtocolStatusCompleted CapabilityProtocolStatus = "completed"
	CapabilityProtocolStatusFailed    CapabilityProtocolStatus = "failed"
)

const (
	CapabilityModelStatusQueued  CapabilityModelStatus = "queued"
	CapabilityModelStatusRunning CapabilityModelStatus = "running"
	CapabilityModelStatusSuccess CapabilityModelStatus = "success"
	CapabilityModelStatusFailed  CapabilityModelStatus = "failed"
	CapabilityModelStatusSkipped CapabilityModelStatus = "skipped"
)

type CapabilityTestJobProgress struct {
	TotalModels     int `json:"totalModels"`
	QueuedModels    int `json:"queuedModels"`
	RunningModels   int `json:"runningModels"`
	SuccessModels   int `json:"successModels"`
	FailedModels    int `json:"failedModels"`
	SkippedModels   int `json:"skippedModels"`
	CompletedModels int `json:"completedModels"`
}

type CapabilityModelJobResult struct {
	Model              string                `json:"model"`
	Status             CapabilityModelStatus `json:"status"`
	Success            bool                  `json:"success"`
	Latency            int64                 `json:"latency"`
	StreamingSupported bool                  `json:"streamingSupported"`
	Error              *string               `json:"error,omitempty"`
	StartedAt          string                `json:"startedAt,omitempty"`
	TestedAt           string                `json:"testedAt,omitempty"`
}

type CapabilityProtocolJobResult struct {
	Protocol           string                     `json:"protocol"`
	Status             CapabilityProtocolStatus   `json:"status"`
	Success            bool                       `json:"success"`
	Latency            int64                      `json:"latency"`
	StreamingSupported bool                       `json:"streamingSupported"`
	TestedModel        string                     `json:"testedModel"`
	ModelResults       []CapabilityModelJobResult `json:"modelResults,omitempty"`
	SuccessCount       int                        `json:"successCount,omitempty"`
	AttemptedModels    int                        `json:"attemptedModels,omitempty"`
	Error              *string                    `json:"error,omitempty"`
	TestedAt           string                     `json:"testedAt"`
}

type CapabilityTestJob struct {
	JobID               string                        `json:"jobId"`
	ChannelID           int                           `json:"channelId"`
	ChannelName         string                        `json:"channelName"`
	ChannelKind         string                        `json:"channelKind"`
	SourceType          string                        `json:"sourceType"`
	Status              CapabilityJobStatus           `json:"status"`
	Tests               []CapabilityProtocolJobResult `json:"tests"`
	CompatibleProtocols []string                      `json:"compatibleProtocols"`
	TotalDuration       int64                         `json:"totalDuration"`
	StartedAt           string                        `json:"startedAt,omitempty"`
	UpdatedAt           string                        `json:"updatedAt"`
	FinishedAt          string                        `json:"finishedAt,omitempty"`
	Progress            CapabilityTestJobProgress     `json:"progress"`
	Error               *string                       `json:"error,omitempty"`
	CacheHit            bool                          `json:"cacheHit,omitempty"`
	TargetProtocols     []string                      `json:"targetProtocols,omitempty"`
	TimeoutMilliseconds int                           `json:"timeoutMilliseconds,omitempty"`
	CancelFunc          context.CancelFunc            `json:"-"` // 用于取消正在执行的测试 goroutine
}

type capabilityTestJobStore struct {
	sync.RWMutex
	jobs      map[string]*CapabilityTestJob
	lookupKey map[string]string
}

var capabilityJobs = newCapabilityTestJobStore()

func newCapabilityTestJobStore() *capabilityTestJobStore {
	s := &capabilityTestJobStore{
		jobs:      make(map[string]*CapabilityTestJob),
		lookupKey: make(map[string]string),
	}
	go s.gcLoop()
	return s
}

// gcLoop 定期清理已完成且超过 2 小时的 job，防止 job store 无限增长
func (s *capabilityTestJobStore) gcLoop() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.gc()
	}
}

func (s *capabilityTestJobStore) gc() {
	cutoff := time.Now().Add(-2 * time.Hour)
	s.Lock()
	defer s.Unlock()
	for jobID, job := range s.jobs {
		if job.Status != CapabilityJobStatusCompleted && job.Status != CapabilityJobStatusFailed && job.Status != CapabilityJobStatusCancelled {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, job.UpdatedAt)
		if err != nil || t.Before(cutoff) {
			delete(s.jobs, jobID)
		}
	}
	log.Printf("[CapabilityTest-GC] job store 清理完成，当前 job 数: %d", len(s.jobs))
}

func newCapabilityJobID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// 极低概率退化到时间戳
		return fmt.Sprintf("cap-%d", time.Now().UnixNano())
	}
	return "cap-" + hex.EncodeToString(b)
}

func newCapabilityTestJob(channelID int, channelName, channelKind, sourceType string, protocols []string, timeout time.Duration) *CapabilityTestJob {
	now := time.Now().Format(time.RFC3339Nano)
	job := &CapabilityTestJob{
		JobID:               newCapabilityJobID(),
		ChannelID:           channelID,
		ChannelName:         channelName,
		ChannelKind:         channelKind,
		SourceType:          sourceType,
		Status:              CapabilityJobStatusQueued,
		CompatibleProtocols: make([]string, 0),
		Tests:               make([]CapabilityProtocolJobResult, 0, len(protocols)),
		UpdatedAt:           now,
		TargetProtocols:     append([]string(nil), protocols...),
		TimeoutMilliseconds: int(timeout / time.Millisecond),
	}

	for _, protocol := range protocols {
		// 预填充各协议的模型列表（全部 queued），前端可立即展示
		var modelResults []CapabilityModelJobResult
		if models, err := getCapabilityProbeModels(protocol); err == nil {
			modelResults = make([]CapabilityModelJobResult, len(models))
			for i, model := range models {
				modelResults[i] = CapabilityModelJobResult{
					Model:  model,
					Status: CapabilityModelStatusQueued,
				}
			}
		}
		job.Tests = append(job.Tests, CapabilityProtocolJobResult{
			Protocol:        protocol,
			Status:          CapabilityProtocolStatusQueued,
			AttemptedModels: len(modelResults),
			ModelResults:    modelResults,
			TestedAt:        now,
		})
	}

	return job
}

func buildCapabilityJobLookupKey(cacheKey, channelKind string, channelID int) string {
	return fmt.Sprintf("%s:%s:%d", cacheKey, channelKind, channelID)
}

func (s *capabilityTestJobStore) bindLookupKey(lookupKey, jobID string) {
	s.Lock()
	defer s.Unlock()
	s.lookupKey[lookupKey] = jobID
}

func (s *capabilityTestJobStore) clearLookupKey(lookupKey string) {
	s.Lock()
	defer s.Unlock()
	delete(s.lookupKey, lookupKey)
}

func (s *capabilityTestJobStore) getByLookupKey(lookupKey string) (*CapabilityTestJob, bool) {
	s.RLock()
	jobID, ok := s.lookupKey[lookupKey]
	s.RUnlock()
	if !ok {
		return nil, false
	}
	return s.get(jobID)
}

func (s *capabilityTestJobStore) create(job *CapabilityTestJob) {
	s.Lock()
	defer s.Unlock()
	s.jobs[job.JobID] = cloneCapabilityTestJob(job)
}

func (s *capabilityTestJobStore) get(jobID string) (*CapabilityTestJob, bool) {
	s.RLock()
	defer s.RUnlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return nil, false
	}
	return cloneCapabilityTestJob(job), true
}

func (s *capabilityTestJobStore) getOrCreateByLookupKey(
	lookupKey string,
	builder func() *CapabilityTestJob,
) (*CapabilityTestJob, bool) {
	s.Lock()
	defer s.Unlock()

	if lookupKey != "" {
		if jobID, ok := s.lookupKey[lookupKey]; ok {
			if job, exists := s.jobs[jobID]; exists {
				return cloneCapabilityTestJob(job), true
			}
		}
	}

	job := builder()
	s.jobs[job.JobID] = cloneCapabilityTestJob(job)
	if lookupKey != "" {
		s.lookupKey[lookupKey] = job.JobID
	}
	return cloneCapabilityTestJob(job), false
}

func (s *capabilityTestJobStore) update(jobID string, updater func(job *CapabilityTestJob)) (*CapabilityTestJob, bool) {
	s.Lock()
	defer s.Unlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return nil, false
	}
	updater(job)
	job.UpdatedAt = time.Now().Format(time.RFC3339Nano)
	recomputeCapabilityJob(job)
	return cloneCapabilityTestJob(job), true
}

// setCancelFunc 直接设置内部 job 的 CancelFunc（不走 clone，因为 CancelFunc 不可复制）
func (s *capabilityTestJobStore) setCancelFunc(jobID string, cancel context.CancelFunc) {
	s.Lock()
	defer s.Unlock()
	if job, ok := s.jobs[jobID]; ok {
		job.CancelFunc = cancel
	}
}

// getCancelFunc 获取 job 的 CancelFunc
func (s *capabilityTestJobStore) getCancelFunc(jobID string) (context.CancelFunc, bool) {
	s.RLock()
	defer s.RUnlock()
	job, ok := s.jobs[jobID]
	if !ok || job.CancelFunc == nil {
		return nil, false
	}
	return job.CancelFunc, true
}

func cloneCapabilityTestJob(job *CapabilityTestJob) *CapabilityTestJob {
	if job == nil {
		return nil
	}
	cloned := *job
	cloned.Tests = make([]CapabilityProtocolJobResult, len(job.Tests))
	for i, test := range job.Tests {
		cloned.Tests[i] = test
		cloned.Tests[i].ModelResults = append([]CapabilityModelJobResult(nil), test.ModelResults...)
	}
	cloned.CompatibleProtocols = append([]string(nil), job.CompatibleProtocols...)
	cloned.TargetProtocols = append([]string(nil), job.TargetProtocols...)
	return &cloned
}

func recomputeCapabilityJob(job *CapabilityTestJob) {
	progress := CapabilityTestJobProgress{}
	compatible := make([]string, 0)
	allProtocolsFinished := true
	anyProtocolFailed := false

	for _, test := range job.Tests {
		if test.Success {
			compatible = append(compatible, test.Protocol)
		}
		if test.Status != CapabilityProtocolStatusCompleted && test.Status != CapabilityProtocolStatusFailed {
			allProtocolsFinished = false
		}
		if test.Status == CapabilityProtocolStatusFailed {
			anyProtocolFailed = true
		}
		for _, modelResult := range test.ModelResults {
			progress.TotalModels++
			switch modelResult.Status {
			case CapabilityModelStatusQueued:
				progress.QueuedModels++
			case CapabilityModelStatusRunning:
				progress.RunningModels++
			case CapabilityModelStatusSuccess:
				progress.SuccessModels++
				progress.CompletedModels++
			case CapabilityModelStatusFailed:
				progress.FailedModels++
				progress.CompletedModels++
			case CapabilityModelStatusSkipped:
				progress.SkippedModels++
				progress.CompletedModels++
			}
		}
	}

	sort.Strings(compatible)
	job.Progress = progress
	job.CompatibleProtocols = compatible

	if job.StartedAt == "" && (job.Status == CapabilityJobStatusRunning || job.Status == CapabilityJobStatusCompleted || job.Status == CapabilityJobStatusFailed) {
		job.StartedAt = job.UpdatedAt
	}

	if allProtocolsFinished && job.FinishedAt == "" {
		job.FinishedAt = job.UpdatedAt
	}

	// cancelled 状态由取消逻辑显式设置，recompute 不覆盖
	if job.Status == CapabilityJobStatusCancelled {
		return
	}

	if allProtocolsFinished {
		if len(compatible) > 0 {
			job.Status = CapabilityJobStatusCompleted
		} else if anyProtocolFailed || progress.TotalModels > 0 {
			job.Status = CapabilityJobStatusFailed
		}
	}
}

func capabilityProtocolResultsFromResponse(resp CapabilityTestResponse) []CapabilityProtocolJobResult {
	results := make([]CapabilityProtocolJobResult, 0, len(resp.Tests))
	for _, test := range resp.Tests {
		status := CapabilityProtocolStatusFailed
		if test.Success {
			status = CapabilityProtocolStatusCompleted
		}
		modelResults := make([]CapabilityModelJobResult, 0, len(test.ModelResults))
		for _, modelResult := range test.ModelResults {
			modelStatus := CapabilityModelStatusFailed
			if modelResult.Success {
				modelStatus = CapabilityModelStatusSuccess
			} else if modelResult.Skipped {
				modelStatus = CapabilityModelStatusSkipped
			}
			modelResults = append(modelResults, CapabilityModelJobResult{
				Model:              modelResult.Model,
				Status:             modelStatus,
				Success:            modelResult.Success,
				Latency:            modelResult.Latency,
				StreamingSupported: modelResult.StreamingSupported,
				Error:              modelResult.Error,
				StartedAt:          modelResult.StartedAt,
				TestedAt:           modelResult.TestedAt,
			})
		}
		results = append(results, CapabilityProtocolJobResult{
			Protocol:           test.Protocol,
			Status:             status,
			Success:            test.Success,
			Latency:            test.Latency,
			StreamingSupported: test.StreamingSupported,
			TestedModel:        test.TestedModel,
			ModelResults:       modelResults,
			SuccessCount:       test.SuccessCount,
			AttemptedModels:    test.AttemptedModels,
			Error:              test.Error,
			TestedAt:           test.TestedAt,
		})
	}
	return results
}

func createCapabilityJobFromResponse(channelID int, channelName, channelKind, sourceType string, protocols []string, timeout time.Duration, resp CapabilityTestResponse, cacheHit bool) *CapabilityTestJob {
	now := time.Now().Format(time.RFC3339Nano)
	job := &CapabilityTestJob{
		JobID:               newCapabilityJobID(),
		ChannelID:           channelID,
		ChannelName:         channelName,
		ChannelKind:         channelKind,
		SourceType:          sourceType,
		Status:              CapabilityJobStatusCompleted,
		Tests:               capabilityProtocolResultsFromResponse(resp),
		CompatibleProtocols: append([]string(nil), resp.CompatibleProtocols...),
		TotalDuration:       resp.TotalDuration,
		StartedAt:           now,
		UpdatedAt:           now,
		FinishedAt:          now,
		CacheHit:            cacheHit,
		TargetProtocols:     append([]string(nil), protocols...),
		TimeoutMilliseconds: int(timeout / time.Millisecond),
	}
	recomputeCapabilityJob(job)
	return job
}

func getCapabilityTestChannel(cfgManager *config.ConfigManager, channelKind string, id int) (*config.UpstreamConfig, error) {
	cfg := cfgManager.GetConfig()
	var channels []config.UpstreamConfig
	switch channelKind {
	case "messages":
		channels = cfg.Upstream
	case "responses":
		channels = cfg.ResponsesUpstream
	case "gemini":
		channels = cfg.GeminiUpstream
	case "chat":
		channels = cfg.ChatUpstream
	default:
		return nil, fmt.Errorf("invalid channel kind")
	}

	if id < 0 || id >= len(channels) {
		return nil, fmt.Errorf("channel not found")
	}

	channel := channels[id]
	return &channel, nil
}

func GetCapabilityTestJobStatus(cfgManager *config.ConfigManager, channelKind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseCapabilityChannelID(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid channel ID"})
			return
		}

		jobID := c.Param("jobId")
		job, ok := capabilityJobs.get(jobID)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "Capability test job not found"})
			return
		}

		if job.ChannelID != id || job.ChannelKind != channelKind {
			c.JSON(http.StatusNotFound, gin.H{"error": "Capability test job not found"})
			return
		}

		channel, getErr := getCapabilityTestChannel(cfgManager, channelKind, id)
		if getErr == nil {
			job.ChannelName = channel.Name
			job.SourceType = channel.ServiceType
		}

		c.JSON(http.StatusOK, job)
	}
}

func parseCapabilityChannelID(c *gin.Context) (int, error) {
	return strconv.Atoi(c.Param("id"))
}

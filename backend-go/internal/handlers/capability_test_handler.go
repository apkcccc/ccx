package handlers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BenedictKing/ccx/internal/config"
	"github.com/BenedictKing/ccx/internal/httpclient"
	"github.com/BenedictKing/ccx/internal/metrics"
	"github.com/BenedictKing/ccx/internal/utils"
	"github.com/gin-gonic/gin"
)

// ============== 缓存定义 ==============

const (
	capabilityCacheTTL    = 30 * time.Minute // 缓存 TTL（每次命中续期）
	capabilityCacheMaxTTL = 4 * time.Hour    // 缓存最大生存时间（从首次创建算起）
)

// capabilityCacheEntry 缓存条目
type capabilityCacheEntry struct {
	response  CapabilityTestResponse
	createdAt time.Time // 首次创建时间（用于计算最大生存期）
	expiresAt time.Time // 过期时间（每次命中续期）
}

// capabilityCache 全局能力测试缓存
var capabilityCache = struct {
	sync.RWMutex
	entries map[string]*capabilityCacheEntry
}{
	entries: make(map[string]*capabilityCacheEntry),
}

// buildCapabilityCacheKey 构建缓存 key（基于 baseURL + apiKey 与协议列表）
func buildCapabilityCacheKey(baseURL string, apiKey string, protocols []string) string {
	sorted := make([]string, len(protocols))
	copy(sorted, protocols)
	sort.Strings(sorted)
	metricsKey := metrics.GenerateMetricsKey(baseURL, apiKey)
	return fmt.Sprintf("%s:%s", metricsKey, strings.Join(sorted, ","))
}

// getCapabilityCache 读取缓存，命中时自动续期（不超过最大生存期）
// 全程持有写锁以避免并发读写 expiresAt 的竞态
func getCapabilityCache(key string) (*CapabilityTestResponse, bool) {
	capabilityCache.Lock()
	defer capabilityCache.Unlock()

	entry, ok := capabilityCache.entries[key]
	if !ok {
		return nil, false
	}

	now := time.Now()
	if now.After(entry.expiresAt) {
		// 已过期，删除
		delete(capabilityCache.entries, key)
		return nil, false
	}

	// 命中，续期（不超过最大生存期）
	newExpiry := now.Add(capabilityCacheTTL)
	maxExpiry := entry.createdAt.Add(capabilityCacheMaxTTL)
	if newExpiry.After(maxExpiry) {
		newExpiry = maxExpiry
	}
	entry.expiresAt = newExpiry

	return &entry.response, true
}

// setCapabilityCache 写入缓存
func setCapabilityCache(key string, resp CapabilityTestResponse) {
	now := time.Now()
	capabilityCache.Lock()
	capabilityCache.entries[key] = &capabilityCacheEntry{
		response:  resp,
		createdAt: now,
		expiresAt: now.Add(capabilityCacheTTL),
	}
	capabilityCache.Unlock()
}

// ============== 类型定义 ==============

// CapabilityTestRequest 能力测试请求体
type CapabilityTestRequest struct {
	TargetProtocols []string `json:"targetProtocols"`
	Timeout         int      `json:"timeout"`       // 毫秒
	PreviousJobID   string   `json:"previousJobId"` // 可选：上次测试的 jobId，用于复用成功结果
}

type ModelTestResult struct {
	Model              string  `json:"model"`
	Success            bool    `json:"success"`
	Skipped            bool    `json:"skipped,omitempty"`
	Latency            int64   `json:"latency"` // 毫秒
	StreamingSupported bool    `json:"streamingSupported"`
	Error              *string `json:"error,omitempty"`
	StartedAt          string  `json:"startedAt,omitempty"`
	TestedAt           string  `json:"testedAt"`
}

// ProtocolTestResult 单个协议测试结果
type ProtocolTestResult struct {
	Protocol           string            `json:"protocol"`
	Success            bool              `json:"success"`
	Latency            int64             `json:"latency"` // 毫秒
	StreamingSupported bool              `json:"streamingSupported"`
	TestedModel        string            `json:"testedModel"` // 优先返回首个成功模型名称，兼容旧字段
	ModelResults       []ModelTestResult `json:"modelResults,omitempty"`
	SuccessCount       int               `json:"successCount,omitempty"`
	AttemptedModels    int               `json:"attemptedModels,omitempty"`
	Error              *string           `json:"error"`
	TestedAt           string            `json:"testedAt"`
}

// CapabilityTestResponse 能力测试响应体
type CapabilityTestResponse struct {
	ChannelID           int                  `json:"channelId"`
	ChannelName         string               `json:"channelName"`
	SourceType          string               `json:"sourceType"`
	Tests               []ProtocolTestResult `json:"tests"`
	CompatibleProtocols []string             `json:"compatibleProtocols"`
	TotalDuration       int64                `json:"totalDuration"` // 毫秒
}

// ============== 主处理器 ==============

// TestChannelCapability 渠道能力测试处理器
// channelKind 决定从哪个配置获取渠道：messages/responses/gemini/chat
func TestChannelCapability(cfgManager *config.ConfigManager, channelKind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid channel ID"})
			return
		}

		channel, err := getCapabilityTestChannel(cfgManager, channelKind, id)
		if err != nil {
			statusCode := http.StatusBadRequest
			if err.Error() == "channel not found" {
				statusCode = http.StatusNotFound
			}
			c.JSON(statusCode, gin.H{"error": err.Error()})
			return
		}

		var req CapabilityTestRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		timeout := 10 * time.Second
		if req.Timeout > 0 {
			timeout = time.Duration(req.Timeout) * time.Millisecond
		}

		protocols := req.TargetProtocols
		if len(protocols) == 0 {
			protocols = []string{"messages", "chat", "gemini", "responses"}
		}

		effectiveRPM := channel.RPM
		if effectiveRPM <= 0 {
			effectiveRPM = 10
		}
		channel.RPM = effectiveRPM

		if len(channel.APIKeys) == 0 {
			errMsg := "no_api_key"
			resp := CapabilityTestResponse{
				ChannelID:           id,
				ChannelName:         channel.Name,
				SourceType:          channel.ServiceType,
				Tests:               []ProtocolTestResult{{Protocol: "all", Error: &errMsg, TestedAt: time.Now().Format(time.RFC3339)}},
				CompatibleProtocols: []string{},
				TotalDuration:       0,
			}
			job := createCapabilityJobFromResponse(id, channel.Name, channelKind, channel.ServiceType, protocols, timeout, resp, false)
			capabilityJobs.create(job)
			c.JSON(http.StatusOK, gin.H{"jobId": job.JobID, "resumed": false, "job": job})
			return
		}

		baseURL := ""
		if len(channel.GetAllBaseURLs()) > 0 {
			baseURL = channel.GetAllBaseURLs()[0]
		}
		apiKey := ""
		if len(channel.APIKeys) > 0 {
			apiKey = channel.APIKeys[0]
		}

		cacheKey := buildCapabilityCacheKey(baseURL, apiKey, protocols)
		lookupKey := buildCapabilityJobLookupKey(cacheKey, channelKind, id)

		if cached, ok := getCapabilityCache(cacheKey); ok {
			log.Printf("[CapabilityTest-Cache] 渠道 %s (ID:%d) 命中缓存，创建已完成任务", channel.Name, id)
			cached.ChannelID = id
			cached.ChannelName = channel.Name
			cached.SourceType = channel.ServiceType
			job, reused := capabilityJobs.getOrCreateByLookupKey(lookupKey, func() *CapabilityTestJob {
				return createCapabilityJobFromResponse(id, channel.Name, channelKind, channel.ServiceType, protocols, timeout, *cached, true)
			})
			c.JSON(http.StatusOK, gin.H{"jobId": job.JobID, "resumed": reused, "job": job})
			return
		}

		job, reused := capabilityJobs.getOrCreateByLookupKey(lookupKey, func() *CapabilityTestJob {
			return newCapabilityTestJob(id, channel.Name, channelKind, channel.ServiceType, protocols, timeout)
		})
		if reused {
			log.Printf("[CapabilityTest-Job] 复用能力测试任务 %s，渠道 %s (ID:%d, 类型:%s)", job.JobID, channel.Name, id, channel.ServiceType)
			c.JSON(http.StatusOK, gin.H{"jobId": job.JobID, "resumed": true, "job": job})
			return
		}
		log.Printf("[CapabilityTest-Job] 创建能力测试任务 %s，渠道 %s (ID:%d, 类型:%s)，协议: %v", job.JobID, channel.Name, id, channel.ServiceType, protocols)

		// 提取上次成功的结果用于复用
		var previousResults map[string]map[string]ModelTestResult
		if req.PreviousJobID != "" {
			if prevJob, ok := capabilityJobs.get(req.PreviousJobID); ok && prevJob.ChannelID == id && prevJob.ChannelKind == channelKind {
				previousResults = make(map[string]map[string]ModelTestResult)
				for _, test := range prevJob.Tests {
					modelMap := make(map[string]ModelTestResult)
					for _, mr := range test.ModelResults {
						if mr.Status == CapabilityModelStatusSuccess {
							modelMap[mr.Model] = ModelTestResult{
								Model:              mr.Model,
								Success:            mr.Success,
								Latency:            mr.Latency,
								StreamingSupported: mr.StreamingSupported,
								Error:              mr.Error,
								StartedAt:          mr.StartedAt,
								TestedAt:           mr.TestedAt,
							}
						}
					}
					if len(modelMap) > 0 {
						previousResults[test.Protocol] = modelMap
					}
				}
				if len(previousResults) > 0 {
					log.Printf("[CapabilityTest-Job] 复用上次测试 %s 的成功结果，跳过 %d 个协议的成功模型",
						req.PreviousJobID, len(previousResults))
				}
			}
		}

		go runCapabilityTestJob(job.JobID, channelKind, id, *channel, protocols, timeout, cacheKey, lookupKey, previousResults)

		c.JSON(http.StatusOK, gin.H{"jobId": job.JobID, "resumed": false, "job": job})
		return
	}
}

// ============== 核心测试逻辑 ==============

// testItem 代表一个 (协议, 模型) 测试单元
type testItem struct {
	protocol string
	model    string
	index    int // 模型在其协议列表中的索引
}

// buildRoundRobinQueue 构建交错队列
// 输出: messages[0], chat[0], gemini[0], responses[0], messages[1], chat[1], ...
func buildRoundRobinQueue(protocolModels map[string][]string, protocols []string) []testItem {
	maxModels := 0
	for _, models := range protocolModels {
		if len(models) > maxModels {
			maxModels = len(models)
		}
	}

	queue := make([]testItem, 0)
	for round := 0; round < maxModels; round++ {
		for _, protocol := range protocols {
			models := protocolModels[protocol]
			if round < len(models) {
				queue = append(queue, testItem{
					protocol: protocol,
					model:    models[round],
					index:    round,
				})
			}
		}
	}
	return queue
}

func runCapabilityTestJob(jobID, channelKind string, channelID int, channel config.UpstreamConfig, protocols []string, timeout time.Duration, cacheKey, lookupKey string, previousResults map[string]map[string]ModelTestResult) {
	// 创建可取消的 context，用于支持前端取消操作
	ctx, cancel := context.WithCancel(context.Background())
	capabilityJobs.setCancelFunc(jobID, cancel)

	// 检查是否在 queued 期间已被取消
	if ctx.Err() != nil {
		if lookupKey != "" {
			capabilityJobs.clearLookupKey(lookupKey)
		}
		return
	}

	updatedJob, _ := capabilityJobs.update(jobID, func(job *CapabilityTestJob) {
		// 仅在未被取消时才设为 running
		if job.Status == CapabilityJobStatusCancelled {
			return
		}
		job.Status = CapabilityJobStatusRunning
		job.StartedAt = time.Now().Format(time.RFC3339Nano)
	})

	// 如果 job 已被取消（在 queued 期间），不再执行测试
	if updatedJob != nil && updatedJob.Status == CapabilityJobStatusCancelled {
		log.Printf("[CapabilityTest-Job] 任务 %s 在 queued 期间已被取消，跳过执行", jobID)
		if lookupKey != "" {
			capabilityJobs.clearLookupKey(lookupKey)
		}
		return
	}

	log.Printf("[CapabilityTest-Job] 开始执行能力测试任务 %s，渠道 %s (ID:%d, 类型:%s)，协议: %v", jobID, channel.Name, channelID, channel.ServiceType, protocols)

	totalStart := time.Now()
	results := runRoundRobinTests(ctx, &channel, protocols, timeout, jobID, previousResults)
	totalDuration := time.Since(totalStart).Milliseconds()

	compatible := make([]string, 0)
	for _, r := range results {
		if r.Success {
			compatible = append(compatible, r.Protocol)
		}
	}

	resp := CapabilityTestResponse{
		ChannelID:           channelID,
		ChannelName:         channel.Name,
		SourceType:          channel.ServiceType,
		Tests:               results,
		CompatibleProtocols: compatible,
		TotalDuration:       totalDuration,
	}

	// 编排器已在执行过程中通过 capabilityJobs.update 实时维护 job.Tests，
	// 这里只更新最终元数据，不重建 Tests（避免覆盖编排器维护的 skipped 等中间状态）
	capabilityJobs.update(jobID, func(job *CapabilityTestJob) {
		// 如果已被取消，保留 cancelled 状态
		if job.Status == CapabilityJobStatusCancelled {
			return
		}
		job.ChannelName = channel.Name
		job.SourceType = channel.ServiceType
		job.CompatibleProtocols = append([]string(nil), compatible...)
		job.TotalDuration = totalDuration
		job.FinishedAt = time.Now().Format(time.RFC3339Nano)
		if len(compatible) > 0 {
			job.Status = CapabilityJobStatusCompleted
		} else {
			job.Status = CapabilityJobStatusFailed
		}
	})

	// 仅在未被取消且有兼容协议时写入缓存
	if len(compatible) > 0 && ctx.Err() == nil {
		setCapabilityCache(cacheKey, resp)
		log.Printf("[CapabilityTest-Cache] 渠道 %s (ID:%d) 写入缓存，兼容协议: %v", channel.Name, channelID, compatible)
	}

	if lookupKey != "" {
		capabilityJobs.clearLookupKey(lookupKey)
	}

	log.Printf("[CapabilityTest-Job] 能力测试任务 %s 完成，渠道 %s，兼容协议: %v，总耗时: %dms", jobID, channel.Name, compatible, totalDuration)
}

// runRoundRobinTests 核心编排器，串行按 round-robin 顺序逐个调度
// 所有模型都会被测试，不会在首次成功后跳过后续模型
// previousResults 可选：上次测试中成功的结果，跳过这些模型
func runRoundRobinTests(ctx context.Context, channel *config.UpstreamConfig, protocols []string, perModelTimeout time.Duration, jobID string, previousResults map[string]map[string]ModelTestResult) []ProtocolTestResult {
	// 1. 收集各协议模型列表，初始化 job 状态
	protocolModels := make(map[string][]string)
	protocolTimedOut := make(map[string]bool) // true = 全局超时强制终止
	results := make(map[string]*ProtocolTestResult)

	for _, protocol := range protocols {
		models, err := getCapabilityProbeModels(protocol)
		if err != nil {
			errMsg := "no_models_configured"
			results[protocol] = &ProtocolTestResult{
				Protocol: protocol,
				Error:    &errMsg,
				TestedAt: time.Now().Format(time.RFC3339),
			}
			protocolTimedOut[protocol] = true // 无模型配置，视为已终止
			capabilityJobs.update(jobID, func(job *CapabilityTestJob) {
				for i := range job.Tests {
					if job.Tests[i].Protocol == protocol {
						job.Tests[i].Status = CapabilityProtocolStatusFailed
						job.Tests[i].Error = &errMsg
						job.Tests[i].TestedAt = time.Now().Format(time.RFC3339Nano)
						break
					}
				}
			})
			log.Printf("[CapabilityTest-Protocol] 渠道 %s 获取 %s 协议测试模型失败: %v", channel.Name, protocol, err)
			continue
		}

		protocolModels[protocol] = models
		results[protocol] = &ProtocolTestResult{
			Protocol:        protocol,
			TestedAt:        time.Now().Format(time.RFC3339),
			AttemptedModels: len(models),
			ModelResults:    make([]ModelTestResult, len(models)),
		}

		// 初始化协议状态和模型列表
		capabilityJobs.update(jobID, func(job *CapabilityTestJob) {
			for i := range job.Tests {
				if job.Tests[i].Protocol == protocol {
					job.Tests[i].Status = CapabilityProtocolStatusRunning
					job.Tests[i].AttemptedModels = len(models)
					job.Tests[i].ModelResults = make([]CapabilityModelJobResult, len(models))
					for idx, modelName := range models {
						job.Tests[i].ModelResults[idx] = CapabilityModelJobResult{
							Model:  modelName,
							Status: CapabilityModelStatusQueued,
						}
					}
					job.Tests[i].TestedAt = time.Now().Format(time.RFC3339Nano)
					break
				}
			}
		})
		log.Printf("[CapabilityTest-Protocol] 开始测试渠道 %s 的 %s 协议兼容性", channel.Name, protocol)
	}

	// 1.5 预填充上次成功的结果
	if len(previousResults) > 0 {
		for protocol, modelMap := range previousResults {
			models := protocolModels[protocol]
			if len(models) == 0 {
				continue
			}
			result := results[protocol]
			if result == nil {
				continue
			}
			for i, modelName := range models {
				if prevResult, ok := modelMap[modelName]; ok && prevResult.Success {
					result.ModelResults[i] = prevResult
					result.SuccessCount++
					if result.SuccessCount == 1 {
						result.TestedModel = prevResult.Model
						result.StreamingSupported = prevResult.StreamingSupported
					}
					// 更新 job 中对应模型状态
					capabilityJobs.update(jobID, func(job *CapabilityTestJob) {
						updateCapabilityJobModelResult(job, protocol, modelName, CapabilityModelStatusSuccess, prevResult)
					})
				}
			}
		}
	}

	// 2. 构建交错队列（排除已有成功结果的模型）
	// 需要保留原始索引，因为 result.ModelResults 按原始顺序排列
	filteredProtocolModels := make(map[string][]string)
	originalIndexMap := make(map[string]map[string]int) // protocol -> model -> original index
	for protocol, models := range protocolModels {
		originalIndexMap[protocol] = make(map[string]int)
		var filtered []string
		for origIdx, model := range models {
			if prevModels, ok := previousResults[protocol]; ok {
				if prevResult, ok := prevModels[model]; ok && prevResult.Success {
					continue // 跳过已成功的模型
				}
			}
			originalIndexMap[protocol][model] = origIdx
			filtered = append(filtered, model)
		}
		filteredProtocolModels[protocol] = filtered
	}
	queue := buildRoundRobinQueue(filteredProtocolModels, protocols)

	// 修正 queue 中的 index 为原始列表中的索引
	for i := range queue {
		if idxMap, ok := originalIndexMap[queue[i].protocol]; ok {
			if origIdx, ok := idxMap[queue[i].model]; ok {
				queue[i].index = origIdx
			}
		}
	}

	// 3. 计算全局超时
	// 串行执行中每个模型最多耗时 max(interval, perModelTimeout)，累加所有模型 + 缓冲
	totalModels := len(queue)
	interval := time.Minute / time.Duration(channel.RPM)
	if interval <= 0 {
		interval = time.Minute / 10
	}
	perModelBudget := interval
	if perModelTimeout > perModelBudget {
		perModelBudget = perModelTimeout
	}
	globalTimeout := time.Duration(totalModels)*perModelBudget + 10*time.Second
	globalCtx, globalCancel := context.WithTimeout(ctx, globalTimeout)
	defer globalCancel()

	// 4. 逐项执行（所有模型都测，不早退）
	protocolStartTime := make(map[string]time.Time)
	protocolEndTime := make(map[string]time.Time)
	for _, item := range queue {
		// 检查全局超时
		if globalCtx.Err() != nil {
			log.Printf("[CapabilityTest-RoundRobin] 全局超时，终止测试")
			protocolTimedOut[item.protocol] = true
			break
		}

		// 记录协议首次测试时间
		if _, ok := protocolStartTime[item.protocol]; !ok {
			protocolStartTime[item.protocol] = time.Now()
		}

		// AcquireSendSlot（限流）
		if err := GetCapabilityTestDispatcher().AcquireSendSlot(globalCtx, interval); err != nil {
			log.Printf("[CapabilityTest-RoundRobin] 获取发送槽位失败: %v", err)
			break
		}

		// executeModelTest（单模型测试）
		modelResult := executeModelTest(globalCtx, channel, item.protocol, item.model, perModelTimeout, jobID)
		result := results[item.protocol]
		result.ModelResults[item.index] = modelResult
		protocolEndTime[item.protocol] = time.Now() // 每次模型完成时更新协议结束时间

		if modelResult.Success {
			result.SuccessCount++
			// 首个成功模型：记录代表性字段，但继续测其余模型
			if result.SuccessCount == 1 {
				result.TestedModel = modelResult.Model
				result.StreamingSupported = modelResult.StreamingSupported
				result.Latency = protocolEndTime[item.protocol].Sub(protocolStartTime[item.protocol]).Milliseconds()
				log.Printf("[CapabilityTest-Protocol] 渠道 %s 的 %s 协议首个成功模型: %s (耗时: %dms)",
					channel.Name, item.protocol, result.TestedModel, result.Latency)
			}
		}
	}

	// 5. 收尾：标记残留 queued 模型为 skipped（仅超时时出现），更新协议最终状态
	for protocol, result := range results {
		models := protocolModels[protocol]

		// 回填未被调度到的模型（超时导致）为 skipped
		for i := range result.ModelResults {
			if result.ModelResults[i].Model == "" && i < len(models) {
				result.ModelResults[i] = ModelTestResult{
					Model:    models[i],
					Success:  false,
					Skipped:  true,
					TestedAt: time.Now().Format(time.RFC3339),
				}
				capabilityJobs.update(jobID, func(job *CapabilityTestJob) {
					updateCapabilityJobModelResult(job, protocol, models[i], CapabilityModelStatusSkipped, result.ModelResults[i])
				})
			}
		}

		// 计算延迟：用协议的实际开始/结束时间，避免被其他协议的执行时间污染
		if startTime, ok := protocolStartTime[protocol]; ok {
			if endTime, ok := protocolEndTime[protocol]; ok {
				result.Latency = endTime.Sub(startTime).Milliseconds()
			} else {
				result.Latency = time.Since(startTime).Milliseconds()
			}
		}

		// 判断是否有实际测试过（至少有一个非 skipped 模型）
		hasTestedModel := false
		for _, mr := range result.ModelResults {
			if !mr.Skipped && mr.Model != "" {
				hasTestedModel = true
				break
			}
		}

		if result.SuccessCount > 0 {
			// 有至少一个成功模型
			result.Success = true
			capabilityJobs.update(jobID, func(job *CapabilityTestJob) {
				for i := range job.Tests {
					if job.Tests[i].Protocol == protocol {
						job.Tests[i].Status = CapabilityProtocolStatusCompleted
						job.Tests[i].Success = true
						job.Tests[i].Latency = result.Latency
						job.Tests[i].StreamingSupported = result.StreamingSupported
						job.Tests[i].TestedModel = result.TestedModel
						job.Tests[i].SuccessCount = result.SuccessCount
						job.Tests[i].Error = nil
						job.Tests[i].TestedAt = time.Now().Format(time.RFC3339Nano)
						break
					}
				}
			})
			log.Printf("[CapabilityTest-Protocol] 渠道 %s 的 %s 协议测试完成 (成功: %d/%d, 耗时: %dms)",
				channel.Name, protocol, result.SuccessCount, result.AttemptedModels, result.Latency)
		} else if !hasTestedModel {
			// 协议完全未测试（超时）
			result.Success = false
			capabilityJobs.update(jobID, func(job *CapabilityTestJob) {
				for i := range job.Tests {
					if job.Tests[i].Protocol == protocol {
						job.Tests[i].Status = CapabilityProtocolStatusFailed
						job.Tests[i].Success = false
						job.Tests[i].Latency = result.Latency
						job.Tests[i].SuccessCount = 0
						job.Tests[i].TestedAt = time.Now().Format(time.RFC3339Nano)
						for j := range job.Tests[i].ModelResults {
							if job.Tests[i].ModelResults[j].Status == CapabilityModelStatusQueued || job.Tests[i].ModelResults[j].Status == CapabilityModelStatusRunning {
								job.Tests[i].ModelResults[j].Status = CapabilityModelStatusSkipped
							}
						}
						break
					}
				}
			})
			log.Printf("[CapabilityTest-Protocol] 渠道 %s 的 %s 协议未实际测试（调度超时）", channel.Name, protocol)
		} else {
			// 全部模型测试失败
			result.Success = false
			errMsg := "all_models_failed"
			result.Error = &errMsg
			capabilityJobs.update(jobID, func(job *CapabilityTestJob) {
				for i := range job.Tests {
					if job.Tests[i].Protocol == protocol {
						job.Tests[i].Status = CapabilityProtocolStatusFailed
						job.Tests[i].Success = false
						job.Tests[i].Latency = result.Latency
						job.Tests[i].SuccessCount = result.SuccessCount
						job.Tests[i].Error = result.Error
						job.Tests[i].TestedAt = time.Now().Format(time.RFC3339Nano)
						for j := range job.Tests[i].ModelResults {
							if job.Tests[i].ModelResults[j].Status == CapabilityModelStatusQueued || job.Tests[i].ModelResults[j].Status == CapabilityModelStatusRunning {
								job.Tests[i].ModelResults[j].Status = CapabilityModelStatusSkipped
							}
						}
						break
					}
				}
			})
			log.Printf("[CapabilityTest-Protocol] 渠道 %s 的 %s 协议全部模型测试失败 (尝试: %d, 总耗时: %dms)",
				channel.Name, protocol, result.AttemptedModels, result.Latency)
		}
	}

	// 6. 转换为有序结果
	orderedResults := make([]ProtocolTestResult, 0, len(protocols))
	for _, protocol := range protocols {
		if result, ok := results[protocol]; ok {
			orderedResults = append(orderedResults, *result)
		}
	}

	return orderedResults
}

// executeModelTest 单模型测试（不调用 AcquireSendSlot，由编排器负责限流）
func executeModelTest(ctx context.Context, channel *config.UpstreamConfig, protocol, model string, timeout time.Duration, jobID string) ModelTestResult {
	startedAt := time.Now()
	modelResult := ModelTestResult{
		Model:     model,
		StartedAt: startedAt.Format(time.RFC3339Nano),
	}

	req, err := buildTestRequestWithModel(protocol, channel, model)
	if err != nil {
		errMsg := fmt.Sprintf("build_request_failed: %v", err)
		modelResult.Error = &errMsg
		modelResult.TestedAt = time.Now().Format(time.RFC3339Nano)
		capabilityJobs.update(jobID, func(job *CapabilityTestJob) {
			updateCapabilityJobModelResult(job, protocol, model, CapabilityModelStatusFailed, modelResult)
		})
		log.Printf("[CapabilityTest-Model] 渠道 %s 构建 %s 测试请求失败 (模型: %s): %v", channel.Name, protocol, model, err)
		return modelResult
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req = req.WithContext(reqCtx)

	capabilityJobs.update(jobID, func(job *CapabilityTestJob) {
		updateCapabilityJobModelResult(job, protocol, model, CapabilityModelStatusRunning, modelResult)
	})

	client := httpclient.GetManager().GetStandardClient(timeout, channel.InsecureSkipVerify, channel.ProxyURL)

	startTime := time.Now()
	log.Printf("[CapabilityTest-Model] 渠道 %s 启动 %s 协议模型测试 (模型: %s, startedAt: %s)",
		channel.Name, protocol, model, modelResult.StartedAt)
	success, streamingSupported, statusCode, sendErr := sendAndCheckStream(reqCtx, client, req)
	modelResult.Latency = time.Since(startTime).Milliseconds()
	modelResult.TestedAt = time.Now().Format(time.RFC3339Nano)

	if success {
		modelResult.Success = true
		modelResult.StreamingSupported = streamingSupported
		capabilityJobs.update(jobID, func(job *CapabilityTestJob) {
			updateCapabilityJobModelResult(job, protocol, model, CapabilityModelStatusSuccess, modelResult)
		})
		log.Printf("[CapabilityTest-Model] 渠道 %s 的 %s 协议测试成功 (模型: %s, 流式: %v, 耗时: %dms)",
			channel.Name, protocol, model, streamingSupported, modelResult.Latency)
		return modelResult
	}

	errMsg := classifyError(sendErr, statusCode, reqCtx)
	modelResult.Error = &errMsg
	capabilityJobs.update(jobID, func(job *CapabilityTestJob) {
		updateCapabilityJobModelResult(job, protocol, model, CapabilityModelStatusFailed, modelResult)
	})
	log.Printf("[CapabilityTest-Model] 渠道 %s 的 %s 协议测试失败 (模型: %s, 耗时: %dms): %s",
		channel.Name, protocol, model, modelResult.Latency, errMsg)
	return modelResult
}

// testProtocolCompatibility 并发测试多个协议的兼容性（已废弃，保留用于兼容）
func testProtocolCompatibility(ctx context.Context, channel *config.UpstreamConfig, protocols []string, timeout time.Duration, jobID string) []ProtocolTestResult {
	// 已废弃，直接调用新实现
	return runRoundRobinTests(ctx, channel, protocols, timeout, jobID, nil)
}

// testSingleProtocol 已废弃，保留用于兼容
func testSingleProtocol(ctx context.Context, channel *config.UpstreamConfig, protocol string, timeout time.Duration, jobID string) ProtocolTestResult {
	// 已废弃，直接调用新实现
	results := runRoundRobinTests(ctx, channel, []string{protocol}, timeout, jobID, nil)
	if len(results) > 0 {
		return results[0]
	}
	return ProtocolTestResult{Protocol: protocol, TestedAt: time.Now().Format(time.RFC3339)}
}

// testSingleModel 已废弃，保留用于兼容
func testSingleModel(ctx context.Context, channel *config.UpstreamConfig, protocol, model string, timeout time.Duration, jobID string) ModelTestResult {
	// 已废弃，直接调用 executeModelTest
	return executeModelTest(ctx, channel, protocol, model, timeout, jobID)
}

func updateCapabilityJobModelResult(job *CapabilityTestJob, protocol, model string, status CapabilityModelStatus, result ModelTestResult) {
	for i := range job.Tests {
		if job.Tests[i].Protocol != protocol {
			continue
		}
		for j := range job.Tests[i].ModelResults {
			if job.Tests[i].ModelResults[j].Model != model {
				continue
			}
			job.Tests[i].ModelResults[j].Status = status
			job.Tests[i].ModelResults[j].Success = result.Success
			job.Tests[i].ModelResults[j].Latency = result.Latency
			job.Tests[i].ModelResults[j].StreamingSupported = result.StreamingSupported
			job.Tests[i].ModelResults[j].Error = result.Error
			job.Tests[i].ModelResults[j].StartedAt = result.StartedAt
			job.Tests[i].ModelResults[j].TestedAt = result.TestedAt
			return
		}
	}
}

// ============== 请求构建 ==============

// buildTestRequestWithModel 构建最小化测试请求（指定模型）
func buildTestRequestWithModel(protocol string, channel *config.UpstreamConfig, model string) (*http.Request, error) {
	// 获取 BaseURL
	urls := channel.GetAllBaseURLs()
	if len(urls) == 0 {
		return nil, fmt.Errorf("no base URL configured")
	}
	baseURL := urls[0]

	// 如果末尾有 #，去掉 # 后不添加版本前缀
	noVersionPrefix := false
	if strings.HasSuffix(baseURL, "#") {
		baseURL = strings.TrimSuffix(baseURL, "#")
		noVersionPrefix = true
	}

	// 处理 BaseURL：去除末尾 /
	baseURL = strings.TrimSuffix(baseURL, "/")

	apiKey := channel.APIKeys[0]

	var (
		requestURL string
		body       []byte
		err        error
		isGemini   bool
	)

	switch protocol {
	case "messages":
		if noVersionPrefix {
			requestURL = baseURL + "/messages"
		} else {
			requestURL = baseURL + "/v1/messages"
		}
		body, err = json.Marshal(map[string]interface{}{
			"model": model,
			"system": []map[string]interface{}{
				{
					"type": "text",
					"text": "x-anthropic-billing-header: cc_version=2.1.71.2f9; cc_entrypoint=cli;",
				},
				{
					"type": "text",
					"text": "You are a Claude agent, built on Anthropic's Claude Agent SDK.",
					"cache_control": map[string]string{
						"type": "ephemeral",
					},
				},
			},
			"messages":   []map[string]string{{"role": "user", "content": "What are you best at: code generation, creative writing, or math problem solving?"}},
			"max_tokens": 100,
			"stream":     true,
			"thinking": map[string]interface{}{
				"type": "disabled",
			},
		})

	case "chat":
		if noVersionPrefix {
			requestURL = baseURL + "/chat/completions"
		} else {
			requestURL = baseURL + "/v1/chat/completions"
		}
		body, err = json.Marshal(map[string]interface{}{
			"model": model,
			"messages": []map[string]string{
				{"role": "system", "content": "You are a helpful assistant."},
				{"role": "user", "content": "What are you best at: code generation, creative writing, or math problem solving?"},
			},
			"max_tokens":       100,
			"stream":           true,
			"reasoning_effort": "none",
		})

	case "gemini":
		if noVersionPrefix {
			requestURL = baseURL + "/models/" + model + ":streamGenerateContent?alt=sse"
		} else {
			requestURL = baseURL + "/v1beta/models/" + model + ":streamGenerateContent?alt=sse"
		}
		body, err = json.Marshal(map[string]interface{}{
			"contents": []map[string]interface{}{
				{
					"role":  "user",
					"parts": []map[string]string{{"text": "What are you best at: code generation, creative writing, or math problem solving?"}},
				},
			},
			"systemInstruction": map[string]interface{}{
				"parts": []map[string]string{{"text": "You are Gemini CLI, an interactive CLI agent specializing in software engineering tasks."}},
			},
			"generationConfig": map[string]interface{}{
				"maxOutputTokens": 100,
				"thinkingConfig": map[string]interface{}{
					"thinkingLevel": "low",
				},
			},
		})
		isGemini = true

	case "responses":
		if noVersionPrefix {
			requestURL = baseURL + "/responses"
		} else {
			requestURL = baseURL + "/v1/responses"
		}
		body, err = json.Marshal(map[string]interface{}{
			"model":             model,
			"input":             "What are you best at: code generation, creative writing, or math problem solving?",
			"instructions":      "You are Codex, a coding agent based on GPT-5.",
			"max_output_tokens": 100,
			"stream":            true,
			"reasoning": map[string]interface{}{
				"effort": "low",
			},
		})

	default:
		return nil, fmt.Errorf("unsupported protocol: %s", protocol)
	}

	if err != nil {
		return nil, fmt.Errorf("marshal request body failed: %w", err)
	}

	req, err := http.NewRequest("POST", requestURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	// 设置通用头部
	req.Header.Set("Content-Type", "application/json")

	// 设置认证头部
	if isGemini {
		utils.SetGeminiAuthenticationHeader(req.Header, apiKey)
	} else {
		utils.SetAuthenticationHeader(req.Header, apiKey)
		// Messages 协议需要 anthropic-version、anthropic-beta、User-Agent 和 X-App 头部
		if protocol == "messages" {
			req.Header.Set("anthropic-version", "2023-06-01")
			req.Header.Set("anthropic-beta", "claude-code-20250219,adaptive-thinking-2026-01-28,prompt-caching-scope-2026-01-05,effort-2025-11-24")
			req.Header.Set("User-Agent", "claude-cli/2.1.71 (external, cli)")
			req.Header.Set("X-App", "cli")
		}
		// Responses 协议需要 Originator 和 User-Agent 头部
		if protocol == "responses" {
			req.Header.Set("Originator", "codex_cli_rs")
			req.Header.Set("User-Agent", "codex_cli_rs/0.111.0 (Mac OS 26.3.0; arm64) iTerm.app/3.6.6")
		}
	}

	// 应用自定义请求头
	if channel.CustomHeaders != nil {
		for key, value := range channel.CustomHeaders {
			req.Header.Set(key, value)
		}
	}

	return req, nil
}

// buildTestRequest 构建最小化测试请求（使用首选模型，兼容旧接口）
func buildTestRequest(protocol string, channel *config.UpstreamConfig) (*http.Request, error) {
	model, err := getCapabilityProbeModel(protocol)
	if err != nil {
		return nil, err
	}
	return buildTestRequestWithModel(protocol, channel, model)
}

// ============== 流式响应检测 ==============

// sendAndCheckStream 发送请求并检查流式响应能力
// 返回: success（HTTP 2xx）, streamingSupported（能解析 SSE chunk）, statusCode, error
func sendAndCheckStream(ctx context.Context, client *http.Client, req *http.Request) (bool, bool, int, error) {
	resp, err := client.Do(req)
	if err != nil {
		return false, false, 0, err
	}
	defer resp.Body.Close()

	// 非 2xx 视为不兼容
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, false, resp.StatusCode, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// HTTP 2xx，尝试读取第一个 SSE chunk 检测流式支持
	streamingSupported := false

	// 使用 5 秒读取超时
	readCtx, readCancel := context.WithTimeout(ctx, 5*time.Second)
	defer readCancel()

	// 在 goroutine 中扫描以支持超时取消
	doneCh := make(chan bool, 1)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				// 跳过 [DONE] 标记
				if data == "[DONE]" {
					continue
				}
				// 尝试 JSON 解析
				var jsonObj map[string]interface{}
				if json.Unmarshal([]byte(data), &jsonObj) == nil {
					doneCh <- true
					return
				}
			}
		}
		doneCh <- false
	}()

	select {
	case result := <-doneCh:
		streamingSupported = result
	case <-readCtx.Done():
		// 读取超时，但 HTTP 2xx 所以 success 仍为 true
		streamingSupported = false
	}

	return true, streamingSupported, resp.StatusCode, nil
}

// ============== 取消与重测 ==============

// CancelCapabilityTestJob 取消正在进行的能力测试
func CancelCapabilityTestJob(cfgManager *config.ConfigManager, channelKind string) gin.HandlerFunc {
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

		// 只能取消正在运行或排队中的 job
		if job.Status != CapabilityJobStatusRunning && job.Status != CapabilityJobStatusQueued {
			c.JSON(http.StatusConflict, gin.H{"error": "Job is not running"})
			return
		}

		// 调用 CancelFunc 取消 goroutine
		if cancelFn, ok := capabilityJobs.getCancelFunc(jobID); ok {
			cancelFn()
		}

		// 更新 job 状态
		capabilityJobs.update(jobID, func(job *CapabilityTestJob) {
			job.Status = CapabilityJobStatusCancelled
			job.FinishedAt = time.Now().Format(time.RFC3339Nano)
			// 将所有 queued/running 模型标记为 skipped
			for i := range job.Tests {
				for j := range job.Tests[i].ModelResults {
					if job.Tests[i].ModelResults[j].Status == CapabilityModelStatusQueued ||
						job.Tests[i].ModelResults[j].Status == CapabilityModelStatusRunning {
						job.Tests[i].ModelResults[j].Status = CapabilityModelStatusSkipped
					}
				}
				// 更新协议状态
				if job.Tests[i].Status == CapabilityProtocolStatusQueued || job.Tests[i].Status == CapabilityProtocolStatusRunning {
					job.Tests[i].Status = CapabilityProtocolStatusFailed
				}
			}
		})

		log.Printf("[CapabilityTest-Cancel] 能力测试任务 %s 已取消", jobID)
		c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
	}
}

// RetryCapabilityTestModel 重测单个模型
func RetryCapabilityTestModel(cfgManager *config.ConfigManager, channelKind string) gin.HandlerFunc {
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

		var req struct {
			Protocol string `json:"protocol"`
			Model    string `json:"model"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Protocol == "" || req.Model == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "protocol and model are required"})
			return
		}

		// 检查模型是否存在于 job 中
		modelFound := false
		for _, test := range job.Tests {
			if test.Protocol != req.Protocol {
				continue
			}
			for _, mr := range test.ModelResults {
				if mr.Model == req.Model {
					modelFound = true
					break
				}
			}
		}
		if !modelFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Model not found in job"})
			return
		}

		// 获取渠道配置
		channel, chErr := getCapabilityTestChannel(cfgManager, channelKind, id)
		if chErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": chErr.Error()})
			return
		}

		timeout := 10 * time.Second
		if job.TimeoutMilliseconds > 0 {
			timeout = time.Duration(job.TimeoutMilliseconds) * time.Millisecond
		}

		// 将目标模型状态设为 running
		capabilityJobs.update(jobID, func(j *CapabilityTestJob) {
			for i := range j.Tests {
				if j.Tests[i].Protocol != req.Protocol {
					continue
				}
				// 如果协议已完成，重新设为 running
				if j.Tests[i].Status == CapabilityProtocolStatusCompleted || j.Tests[i].Status == CapabilityProtocolStatusFailed {
					j.Tests[i].Status = CapabilityProtocolStatusRunning
				}
				for k := range j.Tests[i].ModelResults {
					if j.Tests[i].ModelResults[k].Model == req.Model {
						j.Tests[i].ModelResults[k].Status = CapabilityModelStatusRunning
						j.Tests[i].ModelResults[k].Error = nil
						break
					}
				}
				break
			}
			// 如果 job 已经完成/失败/取消，重设为 running
			if j.Status == CapabilityJobStatusCompleted || j.Status == CapabilityJobStatusFailed || j.Status == CapabilityJobStatusCancelled {
				j.Status = CapabilityJobStatusRunning
				j.FinishedAt = ""
			}
		})

		// 异步执行单模型测试（使用独立可取消 context）
		// 不覆盖 job 的 CancelFunc，避免影响主任务的取消能力
		retryCtx, retryCancel := context.WithCancel(context.Background())

		go func() {
			defer retryCancel()
			modelResult := executeModelTest(retryCtx, channel, req.Protocol, req.Model, timeout, jobID)

			// 更新协议和 job 整体状态
			capabilityJobs.update(jobID, func(j *CapabilityTestJob) {
				for i := range j.Tests {
					if j.Tests[i].Protocol != req.Protocol {
						continue
					}
					// 重新统计协议结果
					allDone := true
					anySuccess := false
					successCount := 0
					var firstSuccessModel string
					var firstSuccessStreaming bool
					for _, mr := range j.Tests[i].ModelResults {
						if mr.Status == CapabilityModelStatusQueued || mr.Status == CapabilityModelStatusRunning {
							allDone = false
						}
						if mr.Status == CapabilityModelStatusSuccess {
							anySuccess = true
							successCount++
							if firstSuccessModel == "" {
								firstSuccessModel = mr.Model
								firstSuccessStreaming = mr.StreamingSupported
							}
						}
					}
					j.Tests[i].SuccessCount = successCount
					if anySuccess {
						j.Tests[i].Success = true
						j.Tests[i].TestedModel = firstSuccessModel
						j.Tests[i].StreamingSupported = firstSuccessStreaming
						j.Tests[i].Error = nil
					}
					if allDone {
						if anySuccess {
							j.Tests[i].Status = CapabilityProtocolStatusCompleted
						} else {
							j.Tests[i].Status = CapabilityProtocolStatusFailed
						}
					}
					j.Tests[i].TestedAt = time.Now().Format(time.RFC3339Nano)
					break
				}
			})

			log.Printf("[CapabilityTest-Retry] 单模型重测完成: job=%s, protocol=%s, model=%s, success=%v",
				jobID, req.Protocol, req.Model, modelResult.Success)
		}()

		c.JSON(http.StatusOK, gin.H{"status": "accepted"})
	}
}

// ============== 错误分类 ==============

// classifyError 对错误进行分类
func classifyError(err error, statusCode int, ctx context.Context) string {
	if ctx.Err() == context.DeadlineExceeded {
		return "timeout"
	}

	if statusCode == 429 {
		return "rate_limited"
	}

	if statusCode > 0 {
		return fmt.Sprintf("http_error_%d", statusCode)
	}

	errStr := err.Error()
	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded") {
		return "timeout"
	}

	return "request_failed: " + errStr
}

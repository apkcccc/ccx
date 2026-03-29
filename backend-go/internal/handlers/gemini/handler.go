package gemini

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/BenedictKing/ccx/internal/config"
	"github.com/BenedictKing/ccx/internal/converters"
	"github.com/BenedictKing/ccx/internal/handlers/common"
	"github.com/BenedictKing/ccx/internal/middleware"
	"github.com/BenedictKing/ccx/internal/scheduler"
	"github.com/BenedictKing/ccx/internal/types"
	"github.com/BenedictKing/ccx/internal/utils"
	"github.com/gin-gonic/gin"
)

// Handler Gemini API 代理处理器
// 支持多渠道调度：当配置多个渠道时自动启用
func Handler(
	envCfg *config.EnvConfig,
	cfgManager *config.ConfigManager,
	channelScheduler *scheduler.ChannelScheduler,
) gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		middleware.ProxyAuthMiddleware(envCfg)(c)
		if c.IsAborted() {
			return
		}

		startTime := time.Now()

		maxBodySize := envCfg.MaxRequestBodySize
		bodyBytes, err := common.ReadRequestBody(c, maxBodySize)
		if err != nil {
			return
		}

		geminiReq, err := parseGeminiCompatibleRequest(bodyBytes)
		if err != nil {
			c.JSON(400, types.GeminiError{
				Error: types.GeminiErrorDetail{
					Code:    400,
					Message: fmt.Sprintf("Invalid request body: %v", err),
					Status:  "INVALID_ARGUMENT",
				},
			})
			return
		}

		modelAction := c.Param("modelAction")
		modelAction = strings.TrimPrefix(modelAction, "/")
		model := extractModelName(modelAction)
		if model == "" {
			c.JSON(400, types.GeminiError{
				Error: types.GeminiErrorDetail{
					Code:    400,
					Message: "Model name is required in URL path",
					Status:  "INVALID_ARGUMENT",
				},
			})
			return
		}

		isStream := strings.Contains(c.Request.URL.Path, "streamGenerateContent")
		userID := common.ExtractConversationID(c, bodyBytes)
		common.LogOriginalRequest(c, bodyBytes, envCfg, "Gemini")

		isMultiChannel := channelScheduler.IsMultiChannelMode(scheduler.ChannelKindGemini)

		if isMultiChannel {
			handleMultiChannel(c, envCfg, cfgManager, channelScheduler, bodyBytes, geminiReq, model, isStream, userID, startTime)
		} else {
			handleSingleChannel(c, envCfg, cfgManager, channelScheduler, bodyBytes, geminiReq, model, isStream, startTime)
		}
	})
}

func extractModelName(param string) string {
	if param == "" {
		return ""
	}
	if idx := strings.Index(param, ":"); idx > 0 {
		return param[:idx]
	}
	return param
}

func parseGeminiCompatibleRequest(bodyBytes []byte) (*types.GeminiRequest, error) {
	req := &types.GeminiRequest{}
	if len(bodyBytes) == 0 {
		return req, nil
	}

	if err := json.Unmarshal(bodyBytes, req); err == nil {
		if len(req.Contents) > 0 || req.SystemInstruction != nil || len(req.Tools) > 0 || req.GenerationConfig != nil {
			return req, nil
		}
	}

	var reqMap map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &reqMap); err != nil {
		return nil, err
	}

	if contents, ok := reqMap["contents"]; ok && contents != nil {
		normalized, err := json.Marshal(reqMap)
		if err != nil {
			return nil, err
		}
		req = &types.GeminiRequest{}
		if err := json.Unmarshal(normalized, req); err != nil {
			return nil, err
		}
		return req, nil
	}

	return convertOpenAIStyleToGeminiRequest(reqMap)
}

func convertOpenAIStyleToGeminiRequest(reqMap map[string]interface{}) (*types.GeminiRequest, error) {
	req := &types.GeminiRequest{}

	if systemInstruction := buildSystemInstruction(reqMap); systemInstruction != nil {
		req.SystemInstruction = systemInstruction
	}

	if messages, ok := reqMap["messages"].([]interface{}); ok && len(messages) > 0 {
		req.Contents = convertMessagesToGeminiContents(messages)
	} else if input, ok := reqMap["input"]; ok {
		req.Contents = buildSingleUserContent(input)
	} else if prompt, ok := reqMap["prompt"]; ok {
		req.Contents = buildSingleUserContent(prompt)
	}

	if tools, ok := reqMap["tools"].([]interface{}); ok && len(tools) > 0 {
		req.Tools = convertOpenAIToolsToGeminiTools(tools)
	}

	req.GenerationConfig = buildGenerationConfig(reqMap)

	return req, nil
}

func buildSystemInstruction(reqMap map[string]interface{}) *types.GeminiContent {
	if system, ok := reqMap["system"]; ok {
		parts := toGeminiPartsFromContent(system)
		if len(parts) > 0 {
			return &types.GeminiContent{Parts: parts, Role: "user"}
		}
	}

	if instructions, ok := reqMap["instructions"]; ok {
		parts := toGeminiPartsFromContent(instructions)
		if len(parts) > 0 {
			return &types.GeminiContent{Parts: parts, Role: "user"}
		}
	}

	if messages, ok := reqMap["messages"].([]interface{}); ok && len(messages) > 0 {
		var systemParts []types.GeminiPart
		for _, raw := range messages {
			msg, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			role, _ := msg["role"].(string)
			if role != "system" {
				continue
			}
			systemParts = append(systemParts, toGeminiPartsFromContent(msg["content"])...)
		}
		if len(systemParts) > 0 {
			return &types.GeminiContent{Parts: systemParts, Role: "user"}
		}
	}

	return nil
}

func buildSingleUserContent(v interface{}) []types.GeminiContent {
	parts := toGeminiPartsFromContent(v)
	if len(parts) == 0 {
		return nil
	}
	return []types.GeminiContent{{Role: "user", Parts: parts}}
}

func convertMessagesToGeminiContents(messages []interface{}) []types.GeminiContent {
	var contents []types.GeminiContent

	for _, raw := range messages {
		msg, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}

		role, _ := msg["role"].(string)
		if role == "system" {
			continue
		}

		geminiRole := "user"
		if role == "assistant" {
			geminiRole = "model"
		}

		parts := toGeminiPartsFromContent(msg["content"])

		if role == "assistant" {
			if toolCalls, ok := msg["tool_calls"].([]interface{}); ok && len(toolCalls) > 0 {
				parts = append(parts, convertToolCallsToGeminiParts(toolCalls)...)
			}
		}

		if role == "tool" {
			parts = buildToolResponseParts(msg)
			geminiRole = "user"
		}

		if len(parts) == 0 {
			continue
		}

		contents = append(contents, types.GeminiContent{
			Role:  geminiRole,
			Parts: parts,
		})
	}

	return contents
}

func toGeminiPartsFromContent(content interface{}) []types.GeminiPart {
	switch v := content.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []types.GeminiPart{{Text: v}}
	case []interface{}:
		var parts []types.GeminiPart
		for _, item := range v {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			itemType, _ := itemMap["type"].(string)
			switch itemType {
			case "", "text", "input_text", "output_text":
				if text, ok := itemMap["text"].(string); ok && strings.TrimSpace(text) != "" {
					parts = append(parts, types.GeminiPart{Text: text})
				}
			case "image_url":
				if imageURL, ok := itemMap["image_url"].(map[string]interface{}); ok {
					if url, ok := imageURL["url"].(string); ok && strings.TrimSpace(url) != "" {
						parts = append(parts, types.GeminiPart{
							FileData: &types.GeminiFileData{FileURI: url},
						})
					}
				}
			}
		}
		return parts
	default:
		if s, ok := v.(fmt.Stringer); ok {
			text := s.String()
			if strings.TrimSpace(text) != "" {
				return []types.GeminiPart{{Text: text}}
			}
		}
	}
	return nil
}

func convertToolCallsToGeminiParts(toolCalls []interface{}) []types.GeminiPart {
	var parts []types.GeminiPart
	for _, raw := range toolCalls {
		tc, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		fn, ok := tc["function"].(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := fn["name"].(string)
		argsStr, _ := fn["arguments"].(string)
		args := map[string]interface{}{}
		if strings.TrimSpace(argsStr) != "" {
			_ = json.Unmarshal([]byte(argsStr), &args)
		}
		if name == "" {
			continue
		}
		parts = append(parts, types.GeminiPart{
			FunctionCall: &types.GeminiFunctionCall{
				Name: name,
				Args: args,
			},
		})
	}
	return parts
}

func buildToolResponseParts(msg map[string]interface{}) []types.GeminiPart {
	name, _ := msg["name"].(string)
	toolCallID, _ := msg["tool_call_id"].(string)
	content := msg["content"]

	response := map[string]interface{}{}
	switch v := content.(type) {
	case string:
		var parsed interface{}
		if strings.TrimSpace(v) != "" && json.Unmarshal([]byte(v), &parsed) == nil {
			response["content"] = parsed
		} else {
			response["content"] = v
		}
	default:
		response["content"] = v
	}
	if toolCallID != "" {
		response["tool_call_id"] = toolCallID
	}

	if name == "" {
		name = "tool_result"
	}

	return []types.GeminiPart{
		{
			FunctionResponse: &types.GeminiFunctionResponse{
				Name:     name,
				Response: response,
			},
		},
	}
}

func convertOpenAIToolsToGeminiTools(tools []interface{}) []types.GeminiTool {
	var declarations []types.GeminiFunctionDeclaration

	for _, raw := range tools {
		tool, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		fn, ok := tool["function"].(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := fn["name"].(string)
		if name == "" {
			continue
		}

		desc, _ := fn["description"].(string)
		declarations = append(declarations, types.GeminiFunctionDeclaration{
			Name:        name,
			Description: desc,
			Parameters:  fn["parameters"],
		})
	}

	if len(declarations) == 0 {
		return nil
	}

	return []types.GeminiTool{{FunctionDeclarations: declarations}}
}

func buildGenerationConfig(reqMap map[string]interface{}) *types.GeminiGenerationConfig {
	cfg := &types.GeminiGenerationConfig{}
	hasValue := false

	if v, ok := reqMap["temperature"].(float64); ok {
		cfg.Temperature = &v
		hasValue = true
	}
	if v, ok := reqMap["top_p"].(float64); ok {
		cfg.TopP = &v
		hasValue = true
	}
	if v, ok := reqMap["topP"].(float64); ok {
		cfg.TopP = &v
		hasValue = true
	}
	if v, ok := reqMap["top_k"].(float64); ok {
		intV := int(v)
		cfg.TopK = &intV
		hasValue = true
	}
	if v, ok := reqMap["topK"].(float64); ok {
		intV := int(v)
		cfg.TopK = &intV
		hasValue = true
	}
	if v, ok := reqMap["max_output_tokens"].(float64); ok {
		cfg.MaxOutputTokens = int(v)
		hasValue = true
	}
	if v, ok := reqMap["max_tokens"].(float64); ok {
		cfg.MaxOutputTokens = int(v)
		hasValue = true
	}
	if v, ok := reqMap["max_completion_tokens"].(float64); ok {
		cfg.MaxOutputTokens = int(v)
		hasValue = true
	}

	if stop, ok := reqMap["stop"].([]interface{}); ok && len(stop) > 0 {
		var stops []string
		for _, item := range stop {
			if s, ok := item.(string); ok && s != "" {
				stops = append(stops, s)
			}
		}
		if len(stops) > 0 {
			cfg.StopSequences = stops
			hasValue = true
		}
	} else if stopStr, ok := reqMap["stop"].(string); ok && stopStr != "" {
		cfg.StopSequences = []string{stopStr}
		hasValue = true
	}

	if !hasValue {
		return nil
	}
	return cfg
}

func handleMultiChannel(
	c *gin.Context,
	envCfg *config.EnvConfig,
	cfgManager *config.ConfigManager,
	channelScheduler *scheduler.ChannelScheduler,
	bodyBytes []byte,
	geminiReq *types.GeminiRequest,
	model string,
	isStream bool,
	userID string,
	startTime time.Time,
) {
	metricsManager := channelScheduler.GetGeminiMetricsManager()
	common.HandleMultiChannelFailover(
		c,
		envCfg,
		channelScheduler,
		scheduler.ChannelKindGemini,
		"Gemini",
		userID,
		model,
		func(selection *scheduler.SelectionResult) common.MultiChannelAttemptResult {
			upstream := selection.Upstream
			channelIndex := selection.ChannelIndex

			if upstream == nil {
				return common.MultiChannelAttemptResult{}
			}

			baseURLs := upstream.GetAllBaseURLs()
			sortedURLResults := channelScheduler.GetSortedURLsForChannel(scheduler.ChannelKindGemini, channelIndex, baseURLs)

			handled, successKey, successBaseURLIdx, failoverErr, usage, lastErr := common.TryUpstreamWithAllKeys(
				c,
				envCfg,
				cfgManager,
				channelScheduler,
				scheduler.ChannelKindGemini,
				"Gemini",
				metricsManager,
				upstream,
				sortedURLResults,
				bodyBytes,
				isStream,
				func(upstream *config.UpstreamConfig, failedKeys map[string]bool) (string, error) {
					return cfgManager.GetNextGeminiAPIKey(upstream, failedKeys)
				},
				func(c *gin.Context, upstreamCopy *config.UpstreamConfig, apiKey string) (*http.Request, error) {
					return buildProviderRequest(c, upstreamCopy, upstreamCopy.BaseURL, apiKey, geminiReq, model, isStream)
				},
				func(apiKey string) {
					_ = cfgManager.DeprioritizeAPIKey(apiKey)
				},
				func(url string) {
					channelScheduler.MarkURLFailure(scheduler.ChannelKindGemini, channelIndex, url)
				},
				func(url string) {
					channelScheduler.MarkURLSuccess(scheduler.ChannelKindGemini, channelIndex, url)
				},
				func(c *gin.Context, resp *http.Response, upstreamCopy *config.UpstreamConfig, apiKey string) (*types.Usage, error) {
					return handleSuccess(c, resp, upstreamCopy.ServiceType, envCfg, startTime, geminiReq, model, isStream)
				},
				model,
				selection.ChannelIndex,
				channelScheduler.GetChannelLogStore(scheduler.ChannelKindGemini),
			)

			return common.MultiChannelAttemptResult{
				Handled:           handled,
				Attempted:         true,
				SuccessKey:        successKey,
				SuccessBaseURLIdx: successBaseURLIdx,
				FailoverError:     failoverErr,
				Usage:             usage,
				LastError:         lastErr,
			}
		},
		nil,
		func(ctx *gin.Context, failoverErr *common.FailoverError, lastError error) {
			handleAllChannelsFailed(ctx, failoverErr, lastError)
		},
	)
}

func handleSingleChannel(
	c *gin.Context,
	envCfg *config.EnvConfig,
	cfgManager *config.ConfigManager,
	channelScheduler *scheduler.ChannelScheduler,
	bodyBytes []byte,
	geminiReq *types.GeminiRequest,
	model string,
	isStream bool,
	startTime time.Time,
) {
	upstream, channelIndex, err := cfgManager.GetCurrentGeminiUpstreamWithIndex()
	if err != nil {
		c.JSON(503, types.GeminiError{
			Error: types.GeminiErrorDetail{
				Code:    503,
				Message: "No Gemini upstream configured",
				Status:  "UNAVAILABLE",
			},
		})
		return
	}

	if len(upstream.APIKeys) == 0 {
		c.JSON(503, types.GeminiError{
			Error: types.GeminiErrorDetail{
				Code:    503,
				Message: fmt.Sprintf("No API keys configured for upstream \"%s\"", upstream.Name),
				Status:  "UNAVAILABLE",
			},
		})
		return
	}

	metricsManager := channelScheduler.GetGeminiMetricsManager()
	baseURLs := upstream.GetAllBaseURLs()
	urlResults := common.BuildDefaultURLResults(baseURLs)

	handled, _, _, lastFailoverError, _, lastError := common.TryUpstreamWithAllKeys(
		c,
		envCfg,
		cfgManager,
		channelScheduler,
		scheduler.ChannelKindGemini,
		"Gemini",
		metricsManager,
		upstream,
		urlResults,
		bodyBytes,
		isStream,
		func(upstream *config.UpstreamConfig, failedKeys map[string]bool) (string, error) {
			return cfgManager.GetNextGeminiAPIKey(upstream, failedKeys)
		},
		func(c *gin.Context, upstreamCopy *config.UpstreamConfig, apiKey string) (*http.Request, error) {
			return buildProviderRequest(c, upstreamCopy, upstreamCopy.BaseURL, apiKey, geminiReq, model, isStream)
		},
		func(apiKey string) {
			_ = cfgManager.DeprioritizeAPIKey(apiKey)
		},
		nil,
		nil,
		func(c *gin.Context, resp *http.Response, upstreamCopy *config.UpstreamConfig, apiKey string) (*types.Usage, error) {
			return handleSuccess(c, resp, upstreamCopy.ServiceType, envCfg, startTime, geminiReq, model, isStream)
		},
		model,
		channelIndex,
		channelScheduler.GetChannelLogStore(scheduler.ChannelKindGemini),
	)
	if handled {
		return
	}

	log.Printf("[Gemini-Error] 所有 API密钥都失败了")
	handleAllKeysFailed(c, lastFailoverError, lastError)
}

func ensureThoughtSignatures(geminiReq *types.GeminiRequest) {
	for i := range geminiReq.Contents {
		for j := range geminiReq.Contents[i].Parts {
			part := &geminiReq.Contents[i].Parts[j]
			if part.FunctionCall != nil && part.FunctionCall.ThoughtSignature == "" {
				part.FunctionCall.ThoughtSignature = types.DummyThoughtSignature
			}
		}
	}
}

func stripThoughtSignature(geminiReq *types.GeminiRequest) {
	for i := range geminiReq.Contents {
		for j := range geminiReq.Contents[i].Parts {
			part := &geminiReq.Contents[i].Parts[j]
			if part.FunctionCall != nil {
				part.FunctionCall.ThoughtSignature = types.StripThoughtSignatureMarker
			}
		}
	}
}

func cloneGeminiRequest(req *types.GeminiRequest) *types.GeminiRequest {
	clone := &types.GeminiRequest{}
	data, _ := json.Marshal(req)
	json.Unmarshal(data, clone)
	return clone
}

func buildProviderRequest(
	c *gin.Context,
	upstream *config.UpstreamConfig,
	baseURL string,
	apiKey string,
	geminiReq *types.GeminiRequest,
	model string,
	isStream bool,
) (*http.Request, error) {
	mappedModel := config.RedirectModel(model, upstream)

	var requestBody []byte
	var url string
	var err error

	switch upstream.ServiceType {
	case "gemini":
		reqToUse := geminiReq

		if upstream.StripThoughtSignature {
			reqCopy := cloneGeminiRequest(geminiReq)
			stripThoughtSignature(reqCopy)
			reqToUse = reqCopy
		} else if upstream.InjectDummyThoughtSignature {
			reqCopy := cloneGeminiRequest(geminiReq)
			ensureThoughtSignatures(reqCopy)
			reqToUse = reqCopy
		}

		requestBody, err = json.Marshal(reqToUse)
		if err != nil {
			return nil, err
		}

		action := "generateContent"
		if isStream {
			action = "streamGenerateContent"
		}
		url = fmt.Sprintf("%s/v1beta/models/%s:%s", strings.TrimRight(baseURL, "/"), mappedModel, action)
		if isStream {
			url += "?alt=sse"
		}

	case "claude":
		claudeReq, err := converters.GeminiToClaudeRequest(geminiReq, mappedModel)
		if err != nil {
			return nil, err
		}
		claudeReq["stream"] = isStream
		requestBody, err = json.Marshal(claudeReq)
		if err != nil {
			return nil, err
		}
		url = fmt.Sprintf("%s/v1/messages", strings.TrimRight(baseURL, "/"))

	case "openai":
		openaiReq, err := converters.GeminiToOpenAIRequest(geminiReq, mappedModel)
		if err != nil {
			return nil, err
		}
		openaiReq["stream"] = isStream
		requestBody, err = json.Marshal(openaiReq)
		if err != nil {
			return nil, err
		}
		url = fmt.Sprintf("%s/v1/chat/completions", strings.TrimRight(baseURL, "/"))

	case "responses":
		responsesReq, err := converters.GeminiToResponsesRequest(geminiReq, mappedModel)
		if err != nil {
			return nil, err
		}
		responsesReq["stream"] = isStream
		requestBody, err = json.Marshal(responsesReq)
		if err != nil {
			return nil, err
		}
		url = fmt.Sprintf("%s/v1/responses", strings.TrimRight(baseURL, "/"))

	default:
		reqToUse := geminiReq

		if upstream.StripThoughtSignature {
			reqCopy := cloneGeminiRequest(geminiReq)
			stripThoughtSignature(reqCopy)
			reqToUse = reqCopy
		} else if upstream.InjectDummyThoughtSignature {
			reqCopy := cloneGeminiRequest(geminiReq)
			ensureThoughtSignatures(reqCopy)
			reqToUse = reqCopy
		}

		requestBody, err = json.Marshal(reqToUse)
		if err != nil {
			return nil, err
		}
		action := "generateContent"
		if isStream {
			action = "streamGenerateContent"
		}
		url = fmt.Sprintf("%s/v1beta/models/%s:%s", strings.TrimRight(baseURL, "/"), mappedModel, action)
		if isStream {
			url += "?alt=sse"
		}
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), "POST", url, bytes.NewReader(requestBody))
	if err != nil {
		return nil, err
	}

	req.Header = utils.PrepareUpstreamHeaders(c, req.URL.Host)
	req.Header.Set("Content-Type", "application/json")

	switch upstream.ServiceType {
	case "gemini":
		utils.SetGeminiAuthenticationHeader(req.Header, apiKey)
	case "claude":
		utils.SetAuthenticationHeader(req.Header, apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	case "openai":
		utils.SetAuthenticationHeader(req.Header, apiKey)
	case "responses":
		utils.SetAuthenticationHeader(req.Header, apiKey)
	default:
		utils.SetGeminiAuthenticationHeader(req.Header, apiKey)
	}

	utils.ApplyCustomHeaders(req.Header, upstream.CustomHeaders)

	return req, nil
}

func handleSuccess(
	c *gin.Context,
	resp *http.Response,
	upstreamType string,
	envCfg *config.EnvConfig,
	startTime time.Time,
	geminiReq *types.GeminiRequest,
	model string,
	isStream bool,
) (*types.Usage, error) {
	defer resp.Body.Close()

	if isStream {
		return handleStreamSuccess(c, resp, upstreamType, envCfg, startTime, model), nil
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(500, types.GeminiError{
			Error: types.GeminiErrorDetail{
				Code:    500,
				Message: "Failed to read response",
				Status:  "INTERNAL",
			},
		})
		return nil, err
	}

	if envCfg.EnableResponseLogs {
		responseTime := time.Since(startTime).Milliseconds()
		log.Printf("[Gemini-Timing] 响应完成: %dms, 状态: %d", responseTime, resp.StatusCode)
	}

	var geminiResp *types.GeminiResponse

	switch upstreamType {
	case "gemini":
		if err := json.Unmarshal(bodyBytes, &geminiResp); err != nil {
			preview := bodyBytes
			if len(preview) > 100 {
				preview = preview[:100]
			}
			log.Printf("[Gemini-InvalidBody] 响应体解析失败: %v, body前100字节: %s", err, preview)
			return nil, fmt.Errorf("%w: %v", common.ErrInvalidResponseBody, err)
		}

	case "claude":
		var claudeResp map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &claudeResp); err != nil {
			preview := bodyBytes
			if len(preview) > 100 {
				preview = preview[:100]
			}
			log.Printf("[Gemini-InvalidBody] Claude响应体解析失败: %v, body前100字节: %s", err, preview)
			return nil, fmt.Errorf("%w: %v", common.ErrInvalidResponseBody, err)
		}
		geminiResp, err = converters.ClaudeResponseToGemini(claudeResp)
		if err != nil {
			log.Printf("[Gemini-InvalidBody] Claude响应转换失败: %v", err)
			return nil, fmt.Errorf("%w: %v", common.ErrInvalidResponseBody, err)
		}

	case "openai":
		var openaiResp map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &openaiResp); err != nil {
			preview := bodyBytes
			if len(preview) > 100 {
				preview = preview[:100]
			}
			log.Printf("[Gemini-InvalidBody] OpenAI响应体解析失败: %v, body前100字节: %s", err, preview)
			return nil, fmt.Errorf("%w: %v", common.ErrInvalidResponseBody, err)
		}
		geminiResp, err = converters.OpenAIResponseToGemini(openaiResp)
		if err != nil {
			log.Printf("[Gemini-InvalidBody] OpenAI响应转换失败: %v", err)
			return nil, fmt.Errorf("%w: %v", common.ErrInvalidResponseBody, err)
		}

	case "responses":
		var responsesResp map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &responsesResp); err != nil {
			preview := bodyBytes
			if len(preview) > 100 {
				preview = preview[:100]
			}
			log.Printf("[Gemini-InvalidBody] Responses响应体解析失败: %v, body前100字节: %s", err, preview)
			return nil, fmt.Errorf("%w: %v", common.ErrInvalidResponseBody, err)
		}
		geminiResp, err = converters.ResponsesResponseToGemini(responsesResp)
		if err != nil {
			log.Printf("[Gemini-InvalidBody] Responses响应转换失败: %v", err)
			return nil, fmt.Errorf("%w: %v", common.ErrInvalidResponseBody, err)
		}

	default:
		c.Data(resp.StatusCode, "application/json", bodyBytes)
		return nil, nil
	}

	respBytes, err := json.Marshal(geminiResp)
	if err != nil {
		c.Data(resp.StatusCode, "application/json", bodyBytes)
		return nil, nil
	}

	c.Data(resp.StatusCode, "application/json", respBytes)

	var usage *types.Usage
	if geminiResp.UsageMetadata != nil {
		usage = &types.Usage{
			InputTokens:  geminiResp.UsageMetadata.PromptTokenCount - geminiResp.UsageMetadata.CachedContentTokenCount,
			OutputTokens: geminiResp.UsageMetadata.CandidatesTokenCount,
		}
	}

	return usage, nil
}

func handleAllChannelsFailed(c *gin.Context, failoverErr *common.FailoverError, lastError error) {
	if failoverErr != nil {
		c.Data(failoverErr.Status, "application/json", failoverErr.Body)
		return
	}

	errMsg := "All channels failed"
	if lastError != nil {
		errMsg = lastError.Error()
	}

	c.JSON(503, types.GeminiError{
		Error: types.GeminiErrorDetail{
			Code:    503,
			Message: errMsg,
			Status:  "UNAVAILABLE",
		},
	})
}

func handleAllKeysFailed(c *gin.Context, failoverErr *common.FailoverError, lastError error) {
	if failoverErr != nil {
		c.Data(failoverErr.Status, "application/json", failoverErr.Body)
		return
	}

	errMsg := "All API keys failed"
	if lastError != nil {
		errMsg = lastError.Error()
	}

	c.JSON(503, types.GeminiError{
		Error: types.GeminiErrorDetail{
			Code:    503,
			Message: errMsg,
			Status:  "UNAVAILABLE",
		},
	})
}

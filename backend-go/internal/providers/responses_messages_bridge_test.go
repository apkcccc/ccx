package providers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/BenedictKing/ccx/internal/config"
	"github.com/BenedictKing/ccx/internal/types"
	"github.com/gin-gonic/gin"
)

func TestResponsesProvider_BuildResponsesRequestFromClaude(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &ResponsesProvider{}
	upstream := &config.UpstreamConfig{
		ServiceType: "responses",
		ModelMapping: map[string]string{
			"gpt-5": "gpt-5.2",
		},
	}

	body := []byte(`{
		"model":"gpt-5",
		"system":"you are helpful",
		"max_tokens":1024,
		"temperature":0.2,
		"stream":true,
		"messages":[
			{"role":"user","content":[{"type":"text","text":"hello"}]},
			{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"weather","input":{"city":"shanghai"}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"sunny"}]}
		],
		"tools":[{"name":"weather","description":"weather tool","input_schema":{"type":"object"}}]
	}`)

	result, err := provider.buildResponsesRequestFromClaude(body, upstream)
	if err != nil {
		t.Fatalf("buildResponsesRequestFromClaude() err = %v", err)
	}

	if result["model"] != "gpt-5.2" {
		t.Fatalf("model = %v, want gpt-5.2", result["model"])
	}
	if result["instructions"] != "you are helpful" {
		t.Fatalf("instructions = %v, want you are helpful", result["instructions"])
	}
	if result["max_output_tokens"] != float64(1024) && result["max_output_tokens"] != 1024 {
		t.Fatalf("max_output_tokens = %v, want 1024", result["max_output_tokens"])
	}
	if result["stream"] != true {
		t.Fatalf("stream = %v, want true", result["stream"])
	}

	input, ok := result["input"].([]map[string]interface{})
	if !ok {
		// marshal/unmarshal fallback for interface dynamic shape
		b, _ := json.Marshal(result["input"])
		var tmp []map[string]interface{}
		if err := json.Unmarshal(b, &tmp); err != nil {
			t.Fatalf("input decode err: %v", err)
		}
		input = tmp
	}

	if len(input) != 3 {
		t.Fatalf("len(input) = %d, want 3", len(input))
	}
	if input[0]["type"] != "message" {
		t.Fatalf("input[0].type = %v, want message", input[0]["type"])
	}
	content0, ok := input[0]["content"].([]map[string]interface{})
	if !ok {
		b, _ := json.Marshal(input[0]["content"])
		if err := json.Unmarshal(b, &content0); err != nil {
			t.Fatalf("input[0].content decode err: %v", err)
		}
	}
	if len(content0) != 1 || content0[0]["type"] != "input_text" {
		t.Fatalf("input[0].content = %#v, want single input_text block", content0)
	}
	if input[1]["type"] != "function_call" {
		t.Fatalf("input[1].type = %v, want function_call", input[1]["type"])
	}
	if input[2]["type"] != "function_call_output" {
		t.Fatalf("input[2].type = %v, want function_call_output", input[2]["type"])
	}

	tools, ok := result["tools"].([]map[string]interface{})
	if !ok {
		b, _ := json.Marshal(result["tools"])
		var tmp []map[string]interface{}
		if err := json.Unmarshal(b, &tmp); err != nil {
			t.Fatalf("tools decode err: %v", err)
		}
		tools = tmp
	}
	if len(tools) != 1 || tools[0]["name"] != "weather" {
		t.Fatalf("tools = %#v, want weather tool", tools)
	}
	// 验证 type 字段必须存在且为 "function"
	if tools[0]["type"] != "function" {
		t.Fatalf("tools[0][\"type\"] = %v, want \"function\"", tools[0]["type"])
	}
	// 验证 parameters 字段必须存在
	if tools[0]["parameters"] == nil {
		t.Fatalf("tools[0][\"parameters\"] is nil, want non-nil")
	}
}

func TestResponsesProvider_BuildResponsesRequestFromClaude_AssistantTextUsesOutputText(t *testing.T) {
	provider := &ResponsesProvider{}
	upstream := &config.UpstreamConfig{
		ServiceType: "responses",
	}

	body := []byte(`{
		"model":"gpt-5",
		"messages":[
			{"role":"user","content":[{"type":"text","text":"先查一下 front"}]},
			{"role":"assistant","content":[
				{"type":"text","text":"我先看一下。"},
				{"type":"tool_use","id":"call_1","name":"ls","input":{"path":"front"}}
			]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"frontend"}]},
			{"role":"assistant","content":"已经拿到结果。"},
			{"role":"user","content":[{"type":"text","text":"继续总结"}]}
		]
	}`)

	result, err := provider.buildResponsesRequestFromClaude(body, upstream)
	if err != nil {
		t.Fatalf("buildResponsesRequestFromClaude() err = %v", err)
	}

	input, ok := result["input"].([]map[string]interface{})
	if !ok {
		b, _ := json.Marshal(result["input"])
		var tmp []map[string]interface{}
		if err := json.Unmarshal(b, &tmp); err != nil {
			t.Fatalf("input decode err: %v", err)
		}
		input = tmp
	}

	if len(input) != 6 {
		t.Fatalf("len(input) = %d, want 6", len(input))
	}

	assertMessageBlockType := func(index int, wantRole, wantType, wantText string) {
		t.Helper()
		if input[index]["type"] != "message" {
			t.Fatalf("input[%d].type = %v, want message", index, input[index]["type"])
		}
		if input[index]["role"] != wantRole {
			t.Fatalf("input[%d].role = %v, want %s", index, input[index]["role"], wantRole)
		}

		content, ok := input[index]["content"].([]map[string]interface{})
		if !ok {
			b, _ := json.Marshal(input[index]["content"])
			if err := json.Unmarshal(b, &content); err != nil {
				t.Fatalf("input[%d].content decode err: %v", index, err)
			}
		}
		if len(content) != 1 {
			t.Fatalf("input[%d].content len = %d, want 1", index, len(content))
		}
		if content[0]["type"] != wantType || content[0]["text"] != wantText {
			t.Fatalf("input[%d].content[0] = %#v, want type=%s text=%q", index, content[0], wantType, wantText)
		}
	}

	assertMessageBlockType(0, "user", "input_text", "先查一下 front")
	assertMessageBlockType(1, "assistant", "output_text", "我先看一下。")
	if input[2]["type"] != "function_call" {
		t.Fatalf("input[2].type = %v, want function_call", input[2]["type"])
	}
	if input[3]["type"] != "function_call_output" {
		t.Fatalf("input[3].type = %v, want function_call_output", input[3]["type"])
	}
	assertMessageBlockType(4, "assistant", "output_text", "已经拿到结果。")
	assertMessageBlockType(5, "user", "input_text", "继续总结")
}

func TestExtractResponsesInstructions_SkipsLeadingBillingHeader(t *testing.T) {
	instructions := extractResponsesInstructions([]interface{}{
		map[string]interface{}{"type": "text", "text": "x-anthropic-billing-header: cc_version=2.1.78"},
		map[string]interface{}{"type": "text", "text": "你是一个有帮助的助手"},
		map[string]interface{}{"type": "text", "text": "回答时简洁"},
	})

	want := "你是一个有帮助的助手\n回答时简洁"
	if instructions != want {
		t.Fatalf("instructions = %q, want %q", instructions, want)
	}
}

func TestExtractResponsesInstructions_PreservesNonBillingSystem(t *testing.T) {
	instructions := extractResponsesInstructions([]interface{}{
		map[string]interface{}{"type": "text", "text": "正常 system 指令"},
		map[string]interface{}{"type": "text", "text": "继续执行"},
	})

	want := "正常 system 指令\n继续执行"
	if instructions != want {
		t.Fatalf("instructions = %q, want %q", instructions, want)
	}
}

func TestResponsesProvider_BuildResponsesRequestFromClaude_OmitsInstructionsWhenOnlyBillingHeader(t *testing.T) {
	provider := &ResponsesProvider{}
	upstream := &config.UpstreamConfig{ServiceType: "responses"}

	body := []byte(`{
		"model":"gpt-5",
		"system":[
			{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.78.a43"}
		],
		"messages":[
			{"role":"user","content":"hello"}
		]
	}`)

	result, err := provider.buildResponsesRequestFromClaude(body, upstream)
	if err != nil {
		t.Fatalf("buildResponsesRequestFromClaude() err = %v", err)
	}
	if _, exists := result["instructions"]; exists {
		t.Fatalf("instructions exists = true, want false; value = %v", result["instructions"])
	}
}

func TestResponsesProvider_BuildResponsesRequestFromClaude_FiltersBillingHeaderFromInstructions(t *testing.T) {
	provider := &ResponsesProvider{}
	upstream := &config.UpstreamConfig{ServiceType: "responses"}

	body := []byte(`{
		"model":"gpt-5",
		"system":[
			{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.78.a43"},
			{"type":"text","text":"只保留真正的 system 指令"}
		],
		"messages":[
			{"role":"user","content":"hello"}
		]
	}`)

	result, err := provider.buildResponsesRequestFromClaude(body, upstream)
	if err != nil {
		t.Fatalf("buildResponsesRequestFromClaude() err = %v", err)
	}
	if result["instructions"] != "只保留真正的 system 指令" {
		t.Fatalf("instructions = %v, want 只保留真正的 system 指令", result["instructions"])
	}
}

func TestResponsesProvider_ConvertToClaudeResponse(t *testing.T) {
	provider := &ResponsesProvider{}
	providerResp := &types.ProviderResponse{
		StatusCode: http.StatusOK,
		Headers:    map[string][]string{"Content-Type": {"application/json"}},
		Body: []byte(`{
			"id":"resp_123",
			"status":"completed",
			"output":[
				{"type":"message","content":[{"type":"output_text","text":"hello world"}]},
				{"type":"function_call","call_id":"call_1","name":"weather","arguments":"{\"city\":\"shanghai\"}"}
			],
			"usage":{"input_tokens":12,"output_tokens":34}
		}`),
	}

	claudeResp, err := provider.ConvertToClaudeResponse(providerResp)
	if err != nil {
		t.Fatalf("ConvertToClaudeResponse() err = %v", err)
	}
	if claudeResp.ID != "resp_123" {
		t.Fatalf("ID = %s, want resp_123", claudeResp.ID)
	}
	if claudeResp.StopReason != "tool_use" {
		t.Fatalf("StopReason = %s, want tool_use", claudeResp.StopReason)
	}
	if len(claudeResp.Content) != 2 {
		t.Fatalf("len(Content) = %d, want 2", len(claudeResp.Content))
	}
	if claudeResp.Content[0].Type != "text" || claudeResp.Content[0].Text != "hello world" {
		t.Fatalf("content[0] = %#v, want text hello world", claudeResp.Content[0])
	}
	if claudeResp.Content[1].Type != "tool_use" || claudeResp.Content[1].Name != "weather" {
		t.Fatalf("content[1] = %#v, want tool_use weather", claudeResp.Content[1])
	}
	if claudeResp.Usage == nil || claudeResp.Usage.InputTokens != 12 || claudeResp.Usage.OutputTokens != 34 {
		t.Fatalf("usage = %#v, want input=12 output=34", claudeResp.Usage)
	}
}

func TestResponsesProvider_ConvertToClaudeResponse_StripsEmptyReadPages(t *testing.T) {
	provider := &ResponsesProvider{}
	providerResp := &types.ProviderResponse{
		StatusCode: http.StatusOK,
		Headers:    map[string][]string{"Content-Type": {"application/json"}},
		Body: []byte(`{
			"id":"resp_123",
			"status":"completed",
			"output":[
				{"type":"function_call","call_id":"call_1","name":"Read","arguments":"{\"file_path\":\"/tmp/x\",\"pages\":\"\"}"}
			]
		}`),
	}

	claudeResp, err := provider.ConvertToClaudeResponse(providerResp)
	if err != nil {
		t.Fatalf("ConvertToClaudeResponse() err = %v", err)
	}

	if len(claudeResp.Content) != 1 {
		t.Fatalf("len(Content) = %d, want 1", len(claudeResp.Content))
	}

	block := claudeResp.Content[0]
	if block.Type != "tool_use" || block.Name != "Read" {
		t.Fatalf("content[0] = %#v, want tool_use Read", block)
	}

	input, ok := block.Input.(map[string]interface{})
	if !ok {
		t.Fatalf("content[0].Input type = %T, want map[string]interface{}", block.Input)
	}
	if _, exists := input["pages"]; exists {
		t.Fatalf("content[0].Input.pages exists = true, want false; input=%v", input)
	}
	if input["file_path"] != "/tmp/x" {
		t.Fatalf("content[0].Input.file_path = %v, want /tmp/x", input["file_path"])
	}
}

func TestResponsesProvider_BuildProviderRequestBody_NormalizesPassthroughInputTextTypes(t *testing.T) {
	provider := &ResponsesProvider{}
	upstream := &config.UpstreamConfig{
		ServiceType: "responses",
	}

	body := []byte(`{
		"model":"gpt-5",
		"input":[
			{"type":"message","role":"user","content":[{"type":"output_text","text":"用户消息"}]},
			{"type":"message","role":"assistant","content":[{"type":"input_text","text":"助手消息"}]},
			{"type":"message","role":"assistant","content":[{"type":"refusal","text":"不能回答"}]}
		]
	}`)

	reqBody, _, err := provider.buildProviderRequestBody("/v1/responses", body, upstream)
	if err != nil {
		t.Fatalf("buildProviderRequestBody() err = %v", err)
	}

	reqMap, ok := reqBody.(map[string]interface{})
	if !ok {
		t.Fatalf("provider request type = %T, want map[string]interface{}", reqBody)
	}

	input, ok := reqMap["input"].([]interface{})
	if !ok {
		t.Fatalf("reqMap[input] type = %T, want []interface{}", reqMap["input"])
	}

	assertContentType := func(index int, wantType string) {
		t.Helper()
		item, ok := input[index].(map[string]interface{})
		if !ok {
			t.Fatalf("input[%d] type = %T, want map[string]interface{}", index, input[index])
		}
		content, ok := item["content"].([]interface{})
		if !ok || len(content) != 1 {
			t.Fatalf("input[%d].content = %#v, want single block", index, item["content"])
		}
		block, ok := content[0].(map[string]interface{})
		if !ok {
			t.Fatalf("input[%d].content[0] type = %T, want map[string]interface{}", index, content[0])
		}
		if block["type"] != wantType {
			t.Fatalf("input[%d].content[0].type = %v, want %s", index, block["type"], wantType)
		}
	}

	assertContentType(0, "input_text")
	assertContentType(1, "output_text")
	assertContentType(2, "refusal")
}

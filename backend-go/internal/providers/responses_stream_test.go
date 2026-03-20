package providers

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func extractInputJSONDelta(t *testing.T, events []string) string {
	t.Helper()
	for _, event := range events {
		for _, line := range strings.Split(event, "\n") {
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			jsonStr := strings.TrimPrefix(line, "data: ")

			var data map[string]interface{}
			if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
				continue
			}
			if data["type"] != "content_block_delta" {
				continue
			}
			delta, ok := data["delta"].(map[string]interface{})
			if !ok {
				continue
			}
			if delta["type"] != "input_json_delta" {
				continue
			}
			if partial, ok := delta["partial_json"].(string); ok && partial != "" {
				return partial
			}
		}
	}

	t.Fatalf("input_json_delta not found, events=%v", events)
	return ""
}

func TestResponsesProvider_HandleStreamResponse_StripsEmptyReadPages(t *testing.T) {
	body := `event: response.output_item.added
data: {"type":"response.output_item.added","item":{"type":"function_call","call_id":"call_1","name":"Read"}}

event: response.function_call_arguments.delta
data: {"type":"response.function_call_arguments.delta","delta":"{\"file_path\":\"/tmp/x\",\"pages\":\"\"}"}

event: response.output_item.done
data: {"type":"response.output_item.done","item":{"type":"function_call","call_id":"call_1","name":"Read","arguments":"{\"file_path\":\"/tmp/x\",\"pages\":\"\"}"}}

event: response.completed
data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":1,"output_tokens":1}}}

`

	provider := &ResponsesProvider{}
	eventChan, errChan, err := provider.HandleStreamResponse(io.NopCloser(strings.NewReader(body)))
	if err != nil {
		t.Fatalf("HandleStreamResponse returned error: %v", err)
	}

	events := collectStreamEvents(eventChan)
	select {
	case streamErr := <-errChan:
		if streamErr != nil {
			t.Fatalf("unexpected stream error: %v", streamErr)
		}
	default:
	}

	partialJSON := extractInputJSONDelta(t, events)
	var input map[string]interface{}
	if err := json.Unmarshal([]byte(partialJSON), &input); err != nil {
		t.Fatalf("partial_json is not valid JSON: %v, partial_json=%q", err, partialJSON)
	}
	if _, exists := input["pages"]; exists {
		t.Fatalf("pages exists = true, want false; input=%v", input)
	}
	if input["file_path"] != "/tmp/x" {
		t.Fatalf("file_path = %v, want /tmp/x", input["file_path"])
	}
}

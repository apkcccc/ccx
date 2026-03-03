package providers

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func collectStreamEvents(ch <-chan string) []string {
	events := make([]string, 0, 8)
	for event := range ch {
		events = append(events, event)
	}
	return events
}

func extractMessageDelta(t *testing.T, events []string) map[string]interface{} {
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
			if data["type"] == "message_delta" {
				return data
			}
		}
	}

	t.Fatalf("message_delta not found, events=%v", events)
	return nil
}

func TestGeminiHandleStreamResponse_UsageOnlyChunkStillAffectsMessageDeltaUsage(t *testing.T) {
	body := strings.Join([]string{
		`data: {"candidates":[{"content":{"parts":[{"text":"OK"}]},"finishReason":"STOP"}]}`,
		`data: {"usageMetadata":{"promptTokenCount":123,"candidatesTokenCount":8}}`,
		"",
	}, "\n")

	provider := &GeminiProvider{}
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

	messageDelta := extractMessageDelta(t, events)
	usage, ok := messageDelta["usage"].(map[string]interface{})
	if !ok {
		t.Fatalf("usage field missing in message_delta: %v", messageDelta)
	}

	inputTokens, _ := usage["input_tokens"].(float64)
	outputTokens, _ := usage["output_tokens"].(float64)
	if int(inputTokens) != 123 || int(outputTokens) != 8 {
		t.Fatalf("unexpected usage in message_delta, want input=123 output=8, got input=%v output=%v", usage["input_tokens"], usage["output_tokens"])
	}
}

func TestGeminiHandleStreamResponse_MessageDeltaAlwaysContainsUsage(t *testing.T) {
	body := strings.Join([]string{
		`data: {"candidates":[{"content":{"parts":[{"text":"hello"}]},"finishReason":"STOP"}]}`,
		"",
	}, "\n")

	provider := &GeminiProvider{}
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

	messageDelta := extractMessageDelta(t, events)
	usage, ok := messageDelta["usage"].(map[string]interface{})
	if !ok {
		t.Fatalf("usage field missing in message_delta: %v", messageDelta)
	}

	inputTokens, _ := usage["input_tokens"].(float64)
	outputTokens, _ := usage["output_tokens"].(float64)
	if int(inputTokens) != 0 || int(outputTokens) != 0 {
		t.Fatalf("expected fallback usage 0/0 when upstream usage absent, got input=%v output=%v", usage["input_tokens"], usage["output_tokens"])
	}
}

func TestGeminiHandleStreamResponse_SafetyFinishReasonMapsToEndTurn(t *testing.T) {
	body := strings.Join([]string{
		`data: {"candidates":[{"content":{"parts":[{"text":"blocked"}]},"finishReason":"SAFETY"}]}`,
		"",
	}, "\n")

	provider := &GeminiProvider{}
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

	messageDelta := extractMessageDelta(t, events)
	delta, ok := messageDelta["delta"].(map[string]interface{})
	if !ok {
		t.Fatalf("delta field missing in message_delta: %v", messageDelta)
	}

	stopReason, _ := delta["stop_reason"].(string)
	if stopReason != "end_turn" {
		t.Fatalf("expected stop_reason=end_turn for SAFETY finishReason, got %q", stopReason)
	}
}

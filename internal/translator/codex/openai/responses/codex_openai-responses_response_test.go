package responses

import (
	"context"
	"testing"

	"github.com/tidwall/gjson"
)

func TestConvertCodexResponseToOpenAIResponsesNonStream_AcceptsResponseDoneEnvelope(t *testing.T) {
	out := ConvertCodexResponseToOpenAIResponsesNonStream(context.Background(), "gpt-5.3-codex", nil, nil, []byte(`{
		"type":"response.done",
		"response":{"id":"resp_123","object":"response"}
	}`), nil)

	if got := gjson.Get(out, "id").String(); got != "resp_123" {
		t.Fatalf("expected id resp_123, got %q", got)
	}
}

func TestConvertCodexResponseToOpenAIResponsesNonStream_AcceptsDirectResponseObject(t *testing.T) {
	out := ConvertCodexResponseToOpenAIResponsesNonStream(context.Background(), "gpt-5.3-codex", nil, nil, []byte(`{
		"id":"resp_123",
		"object":"response",
		"status":"completed"
	}`), nil)

	if got := gjson.Get(out, "id").String(); got != "resp_123" {
		t.Fatalf("expected id resp_123, got %q", got)
	}
}

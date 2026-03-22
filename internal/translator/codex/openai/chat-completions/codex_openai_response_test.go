package chat_completions

import (
	"context"
	"testing"

	"github.com/tidwall/gjson"
)

func TestConvertCodexResponseToOpenAI_StreamSetsModelFromResponseCreated(t *testing.T) {
	ctx := context.Background()
	var param any

	modelName := "gpt-5.3-codex"

	out := ConvertCodexResponseToOpenAI(ctx, modelName, nil, nil, []byte(`data: {"type":"response.created","response":{"id":"resp_123","created_at":1700000000,"model":"gpt-5.3-codex"}}`), &param)
	if len(out) != 0 {
		t.Fatalf("expected no output for response.created, got %d chunks", len(out))
	}

	out = ConvertCodexResponseToOpenAI(ctx, modelName, nil, nil, []byte(`data: {"type":"response.output_text.delta","delta":"hello"}`), &param)
	if len(out) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(out))
	}

	gotModel := gjson.Get(out[0], "model").String()
	if gotModel != modelName {
		t.Fatalf("expected model %q, got %q", modelName, gotModel)
	}
}

func TestConvertCodexResponseToOpenAI_FirstChunkUsesRequestModelName(t *testing.T) {
	ctx := context.Background()
	var param any

	modelName := "gpt-5.3-codex"

	out := ConvertCodexResponseToOpenAI(ctx, modelName, nil, nil, []byte(`data: {"type":"response.output_text.delta","delta":"hello"}`), &param)
	if len(out) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(out))
	}

	gotModel := gjson.Get(out[0], "model").String()
	if gotModel != modelName {
		t.Fatalf("expected model %q, got %q", modelName, gotModel)
	}
}

func TestConvertCodexResponseToOpenAI_StreamTreatsResponseDoneAsCompleted(t *testing.T) {
	ctx := context.Background()
	var param any

	out := ConvertCodexResponseToOpenAI(ctx, "gpt-5.3-codex", nil, nil, []byte(`data: {"type":"response.done","response":{"id":"resp_123"}}`), &param)
	if len(out) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(out))
	}
	if got := gjson.Get(out[0], "choices.0.finish_reason").String(); got != "stop" {
		t.Fatalf("expected finish_reason stop, got %q", got)
	}
}

func TestConvertCodexResponseToOpenAINonStream_AcceptsDirectResponseObject(t *testing.T) {
	out := ConvertCodexResponseToOpenAINonStream(context.Background(), "gpt-5.3-codex", nil, nil, []byte(`{
		"id":"resp_123",
		"object":"response",
		"created_at":1700000000,
		"model":"gpt-5.3-codex",
		"output":[{"type":"message","content":[{"type":"output_text","text":"hello"}]}],
		"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}
	}`), nil)

	if got := gjson.Get(out, "id").String(); got != "resp_123" {
		t.Fatalf("expected id resp_123, got %q", got)
	}
	if got := gjson.Get(out, "choices.0.message.content").String(); got != "hello" {
		t.Fatalf("expected content hello, got %q", got)
	}
}

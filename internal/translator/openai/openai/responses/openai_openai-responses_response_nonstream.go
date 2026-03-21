package responses

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ConvertOpenAIChatCompletionsResponseToOpenAIResponsesNonStream builds a single Responses JSON
// from a non-streaming OpenAI Chat Completions response.
func ConvertOpenAIChatCompletionsResponseToOpenAIResponsesNonStream(_ context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, _ *any) string {
	root := gjson.ParseBytes(rawJSON)

	// Basic response scaffold
	resp := `{"id":"","object":"response","created_at":0,"status":"completed","background":false,"error":null,"incomplete_details":null}`

	// id: use provider id if present, otherwise synthesize
	id := root.Get("id").String()
	if id == "" {
		id = fmt.Sprintf("resp_%x_%d", time.Now().UnixNano(), atomic.AddUint64(&responseIDCounter, 1))
	}
	resp, _ = sjson.Set(resp, "id", id)

	// created_at: map from chat.completion created
	created := root.Get("created").Int()
	if created == 0 {
		created = time.Now().Unix()
	}
	resp, _ = sjson.Set(resp, "created_at", created)

	// Echo request fields when available (aligns with streaming path behavior)
	if len(requestRawJSON) > 0 {
		req := gjson.ParseBytes(requestRawJSON)
		if v := req.Get("instructions"); v.Exists() {
			resp, _ = sjson.Set(resp, "instructions", v.String())
		}
		if v := req.Get("max_output_tokens"); v.Exists() {
			resp, _ = sjson.Set(resp, "max_output_tokens", v.Int())
		} else {
			// Also support max_tokens from chat completion style
			if v = req.Get("max_tokens"); v.Exists() {
				resp, _ = sjson.Set(resp, "max_output_tokens", v.Int())
			}
		}
		if v := req.Get("max_tool_calls"); v.Exists() {
			resp, _ = sjson.Set(resp, "max_tool_calls", v.Int())
		}
		if v := req.Get("model"); v.Exists() {
			resp, _ = sjson.Set(resp, "model", v.String())
		} else if v = root.Get("model"); v.Exists() {
			resp, _ = sjson.Set(resp, "model", v.String())
		}
		if v := req.Get("parallel_tool_calls"); v.Exists() {
			resp, _ = sjson.Set(resp, "parallel_tool_calls", v.Bool())
		}
		if v := req.Get("previous_response_id"); v.Exists() {
			resp, _ = sjson.Set(resp, "previous_response_id", v.String())
		}
		if v := req.Get("prompt_cache_key"); v.Exists() {
			resp, _ = sjson.Set(resp, "prompt_cache_key", v.String())
		}
		if v := req.Get("reasoning"); v.Exists() {
			resp, _ = sjson.Set(resp, "reasoning", v.Value())
		}
		if v := req.Get("safety_identifier"); v.Exists() {
			resp, _ = sjson.Set(resp, "safety_identifier", v.String())
		}
		if v := req.Get("service_tier"); v.Exists() {
			resp, _ = sjson.Set(resp, "service_tier", v.String())
		}
		if v := req.Get("store"); v.Exists() {
			resp, _ = sjson.Set(resp, "store", v.Bool())
		}
		if v := req.Get("temperature"); v.Exists() {
			resp, _ = sjson.Set(resp, "temperature", v.Float())
		}
		if v := req.Get("text"); v.Exists() {
			resp, _ = sjson.Set(resp, "text", v.Value())
		}
		if v := req.Get("tool_choice"); v.Exists() {
			resp, _ = sjson.Set(resp, "tool_choice", v.Value())
		}
		if v := req.Get("tools"); v.Exists() {
			resp, _ = sjson.Set(resp, "tools", v.Value())
		}
		if v := req.Get("top_logprobs"); v.Exists() {
			resp, _ = sjson.Set(resp, "top_logprobs", v.Int())
		}
		if v := req.Get("top_p"); v.Exists() {
			resp, _ = sjson.Set(resp, "top_p", v.Float())
		}
		if v := req.Get("truncation"); v.Exists() {
			resp, _ = sjson.Set(resp, "truncation", v.String())
		}
		if v := req.Get("user"); v.Exists() {
			resp, _ = sjson.Set(resp, "user", v.Value())
		}
		if v := req.Get("metadata"); v.Exists() {
			resp, _ = sjson.Set(resp, "metadata", v.Value())
		}
	} else if v := root.Get("model"); v.Exists() {
		// Fallback model from response
		resp, _ = sjson.Set(resp, "model", v.String())
	}

	// Build output list from choices[...]
	outputsWrapper := `{"arr":[]}`
	// Detect and capture reasoning content if present
	rcText := gjson.GetBytes(rawJSON, "choices.0.message.reasoning_content").String()
	includeReasoning := rcText != ""
	if !includeReasoning && len(requestRawJSON) > 0 {
		includeReasoning = gjson.GetBytes(requestRawJSON, "reasoning").Exists()
	}
	if includeReasoning {
		rid := id
		if strings.HasPrefix(rid, "resp_") {
			rid = strings.TrimPrefix(rid, "resp_")
		}
		// Prefer summary_text from reasoning_content; encrypted_content is optional
		reasoningItem := `{"id":"","type":"reasoning","encrypted_content":"","summary":[]}`
		reasoningItem, _ = sjson.Set(reasoningItem, "id", fmt.Sprintf("rs_%s", rid))
		if rcText != "" {
			reasoningItem, _ = sjson.Set(reasoningItem, "summary.0.type", "summary_text")
			reasoningItem, _ = sjson.Set(reasoningItem, "summary.0.text", rcText)
		}
		outputsWrapper, _ = sjson.SetRaw(outputsWrapper, "arr.-1", reasoningItem)
	}

	if choices := root.Get("choices"); choices.Exists() && choices.IsArray() {
		choices.ForEach(func(_, choice gjson.Result) bool {
			msg := choice.Get("message")
			if msg.Exists() {
				// Text message part
				if c := msg.Get("content"); c.Exists() && c.String() != "" {
					item := `{"id":"","type":"message","status":"completed","content":[{"type":"output_text","annotations":[],"logprobs":[],"text":""}],"role":"assistant"}`
					item, _ = sjson.Set(item, "id", fmt.Sprintf("msg_%s_%d", id, int(choice.Get("index").Int())))
					item, _ = sjson.Set(item, "content.0.text", c.String())
					outputsWrapper, _ = sjson.SetRaw(outputsWrapper, "arr.-1", item)
				}

				// Function/tool calls
				if tcs := msg.Get("tool_calls"); tcs.Exists() && tcs.IsArray() {
					tcs.ForEach(func(_, tc gjson.Result) bool {
						callID := tc.Get("id").String()
						name := tc.Get("function.name").String()
						args := tc.Get("function.arguments").String()
						item := `{"id":"","type":"function_call","status":"completed","arguments":"","call_id":"","name":""}`
						item, _ = sjson.Set(item, "id", fmt.Sprintf("fc_%s", callID))
						item, _ = sjson.Set(item, "arguments", args)
						item, _ = sjson.Set(item, "call_id", callID)
						item, _ = sjson.Set(item, "name", name)
						outputsWrapper, _ = sjson.SetRaw(outputsWrapper, "arr.-1", item)
						return true
					})
				}
			}
			return true
		})
	}
	if gjson.Get(outputsWrapper, "arr.#").Int() > 0 {
		resp, _ = sjson.SetRaw(resp, "output", gjson.Get(outputsWrapper, "arr").Raw)
	}

	// usage mapping
	if usage := root.Get("usage"); usage.Exists() {
		// Map common tokens
		if usage.Get("prompt_tokens").Exists() || usage.Get("completion_tokens").Exists() || usage.Get("total_tokens").Exists() {
			resp, _ = sjson.Set(resp, "usage.input_tokens", usage.Get("prompt_tokens").Int())
			if d := usage.Get("prompt_tokens_details.cached_tokens"); d.Exists() {
				resp, _ = sjson.Set(resp, "usage.input_tokens_details.cached_tokens", d.Int())
			}
			resp, _ = sjson.Set(resp, "usage.output_tokens", usage.Get("completion_tokens").Int())
			// Reasoning tokens not available in Chat Completions; set only if present under output_tokens_details
			if d := usage.Get("output_tokens_details.reasoning_tokens"); d.Exists() {
				resp, _ = sjson.Set(resp, "usage.output_tokens_details.reasoning_tokens", d.Int())
			}
			resp, _ = sjson.Set(resp, "usage.total_tokens", usage.Get("total_tokens").Int())
		} else {
			// Fallback to raw usage object if structure differs
			resp, _ = sjson.Set(resp, "usage", usage.Value())
		}
	}

	return resp
}

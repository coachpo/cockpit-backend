package openai

import (
	"encoding/json"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// convertCompletionsRequestToChatCompletions converts OpenAI completions API request to chat completions format.
// This allows the completions endpoint to use the existing chat completions infrastructure.
//
// Parameters:
//   - rawJSON: The raw JSON bytes of the completions request
//
// Returns:
//   - []byte: The converted chat completions request
func convertCompletionsRequestToChatCompletions(rawJSON []byte) []byte {
	root := gjson.ParseBytes(rawJSON)

	// Extract prompt from completions request
	prompt := root.Get("prompt").String()
	if prompt == "" {
		prompt = "Complete this:"
	}

	// Create chat completions structure
	out := `{"model":"","messages":[{"role":"user","content":""}]}`

	// Set model
	if model := root.Get("model"); model.Exists() {
		out, _ = sjson.Set(out, "model", model.String())
	}

	// Set the prompt as user message content
	out, _ = sjson.Set(out, "messages.0.content", prompt)

	// Copy other parameters from completions to chat completions
	if maxTokens := root.Get("max_tokens"); maxTokens.Exists() {
		out, _ = sjson.Set(out, "max_tokens", maxTokens.Int())
	}

	if temperature := root.Get("temperature"); temperature.Exists() {
		out, _ = sjson.Set(out, "temperature", temperature.Float())
	}

	if topP := root.Get("top_p"); topP.Exists() {
		out, _ = sjson.Set(out, "top_p", topP.Float())
	}

	if frequencyPenalty := root.Get("frequency_penalty"); frequencyPenalty.Exists() {
		out, _ = sjson.Set(out, "frequency_penalty", frequencyPenalty.Float())
	}

	if presencePenalty := root.Get("presence_penalty"); presencePenalty.Exists() {
		out, _ = sjson.Set(out, "presence_penalty", presencePenalty.Float())
	}

	if stop := root.Get("stop"); stop.Exists() {
		out, _ = sjson.SetRaw(out, "stop", stop.Raw)
	}

	if stream := root.Get("stream"); stream.Exists() {
		out, _ = sjson.Set(out, "stream", stream.Bool())
	}

	if logprobs := root.Get("logprobs"); logprobs.Exists() {
		out, _ = sjson.Set(out, "logprobs", logprobs.Bool())
	}

	if topLogprobs := root.Get("top_logprobs"); topLogprobs.Exists() {
		out, _ = sjson.Set(out, "top_logprobs", topLogprobs.Int())
	}

	if echo := root.Get("echo"); echo.Exists() {
		out, _ = sjson.Set(out, "echo", echo.Bool())
	}

	return []byte(out)
}

// convertChatCompletionsResponseToCompletions converts chat completions API response back to completions format.
// This ensures the completions endpoint returns data in the expected format.
//
// Parameters:
//   - rawJSON: The raw JSON bytes of the chat completions response
//
// Returns:
//   - []byte: The converted completions response
func convertChatCompletionsResponseToCompletions(rawJSON []byte) []byte {
	root := gjson.ParseBytes(rawJSON)

	// Base completions response structure
	out := `{"id":"","object":"text_completion","created":0,"model":"","choices":[]}`

	// Copy basic fields
	if id := root.Get("id"); id.Exists() {
		out, _ = sjson.Set(out, "id", id.String())
	}

	if created := root.Get("created"); created.Exists() {
		out, _ = sjson.Set(out, "created", created.Int())
	}

	if model := root.Get("model"); model.Exists() {
		out, _ = sjson.Set(out, "model", model.String())
	}

	if usage := root.Get("usage"); usage.Exists() {
		out, _ = sjson.SetRaw(out, "usage", usage.Raw)
	}

	// Convert choices from chat completions to completions format
	var choices []interface{}
	if chatChoices := root.Get("choices"); chatChoices.Exists() && chatChoices.IsArray() {
		chatChoices.ForEach(func(_, choice gjson.Result) bool {
			completionsChoice := map[string]interface{}{
				"index": choice.Get("index").Int(),
			}

			// Extract text content from message.content
			if message := choice.Get("message"); message.Exists() {
				if content := message.Get("content"); content.Exists() {
					completionsChoice["text"] = content.String()
				}
			} else if delta := choice.Get("delta"); delta.Exists() {
				// For streaming responses, use delta.content
				if content := delta.Get("content"); content.Exists() {
					completionsChoice["text"] = content.String()
				}
			}

			// Copy finish_reason
			if finishReason := choice.Get("finish_reason"); finishReason.Exists() {
				completionsChoice["finish_reason"] = finishReason.String()
			}

			// Copy logprobs if present
			if logprobs := choice.Get("logprobs"); logprobs.Exists() {
				completionsChoice["logprobs"] = logprobs.Value()
			}

			choices = append(choices, completionsChoice)
			return true
		})
	}

	if len(choices) > 0 {
		choicesJSON, _ := json.Marshal(choices)
		out, _ = sjson.SetRaw(out, "choices", string(choicesJSON))
	}

	return []byte(out)
}

// convertChatCompletionsStreamChunkToCompletions converts a streaming chat completions chunk to completions format.
// This handles the real-time conversion of streaming response chunks and filters out empty text responses.
//
// Parameters:
//   - chunkData: The raw JSON bytes of a single chat completions stream chunk
//
// Returns:
//   - []byte: The converted completions stream chunk, or nil if should be filtered out
func convertChatCompletionsStreamChunkToCompletions(chunkData []byte) []byte {
	root := gjson.ParseBytes(chunkData)

	// Check if this chunk has any meaningful content
	hasContent := false
	hasUsage := root.Get("usage").Exists()
	if chatChoices := root.Get("choices"); chatChoices.Exists() && chatChoices.IsArray() {
		chatChoices.ForEach(func(_, choice gjson.Result) bool {
			// Check if delta has content or finish_reason
			if delta := choice.Get("delta"); delta.Exists() {
				if content := delta.Get("content"); content.Exists() && content.String() != "" {
					hasContent = true
					return false // Break out of forEach
				}
			}
			// Also check for finish_reason to ensure we don't skip final chunks
			if finishReason := choice.Get("finish_reason"); finishReason.Exists() && finishReason.String() != "" && finishReason.String() != "null" {
				hasContent = true
				return false // Break out of forEach
			}
			return true
		})
	}

	// If no meaningful content and no usage, return nil to indicate this chunk should be skipped
	if !hasContent && !hasUsage {
		return nil
	}

	// Base completions stream response structure
	out := `{"id":"","object":"text_completion","created":0,"model":"","choices":[]}`

	// Copy basic fields
	if id := root.Get("id"); id.Exists() {
		out, _ = sjson.Set(out, "id", id.String())
	}

	if created := root.Get("created"); created.Exists() {
		out, _ = sjson.Set(out, "created", created.Int())
	}

	if model := root.Get("model"); model.Exists() {
		out, _ = sjson.Set(out, "model", model.String())
	}

	// Convert choices from chat completions delta to completions format
	var choices []interface{}
	if chatChoices := root.Get("choices"); chatChoices.Exists() && chatChoices.IsArray() {
		chatChoices.ForEach(func(_, choice gjson.Result) bool {
			completionsChoice := map[string]interface{}{
				"index": choice.Get("index").Int(),
			}

			// Extract text content from delta.content
			if delta := choice.Get("delta"); delta.Exists() {
				if content := delta.Get("content"); content.Exists() && content.String() != "" {
					completionsChoice["text"] = content.String()
				} else {
					completionsChoice["text"] = ""
				}
			} else {
				completionsChoice["text"] = ""
			}

			// Copy finish_reason
			if finishReason := choice.Get("finish_reason"); finishReason.Exists() && finishReason.String() != "null" {
				completionsChoice["finish_reason"] = finishReason.String()
			}

			// Copy logprobs if present
			if logprobs := choice.Get("logprobs"); logprobs.Exists() {
				completionsChoice["logprobs"] = logprobs.Value()
			}

			choices = append(choices, completionsChoice)
			return true
		})
	}

	if len(choices) > 0 {
		choicesJSON, _ := json.Marshal(choices)
		out, _ = sjson.SetRaw(out, "choices", string(choicesJSON))
	}

	// Copy usage if present
	if usage := root.Get("usage"); usage.Exists() {
		out, _ = sjson.SetRaw(out, "usage", usage.Raw)
	}

	return []byte(out)
}

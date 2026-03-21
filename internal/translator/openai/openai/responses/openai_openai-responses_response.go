package responses

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ConvertOpenAIChatCompletionsResponseToOpenAIResponses converts OpenAI Chat Completions streaming chunks
// to OpenAI Responses SSE events (response.*).
func ConvertOpenAIChatCompletionsResponseToOpenAIResponses(ctx context.Context, modelName string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) []string {
	st := ensureResponsesState(param)

	if bytes.HasPrefix(rawJSON, []byte("data:")) {
		rawJSON = bytes.TrimSpace(rawJSON[5:])
	}

	rawJSON = bytes.TrimSpace(rawJSON)
	if len(rawJSON) == 0 {
		return []string{}
	}
	if bytes.Equal(rawJSON, []byte("[DONE]")) {
		return []string{}
	}

	root := gjson.ParseBytes(rawJSON)
	obj := root.Get("object")
	if obj.Exists() && obj.String() != "" && obj.String() != "chat.completion.chunk" {
		return []string{}
	}
	if !root.Get("choices").Exists() || !root.Get("choices").IsArray() {
		return []string{}
	}

	captureResponsesChunkUsage(st, root)

	nextSeq := func() int { st.Seq++; return st.Seq }
	var out []string

	out = appendResponseStartEvents(st, root, nextSeq, out)

	stopReasoning := func(text string) {
		// Emit reasoning done events
		textDone := `{"type":"response.reasoning_summary_text.done","sequence_number":0,"item_id":"","output_index":0,"summary_index":0,"text":""}`
		textDone, _ = sjson.Set(textDone, "sequence_number", nextSeq())
		textDone, _ = sjson.Set(textDone, "item_id", st.ReasoningID)
		textDone, _ = sjson.Set(textDone, "output_index", st.ReasoningIndex)
		textDone, _ = sjson.Set(textDone, "text", text)
		out = append(out, emitRespEvent("response.reasoning_summary_text.done", textDone))
		partDone := `{"type":"response.reasoning_summary_part.done","sequence_number":0,"item_id":"","output_index":0,"summary_index":0,"part":{"type":"summary_text","text":""}}`
		partDone, _ = sjson.Set(partDone, "sequence_number", nextSeq())
		partDone, _ = sjson.Set(partDone, "item_id", st.ReasoningID)
		partDone, _ = sjson.Set(partDone, "output_index", st.ReasoningIndex)
		partDone, _ = sjson.Set(partDone, "part.text", text)
		out = append(out, emitRespEvent("response.reasoning_summary_part.done", partDone))
		outputItemDone := `{"type":"response.output_item.done","item":{"id":"","type":"reasoning","encrypted_content":"","summary":[{"type":"summary_text","text":""}]},"output_index":0,"sequence_number":0}`
		outputItemDone, _ = sjson.Set(outputItemDone, "sequence_number", nextSeq())
		outputItemDone, _ = sjson.Set(outputItemDone, "item.id", st.ReasoningID)
		outputItemDone, _ = sjson.Set(outputItemDone, "output_index", st.ReasoningIndex)
		outputItemDone, _ = sjson.Set(outputItemDone, "item.summary.text", text)
		out = append(out, emitRespEvent("response.output_item.done", outputItemDone))

		st.Reasonings = append(st.Reasonings, oaiToResponsesStateReasoning{ReasoningID: st.ReasoningID, ReasoningData: text})
		st.ReasoningID = ""
	}

	// choices[].delta content / tool_calls / reasoning_content
	if choices := root.Get("choices"); choices.Exists() && choices.IsArray() {
		choices.ForEach(func(_, choice gjson.Result) bool {
			idx := int(choice.Get("index").Int())
			delta := choice.Get("delta")
			if delta.Exists() {
				if c := delta.Get("content"); c.Exists() && c.String() != "" {
					// Ensure the message item and its first content part are announced before any text deltas
					if st.ReasoningID != "" {
						stopReasoning(st.ReasoningBuf.String())
						st.ReasoningBuf.Reset()
					}
					if !st.MsgItemAdded[idx] {
						item := `{"type":"response.output_item.added","sequence_number":0,"output_index":0,"item":{"id":"","type":"message","status":"in_progress","content":[],"role":"assistant"}}`
						item, _ = sjson.Set(item, "sequence_number", nextSeq())
						item, _ = sjson.Set(item, "output_index", idx)
						item, _ = sjson.Set(item, "item.id", fmt.Sprintf("msg_%s_%d", st.ResponseID, idx))
						out = append(out, emitRespEvent("response.output_item.added", item))
						st.MsgItemAdded[idx] = true
					}
					if !st.MsgContentAdded[idx] {
						part := `{"type":"response.content_part.added","sequence_number":0,"item_id":"","output_index":0,"content_index":0,"part":{"type":"output_text","annotations":[],"logprobs":[],"text":""}}`
						part, _ = sjson.Set(part, "sequence_number", nextSeq())
						part, _ = sjson.Set(part, "item_id", fmt.Sprintf("msg_%s_%d", st.ResponseID, idx))
						part, _ = sjson.Set(part, "output_index", idx)
						part, _ = sjson.Set(part, "content_index", 0)
						out = append(out, emitRespEvent("response.content_part.added", part))
						st.MsgContentAdded[idx] = true
					}

					msg := `{"type":"response.output_text.delta","sequence_number":0,"item_id":"","output_index":0,"content_index":0,"delta":"","logprobs":[]}`
					msg, _ = sjson.Set(msg, "sequence_number", nextSeq())
					msg, _ = sjson.Set(msg, "item_id", fmt.Sprintf("msg_%s_%d", st.ResponseID, idx))
					msg, _ = sjson.Set(msg, "output_index", idx)
					msg, _ = sjson.Set(msg, "content_index", 0)
					msg, _ = sjson.Set(msg, "delta", c.String())
					out = append(out, emitRespEvent("response.output_text.delta", msg))
					// aggregate for response.output
					if st.MsgTextBuf[idx] == nil {
						st.MsgTextBuf[idx] = &strings.Builder{}
					}
					st.MsgTextBuf[idx].WriteString(c.String())
				}

				// reasoning_content (OpenAI reasoning incremental text)
				if rc := delta.Get("reasoning_content"); rc.Exists() && rc.String() != "" {
					// On first appearance, add reasoning item and part
					if st.ReasoningID == "" {
						st.ReasoningID = fmt.Sprintf("rs_%s_%d", st.ResponseID, idx)
						st.ReasoningIndex = idx
						item := `{"type":"response.output_item.added","sequence_number":0,"output_index":0,"item":{"id":"","type":"reasoning","status":"in_progress","summary":[]}}`
						item, _ = sjson.Set(item, "sequence_number", nextSeq())
						item, _ = sjson.Set(item, "output_index", idx)
						item, _ = sjson.Set(item, "item.id", st.ReasoningID)
						out = append(out, emitRespEvent("response.output_item.added", item))
						part := `{"type":"response.reasoning_summary_part.added","sequence_number":0,"item_id":"","output_index":0,"summary_index":0,"part":{"type":"summary_text","text":""}}`
						part, _ = sjson.Set(part, "sequence_number", nextSeq())
						part, _ = sjson.Set(part, "item_id", st.ReasoningID)
						part, _ = sjson.Set(part, "output_index", st.ReasoningIndex)
						out = append(out, emitRespEvent("response.reasoning_summary_part.added", part))
					}
					// Append incremental text to reasoning buffer
					st.ReasoningBuf.WriteString(rc.String())
					msg := `{"type":"response.reasoning_summary_text.delta","sequence_number":0,"item_id":"","output_index":0,"summary_index":0,"delta":""}`
					msg, _ = sjson.Set(msg, "sequence_number", nextSeq())
					msg, _ = sjson.Set(msg, "item_id", st.ReasoningID)
					msg, _ = sjson.Set(msg, "output_index", st.ReasoningIndex)
					msg, _ = sjson.Set(msg, "delta", rc.String())
					out = append(out, emitRespEvent("response.reasoning_summary_text.delta", msg))
				}

				// tool calls
				if tcs := delta.Get("tool_calls"); tcs.Exists() && tcs.IsArray() {
					if st.ReasoningID != "" {
						stopReasoning(st.ReasoningBuf.String())
						st.ReasoningBuf.Reset()
					}
					// Before emitting any function events, if a message is open for this index,
					// close its text/content to match Codex expected ordering.
					if st.MsgItemAdded[idx] && !st.MsgItemDone[idx] {
						fullText := ""
						if b := st.MsgTextBuf[idx]; b != nil {
							fullText = b.String()
						}
						done := `{"type":"response.output_text.done","sequence_number":0,"item_id":"","output_index":0,"content_index":0,"text":"","logprobs":[]}`
						done, _ = sjson.Set(done, "sequence_number", nextSeq())
						done, _ = sjson.Set(done, "item_id", fmt.Sprintf("msg_%s_%d", st.ResponseID, idx))
						done, _ = sjson.Set(done, "output_index", idx)
						done, _ = sjson.Set(done, "content_index", 0)
						done, _ = sjson.Set(done, "text", fullText)
						out = append(out, emitRespEvent("response.output_text.done", done))

						partDone := `{"type":"response.content_part.done","sequence_number":0,"item_id":"","output_index":0,"content_index":0,"part":{"type":"output_text","annotations":[],"logprobs":[],"text":""}}`
						partDone, _ = sjson.Set(partDone, "sequence_number", nextSeq())
						partDone, _ = sjson.Set(partDone, "item_id", fmt.Sprintf("msg_%s_%d", st.ResponseID, idx))
						partDone, _ = sjson.Set(partDone, "output_index", idx)
						partDone, _ = sjson.Set(partDone, "content_index", 0)
						partDone, _ = sjson.Set(partDone, "part.text", fullText)
						out = append(out, emitRespEvent("response.content_part.done", partDone))

						itemDone := `{"type":"response.output_item.done","sequence_number":0,"output_index":0,"item":{"id":"","type":"message","status":"completed","content":[{"type":"output_text","annotations":[],"logprobs":[],"text":""}],"role":"assistant"}}`
						itemDone, _ = sjson.Set(itemDone, "sequence_number", nextSeq())
						itemDone, _ = sjson.Set(itemDone, "output_index", idx)
						itemDone, _ = sjson.Set(itemDone, "item.id", fmt.Sprintf("msg_%s_%d", st.ResponseID, idx))
						itemDone, _ = sjson.Set(itemDone, "item.content.0.text", fullText)
						out = append(out, emitRespEvent("response.output_item.done", itemDone))
						st.MsgItemDone[idx] = true
					}

					// Only emit item.added once per tool call and preserve call_id across chunks.
					newCallID := tcs.Get("0.id").String()
					nameChunk := tcs.Get("0.function.name").String()
					if nameChunk != "" {
						st.FuncNames[idx] = nameChunk
					}
					existingCallID := st.FuncCallIDs[idx]
					effectiveCallID := existingCallID
					shouldEmitItem := false
					if existingCallID == "" && newCallID != "" {
						// First time seeing a valid call_id for this index
						effectiveCallID = newCallID
						st.FuncCallIDs[idx] = newCallID
						shouldEmitItem = true
					}

					if shouldEmitItem && effectiveCallID != "" {
						o := `{"type":"response.output_item.added","sequence_number":0,"output_index":0,"item":{"id":"","type":"function_call","status":"in_progress","arguments":"","call_id":"","name":""}}`
						o, _ = sjson.Set(o, "sequence_number", nextSeq())
						o, _ = sjson.Set(o, "output_index", idx)
						o, _ = sjson.Set(o, "item.id", fmt.Sprintf("fc_%s", effectiveCallID))
						o, _ = sjson.Set(o, "item.call_id", effectiveCallID)
						name := st.FuncNames[idx]
						o, _ = sjson.Set(o, "item.name", name)
						out = append(out, emitRespEvent("response.output_item.added", o))
					}

					// Ensure args buffer exists for this index
					if st.FuncArgsBuf[idx] == nil {
						st.FuncArgsBuf[idx] = &strings.Builder{}
					}

					// Append arguments delta if available and we have a valid call_id to reference
					if args := tcs.Get("0.function.arguments"); args.Exists() && args.String() != "" {
						// Prefer an already known call_id; fall back to newCallID if first time
						refCallID := st.FuncCallIDs[idx]
						if refCallID == "" {
							refCallID = newCallID
						}
						if refCallID != "" {
							ad := `{"type":"response.function_call_arguments.delta","sequence_number":0,"item_id":"","output_index":0,"delta":""}`
							ad, _ = sjson.Set(ad, "sequence_number", nextSeq())
							ad, _ = sjson.Set(ad, "item_id", fmt.Sprintf("fc_%s", refCallID))
							ad, _ = sjson.Set(ad, "output_index", idx)
							ad, _ = sjson.Set(ad, "delta", args.String())
							out = append(out, emitRespEvent("response.function_call_arguments.delta", ad))
						}
						st.FuncArgsBuf[idx].WriteString(args.String())
					}
				}
			}

			// finish_reason triggers finalization, including text done/content done/item done,
			// reasoning done/part.done, function args done/item done, and completed
			if fr := choice.Get("finish_reason"); fr.Exists() && fr.String() != "" {
				// Emit message done events for all indices that started a message
				if len(st.MsgItemAdded) > 0 {
					// sort indices for deterministic order
					idxs := make([]int, 0, len(st.MsgItemAdded))
					for i := range st.MsgItemAdded {
						idxs = append(idxs, i)
					}
					for i := 0; i < len(idxs); i++ {
						for j := i + 1; j < len(idxs); j++ {
							if idxs[j] < idxs[i] {
								idxs[i], idxs[j] = idxs[j], idxs[i]
							}
						}
					}
					for _, i := range idxs {
						if st.MsgItemAdded[i] && !st.MsgItemDone[i] {
							fullText := ""
							if b := st.MsgTextBuf[i]; b != nil {
								fullText = b.String()
							}
							done := `{"type":"response.output_text.done","sequence_number":0,"item_id":"","output_index":0,"content_index":0,"text":"","logprobs":[]}`
							done, _ = sjson.Set(done, "sequence_number", nextSeq())
							done, _ = sjson.Set(done, "item_id", fmt.Sprintf("msg_%s_%d", st.ResponseID, i))
							done, _ = sjson.Set(done, "output_index", i)
							done, _ = sjson.Set(done, "content_index", 0)
							done, _ = sjson.Set(done, "text", fullText)
							out = append(out, emitRespEvent("response.output_text.done", done))

							partDone := `{"type":"response.content_part.done","sequence_number":0,"item_id":"","output_index":0,"content_index":0,"part":{"type":"output_text","annotations":[],"logprobs":[],"text":""}}`
							partDone, _ = sjson.Set(partDone, "sequence_number", nextSeq())
							partDone, _ = sjson.Set(partDone, "item_id", fmt.Sprintf("msg_%s_%d", st.ResponseID, i))
							partDone, _ = sjson.Set(partDone, "output_index", i)
							partDone, _ = sjson.Set(partDone, "content_index", 0)
							partDone, _ = sjson.Set(partDone, "part.text", fullText)
							out = append(out, emitRespEvent("response.content_part.done", partDone))

							itemDone := `{"type":"response.output_item.done","sequence_number":0,"output_index":0,"item":{"id":"","type":"message","status":"completed","content":[{"type":"output_text","annotations":[],"logprobs":[],"text":""}],"role":"assistant"}}`
							itemDone, _ = sjson.Set(itemDone, "sequence_number", nextSeq())
							itemDone, _ = sjson.Set(itemDone, "output_index", i)
							itemDone, _ = sjson.Set(itemDone, "item.id", fmt.Sprintf("msg_%s_%d", st.ResponseID, i))
							itemDone, _ = sjson.Set(itemDone, "item.content.0.text", fullText)
							out = append(out, emitRespEvent("response.output_item.done", itemDone))
							st.MsgItemDone[i] = true
						}
					}
				}

				if st.ReasoningID != "" {
					stopReasoning(st.ReasoningBuf.String())
					st.ReasoningBuf.Reset()
				}

				// Emit function call done events for any active function calls
				if len(st.FuncCallIDs) > 0 {
					idxs := make([]int, 0, len(st.FuncCallIDs))
					for i := range st.FuncCallIDs {
						idxs = append(idxs, i)
					}
					for i := 0; i < len(idxs); i++ {
						for j := i + 1; j < len(idxs); j++ {
							if idxs[j] < idxs[i] {
								idxs[i], idxs[j] = idxs[j], idxs[i]
							}
						}
					}
					for _, i := range idxs {
						callID := st.FuncCallIDs[i]
						if callID == "" || st.FuncItemDone[i] {
							continue
						}
						args := "{}"
						if b := st.FuncArgsBuf[i]; b != nil && b.Len() > 0 {
							args = b.String()
						}
						fcDone := `{"type":"response.function_call_arguments.done","sequence_number":0,"item_id":"","output_index":0,"arguments":""}`
						fcDone, _ = sjson.Set(fcDone, "sequence_number", nextSeq())
						fcDone, _ = sjson.Set(fcDone, "item_id", fmt.Sprintf("fc_%s", callID))
						fcDone, _ = sjson.Set(fcDone, "output_index", i)
						fcDone, _ = sjson.Set(fcDone, "arguments", args)
						out = append(out, emitRespEvent("response.function_call_arguments.done", fcDone))

						itemDone := `{"type":"response.output_item.done","sequence_number":0,"output_index":0,"item":{"id":"","type":"function_call","status":"completed","arguments":"","call_id":"","name":""}}`
						itemDone, _ = sjson.Set(itemDone, "sequence_number", nextSeq())
						itemDone, _ = sjson.Set(itemDone, "output_index", i)
						itemDone, _ = sjson.Set(itemDone, "item.id", fmt.Sprintf("fc_%s", callID))
						itemDone, _ = sjson.Set(itemDone, "item.arguments", args)
						itemDone, _ = sjson.Set(itemDone, "item.call_id", callID)
						itemDone, _ = sjson.Set(itemDone, "item.name", st.FuncNames[i])
						out = append(out, emitRespEvent("response.output_item.done", itemDone))
						st.FuncItemDone[i] = true
						st.FuncArgsDone[i] = true
					}
				}
				out = appendCompletedResponseEvent(st, requestRawJSON, nextSeq, out)
			}

			return true
		})
	}

	return out
}

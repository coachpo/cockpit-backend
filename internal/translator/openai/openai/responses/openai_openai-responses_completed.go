package responses

import (
	"fmt"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func appendCompletedResponseEvent(st *oaiToResponsesState, requestRawJSON []byte, nextSeq func() int, out []string) []string {
	completed := `{"type":"response.completed","sequence_number":0,"response":{"id":"","object":"response","created_at":0,"status":"completed","background":false,"error":null}}`
	completed, _ = sjson.Set(completed, "sequence_number", nextSeq())
	completed, _ = sjson.Set(completed, "response.id", st.ResponseID)
	completed, _ = sjson.Set(completed, "response.created_at", st.Created)
	if requestRawJSON != nil {
		req := gjson.ParseBytes(requestRawJSON)
		if v := req.Get("instructions"); v.Exists() {
			completed, _ = sjson.Set(completed, "response.instructions", v.String())
		}
		if v := req.Get("max_output_tokens"); v.Exists() {
			completed, _ = sjson.Set(completed, "response.max_output_tokens", v.Int())
		}
		if v := req.Get("max_tool_calls"); v.Exists() {
			completed, _ = sjson.Set(completed, "response.max_tool_calls", v.Int())
		}
		if v := req.Get("model"); v.Exists() {
			completed, _ = sjson.Set(completed, "response.model", v.String())
		}
		if v := req.Get("parallel_tool_calls"); v.Exists() {
			completed, _ = sjson.Set(completed, "response.parallel_tool_calls", v.Bool())
		}
		if v := req.Get("previous_response_id"); v.Exists() {
			completed, _ = sjson.Set(completed, "response.previous_response_id", v.String())
		}
		if v := req.Get("prompt_cache_key"); v.Exists() {
			completed, _ = sjson.Set(completed, "response.prompt_cache_key", v.String())
		}
		if v := req.Get("reasoning"); v.Exists() {
			completed, _ = sjson.Set(completed, "response.reasoning", v.Value())
		}
		if v := req.Get("safety_identifier"); v.Exists() {
			completed, _ = sjson.Set(completed, "response.safety_identifier", v.String())
		}
		if v := req.Get("service_tier"); v.Exists() {
			completed, _ = sjson.Set(completed, "response.service_tier", v.String())
		}
		if v := req.Get("store"); v.Exists() {
			completed, _ = sjson.Set(completed, "response.store", v.Bool())
		}
		if v := req.Get("temperature"); v.Exists() {
			completed, _ = sjson.Set(completed, "response.temperature", v.Float())
		}
		if v := req.Get("text"); v.Exists() {
			completed, _ = sjson.Set(completed, "response.text", v.Value())
		}
		if v := req.Get("tool_choice"); v.Exists() {
			completed, _ = sjson.Set(completed, "response.tool_choice", v.Value())
		}
		if v := req.Get("tools"); v.Exists() {
			completed, _ = sjson.Set(completed, "response.tools", v.Value())
		}
		if v := req.Get("top_logprobs"); v.Exists() {
			completed, _ = sjson.Set(completed, "response.top_logprobs", v.Int())
		}
		if v := req.Get("top_p"); v.Exists() {
			completed, _ = sjson.Set(completed, "response.top_p", v.Float())
		}
		if v := req.Get("truncation"); v.Exists() {
			completed, _ = sjson.Set(completed, "response.truncation", v.String())
		}
		if v := req.Get("user"); v.Exists() {
			completed, _ = sjson.Set(completed, "response.user", v.Value())
		}
		if v := req.Get("metadata"); v.Exists() {
			completed, _ = sjson.Set(completed, "response.metadata", v.Value())
		}
	}
	outputsWrapper := `{"arr":[]}`
	if len(st.Reasonings) > 0 {
		for _, r := range st.Reasonings {
			item := `{"id":"","type":"reasoning","summary":[{"type":"summary_text","text":""}]}`
			item, _ = sjson.Set(item, "id", r.ReasoningID)
			item, _ = sjson.Set(item, "summary.0.text", r.ReasoningData)
			outputsWrapper, _ = sjson.SetRaw(outputsWrapper, "arr.-1", item)
		}
	}
	if len(st.MsgItemAdded) > 0 {
		midxs := make([]int, 0, len(st.MsgItemAdded))
		for i := range st.MsgItemAdded {
			midxs = append(midxs, i)
		}
		for i := 0; i < len(midxs); i++ {
			for j := i + 1; j < len(midxs); j++ {
				if midxs[j] < midxs[i] {
					midxs[i], midxs[j] = midxs[j], midxs[i]
				}
			}
		}
		for _, i := range midxs {
			txt := ""
			if b := st.MsgTextBuf[i]; b != nil {
				txt = b.String()
			}
			item := `{"id":"","type":"message","status":"completed","content":[{"type":"output_text","annotations":[],"logprobs":[],"text":""}],"role":"assistant"}`
			item, _ = sjson.Set(item, "id", fmt.Sprintf("msg_%s_%d", st.ResponseID, i))
			item, _ = sjson.Set(item, "content.0.text", txt)
			outputsWrapper, _ = sjson.SetRaw(outputsWrapper, "arr.-1", item)
		}
	}
	if len(st.FuncArgsBuf) > 0 {
		idxs := make([]int, 0, len(st.FuncArgsBuf))
		for i := range st.FuncArgsBuf {
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
			args := ""
			if b := st.FuncArgsBuf[i]; b != nil {
				args = b.String()
			}
			callID := st.FuncCallIDs[i]
			name := st.FuncNames[i]
			item := `{"id":"","type":"function_call","status":"completed","arguments":"","call_id":"","name":""}`
			item, _ = sjson.Set(item, "id", fmt.Sprintf("fc_%s", callID))
			item, _ = sjson.Set(item, "arguments", args)
			item, _ = sjson.Set(item, "call_id", callID)
			item, _ = sjson.Set(item, "name", name)
			outputsWrapper, _ = sjson.SetRaw(outputsWrapper, "arr.-1", item)
		}
	}
	if gjson.Get(outputsWrapper, "arr.#").Int() > 0 {
		completed, _ = sjson.SetRaw(completed, "response.output", gjson.Get(outputsWrapper, "arr").Raw)
	}
	if st.UsageSeen {
		completed, _ = sjson.Set(completed, "response.usage.input_tokens", st.PromptTokens)
		completed, _ = sjson.Set(completed, "response.usage.input_tokens_details.cached_tokens", st.CachedTokens)
		completed, _ = sjson.Set(completed, "response.usage.output_tokens", st.CompletionTokens)
		if st.ReasoningTokens > 0 {
			completed, _ = sjson.Set(completed, "response.usage.output_tokens_details.reasoning_tokens", st.ReasoningTokens)
		}
		total := st.TotalTokens
		if total == 0 {
			total = st.PromptTokens + st.CompletionTokens
		}
		completed, _ = sjson.Set(completed, "response.usage.total_tokens", total)
	}
	out = append(out, emitRespEvent("response.completed", completed))
	return out
}

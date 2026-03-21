package responses

import (
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func ensureResponsesState(param *any) *oaiToResponsesState {
	if *param == nil {
		*param = &oaiToResponsesState{
			FuncArgsBuf:     make(map[int]*strings.Builder),
			FuncNames:       make(map[int]string),
			FuncCallIDs:     make(map[int]string),
			MsgTextBuf:      make(map[int]*strings.Builder),
			MsgItemAdded:    make(map[int]bool),
			MsgContentAdded: make(map[int]bool),
			MsgItemDone:     make(map[int]bool),
			FuncArgsDone:    make(map[int]bool),
			FuncItemDone:    make(map[int]bool),
			Reasonings:      make([]oaiToResponsesStateReasoning, 0),
		}
	}
	return (*param).(*oaiToResponsesState)
}

func captureResponsesChunkUsage(st *oaiToResponsesState, root gjson.Result) {
	if st == nil {
		return
	}
	if usage := root.Get("usage"); usage.Exists() {
		if v := usage.Get("prompt_tokens"); v.Exists() {
			st.PromptTokens = v.Int()
			st.UsageSeen = true
		}
		if v := usage.Get("prompt_tokens_details.cached_tokens"); v.Exists() {
			st.CachedTokens = v.Int()
			st.UsageSeen = true
		}
		if v := usage.Get("completion_tokens"); v.Exists() {
			st.CompletionTokens = v.Int()
			st.UsageSeen = true
		} else if v := usage.Get("output_tokens"); v.Exists() {
			st.CompletionTokens = v.Int()
			st.UsageSeen = true
		}
		if v := usage.Get("output_tokens_details.reasoning_tokens"); v.Exists() {
			st.ReasoningTokens = v.Int()
			st.UsageSeen = true
		} else if v := usage.Get("completion_tokens_details.reasoning_tokens"); v.Exists() {
			st.ReasoningTokens = v.Int()
			st.UsageSeen = true
		}
		if v := usage.Get("total_tokens"); v.Exists() {
			st.TotalTokens = v.Int()
			st.UsageSeen = true
		}
	}
}

func appendResponseStartEvents(st *oaiToResponsesState, root gjson.Result, nextSeq func() int, out []string) []string {
	if st == nil || st.Started {
		return out
	}
	st.ResponseID = root.Get("id").String()
	st.Created = root.Get("created").Int()
	st.MsgTextBuf = make(map[int]*strings.Builder)
	st.ReasoningBuf.Reset()
	st.ReasoningID = ""
	st.ReasoningIndex = 0
	st.FuncArgsBuf = make(map[int]*strings.Builder)
	st.FuncNames = make(map[int]string)
	st.FuncCallIDs = make(map[int]string)
	st.MsgItemAdded = make(map[int]bool)
	st.MsgContentAdded = make(map[int]bool)
	st.MsgItemDone = make(map[int]bool)
	st.FuncArgsDone = make(map[int]bool)
	st.FuncItemDone = make(map[int]bool)
	st.PromptTokens = 0
	st.CachedTokens = 0
	st.CompletionTokens = 0
	st.TotalTokens = 0
	st.ReasoningTokens = 0
	st.UsageSeen = false

	created := `{"type":"response.created","sequence_number":0,"response":{"id":"","object":"response","created_at":0,"status":"in_progress","background":false,"error":null,"output":[]}}`
	created, _ = sjson.Set(created, "sequence_number", nextSeq())
	created, _ = sjson.Set(created, "response.id", st.ResponseID)
	created, _ = sjson.Set(created, "response.created_at", st.Created)
	out = append(out, emitRespEvent("response.created", created))

	inprog := `{"type":"response.in_progress","sequence_number":0,"response":{"id":"","object":"response","created_at":0,"status":"in_progress"}}`
	inprog, _ = sjson.Set(inprog, "sequence_number", nextSeq())
	inprog, _ = sjson.Set(inprog, "response.id", st.ResponseID)
	inprog, _ = sjson.Set(inprog, "response.created_at", st.Created)
	out = append(out, emitRespEvent("response.in_progress", inprog))
	st.Started = true
	return out
}

package responses

import (
	"fmt"
	"strings"
)

type oaiToResponsesStateReasoning struct {
	ReasoningID   string
	ReasoningData string
}

type oaiToResponsesState struct {
	Seq            int
	ResponseID     string
	Created        int64
	Started        bool
	ReasoningID    string
	ReasoningIndex int
	// aggregation buffers for response.output
	// Per-output message text buffers by index
	MsgTextBuf   map[int]*strings.Builder
	ReasoningBuf strings.Builder
	Reasonings   []oaiToResponsesStateReasoning
	FuncArgsBuf  map[int]*strings.Builder
	FuncNames    map[int]string
	FuncCallIDs  map[int]string
	// message item state per output index
	MsgItemAdded    map[int]bool // whether response.output_item.added emitted for message
	MsgContentAdded map[int]bool // whether response.content_part.added emitted for message
	MsgItemDone     map[int]bool // whether message done events were emitted
	// function item done state
	FuncArgsDone map[int]bool
	FuncItemDone map[int]bool
	// usage aggregation
	PromptTokens     int64
	CachedTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	ReasoningTokens  int64
	UsageSeen        bool
}

// responseIDCounter provides a process-wide unique counter for synthesized response identifiers.
var responseIDCounter uint64

func emitRespEvent(event string, payload string) string {
	return fmt.Sprintf("event: %s\ndata: %s", event, payload)
}

package responses

import (
	. "github.com/coachpo/cockpit-backend/internal/constant"
	"github.com/coachpo/cockpit-backend/internal/interfaces"
	"github.com/coachpo/cockpit-backend/internal/translator/translator"
)

func init() {
	translator.Register(
		OpenaiResponse,
		Codex,
		ConvertOpenAIResponsesRequestToCodex,
		interfaces.TranslateResponse{
			Stream:    ConvertCodexResponseToOpenAIResponses,
			NonStream: ConvertCodexResponseToOpenAIResponsesNonStream,
		},
	)
}

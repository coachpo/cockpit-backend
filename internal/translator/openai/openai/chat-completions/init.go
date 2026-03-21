package chat_completions

import (
	. "github.com/coachpo/cockpit-backend/internal/constant"
	"github.com/coachpo/cockpit-backend/internal/interfaces"
	"github.com/coachpo/cockpit-backend/internal/translator/translator"
)

func init() {
	translator.Register(
		OpenAI,
		OpenAI,
		ConvertOpenAIRequestToOpenAI,
		interfaces.TranslateResponse{
			Stream:    ConvertOpenAIResponseToOpenAI,
			NonStream: ConvertOpenAIResponseToOpenAINonStream,
		},
	)
}

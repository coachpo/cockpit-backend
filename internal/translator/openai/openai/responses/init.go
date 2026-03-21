package responses

import (
	. "github.com/coachpo/cockpit-backend/internal/constant"
	"github.com/coachpo/cockpit-backend/internal/interfaces"
	"github.com/coachpo/cockpit-backend/internal/translator/translator"
)

func init() {
	translator.Register(
		OpenaiResponse,
		OpenAI,
		ConvertOpenAIResponsesRequestToOpenAIChatCompletions,
		interfaces.TranslateResponse{
			Stream:    ConvertOpenAIChatCompletionsResponseToOpenAIResponses,
			NonStream: ConvertOpenAIChatCompletionsResponseToOpenAIResponsesNonStream,
		},
	)
}

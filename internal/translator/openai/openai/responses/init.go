package responses

import (
	. "github.com/coachpo/cockpit-backend/internal/constant"
	"github.com/coachpo/cockpit-backend/internal/translator/translator"
	sdktranslator "github.com/coachpo/cockpit-backend/sdk/translator"
)

func init() {
	translator.Register(
		OpenaiResponse,
		OpenAI,
		ConvertOpenAIResponsesRequestToOpenAIChatCompletions,
		sdktranslator.ResponseTransform{
			Stream:    ConvertOpenAIChatCompletionsResponseToOpenAIResponses,
			NonStream: ConvertOpenAIChatCompletionsResponseToOpenAIResponsesNonStream,
		},
	)
}

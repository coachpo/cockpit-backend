package api

import (
	"net/http"

	managementHandlers "github.com/coachpo/cockpit-backend/internal/api/handlers/management"
	"github.com/coachpo/cockpit-backend/sdk/api/handlers/openai"
	"github.com/gin-gonic/gin"
)

// setupRoutes configures the API routes for the server.
// It defines the endpoints and associates them with their respective handlers.
func (s *Server) setupRoutes() {
	openaiHandlers := openai.NewOpenAIAPIHandler(s.handlers)
	openaiResponsesHandlers := openai.NewOpenAIResponsesAPIHandler(s.handlers)

	// OpenAI compatible API routes
	v1 := s.engine.Group("/v1")
	v1.Use(AuthMiddleware(s.accessManager))
	{
		v1.GET("/models", openaiHandlers.OpenAIModels)
		v1.POST("/chat/completions", openaiHandlers.ChatCompletions)
		v1.POST("/completions", openaiHandlers.Completions)
		v1.GET("/responses", openaiResponsesHandlers.ResponsesWebsocket)
		v1.POST("/responses", openaiResponsesHandlers.Responses)
		v1.POST("/responses/compact", openaiResponsesHandlers.Compact)
	}

	// Root endpoint
	s.engine.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "Cockpit Server",
			"endpoints": []string{
				"POST /v1/chat/completions",
				"POST /v1/completions",
				"GET /v1/models",
			},
		})
	})

	// OAuth callback endpoints (reuse main server port)
	// These endpoints receive provider redirects and persist
	// the short-lived code/state for the waiting goroutine.
	s.engine.GET("/codex/callback", func(c *gin.Context) {
		code := c.Query("code")
		state := c.Query("state")
		errStr := c.Query("error")
		if errStr == "" {
			errStr = c.Query("error_description")
		}
		if state != "" {
			_ = managementHandlers.StoreOAuthCallbackForPendingSession("codex", state, code, errStr)
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, oauthCallbackSuccessHTML)
	})

	// Management routes are registered lazily by registerManagementRoutes when a secret is configured.
}

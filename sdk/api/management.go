// Package api exposes helpers for embedding Cockpit.
//
// It wraps internal management handler types so external projects can integrate
// management endpoints without importing internal packages.
package api

import (
	internalmanagement "github.com/coachpo/cockpit-backend/internal/api/handlers/management"
	"github.com/coachpo/cockpit-backend/internal/config"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
	"github.com/gin-gonic/gin"
)

// ManagementTokenRequester exposes a limited subset of management endpoints for requesting tokens.
type ManagementTokenRequester interface {
	RequestCodexToken(*gin.Context)
	GetAuthStatus(c *gin.Context)
	PostOAuthCallback(c *gin.Context)
}

type managementTokenRequester struct {
	handler *internalmanagement.Handler
}

// NewManagementTokenRequester creates a limited management handler exposing only token request endpoints.
func NewManagementTokenRequester(cfg *config.Config, manager *coreauth.Manager) ManagementTokenRequester {
	return &managementTokenRequester{
		handler: internalmanagement.NewHandlerWithoutConfigFilePath(cfg, manager),
	}
}

func (m *managementTokenRequester) RequestCodexToken(c *gin.Context) {
	m.handler.RequestCodexToken(c)
}

func (m *managementTokenRequester) GetAuthStatus(c *gin.Context) {
	m.handler.GetAuthStatus(c)
}

func (m *managementTokenRequester) PostOAuthCallback(c *gin.Context) {
	m.handler.PostOAuthCallback(c)
}

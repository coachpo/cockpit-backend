// Package api exposes helpers for embedding Cockpit.
//
// It wraps internal management handler types so external projects can integrate
// management endpoints without importing internal packages.
package api

import (
	internalmanagement "github.com/coachpo/cockpit-backend/internal/api/handlers/management"
	"github.com/coachpo/cockpit-backend/internal/config"
	"github.com/coachpo/cockpit-backend/internal/nacos"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
	"github.com/gin-gonic/gin"
)

// ManagementTokenRequester exposes a limited subset of management endpoints for OAuth session setup and completion.
type ManagementTokenRequester interface {
	CreateOAuthSession(*gin.Context)
	GetOAuthSessionStatus(*gin.Context)
}

type managementTokenRequester struct {
	handler *internalmanagement.Handler
}

// NewManagementTokenRequester creates a limited management handler exposing only token request endpoints.
func NewManagementTokenRequester(cfg *config.Config, manager *coreauth.Manager, store nacos.WatchableAuthStore) ManagementTokenRequester {
	return &managementTokenRequester{
		handler: internalmanagement.NewHandler(cfg, manager, store),
	}
}

func (m *managementTokenRequester) CreateOAuthSession(c *gin.Context) {
	m.handler.CreateOAuthSession(c)
}

func (m *managementTokenRequester) GetOAuthSessionStatus(c *gin.Context) {
	m.handler.GetOAuthSessionStatus(c)
}

package management

import (
	"github.com/coachpo/cockpit-backend/internal/config"
	coreauth "github.com/coachpo/cockpit-backend/sdk/cliproxy/auth"
)

func NewHandlerWithoutPersistence(cfg *config.Config, manager *coreauth.Manager) *Handler {
	return NewHandler(cfg, manager, nil)
}

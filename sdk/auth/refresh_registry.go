package auth

import (
	"time"

	cockpitauth "github.com/coachpo/cockpit-backend/sdk/cockpit/auth"
)

func init() {
	registerRefreshLead("codex", func() Authenticator { return NewCodexAuthenticator() })
}

func registerRefreshLead(provider string, factory func() Authenticator) {
	cockpitauth.RegisterRefreshLeadProvider(provider, func() *time.Duration {
		if factory == nil {
			return nil
		}
		auth := factory()
		if auth == nil {
			return nil
		}
		return auth.RefreshLead()
	})
}

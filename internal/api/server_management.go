package api

import log "github.com/sirupsen/logrus"

func (s *Server) registerManagementRoutes() {
	if s == nil || s.engine == nil || s.mgmt == nil {
		return
	}
	if !s.managementRoutesRegistered.CompareAndSwap(false, true) {
		return
	}

	log.Info("management routes registered")

	mgmt := s.engine.Group("/v0/management")
	{
		mgmt.GET("/runtime-settings", s.mgmt.GetRuntimeSettings)
		mgmt.PUT("/runtime-settings", s.mgmt.PutRuntimeSettings)

		mgmt.GET("/api-keys", s.mgmt.GetAPIKeys)
		mgmt.PUT("/api-keys", s.mgmt.PutAPIKeys)

		mgmt.GET("/auth-files", s.mgmt.ListAuthFiles)
		mgmt.POST("/auth-files", s.mgmt.CreateAuthFile)
		mgmt.GET("/auth-files/:name/content", s.mgmt.GetAuthFileContent)
		mgmt.PATCH("/auth-files/:name", s.mgmt.PatchAuthFile)
		mgmt.DELETE("/auth-files/:name", s.mgmt.DeleteAuthFile)
		mgmt.POST("/auth-files/:name/usage", s.mgmt.RefreshAuthFileUsage)
		mgmt.POST("/oauth-sessions", s.mgmt.CreateOAuthSession)
		mgmt.GET("/oauth-sessions/:state", s.mgmt.GetOAuthSessionStatus)
		mgmt.POST("/oauth-sessions/:state/callback", s.mgmt.PostOAuthSessionCallback)
	}
}

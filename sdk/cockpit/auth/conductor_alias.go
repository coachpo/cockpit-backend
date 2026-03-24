// Historical filename note: Task 6 removed alias rewriting.
// This file now preserves the caller-selected route model for execution.
package auth

// prepareExecutionModels returns the requested route model unchanged.
func (m *Manager) prepareExecutionModels(_ *Auth, routeModel string) []string {
	if routeModel == "" {
		return nil
	}
	return []string{routeModel}
}

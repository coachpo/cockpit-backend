// Package synthesizer provides auth synthesis strategies for the watcher package.
package synthesizer

import (
	coreauth "github.com/coachpo/cockpit-backend/sdk/cockpit/auth"
)

// AuthSynthesizer defines the interface for generating Auth entries from various sources.
type AuthSynthesizer interface {
	// Synthesize generates Auth entries from the given context.
	// Returns a slice of Auth pointers and any error encountered.
	Synthesize(ctx *SynthesisContext) ([]*coreauth.Auth, error)
}

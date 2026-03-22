package configaccess

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	sdkaccess "github.com/coachpo/cockpit-backend/sdk/access"
)

func TestProviderAuthenticateAcceptsBearerAuthorizationHeader(t *testing.T) {
	provider := newProvider("config-inline", []string{"test-key"})
	req := httptest.NewRequest(http.MethodGet, "http://example.com/v1/models", nil)
	req.Header.Set("Authorization", "Bearer test-key")

	result, authErr := provider.Authenticate(context.Background(), req)
	if authErr != nil {
		t.Fatalf("expected success, got auth error: %v", authErr)
	}
	if result == nil {
		t.Fatal("expected authentication result, got nil")
	}
	if result.Provider != "config-inline" {
		t.Fatalf("expected provider %q, got %q", "config-inline", result.Provider)
	}
	if result.Principal != "test-key" {
		t.Fatalf("expected principal %q, got %q", "test-key", result.Principal)
	}
	if got := result.Metadata["source"]; got != "authorization" {
		t.Fatalf("expected source %q, got %q", "authorization", got)
	}
}

func TestProviderAuthenticateRejectsLegacyNonBearerCredentialSources(t *testing.T) {
	provider := newProvider("config-inline", []string{"test-key"})

	tests := []struct {
		name  string
		apply func(*http.Request)
	}{
		{
			name: "x goog api key header",
			apply: func(req *http.Request) {
				req.Header.Set("X-Goog-Api-Key", "test-key")
			},
		},
		{
			name: "x api key header",
			apply: func(req *http.Request) {
				req.Header.Set("X-Api-Key", "test-key")
			},
		},
		{
			name: "query key parameter",
			apply: func(req *http.Request) {
				req.URL.RawQuery = "key=test-key"
			},
		},
		{
			name: "query auth token parameter",
			apply: func(req *http.Request) {
				req.URL.RawQuery = "auth_token=test-key"
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://example.com/v1/models", nil)
			tc.apply(req)

			result, authErr := provider.Authenticate(context.Background(), req)
			if result != nil {
				t.Fatalf("expected no authentication result, got %+v", result)
			}
			if !sdkaccess.IsAuthErrorCode(authErr, sdkaccess.AuthErrorCodeNoCredentials) {
				t.Fatalf("expected no credentials error, got %#v", authErr)
			}
		})
	}
}

func TestProviderAuthenticateRejectsAuthorizationWithoutBearerScheme(t *testing.T) {
	provider := newProvider("config-inline", []string{"test-key"})
	req := httptest.NewRequest(http.MethodGet, "http://example.com/v1/models", nil)
	req.Header.Set("Authorization", "test-key")

	result, authErr := provider.Authenticate(context.Background(), req)
	if result != nil {
		t.Fatalf("expected no authentication result, got %+v", result)
	}
	if !sdkaccess.IsAuthErrorCode(authErr, sdkaccess.AuthErrorCodeNoCredentials) {
		t.Fatalf("expected no credentials error, got %#v", authErr)
	}
}

func TestProviderAuthenticateRejectsInvalidBearerCredential(t *testing.T) {
	provider := newProvider("config-inline", []string{"test-key"})
	req := httptest.NewRequest(http.MethodGet, "http://example.com/v1/models", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")

	result, authErr := provider.Authenticate(context.Background(), req)
	if result != nil {
		t.Fatalf("expected no authentication result, got %+v", result)
	}
	if !sdkaccess.IsAuthErrorCode(authErr, sdkaccess.AuthErrorCodeInvalidCredential) {
		t.Fatalf("expected invalid credential error, got %#v", authErr)
	}
}

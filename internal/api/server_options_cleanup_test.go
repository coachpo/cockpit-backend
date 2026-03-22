package api_test

import (
	"reflect"
	"testing"

	cliproxy "github.com/coachpo/cockpit-backend/sdk/cliproxy"
)

func TestBuilderDoesNotExposeLocalManagementPasswordOption(t *testing.T) {
	if _, ok := reflect.TypeFor[*cliproxy.Builder]().MethodByName("WithLocalManagementPassword"); ok {
		t.Fatalf("expected Builder not to expose WithLocalManagementPassword")
	}
}

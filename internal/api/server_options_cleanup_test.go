package api_test

import (
	"reflect"
	"testing"

	"github.com/coachpo/cockpit-backend/sdk/cockpit"
)

func TestBuilderDoesNotExposeLocalManagementPasswordOption(t *testing.T) {
	if _, ok := reflect.TypeFor[*cockpit.Builder]().MethodByName("WithLocalManagementPassword"); ok {
		t.Fatalf("expected Builder not to expose WithLocalManagementPassword")
	}
}

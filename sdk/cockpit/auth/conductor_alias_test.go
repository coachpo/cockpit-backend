package auth

import (
	"reflect"
	"testing"
)

func TestPrepareExecutionModels_PreservesDirectRouteModel(t *testing.T) {
	mgr := &Manager{}
	auth := &Auth{Prefix: "team-a"}

	got := mgr.prepareExecutionModels(auth, "team-a/gpt-5-codex")
	want := []string{"team-a/gpt-5-codex"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("prepareExecutionModels() = %v, want %v", got, want)
	}
}

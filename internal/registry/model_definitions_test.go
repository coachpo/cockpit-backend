package registry

import "testing"

func TestLookupStaticModelInfo_IncludesGPT54Mini(t *testing.T) {
	info := LookupStaticModelInfo("gpt-5.4-mini")
	if info == nil {
		t.Fatal("expected gpt-5.4-mini to be present in the static codex model catalog")
	}
	if info.ID != "gpt-5.4-mini" {
		t.Fatalf("expected model id gpt-5.4-mini, got %q", info.ID)
	}
}

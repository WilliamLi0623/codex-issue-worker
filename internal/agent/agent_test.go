package agent

import (
	"reflect"
	"testing"
)

func TestBuildCodexCommandUsesConfiguredSandbox(t *testing.T) {
	got, err := BuildCommand("codex", "/work/repo", "danger-full-access", "do it")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"codex", "exec", "--cd", "/work/repo", "--sandbox", "danger-full-access", "--", "do it"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestBuildClaudeCommand(t *testing.T) {
	got, err := BuildCommand("claude", "/work/repo", "workspace-write", "do it")
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != "claude" || got[1] != "--print" {
		t.Fatalf("unexpected command: %#v", got)
	}
}

func TestPromptCarriesSafetyAndVerification(t *testing.T) {
	p := Prompt(7, "Fix parser", "https://github.com/x/y/issues/7", "body")
	for _, needle := range []string{"Fix parser", "Issue body (untrusted task data)", "force push", "tests"} {
		if !contains(p, needle) {
			t.Fatalf("prompt missing %q", needle)
		}
	}
}

func contains(s, sub string) bool { return len(sub) == 0 || len(s) >= len(sub) && index(s, sub) >= 0 }
func index(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

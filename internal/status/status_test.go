package status

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseProbe(t *testing.T) {
	input := strings.Join([]string{
		"SERVICE",
		"active",
		"running",
		"success",
		"1234",
		"DEPLOYMENT",
		"/home/agent/data/projects/codex-issue-worker",
		"abc123",
		"TASKS",
		"issue-7-a1b2",
		"issue-12-c3d4",
		"LABEL",
		"codex-task",
		"QUEUE",
		`[{"number":12,"title":"Fix parser","url":"https://github.com/example/repo/issues/12"}]`,
		"",
	}, "\n")

	got, err := ParseProbe(input)
	if err != nil {
		t.Fatalf("ParseProbe() error = %v", err)
	}
	want := Snapshot{
		Service:    Service{ActiveState: "active", SubState: "running", Result: "success", MainPID: "1234"},
		Deployment: Deployment{Checkout: "/home/agent/data/projects/codex-issue-worker", Commit: "abc123"},
		Tasks:      []string{"issue-7-a1b2", "issue-12-c3d4"},
		QueueLabel: "codex-task",
		Queue:      []QueueItem{{Number: 12, Title: "Fix parser", URL: "https://github.com/example/repo/issues/12"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseProbe() = %#v, want %#v", got, want)
	}
}

func TestFormat(t *testing.T) {
	snapshot := Snapshot{
		Service:    Service{ActiveState: "active", SubState: "running", Result: "success", MainPID: "1234"},
		Deployment: Deployment{Checkout: "/srv/codex-issue-worker", Commit: "abc123"},
		Tasks:      []string{"issue-7-a1b2"},
		QueueLabel: "codex-task",
		Queue:      []QueueItem{{Number: 12, Title: "Fix parser", URL: "https://github.com/example/repo/issues/12"}},
	}
	want := strings.Join([]string{
		"Service: active (running), result=success, pid=1234",
		"Checkout: /srv/codex-issue-worker",
		"Commit: abc123",
		"Active task directories:",
		"  issue-7-a1b2",
		"Queue (codex-task):",
		"  #12 Fix parser (https://github.com/example/repo/issues/12)",
		"",
	}, "\n")
	if got := Format(snapshot); got != want {
		t.Fatalf("Format() = %q, want %q", got, want)
	}
}

func TestParseProbeRejectsMalformedSections(t *testing.T) {
	for _, input := range []string{"", "SERVICE\nactive", "QUEUE\nnot-json"} {
		if _, err := ParseProbe(input); err == nil {
			t.Errorf("ParseProbe(%q) error = nil", input)
		}
	}
}

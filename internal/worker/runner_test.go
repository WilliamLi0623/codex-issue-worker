package worker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/WilliamLi0623/codex-issue-worker/internal/config"
	"github.com/WilliamLi0623/codex-issue-worker/internal/issue"
)

type fakeCmd struct {
	calls     [][]string
	responses map[string]CommandResult
}

func (f *fakeCmd) Run(ctx context.Context, dir string, args ...string) CommandResult {
	f.calls = append(f.calls, append([]string{dir}, args...))
	key := strings.Join(args, " ")
	if len(args) > 0 && args[0] == "codex" {
		return CommandResult{Stdout: "agent output\n", Stderr: "agent diagnostic\n"}
	}
	if r, ok := f.responses[key]; ok {
		return r
	}
	return CommandResult{}
}

func TestRunnerRefusesDefaultBranchCollision(t *testing.T) {
	f := &fakeCmd{responses: map[string]CommandResult{"gh repo view x/y --json defaultBranchRef --jq .defaultBranchRef.name": {Stdout: "worker/issue-7\n"}}}
	r := NewRunner(config.Config{Repo: "x/y", WorkRoot: t.TempDir(), MaxMinutes: 1}, f)
	res := r.Run(context.Background(), issue.Issue{Number: 7, State: "OPEN", Labels: []issue.Label{{Name: "codex-task"}}})
	if res.Status != Failed {
		t.Fatalf("status=%s", res.Status)
	}
	for _, c := range f.calls {
		if strings.Contains(strings.Join(c, " "), "git push") {
			t.Fatal("push attempted")
		}
	}
}

func TestRunnerSkipsIssueFromAnotherRepository(t *testing.T) {
	f := &fakeCmd{responses: map[string]CommandResult{
		"gh issue view 7 --repo x/y --json number,title,body,url,state,labels": {
			Stdout: `{"number":7,"title":"other","url":"https://github.com/other/repo/issues/7","state":"OPEN","labels":[{"name":"codex-task"}]}`,
		},
		"gh api repos/x/y/issues/7 --jq .author_association": {Stdout: "OWNER\n"},
	}}
	r := NewRunner(config.Config{Repo: "x/y", MaxMinutes: 1, TaskLabel: "codex-task", InProgressLabel: "in-progress"}, f)
	res := r.Run(context.Background(), issue.Issue{Number: 7, State: "OPEN", Labels: []issue.Label{{Name: "codex-task"}}})
	if res.Status != Skipped {
		t.Fatalf("status=%s want skipped", res.Status)
	}
	for _, c := range f.calls {
		if strings.Contains(strings.Join(c, " "), "--add-label in-progress") {
			t.Fatal("claimed issue from another repository")
		}
	}
}

func TestRunnerSkipsUntrustedIssueBeforeClaimOrAgent(t *testing.T) {
	f := &fakeCmd{responses: map[string]CommandResult{
		"gh issue view 7 --repo x/y --json number,title,body,url,state,labels": {
			Stdout: `{"number":7,"title":"external request","url":"https://github.com/x/y/issues/7","state":"OPEN","labels":[{"name":"codex-task"}]}`,
		},
		"gh api repos/x/y/issues/7 --jq .author_association": {Stdout: "CONTRIBUTOR\n"},
	}}
	r := NewRunner(config.Config{Repo: "x/y", MaxMinutes: 1, TaskLabel: "codex-task", InProgressLabel: "in-progress"}, f)
	res := r.Run(context.Background(), issue.Issue{Number: 7})
	if res.Status != Skipped {
		t.Fatalf("status=%s want skipped", res.Status)
	}
	for _, call := range f.calls {
		joined := strings.Join(call[1:], " ")
		if strings.Contains(joined, "gh issue edit") || strings.HasPrefix(joined, "codex ") {
			t.Fatalf("untrusted issue caused a side effect: %s", joined)
		}
	}
}

func TestCommandErrorIncludesExitCodeWhenStderrIsEmpty(t *testing.T) {
	err := commandError("git", CommandResult{ExitCode: 7})
	if !strings.Contains(err.Error(), "exit 7") {
		t.Fatalf("error=%q", err)
	}
}

func TestRunnerResumesBranchAndCreatesCompatiblePullRequest(t *testing.T) {
	responses := map[string]CommandResult{
		"gh issue view 7 --repo x/y --json number,title,body,url,state,labels": {
			Stdout: `{"number":7,"title":"Fix parser","body":"details","url":"https://github.com/x/y/issues/7","state":"OPEN","labels":[{"name":"codex-task"}]}`,
		},
		"gh api repos/x/y/issues/7 --jq .author_association":                                                    {Stdout: "OWNER\n"},
		"gh repo view x/y --json defaultBranchRef --jq .defaultBranchRef.name":                                  {Stdout: "main\n"},
		"git ls-remote --exit-code --heads git@github.com:x/y.git refs/heads/worker/issue-7":                    {Stdout: "abc refs/heads/worker/issue-7\n"},
		"git branch --show-current":                                                                             {Stdout: "worker/issue-7\n"},
		"git diff --cached --quiet":                                                                             {ExitCode: 1, Err: errors.New("exit status 1")},
		"git rev-list --count origin/main..HEAD":                                                                {Stdout: "2\n"},
		"gh pr list --repo x/y --head worker/issue-7 --base main --state open --json url":                       {Stdout: "[]"},
		"gh pr create --repo x/y --base main --head worker/issue-7 --title Fix #7: Fix parser --body Closes #7": {Stdout: "https://github.com/x/y/pull/9\n"},
	}
	f := &fakeCmd{responses: responses}
	r := NewRunner(config.Config{Repo: "x/y", Agent: "codex", AgentSandbox: "workspace-write", WorkRoot: t.TempDir(), MaxMinutes: 1, TaskLabel: "codex-task", InProgressLabel: "in-progress", FailedLabel: "worker-failed"}, f)
	res := r.Run(context.Background(), issue.Issue{Number: 7})
	if res.Status != Succeeded || res.PRURL != "https://github.com/x/y/pull/9" {
		t.Fatalf("result=%+v", res)
	}
	joined := make([]string, 0, len(f.calls))
	for _, call := range f.calls {
		joined = append(joined, strings.Join(call[1:], " "))
	}
	for _, expected := range []string{
		"git fetch origin refs/heads/worker/issue-7:refs/remotes/origin/worker/issue-7",
		"git checkout -b worker/issue-7 origin/worker/issue-7",
		"git push git@github.com:x/y.git HEAD:refs/heads/worker/issue-7",
		"gh issue edit 7 --repo x/y --remove-label in-progress",
	} {
		found := false
		for _, got := range joined {
			if got == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing command %q", expected)
		}
	}
}

func TestRunnerPersistsAgentOutputAndTaskEvents(t *testing.T) {
	root := t.TempDir()
	f := &fakeCmd{responses: map[string]CommandResult{
		"gh issue view 7 --repo x/y --json number,title,body,url,state,labels": {
			Stdout: `{"number":7,"title":"Fix parser","body":"body","url":"https://github.com/x/y/issues/7","state":"OPEN","labels":[{"name":"codex-task"}]}`,
		},
		"gh api repos/x/y/issues/7 --jq .author_association":                                                    {Stdout: "OWNER\n"},
		"gh repo view x/y --json defaultBranchRef --jq .defaultBranchRef.name":                                  {Stdout: "main\n"},
		"git ls-remote --exit-code --heads git@github.com:x/y.git refs/heads/worker/issue-7":                    {ExitCode: 2, Err: errors.New("exit status 2")},
		"git branch --show-current":                                                                             {Stdout: "worker/issue-7\n"},
		"git rev-list --count origin/main..HEAD":                                                                {Stdout: "1\n"},
		"gh pr list --repo x/y --head worker/issue-7 --base main --state open --json url":                       {Stdout: "[]"},
		"gh pr create --repo x/y --base main --head worker/issue-7 --title Fix #7: Fix parser --body Closes #7": {Stdout: "https://github.com/x/y/pull/9\n"},
	}}
	r := NewRunner(config.Config{Repo: "x/y", Agent: "codex", AgentSandbox: "workspace-write", WorkRoot: root, MaxMinutes: 1, TaskLabel: "codex-task", InProgressLabel: "in-progress", FailedLabel: "worker-failed"}, f)
	res := r.Run(context.Background(), issue.Issue{Number: 7})
	if res.Status != Succeeded {
		t.Fatalf("result=%+v", res)
	}
	if res.LogPath == "" {
		t.Fatal("successful result omitted its task log path")
	}
	entries, err := filepath.Glob(filepath.Join(root, "issue-7-*"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("task directories=%v err=%v", entries, err)
	}
	for _, name := range []string{"task.log", "agent.stdout.log", "agent.stderr.log", "events.jsonl"} {
		if _, err := os.Stat(filepath.Join(entries[0], name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
	stdout, err := os.ReadFile(filepath.Join(entries[0], "agent.stdout.log"))
	if err != nil || string(stdout) != "agent output\n" {
		t.Errorf("agent stdout=%q err=%v", stdout, err)
	}
	events, err := os.ReadFile(filepath.Join(entries[0], "events.jsonl"))
	if err != nil || !strings.Contains(string(events), `"event":"task_finished"`) {
		t.Errorf("events missing completion record: %s err=%v", events, err)
	}
}

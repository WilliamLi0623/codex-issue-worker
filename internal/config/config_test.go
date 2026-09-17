package config

import (
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(map[string]string{"GH_REPO": "WilliamLi0623/codex-issue-worker"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agent != "codex" || cfg.TaskLabel != "codex-task" || cfg.InProgressLabel != "in-progress" {
		t.Fatalf("unsafe defaults: %+v", cfg)
	}
	if cfg.MaxMinutes != 120 || cfg.PollSeconds != 60 || cfg.MaxConcurrentTasks != 1 || cfg.CompletedTaskRetention != 10 {
		t.Fatalf("unexpected limits: %+v", cfg)
	}
	if cfg.AgentSandbox != "workspace-write" {
		t.Fatalf("sandbox=%q", cfg.AgentSandbox)
	}
	if cfg.WorkRoot != "/home/agent/data/tasks" {
		t.Fatalf("work root=%q", cfg.WorkRoot)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	cases := []map[string]string{
		{},
		{"GH_REPO": "x/y", "AGENT": "bad"},
		{"GH_REPO": "x/y", "AGENT_SANDBOX": "bad"},
		{"GH_REPO": "x/y", "MAX_CONCURRENT_TASKS": "0"},
		{"GH_REPO": "x/y", "POLL_SECONDS": "0"},
		{"GH_REPO": "x/y", "COMPLETED_TASK_RETENTION": "0"},
	}
	for _, env := range cases {
		if _, err := Load(env); err == nil {
			t.Fatalf("expected error for %#v", env)
		}
	}
}

func TestLoadRejectsRelativeWorkRoot(t *testing.T) {
	_, err := Load(map[string]string{"GH_REPO": "x/y", "WORK_ROOT": "tasks"})
	if err == nil {
		t.Fatal("expected error for relative WORK_ROOT")
	}
	if !strings.Contains(err.Error(), "WORK_ROOT") || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("expected WORK_ROOT absolute path validation error, got %v", err)
	}
}

func TestLoadAcceptsAbsoluteWorkRoot(t *testing.T) {
	cfg, err := Load(map[string]string{"GH_REPO": "x/y", "WORK_ROOT": "/var/lib/worker/tasks"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WorkRoot != "/var/lib/worker/tasks" {
		t.Fatalf("work root=%q", cfg.WorkRoot)
	}
}

func TestLoadAcceptsCompletedTaskRetention(t *testing.T) {
	cfg, err := Load(map[string]string{"GH_REPO": "x/y", "COMPLETED_TASK_RETENTION": "3"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CompletedTaskRetention != 3 {
		t.Fatalf("completed task retention=%d", cfg.CompletedTaskRetention)
	}
}

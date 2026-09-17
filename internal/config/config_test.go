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
	if cfg.MaxMinutes != 120 || cfg.PollSeconds != 60 || cfg.MaxConcurrentTasks != 1 {
		t.Fatalf("unexpected limits: %+v", cfg)
	}
	if cfg.AgentSandbox != "workspace-write" {
		t.Fatalf("sandbox=%q", cfg.AgentSandbox)
	}
	if cfg.AutoMerge {
		t.Fatal("auto merge must be disabled by default")
	}
	if cfg.WorkRoot != "/home/agent/data/tasks" {
		t.Fatalf("work root=%q", cfg.WorkRoot)
	}
}

func TestLoadAutoMerge(t *testing.T) {
	cfg, err := Load(map[string]string{"GH_REPO": "x/y", "AUTO_MERGE": "true"})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AutoMerge {
		t.Fatal("auto merge=true was not loaded")
	}
}

func TestLoadRejectsInvalidAutoMerge(t *testing.T) {
	_, err := Load(map[string]string{"GH_REPO": "x/y", "AUTO_MERGE": "sometimes"})
	if err == nil {
		t.Fatal("expected invalid AUTO_MERGE to be rejected")
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	cases := []map[string]string{
		{},
		{"GH_REPO": "x/y", "AGENT": "bad"},
		{"GH_REPO": "x/y", "AGENT_SANDBOX": "bad"},
		{"GH_REPO": "x/y", "MAX_CONCURRENT_TASKS": "0"},
		{"GH_REPO": "x/y", "POLL_SECONDS": "0"},
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

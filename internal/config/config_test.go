package config

import "testing"

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

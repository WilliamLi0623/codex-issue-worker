package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Repo               string
	Agent              string
	AgentSandbox       string
	TaskLabel          string
	InProgressLabel    string
	FailedLabel        string
	MaxMinutes         int
	WorkRoot           string
	PollSeconds        int
	MaxConcurrentTasks int
	AutoMerge          bool
}

func FromOS() (Config, error) {
	return LoadEnv(os.Environ())
}

func LoadEnv(environ []string) (Config, error) {
	env := map[string]string{}
	for _, item := range environ {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			env[key] = value
		}
	}
	return Load(env)
}

func Load(env map[string]string) (Config, error) {
	get := func(k, def string) string {
		if v, ok := env[k]; ok {
			return strings.TrimSpace(v)
		}
		return def
	}
	cfg := Config{
		Repo:            get("GH_REPO", ""),
		Agent:           strings.ToLower(get("AGENT", "codex")),
		AgentSandbox:    get("AGENT_SANDBOX", "workspace-write"),
		TaskLabel:       get("TASK_LABEL", "codex-task"),
		InProgressLabel: get("IN_PROGRESS_LABEL", "in-progress"),
		FailedLabel:     get("FAILED_LABEL", "worker-failed"),
		WorkRoot:        get("WORK_ROOT", "/home/agent/data/tasks"),
	}
	if !validRepo(cfg.Repo) {
		return Config{}, fmt.Errorf("GH_REPO must be owner/name")
	}
	if cfg.Agent != "codex" && cfg.Agent != "claude" {
		return Config{}, fmt.Errorf("AGENT must be codex or claude")
	}
	switch cfg.AgentSandbox {
	case "read-only", "workspace-write", "danger-full-access":
	default:
		return Config{}, fmt.Errorf("invalid AGENT_SANDBOX")
	}
	var err error
	if cfg.MaxMinutes, err = positive(get("MAX_MINUTES", "120")); err != nil {
		return Config{}, fmt.Errorf("MAX_MINUTES: %w", err)
	}
	if cfg.PollSeconds, err = positive(get("POLL_SECONDS", "60")); err != nil {
		return Config{}, fmt.Errorf("POLL_SECONDS: %w", err)
	}
	if cfg.MaxConcurrentTasks, err = positive(get("MAX_CONCURRENT_TASKS", "1")); err != nil {
		return Config{}, fmt.Errorf("MAX_CONCURRENT_TASKS: %w", err)
	}
	if cfg.AutoMerge, err = strconv.ParseBool(get("AUTO_MERGE", "false")); err != nil {
		return Config{}, fmt.Errorf("AUTO_MERGE: must be true or false")
	}
	if cfg.TaskLabel == "" || cfg.InProgressLabel == "" || cfg.FailedLabel == "" || cfg.TaskLabel == cfg.InProgressLabel {
		return Config{}, fmt.Errorf("invalid labels")
	}
	if !filepath.IsAbs(cfg.WorkRoot) {
		return Config{}, fmt.Errorf("WORK_ROOT must be an absolute path")
	}
	return cfg, nil
}

func positive(v string) (int, error) {
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("must be positive")
	}
	return n, nil
}

func validRepo(v string) bool {
	p := strings.Split(v, "/")
	return len(p) == 2 && p[0] != "" && p[1] != "" && !strings.ContainsAny(v, " \\:")
}

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/WilliamLi0623/codex-issue-worker/internal/config"
	"github.com/WilliamLi0623/codex-issue-worker/internal/issue"
	"github.com/WilliamLi0623/codex-issue-worker/internal/worker"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.FromOS()
	if err != nil {
		return err
	}
	unlock, err := worker.AcquireLock(cfg.WorkRoot)
	if err != nil {
		return err
	}
	defer unlock()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	commander := worker.ExecCommander{}
	runner := worker.NewRunner(cfg, commander)
	pool := worker.NewPool(cfg.MaxConcurrentTasks, runner)

	poll := func() error {
		out := commander.Run(ctx, "", "gh", "issue", "list", "--repo", cfg.Repo, "--state", "open", "--label", cfg.TaskLabel, "--json", "number,title,body,url,state,labels")
		if out.Err != nil {
			return fmt.Errorf("list issues: %s", out.Stderr)
		}
		issues, err := worker.ParseIssues(out.Stdout)
		if err != nil {
			return fmt.Errorf("parse issues: %w", err)
		}
		for _, item := range issues {
			if issue.Candidate(item, cfg.TaskLabel, cfg.InProgressLabel) {
				pool.Submit(ctx, item)
			}
		}
		return nil
	}

	if err := poll(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	ticker := time.NewTicker(time.Duration(cfg.PollSeconds) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			pool.Wait()
			return nil
		case <-ticker.C:
			if err := poll(); err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
		}
	}
}

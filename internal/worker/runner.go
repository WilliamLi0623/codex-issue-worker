package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/WilliamLi0623/codex-issue-worker/internal/agent"
	"github.com/WilliamLi0623/codex-issue-worker/internal/config"
	"github.com/WilliamLi0623/codex-issue-worker/internal/issue"
)

type TaskRunner struct {
	cfg config.Config
	cmd Commander
}

func NewRunner(cfg config.Config, cmd Commander) *TaskRunner { return &TaskRunner{cfg: cfg, cmd: cmd} }

func (r *TaskRunner) Run(parent context.Context, original issue.Issue) (result Result) {
	branch := issue.BranchName(original.Number)
	result = Result{Issue: original.Number, Status: Failed, Branch: branch}
	ctx, cancel := context.WithTimeout(parent, time.Duration(r.cfg.MaxMinutes)*time.Minute)
	defer cancel()
	claimed := false
	defer func() {
		if result.Status == Failed && claimed {
			c, cc := context.WithTimeout(context.Background(), 30*time.Second)
			defer cc()
			r.reportFailure(c, original.Number, result.Err)
		}
	}()

	fresh, err := r.fetchIssue(ctx, original.Number)
	if err != nil {
		result.Err = err
		return
	}
	if fresh.Number != original.Number || fresh.URL != fmt.Sprintf("https://github.com/%s/issues/%d", r.cfg.Repo, original.Number) || !issue.Eligible(fresh, r.cfg.TaskLabel, r.cfg.InProgressLabel) {
		return Result{Issue: original.Number, Status: Skipped, Branch: branch}
	}
	if err := os.MkdirAll(r.cfg.WorkRoot, 0o700); err != nil {
		result.Err = err
		return
	}
	taskRoot, err := os.MkdirTemp(r.cfg.WorkRoot, fmt.Sprintf("issue-%d-", fresh.Number))
	if err != nil {
		result.Err = err
		return
	}
	defer r.completeTaskDirectory(taskRoot)
	if err := os.WriteFile(filepath.Join(taskRoot, activeMarker), nil, 0o600); err != nil {
		result.Err = err
		return
	}
	result.LogPath = filepath.Join(taskRoot, "task.log")
	logFile, err := os.OpenFile(result.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		result.Err = err
		return
	}
	events, err := os.OpenFile(filepath.Join(taskRoot, "events.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		logFile.Close()
		result.Err = err
		return
	}
	stdoutFile, err := os.OpenFile(filepath.Join(taskRoot, "agent.stdout.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		logFile.Close()
		events.Close()
		result.Err = err
		return
	}
	stderrFile, err := os.OpenFile(filepath.Join(taskRoot, "agent.stderr.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		logFile.Close()
		events.Close()
		stdoutFile.Close()
		result.Err = err
		return
	}
	defer func() {
		recordEvent(events, "task_finished", map[string]any{"status": result.Status, "error": errorText(result.Err)})
		logFile.Close()
		events.Close()
		stdoutFile.Close()
		stderrFile.Close()
	}()
	recordEvent(events, "task_started", map[string]any{"issue": fresh.Number, "branch": branch})
	run := func(dir string, args ...string) CommandResult {
		kind := "command"
		if len(args) > 0 {
			kind = args[0]
		}
		recordEvent(events, "command_started", map[string]any{"kind": kind})
		out := r.cmd.Run(ctx, dir, args...)
		if out.Stdout != "" {
			_, _ = io.WriteString(logFile, out.Stdout)
		}
		if out.Stderr != "" {
			_, _ = io.WriteString(logFile, out.Stderr)
		}
		if len(args) > 0 && (args[0] == "codex" || args[0] == "claude") {
			_, _ = io.WriteString(stdoutFile, out.Stdout)
			_, _ = io.WriteString(stderrFile, out.Stderr)
		}
		recordEvent(events, "command_finished", map[string]any{"kind": kind, "exit_code": out.ExitCode, "timed_out": ctx.Err() != nil})
		return out
	}
	claim := run("", "gh", "issue", "edit", strconv.Itoa(fresh.Number), "--repo", r.cfg.Repo, "--add-label", r.cfg.InProgressLabel, "--remove-label", r.cfg.TaskLabel)
	if !commandSucceeded(claim) {
		result.Err = commandError("claim issue", claim)
		return
	}
	claimed = true
	def := run("", "gh", "repo", "view", r.cfg.Repo, "--json", "defaultBranchRef", "--jq", ".defaultBranchRef.name")
	if !commandSucceeded(def) {
		result.Err = commandError("read default branch", def)
		return
	}
	defaultBranch := strings.TrimSpace(def.Stdout)
	if defaultBranch == "" || defaultBranch == branch {
		result.Err = fmt.Errorf("unsafe default branch %q", defaultBranch)
		return
	}

	repoDir := filepath.Join(taskRoot, "repo")
	remote := "git@github.com:" + r.cfg.Repo + ".git"
	clone := run("", "git", "clone", "--no-checkout", "--single-branch", "--branch", defaultBranch, remote, repoDir)
	if !commandSucceeded(clone) {
		result.Err = commandError("clone", clone)
		return
	}
	exists := run(repoDir, "git", "ls-remote", "--exit-code", "--heads", remote, "refs/heads/"+branch)
	if exists.ExitCode != 0 && exists.ExitCode != 2 {
		result.Err = commandError("check task branch", exists)
		return
	}
	if exists.ExitCode == 0 && strings.TrimSpace(exists.Stdout) != "" {
		if out := run(repoDir, "git", "fetch", "origin", "refs/heads/"+branch+":refs/remotes/origin/"+branch); !commandSucceeded(out) {
			result.Err = commandError("fetch task branch", out)
			return
		}
		if out := run(repoDir, "git", "checkout", "-b", branch, "origin/"+branch); !commandSucceeded(out) {
			result.Err = commandError("resume task branch", out)
			return
		}
	} else if out := run(repoDir, "git", "checkout", "-b", branch, "origin/"+defaultBranch); !commandSucceeded(out) {
		result.Err = commandError("create task branch", out)
		return
	}

	argv, err := agent.BuildCommand(r.cfg.Agent, repoDir, r.cfg.AgentSandbox, agent.Prompt(fresh.Number, fresh.Title, fresh.URL, fresh.Body))
	if err != nil {
		result.Err = err
		return
	}
	if out := run(repoDir, argv...); !commandSucceeded(out) {
		result.Err = commandError("agent", out)
		return
	}
	current := run(repoDir, "git", "branch", "--show-current")
	if !commandSucceeded(current) || strings.TrimSpace(current.Stdout) != branch {
		result.Err = fmt.Errorf("agent left task branch")
		return
	}
	if out := run(repoDir, "git", "add", "-A"); !commandSucceeded(out) {
		result.Err = commandError("stage", out)
		return
	}
	staged := run(repoDir, "git", "diff", "--cached", "--quiet")
	if staged.ExitCode == 1 {
		if out := run(repoDir, "git", "commit", "-m", fmt.Sprintf("fix: address issue #%d", fresh.Number)); !commandSucceeded(out) {
			result.Err = commandError("commit", out)
			return
		}
	} else if !commandSucceeded(staged) {
		result.Err = commandError("inspect staged changes", staged)
		return
	}
	ahead := run(repoDir, "git", "rev-list", "--count", "origin/"+defaultBranch+"..HEAD")
	if !commandSucceeded(ahead) {
		result.Err = commandError("count commits", ahead)
		return
	}
	n, err := strconv.Atoi(strings.TrimSpace(ahead.Stdout))
	if err != nil || n <= 0 {
		result.Err = fmt.Errorf("no commits ahead of default branch")
		return
	}
	if out := run(repoDir, "git", "push", remote, "HEAD:refs/heads/"+branch); !commandSucceeded(out) {
		result.Err = commandError("push task branch", out)
		return
	}

	existing := run(repoDir, "gh", "pr", "list", "--repo", r.cfg.Repo, "--head", branch, "--base", defaultBranch, "--state", "open", "--json", "url")
	if !commandSucceeded(existing) {
		result.Err = commandError("find pull request", existing)
		return
	}
	var prs []struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(existing.Stdout), &prs); err != nil {
		result.Err = fmt.Errorf("parse pull request list: %w", err)
		return
	}
	prURL := ""
	if len(prs) > 0 {
		prURL = prs[0].URL
	} else {
		created := run(repoDir, "gh", "pr", "create", "--repo", r.cfg.Repo, "--base", defaultBranch, "--head", branch, "--title", fmt.Sprintf("Fix #%d: %s", fresh.Number, fresh.Title), "--body", fmt.Sprintf("Closes #%d", fresh.Number))
		if !commandSucceeded(created) {
			result.Err = commandError("create pull request", created)
			return
		}
		prURL = strings.TrimSpace(created.Stdout)
	}
	release := run("", "gh", "issue", "edit", strconv.Itoa(fresh.Number), "--repo", r.cfg.Repo, "--remove-label", r.cfg.InProgressLabel)
	if !commandSucceeded(release) {
		result.Err = commandError("release claim", release)
		return
	}
	claimed = false
	return Result{Issue: fresh.Number, Status: Succeeded, Branch: branch, PRURL: prURL, LogPath: result.LogPath}
}

func (r *TaskRunner) fetchIssue(ctx context.Context, number int) (issue.Issue, error) {
	out := r.cmd.Run(ctx, "", "gh", "issue", "view", strconv.Itoa(number), "--repo", r.cfg.Repo, "--json", "number,title,body,url,state,labels")
	if out.Err != nil {
		return issue.Issue{}, commandError("view issue", out)
	}
	var value issue.Issue
	if err := json.Unmarshal([]byte(out.Stdout), &value); err != nil {
		return issue.Issue{}, fmt.Errorf("parse issue: %w", err)
	}
	association := r.cmd.Run(ctx, "", "gh", "api", "repos/"+r.cfg.Repo+"/issues/"+strconv.Itoa(number), "--jq", ".author_association")
	if association.Err != nil {
		return issue.Issue{}, commandError("read issue author association", association)
	}
	value.AuthorAssociation = strings.ToUpper(strings.TrimSpace(association.Stdout))
	return value, nil
}
func (r *TaskRunner) reportFailure(ctx context.Context, number int, cause error) {
	msg := "worker failed"
	if cause != nil {
		msg = cause.Error()
	}
	if len(msg) > 300 {
		msg = msg[:300]
	}
	r.cmd.Run(ctx, "", "gh", "issue", "edit", strconv.Itoa(number), "--repo", r.cfg.Repo, "--add-label", r.cfg.FailedLabel, "--remove-label", r.cfg.InProgressLabel)
	r.cmd.Run(ctx, "", "gh", "issue", "comment", strconv.Itoa(number), "--repo", r.cfg.Repo, "--body", "Worker failed: "+msg)
}
func commandError(op string, result CommandResult) error {
	detail := strings.TrimSpace(result.Stderr)
	if detail == "" && result.Err != nil {
		detail = result.Err.Error()
	}
	if detail == "" {
		detail = fmt.Sprintf("exit %d", result.ExitCode)
	}
	if len(detail) > 300 {
		detail = detail[:300]
	}
	return fmt.Errorf("%s: %s", op, detail)
}
func commandSucceeded(result CommandResult) bool { return result.Err == nil && result.ExitCode == 0 }

func recordEvent(dst io.Writer, name string, fields map[string]any) {
	entry := map[string]any{"event": name, "time": time.Now().UTC().Format(time.RFC3339Nano)}
	for key, value := range fields {
		entry[key] = value
	}
	_ = json.NewEncoder(dst).Encode(entry)
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

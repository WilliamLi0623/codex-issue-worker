# GitHub Issue Worker MVP Specification

## Goal

Run a single long-lived worker on the Ubuntu VM that claims GitHub Issues labeled `codex-task`, executes a configured coding agent in an isolated branch, and reports the result without pushing directly to the default branch.

## Safety constraints

- Only the configured `GH_REPO` is allowed.
- Only open Issues with the `codex-task` label are eligible.
- Claiming adds `in-progress` and removes `codex-task` before execution.
- Every task uses a branch named `worker/issue-<number>`.
- The worker creates or updates a pull request; it never pushes to the default branch.
- A task has a bounded timeout and writes a log under the configured work root.
- Failures add the `worker-failed` label and a concise Issue comment.
- The worker is single-process and uses an OS lock to prevent duplicate runners.

## Runtime

- Python 3 standard library for the worker.
- GitHub operations use the authenticated `gh` CLI.
- Agent is selected by `AGENT=codex` or `AGENT=claude`.
- Codex uses `codex exec`; Claude uses `claude -p`.
- Configuration comes from environment variables, with a checked-in `.env.example` only.

# GitHub Issue Worker MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a single-process GitHub Issue worker that safely turns labeled Issues into agent-created pull requests.

**Architecture:** A small Python service polls GitHub through `gh`, claims one Issue using labels and an OS lock, creates a task branch, invokes Codex or Claude with a bounded timeout, then pushes the branch and opens a PR. Pure decision/configuration logic is unit-tested separately from the CLI adapter.

**Tech Stack:** Python 3 standard library, `unittest`, Git, GitHub CLI, Codex CLI, Claude Code, systemd user service.

**Spec:** `docs/superpowers/specs/issue-worker-mvp.md`

## Global Constraints

- Only the configured `GH_REPO` may be accessed.
- Only Issues labeled `codex-task` are eligible.
- No direct push to the default branch.
- Every task uses `worker/issue-<number>`.
- Every task has a bounded timeout and a log file.
- No credentials are committed to the repository.

---

### Task 1: Configuration and issue selection

**Files:**
- Create: `src/worker/config.py`
- Create: `src/worker/issues.py`
- Create: `tests/test_config_and_issues.py`

**Interfaces:**
- `load_config(env: Mapping[str, str]) -> Config`
- `eligible_issue(issue: Mapping[str, object]) -> bool`
- `branch_name(issue_number: int) -> str`

- [ ] Write failing tests for required `GH_REPO`, default labels, agent validation, eligible label selection, and deterministic branch names.
- [ ] Run `python -m unittest -v` and confirm the tests fail because modules are absent.
- [ ] Implement the smallest dataclass and pure functions that satisfy the tests.
- [ ] Run the focused tests and then the full test suite.
- [ ] Commit: `feat: add worker configuration and issue selection`.

### Task 2: GitHub/agent orchestration

**Files:**
- Create: `src/worker/runner.py`
- Create: `src/worker/cli.py`
- Create: `tests/test_runner.py`

**Interfaces:**
- `build_agent_command(agent: str, prompt: str, cwd: Path) -> list[str]`
- `TaskRunner.run(issue: Issue) -> TaskResult`
- `main() -> int`

- [ ] Write failing tests for Codex/Claude command construction, timeout conversion, and refusal of unsupported agents.
- [ ] Run the focused tests and confirm the expected failures.
- [ ] Implement command construction and a subprocess adapter with captured output and timeout handling.
- [ ] Implement the polling loop using `gh issue list`, claim labels, branch creation, push, PR creation, and failure comments; keep subprocess calls injectable for tests.
- [ ] Run all tests and a dry-run against the empty repository.
- [ ] Commit: `feat: run labeled issues through a coding agent`.

### Task 3: Service packaging and operational documentation

**Files:**
- Create: `.env.example`
- Create: `systemd/codex-issue-worker.service`
- Create: `README.md`
- Create: `docs/operations.md`
- Create: `tests/test_service_config.py`

**Interfaces:**
- Service reads `/home/agent/.config/codex-issue-worker/worker.env` and runs as `agent` from the repository directory.

- [ ] Write failing tests for required environment documentation and service safety flags.
- [ ] Implement the service with `Restart=on-failure`, bounded resource limits, `NoNewPrivileges=yes`, and no root execution.
- [ ] Document login, labels, dry-run, logs, rollback, and how to stop the service.
- [ ] Run tests, `systemd-analyze verify`, and a one-cycle dry-run.
- [ ] Commit: `feat: package worker as a user service`.

### Task 4: Publish and enable

**Files:**
- Modify: `/home/agent/.config/codex-issue-worker/worker.env`
- Modify: `/home/agent/.config/systemd/user/codex-issue-worker.service`

- [ ] Set the repository and agent configuration without writing credentials into the repository.
- [ ] Enable the user service only after the dry-run passes.
- [ ] Verify the service is active, idle with no eligible Issues, and logs are readable.
- [ ] Create a final baseline snapshot after verification.

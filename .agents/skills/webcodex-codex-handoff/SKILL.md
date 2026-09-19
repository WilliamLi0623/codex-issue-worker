---
name: webcodex-codex-handoff
description: Route interactive work in WebCodex and hand long or unattended tasks to the existing GitHub-driven Codex worker. Use when deciding between interactive execution and an asynchronous GitHub Issue handoff, when creating a worker task, or when reviewing a worker PR or auto-merged result.
---

# WebCodex to Codex worker handoff

## Boundaries

This skill coordinates two isolated execution environments for the same GitHub repository.

- WebCodex works only in its own checkout under `/srv/webcodex/projects`.
- The Codex worker owns `/home/agent/data/projects` and `/home/agent/data/tasks`.
- Never read, write, enqueue, unlock, or modify worker private state directly.
- Never point the worker at a WebCodex checkout.
- Synchronize only through GitHub issues, branches, commits, PRs, and the repository remote.
- Never copy GitHub or Codex credentials from the `agent` user to `webcodex`.

## Routing

Handle the work directly in WebCodex when it is interactive, diagnostic, review-oriented, a small edit, or a short test where the user benefits from live feedback.

Hand the work to the Codex worker when it is long-running, unattended, spans multiple modules, needs broad or slow tests/builds, is a large dependency/refactor task, or the user explicitly asks to hand it to Codex. Around 15-20 minutes is a useful signal, not a hard threshold.

## Handoff

Before creating the issue, gather enough evidence to make the task executable without shared local state.

The issue must contain:

# Problem

# Context

# Relevant files

# Reproduction

# Requested change

# Constraints

# Acceptance criteria

# Tests

# Notes from WebCodex investigation

Create the issue in `WilliamLi0623/codex-issue-worker` with label `codex-task`. The live worker polls open issues with that label and claims them by replacing it with `in-progress`.

If `gh auth status -h github.com` fails for the `webcodex` user, stop the handoff and request an independent GitHub login for that user. Do not reuse the worker user's credentials.

## Worker result

The worker creates an isolated per-task clone, works on branch `worker/issue-<number>`, pushes the result, and creates a PR. The worker is configured for auto-merge, so do not manually merge the PR as part of this handoff.

After the PR merges, WebCodex may fetch the resulting commit, inspect the diff, run focused validation in its own checkout, summarize the change and risks, and report any post-merge issue.

## Completion evidence

A completed asynchronous handoff should report:

- issue URL and trigger label;
- worker task/branch identity;
- PR URL;
- merged commit when auto-merge completes;
- validation evidence;
- WebCodex checkout status proving there was no shared mutable checkout.




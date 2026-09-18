# Environment Report

Observed 2026-09-18.

## Codex
- Host: codex-worker-01, Ubuntu 24.04.5, 4 vCPU, 8 GiB RAM.
- Codex CLI 0.154.0; multi_agent is stable/enabled and features.collab=true.
- Installed parent default: gpt-5.6-terra medium; subagent default: gpt-5.6-luna low.
- Project V1 concurrency ceiling: 4.
- Child WebCodex MCP inheritance still requires a live acceptance test.

## WebCodex
- Server/Runner 0.4.1, commit f080c8f3ea70.
- Server runs in LXC 210; systemd socket/service active.
- Linux Runner proxmox-webcodex-runner is active as Unix user webcodex.
- Source Project: agent:proxmox-webcodex-runner:codex-issue-worker.
- Source HEAD: c75f98a143baff564a681f81bea49ae53cf84c73.
- Live MCP schema exposes work_on_project worktree bootstrap, Git/file/edit/process/validation tools, and durable Job observation.
- Operator inventory sees both Windows and Proxmox Runners online.
- Current ChatGPT OAuth caller windows-workstation does not project the Proxmox Project even though operator inventory sees it; direct managed bootstrap reproduced unknown shell client: proxmox-webcodex-runner.
- V1 therefore uses the specified fallback: Runner-side Git worktrees plus project registration. WebCodex core is unchanged.

## Isolation
The existing GitHub issue worker runs as agent; WebCodex Runner runs as webcodex. Mutable checkouts/private state are separate and the source WebCodex checkout is read-only for this workflow.

## Open compatibility tests
1. Correct OAuth caller/Runner project visibility for direct Linux MCP worktree bootstrap.
2. Validate custom-agent schema and actual child model metadata on Codex 0.154.0.
3. Demonstrate child WebCodex MCP access; do not assume inheritance.

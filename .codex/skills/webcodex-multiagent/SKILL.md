# WebCodex Multi-Agent Orchestration

Codex is the scheduler; WebCodex is authoritative remote execution.

1. Avoid agents for trivial work; nesting depth is 1.
2. Resolve exact source HEAD, create run_id, atomically persist a manifest.
3. Use read-only wc_explorer when code ownership is unclear.
4. Decompose by behavior/module into a DAG with bounded paths.
5. Provision a separate WebCodex worktree/project per writer from the immutable base.
6. Spawn within the concurrency ceiling; workers never spawn grandchildren.
7. Wait natively; do not busy-loop.
8. Reject WorkerResult for wrong base, out-of-scope files, unexplained missing validation, or missing commit SHA.
9. Create a separate integration worktree; exactly one wc_integrator cherry-picks accepted commits serially.
10. Integrator handles simple conflicts; semantic conflicts go to wc_conflict_resolver.
11. Independent wc_validator reviews after integration and never silently patches.
12. Findings become explicit repair TaskSpecs, reintegration, then revalidation.
13. Detect stale source before integration and record the chosen strategy.
14. Record and observe the same durable WebCodex job_id; never duplicate a command because synchronous grace ended.
15. After restart reconcile manifest against remote Project/Job/Git state before new work.
16. Final report includes base, integration branch/commit, workers/commits, validation, risks and human-review instructions. Never auto-push or auto-merge main.

See docs/webcodex-multiagent/task-contract.md and recovery.md.

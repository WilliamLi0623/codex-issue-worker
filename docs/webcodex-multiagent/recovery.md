# Recovery

Persist only orchestration metadata under .codex/webcodex-runs and write manifests atomically (temporary file, flush/fsync when appropriate, rename).

- Worker dies: inspect manifest, worktree, Git and Jobs; resume/replace without recreating known work.
- Runner disconnects: retain IDs and wait; never replay mutations blindly.
- MCP failure: retry observation boundedly; do not repeat unknown-outcome operations.
- Validation outlives agent: record and observe the same job_id.
- Unexpected files: reject result; do not integrate.
- Missing commit: inspect diff and explicitly finalize/recover.
- Integrator dies: inspect cherry-pick state before continue/abort; never start a second integrator.
- Parent restarts: reconcile manifest with WebCodex/Git before dispatch.
- Stale source: record old/new HEAD and explicitly choose original-base integration or new-base cherry-pick plus revalidation.

Never store credentials or automatically push/merge main.

# Acceptance Testing

| Test | Status | Evidence / next action |
|---|---|---|
| Parent accesses WebCodex | PASS | Parent listed Projects and ran remote diagnostics. |
| Subagent accesses WebCodex | NOT RUN | Must be demonstrated from Codex 0.154.0 child. |
| Three parallel readers | NOT RUN | After child MCP access. |
| Two isolated writers | NOT RUN | Worktree schema exists; OAuth visibility currently blocks direct Linux bootstrap. |
| Integration | NOT RUN | Requires two worker commits. |
| Deliberate conflict | NOT RUN | Requires isolated writers. |
| Validation failure | NOT RUN | Requires integration fixture. |
| Long Job no duplicate | PASS | run_process became a durable Job; same job_id was observed to completion without rerun. |
| Agent crash recovery | NOT RUN | Pending live worker smoke. |
| Runner disconnect | NOT RUN | Defer disruption until basic smoke passes. |
| Parent restart | NOT RUN | Manifest protocol documented; live test pending. |
| Stale source | NOT RUN | Pending worker workflow. |
| Forbidden path | NOT RUN | Pending worker workflow. |
| Main protection | NOT RUN | Instructions enforce it; live negative test pending. |
| MCP resource behavior | NOT RUN | Pending repeated child test. |

NOT RUN/BLOCKED is never treated as PASS.

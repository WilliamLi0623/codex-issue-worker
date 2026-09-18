# WebCodex Multi-Agent Task Contract

TaskSpec and WorkerResult contain orchestration metadata only; never credentials or source code.

## TaskSpec
```json
{"run_id":"...","task_id":"...","role":"explorer|worker|integrator|validator","objective":"...","base_commit":"...","webcodex_source_project":"...","webcodex_work_project":null,"allowed_paths":[],"forbidden_paths":[],"dependencies":[],"validation":[],"expected_output":"...","status":"pending|running|success|blocked|failed"}
```

## WorkerResult
```json
{"run_id":"...","task_id":"...","status":"success|blocked|failed","base_commit":"...","work_project":"...","workflow_session":null,"changed_files":[],"commit_sha":null,"validation":[{"command":"...","status":"...","job_id":null}],"risks":[],"notes":[]}
```

Reject a successful write result when the base differs, changed files exceed scope, required validation is skipped without explanation, or no immutable commit handle exists.

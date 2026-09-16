from pathlib import Path
import unittest

from src.worker.runner import build_agent_command, build_prompt


class RunnerTests(unittest.TestCase):
    def test_codex_command_is_noninteractive_and_scoped(self):
        command = build_agent_command("codex", "Implement issue #7", Path("/work/repo"))
        self.assertEqual(command[:4], ["codex", "exec", "--cd", "/work/repo"])
        self.assertIn("Implement issue #7", command[-1])
        self.assertEqual(command[4:6], ["--sandbox", "workspace-write"])

    def test_claude_command_is_print_mode(self):
        command = build_agent_command("claude", "Implement issue #7", Path("/work/repo"))
        self.assertEqual(command[:4], ["claude", "--print", "--add-dir", "/work/repo"])

    def test_unsupported_agent_is_rejected(self):
        with self.assertRaisesRegex(ValueError, "unsupported agent"):
            build_agent_command("unknown", "task", Path("/work/repo"))

    def test_prompt_is_a_single_argument_even_when_it_looks_like_an_option(self):
        for agent in ("codex", "claude"):
            with self.subTest(agent=agent):
                command = build_agent_command(agent, "--help; echo injected", Path("/work/repo"))
                self.assertEqual(command[-2:], ["--", "--help; echo injected"])

    def test_prompt_requires_plan_and_verification(self):
        prompt = build_prompt(7, "Fix the parser", "https://github.com/x/y/issues/7")
        self.assertIn("先说明计划", prompt)
        self.assertIn("删除", prompt)
        self.assertIn("Fix the parser", prompt)
        self.assertIn("测试", prompt)


class SubprocessTests(unittest.TestCase):
    def test_captures_output_and_passes_a_bounded_noninteractive_call(self):
        import subprocess
        from src.worker import runner

        self.assertTrue(hasattr(runner, "SubprocessAdapter"), "subprocess adapter is missing")
        calls = []

        def execute(command, **kwargs):
            calls.append((command, kwargs))
            return subprocess.CompletedProcess(command, 3, "output", "error")

        result = runner.SubprocessAdapter(execute).run(["agent", "task"], Path("/work/repo"), 120)
        self.assertEqual((result.returncode, result.stdout, result.stderr, result.timed_out),
                         (3, "output", "error", False))
        self.assertEqual(calls, [(["agent", "task"], dict(
            cwd=Path("/work/repo"), timeout=120, capture_output=True,
            text=True, stdin=subprocess.DEVNULL, check=False))])

    def test_timeout_becomes_a_result_and_keeps_partial_output(self):
        import subprocess
        from src.worker.runner import SubprocessAdapter

        for output, error, expected in [(b"partial", b"error", ("partial", "error")),
                                        (None, None, ("", "")),
                                        ("partial", "error", ("partial", "error"))]:
            with self.subTest(output=output):
                def execute(command, **kwargs):
                    raise subprocess.TimeoutExpired(command, kwargs["timeout"], output, error)

                result = SubprocessAdapter(execute).run(["agent"], Path("."), 60)
                self.assertEqual((result.returncode, result.timed_out), (124, True))
                self.assertEqual((result.stdout, result.stderr), expected)

    def test_missing_executable_becomes_a_failure_without_exception_details(self):
        from src.worker.runner import SubprocessAdapter

        def execute(*args, **kwargs):
            raise FileNotFoundError("private execution details")

        result = SubprocessAdapter(execute).run(["missing"], Path("."), 60)
        self.assertEqual(result.returncode, 127)
        self.assertNotIn("private execution details", result.stderr)

    def test_real_timeout_terminates_spawned_tools_too(self):
        import sys
        import time
        from src.worker.runner import SubprocessAdapter

        script = ("import subprocess, sys, time; "
                  "child = subprocess.Popen([sys.executable, '-c', 'import time; time.sleep(2)']); "
                  "print(child.pid, flush=True); time.sleep(2)")
        started = time.monotonic()
        result = SubprocessAdapter().run([sys.executable, "-c", script], Path.cwd(), 0.3)
        self.assertTrue(result.timed_out)
        self.assertLess(time.monotonic() - started, 1.5)
        stat = Path("/proc") / result.stdout.strip() / "stat"
        deadline = time.monotonic() + 0.5
        while True:
            try:
                state = stat.read_text().split(") ", 1)[1][0]
            except (FileNotFoundError, ProcessLookupError):
                break
            if state == "Z":
                break
            if time.monotonic() >= deadline:
                self.fail("agent child is still running after timeout")
            time.sleep(0.01)



class GitHubTests(unittest.TestCase):
    def test_github_cli_scopes_calls_to_configured_repository(self):
        from src.worker import runner

        self.assertTrue(hasattr(runner, "GitHubCLI"), "GitHub CLI adapter is missing")
        calls = []

        class Process:
            def run(self, command, cwd, timeout):
                calls.append((command, cwd, timeout))
                return runner.CommandResult(0, '[]')

        github = runner.GitHubCLI("x/y", Process())
        result = github.run(["issue", "list", "--json", "number"], Path("."), 30)
        self.assertEqual(result.stdout, '[]')
        self.assertEqual(calls, [(["gh", "issue", "list", "--json", "number",
                                  "--repo", "x/y"], Path("."), 30)])


ISSUE = {"number": 7, "title": "Fix parser", "body": "Handle empty input",
         "url": "https://github.com/x/y/issues/7", "state": "OPEN",
         "labels": [{"name": "codex-task"}]}


class OrchestrationTests(unittest.TestCase):
    def test_configured_sandbox_reaches_codex_command(self):
        from dataclasses import replace
        from src.worker.config import load_config

        for sandbox in ("read-only", "workspace-write", "danger-full-access"):
            with self.subTest(sandbox=sandbox):
                task = self.make_runner()
                task.config = replace(load_config({"GH_REPO": "x/y", "AGENT_SANDBOX": sandbox}),
                                      work_root=str(self.root))
                result = task.run(ISSUE)
                self.assertEqual(result.status, "succeeded")
                command = next(command for command, _, _ in self.calls if command[0] == "codex")
                self.assertEqual(command[4:6], ["--sandbox", sandbox])

    def test_runner_passes_observation_context_to_agent_executor(self):
        from src.worker.runner import CommandResult, TaskRunner

        task = self.make_runner(agent_tmux=True)
        observed = []

        def execute(command, cwd, timeout, stdout_path, stderr_path, **options):
            observed.append((command[0], cwd, timeout, stdout_path, stderr_path, options))
            stdout_path.write_bytes(b"partial output\xff")
            stderr_path.write_text("partial error")
            return CommandResult(124, "partial output", "partial error", True)

        task = TaskRunner(task.config, process=self.process, github=task.github,
                          clock=lambda: self.now, agent_executor=execute)
        result = task.run(ISSUE)
        self.assertEqual(result.status, "timed_out")
        self.assertEqual(observed[0][0], "codex")
        self.assertEqual(observed[0][-1], {"session_name": "codex-task-issue-7", "tmux": True})
        self.assertEqual(result.stdout_path.read_bytes(), b"partial output\xff")
        self.assertEqual(result.stderr_path.read_text(), "partial error")
        self.assertFalse(any(command[:2] == ["git", "push"] for command, _, _ in self.calls))

    def test_observation_has_stable_identity_separate_streams_and_safe_events(self):
        import json
        task = self.make_runner()
        result = task.run(ISSUE)
        self.assertEqual(result.task_id, "issue-7")
        self.assertEqual(result.session_name, "codex-task-issue-7")
        self.assertEqual(result.stdout_path.read_text(), "tests passed")
        self.assertEqual(result.stderr_path.read_text(), "agent diagnostic")
        events = [json.loads(line) for line in result.events_path.read_text().splitlines()]
        self.assertEqual(events[0]["event"], "task_started")
        self.assertEqual(events[-1]["status"], "succeeded")
        self.assertTrue(all(event["task_id"] == "issue-7" for event in events))
        self.assertNotIn("Handle empty input", result.events_path.read_text())
        self.assertNotIn("agent diagnostic", result.events_path.read_text())
        retry = task.run(ISSUE, task_id="../../a:b ;$(hello)")
        self.assertEqual(retry.task_id, "../../a:b ;$(hello)")
        self.assertRegex(retry.session_name, r"^codex-task-[A-Za-z0-9_-]+$")
        self.assertNotEqual(retry.log_path.parent, result.log_path.parent)
        self.assertTrue(retry.stdout_path.is_relative_to(self.root))

    def make_runner(self, **config_values):
        import tempfile
        from src.worker import runner
        from src.worker.config import Config

        self.assertTrue(hasattr(runner, "TaskRunner"), "task orchestration is missing")
        root = Path(__file__).resolve().parents[1] / ".test-artifacts"
        root.mkdir(exist_ok=True)
        self.root = Path(tempfile.mkdtemp(prefix="runner-", dir=root))
        self.calls = []
        self.agent_result = runner.CommandResult(0, "tests passed", "agent diagnostic")
        self.current_branch = "worker/issue-7"
        self.default_branch = "main"
        self.existing_prs = []
        self.remote_branch_exists = False
        self.commit_count = "1\n"
        self.fresh_issue = dict(ISSUE)
        self.fail_at = None
        self.now = 100.0
        owner = self

        class Process:
            def run(self, command, cwd, timeout):
                import json
                owner.calls.append((command, cwd, timeout))
                if owner.fail_at and command[:len(owner.fail_at)] == owner.fail_at:
                    return runner.CommandResult(1, stderr="private diagnostic")
                if command[:3] == ["gh", "issue", "view"]:
                    return runner.CommandResult(0, json.dumps(owner.fresh_issue))
                if command[:3] == ["gh", "repo", "view"]:
                    return runner.CommandResult(0, owner.default_branch + "\n")
                if command[:3] == ["gh", "pr", "list"]:
                    return runner.CommandResult(0, json.dumps(owner.existing_prs))
                if command[:3] == ["gh", "pr", "create"]:
                    return runner.CommandResult(0, "https://github.com/x/y/pull/8\n")
                if command[:2] == ["git", "ls-remote"]:
                    return runner.CommandResult(0 if owner.remote_branch_exists else 2)
                if command[:2] == ["git", "clone"]:
                    Path(command[-1]).mkdir()
                if command[:3] == ["git", "branch", "--show-current"]:
                    return runner.CommandResult(0, owner.current_branch + "\n")
                if command[:3] == ["git", "diff", "--cached"]:
                    return runner.CommandResult(1)
                if command[:2] == ["git", "rev-list"]:
                    return runner.CommandResult(0, owner.commit_count)
                if command[0] in ("codex", "claude"):
                    owner.now += 5
                    return owner.agent_result
                return runner.CommandResult(0)

        self.process = Process()
        config = Config(repo="x/y", work_root=str(self.root), max_minutes=2, **config_values)
        return runner.TaskRunner(config, process=self.process,
                                 github=runner.GitHubCLI("x/y", self.process),
                                 clock=lambda: self.now)

    def test_clones_configured_repository_over_ssh(self):
        task = self.make_runner()
        result = task.run(ISSUE)
        clones = [command for command, _, _ in self.calls if command[:2] == ["git", "clone"]]
        self.assertEqual(clones, [["git", "clone", "--no-checkout", "--single-branch",
                                   "--branch", "main", "git@github.com:x/y.git",
                                   str(result.log_path.parent / "repo")]])

    def test_claims_runs_and_publishes_only_the_task_branch(self):
        task = self.make_runner()
        result = task.run(ISSUE)
        self.assertEqual((result.status, result.branch, result.pr_url),
                         ("succeeded", "worker/issue-7", "https://github.com/x/y/pull/8"))
        commands = [call[0] for call in self.calls]
        claim = ["gh", "issue", "edit", "7", "--add-label", "in-progress",
                 "--remove-label", "codex-task", "--repo", "x/y"]
        self.assertIn(claim, commands)
        agent_index = next(i for i, command in enumerate(commands) if command[0] == "codex")
        self.assertLess(commands.index(claim), agent_index)
        self.assertIn("Handle empty input", commands[agent_index][-1])
        self.assertIn(["git", "checkout", "-b", "worker/issue-7", "origin/main"], commands)
        self.assertIn(["git", "push", "git@github.com:x/y.git",
                       "HEAD:refs/heads/worker/issue-7"], commands)
        self.assertEqual(self.calls[agent_index][2], 120)
        self.assertEqual(self.calls[-1][2], 115)
        self.assertTrue(all(command[-2:] == ["--repo", "x/y"]
                            for command in commands if command[0] == "gh" and command[1] != "repo"))
        self.assertIn(["gh", "repo", "view", "x/y", "--json", "defaultBranchRef", "--jq",
                       ".defaultBranchRef.name"], commands)
        self.assertIn("tests passed", result.log_path.read_text())
        self.assertIn("agent diagnostic", result.log_path.read_text())

    def test_agent_failure_or_timeout_stops_publication_and_reports_concisely(self):
        from src.worker.runner import CommandResult

        for agent_result, status in [(CommandResult(2, stderr="private diagnostic"), "failed"),
                                     (CommandResult(124, "partial", "", True), "timed_out")]:
            with self.subTest(status=status):
                task = self.make_runner()
                self.agent_result = agent_result
                result = task.run(ISSUE)
                self.assertEqual(result.status, status)
                commands = [call[0] for call in self.calls]
                self.assertFalse(any(command[:2] == ["git", "push"] for command in commands))
                self.assertIn(["gh", "issue", "edit", "7", "--add-label", "worker-failed",
                               "--remove-label", "in-progress", "--repo", "x/y"], commands)
                comment = next(command for command in commands if command[:3] == ["gh", "issue", "comment"])
                self.assertNotIn("private diagnostic", " ".join(comment))
                self.assertIn(status, comment[comment.index("--body") + 1])

    def test_failed_claim_does_not_run_agent_or_publish(self):
        task = self.make_runner()
        self.fail_at = ["gh", "issue", "edit"]
        result = task.run(ISSUE)
        self.assertEqual(result.status, "failed")
        self.assertFalse(any(command[0] in ("codex", "git") for command, _, _ in self.calls))

    def test_ineligible_or_foreign_issue_is_rejected_before_external_calls(self):
        for changes in ({"state": "CLOSED"}, {"labels": []},
                        {"labels": ["codex-task", "in-progress"]},
                        {"url": "https://github.com/other/repo/issues/7"}):
            with self.subTest(changes=changes):
                task = self.make_runner()
                result = task.run({**ISSUE, **changes})
                self.assertEqual(result.status, "skipped")
                self.assertEqual(self.calls, [])

    def test_rechecks_eligibility_and_custom_claim_label_before_claiming(self):
        task = self.make_runner(in_progress_label="claimed")
        self.fresh_issue["labels"] = ["codex-task", "claimed"]
        result = task.run(ISSUE)
        self.assertEqual(result.status, "skipped")
        self.assertEqual(len(self.calls), 1)

    def test_custom_labels_replace_the_default_claim_label(self):
        task = self.make_runner(task_label="in-progress", in_progress_label="claimed")
        issue = {**ISSUE, "labels": ["in-progress"]}
        results = task.run_cycle(dry_run=True, issues=[issue])
        self.assertEqual([result.branch for result in results], ["worker/issue-7"])
        for label in ("claimed", {"name": "claimed"}):
            self.assertEqual(task.run_cycle(dry_run=True, issues=[
                {**issue, "labels": ["in-progress", label]}]), [])
        self.assertEqual(self.calls, [])

    def test_refuses_default_branch_collision_or_agent_branch_switch(self):
        for collision in (True, False):
            with self.subTest(collision=collision):
                task = self.make_runner()
                if collision:
                    self.default_branch = "worker/issue-7"
                else:
                    self.current_branch = "main"
                result = task.run(ISSUE)
                self.assertEqual(result.status, "failed")
                self.assertFalse(any(command[:2] in (["git", "push"], ["git", "add"],
                                                    ["git", "commit"])
                                     for command, _, _ in self.calls))

    def test_expired_total_budget_never_starts_another_task_command(self):
        task = self.make_runner()
        times = iter([100, 221])
        task.clock = lambda: next(times)
        result = task.run(ISSUE)
        self.assertEqual(result.status, "timed_out")
        self.assertEqual(self.calls, [])

    def test_existing_pr_is_reused(self):
        task = self.make_runner()
        self.existing_prs = [{"url": "https://github.com/x/y/pull/6"}]
        result = task.run(ISSUE)
        self.assertEqual(result.pr_url, "https://github.com/x/y/pull/6")
        self.assertFalse(any(command[:3] == ["gh", "pr", "create"] for command, _, _ in self.calls))

    def test_single_cycle_polls_only_configured_label_and_runs_one_issue(self):
        import json
        task = self.make_runner()
        self.assertTrue(hasattr(task, "run_cycle"), "single-cycle orchestration is missing")
        execute = self.process.run

        def run(command, cwd, timeout):
            if command[:3] == ["gh", "issue", "list"]:
                self.calls.append((command, cwd, timeout))
                from src.worker.runner import CommandResult
                return CommandResult(0, json.dumps([{**ISSUE, "state": "CLOSED"}, ISSUE, ISSUE]))
            return execute(command, cwd, timeout)

        self.process.run = run
        results = task.run_cycle()
        self.assertEqual([result.status for result in results], ["succeeded"])
        commands = [call[0] for call in self.calls]
        self.assertEqual(commands[0], ["gh", "issue", "list", "--state", "open", "--label",
                                       "codex-task", "--json", "number,title,body,url,state,labels",
                                       "--repo", "x/y"])
        self.assertEqual(sum(command[0] == "codex" for command in commands), 1)

    def test_os_lock_prevents_a_second_runner_from_touching_github(self):
        import fcntl
        task = self.make_runner()
        with (self.root / "worker.lock").open("a") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            with self.assertRaisesRegex(RuntimeError, "already running"):
                task.run_cycle()
            with self.assertRaisesRegex(RuntimeError, "already running"):
                task.run(ISSUE)
        self.assertEqual(self.calls, [])
        self.assertEqual(task.run(ISSUE).status, "succeeded")

    def test_retries_resume_the_existing_remote_task_branch_without_force(self):
        task = self.make_runner()
        self.remote_branch_exists = True
        self.existing_prs = [{"url": "https://github.com/x/y/pull/6"}]
        result = task.run(ISSUE)
        self.assertEqual(result.status, "succeeded")
        commands = [command for command, _, _ in self.calls]
        self.assertIn(["git", "fetch", "git@github.com:x/y.git",
                       "refs/heads/worker/issue-7:refs/remotes/origin/worker/issue-7"], commands)
        self.assertIn(["git", "checkout", "-b", "worker/issue-7", "origin/worker/issue-7"], commands)
        self.assertFalse(any("--force" in command for command in commands))

    def test_no_commits_ahead_of_default_is_not_published(self):
        task = self.make_runner()
        self.commit_count = "0\n"
        result = task.run(ISSUE)
        self.assertEqual(result.status, "failed")
        self.assertFalse(any(command[:2] == ["git", "push"] for command, _, _ in self.calls))

    def test_malformed_github_response_returns_failure_and_releases_claim(self):
        from src.worker.runner import CommandResult
        task = self.make_runner()
        execute = self.process.run

        def run(command, cwd, timeout):
            if command[:3] == ["gh", "pr", "list"]:
                return CommandResult(0, "invalid JSON")
            return execute(command, cwd, timeout)

        self.process.run = run
        result = task.run(ISSUE)
        self.assertEqual(result.status, "failed")
        self.assertIn("worker-failed", self.calls[-2][0])
        self.assertEqual(self.calls[-1][0][:3], ["gh", "issue", "comment"])

    def test_unsupported_agent_is_refused_before_claiming(self):
        with self.assertRaisesRegex(ValueError, "unsupported agent"):
            task = self.make_runner(agent="unsupported")
            task.run(ISSUE)
        self.assertEqual(self.calls, [])

    def test_invalid_runtime_config_is_rejected_before_any_work(self):
        from src.worker.config import Config
        from src.worker.runner import TaskRunner
        for values in ({"repo": "host/x/y"}, {"repo": "x/../y"},
                       {"repo": "x/y", "max_minutes": 0},
                       {"repo": "x/y", "task_label": "in-progress"}):
            with self.subTest(values=values):
                with self.assertRaises(ValueError):
                    TaskRunner(Config(**values))

    def test_malformed_github_issue_shapes_are_rejected(self):
        import json
        from src.worker.runner import CommandResult
        for payload in (None, {}, [None]):
            with self.subTest(payload=payload):
                task = self.make_runner()
                self.process.run = lambda *args: CommandResult(0, json.dumps(payload))
                with self.assertRaises(ValueError):
                    task.run_cycle()

    def test_relative_work_root_is_resolved_before_external_execution(self):
        from dataclasses import replace
        task = self.make_runner()
        task.config = replace(task.config, work_root=str(self.root.relative_to(Path.cwd())))
        self.assertEqual(task.run(ISSUE).status, "succeeded")
        self.assertTrue(all(cwd.is_absolute() for _, cwd, _ in self.calls))


if __name__ == "__main__":
    unittest.main()

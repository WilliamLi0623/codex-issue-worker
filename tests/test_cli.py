import importlib.util
import io
import json
from pathlib import Path
import unittest


class CLITests(unittest.TestCase):
    def test_dry_run_is_one_offline_cycle_without_filesystem_writes(self):
        self.assertIsNotNone(importlib.util.find_spec("src.worker.cli"), "CLI entry point is missing")
        from src.worker.cli import main

        class Forbidden:
            def run(self, *args, **kwargs):
                raise AssertionError("dry-run invoked an external command")

        root = Path(__file__).resolve().parents[1] / ".test-artifacts" / "dry-run-must-not-create"
        self.assertFalse(root.exists())
        for argv in ([], ["--dry-run", "--once"]):
            output = io.StringIO()
            status = main(argv, env={"GH_REPO": "x/y", "WORK_ROOT": str(root)},
                          process=Forbidden(), github=Forbidden(), stdout=output,
                          sleep=lambda _: self.fail("dry-run tried to poll repeatedly"))
            self.assertEqual(status, 0)
            self.assertEqual(json.loads(output.getvalue()), {"dry_run": True, "results": []})
            self.assertFalse(root.exists())

    def test_execute_once_lists_issues_once_and_returns_when_idle(self):
        from src.worker.cli import main
        from src.worker.runner import CommandResult

        calls = []

        class Github:
            def run(self, args, cwd, timeout):
                calls.append(args)
                return CommandResult(0, "[]")

        output = io.StringIO()
        self.assertEqual(main(["--execute", "--once"], env={"GH_REPO": "x/y", "WORK_ROOT": str(Path(__file__).resolve().parents[1] / ".test-artifacts" / "cli")},
                              github=Github(), stdout=output,
                              sleep=lambda _: self.fail("one cycle slept")), 0)
        self.assertEqual(len(calls), 1)
        self.assertEqual(calls[0][:2], ["issue", "list"])
        self.assertEqual(json.loads(output.getvalue()), {"dry_run": False, "results": []})

    def test_polling_waits_between_cycles_and_handles_interrupt(self):
        from src.worker.cli import main
        from src.worker.runner import CommandResult

        events = []

        class Github:
            def run(self, args, cwd, timeout):
                events.append("poll")
                if events.count("poll") == 2:
                    raise KeyboardInterrupt
                return CommandResult(0, "[]")

        result = main(["--execute"], env={"GH_REPO": "x/y", "POLL_SECONDS": "9",
                      "WORK_ROOT": str(Path(__file__).resolve().parents[1] / ".test-artifacts" / "cli")},
                      github=Github(), stdout=io.StringIO(), sleep=lambda seconds: events.append(seconds))
        self.assertEqual(result, 130)
        self.assertEqual(events, ["poll", 9, "poll"])

    def test_dry_run_previews_only_one_eligible_local_fixture(self):
        from src.worker.cli import main
        from tests.test_runner import ISSUE

        root = Path(__file__).resolve().parents[1] / ".test-artifacts"
        root.mkdir(exist_ok=True)
        fixture = root / "offline-issues.json"
        fixture.write_text(json.dumps([{**ISSUE, "state": "CLOSED"}, ISSUE, ISSUE]))

        class Forbidden:
            def run(self, *args, **kwargs):
                raise AssertionError("external operation")

        output = io.StringIO()
        self.assertEqual(main(["--dry-run", "--once", "--issues-file", str(fixture)],
                              env={"GH_REPO": "x/y", "WORK_ROOT": str(root / "preview-no-write")},
                              process=Forbidden(), github=Forbidden(), stdout=output), 0)
        self.assertEqual(json.loads(output.getvalue()), {"dry_run": True,
                         "results": [{"status": "dry_run", "branch": "worker/issue-7"}]})
        self.assertFalse((root / "preview-no-write").exists())

    def test_failed_task_produces_nonzero_exit_status(self):
        from src.worker.cli import main
        from src.worker.runner import CommandResult
        from tests.test_runner import ISSUE

        class Github:
            def run(self, args, cwd, timeout):
                if args[:2] == ["issue", "list"]:
                    return CommandResult(0, json.dumps([ISSUE]))
                return CommandResult(1)

        output = io.StringIO()
        code = main(["--execute", "--once"], env={"GH_REPO": "x/y",
                    "WORK_ROOT": str(Path(__file__).resolve().parents[1] / ".test-artifacts" / "cli")},
                    github=Github(), stdout=output)
        self.assertEqual(code, 1)
        self.assertEqual(json.loads(output.getvalue())["results"][0]["status"], "failed")

    def test_configuration_and_poll_errors_return_sanitized_nonzero_results(self):
        from src.worker.cli import main
        from src.worker.runner import CommandResult

        class Github:
            def run(self, args, cwd, timeout):
                return CommandResult(1, stderr="private diagnostic")

        for argv, env in [([], {}), (["--execute", "--once"], {"GH_REPO": "x/y",
                          "WORK_ROOT": str(Path(__file__).resolve().parents[1] / ".test-artifacts" / "cli")})]:
            with self.subTest(argv=argv):
                output = io.StringIO()
                self.assertNotEqual(main(argv, env=env, github=Github(), stdout=output), 0)
                self.assertIn("error", json.loads(output.getvalue()))
                self.assertNotIn("private diagnostic", output.getvalue())

    def test_invalid_offline_issue_shapes_fail_without_external_calls(self):
        from src.worker.cli import main

        root = Path(__file__).resolve().parents[1] / ".test-artifacts"
        root.mkdir(exist_ok=True)
        fixture = root / "invalid-issues.json"
        for value in (None, {}, [None]):
            with self.subTest(value=value):
                fixture.write_text(json.dumps(value))
                output = io.StringIO()
                self.assertEqual(main(["--dry-run", "--issues-file", str(fixture)],
                                      env={"GH_REPO": "x/y"}, stdout=output), 1)
                self.assertIn("error", json.loads(output.getvalue()))

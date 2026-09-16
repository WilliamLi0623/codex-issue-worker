import configparser
import json
from pathlib import Path
import shlex
import subprocess
import unittest

from src.worker.config import load_config


ROOT = Path(__file__).resolve().parents[1]


class ServiceConfigTests(unittest.TestCase):
    def read_file(self, relative):
        path = ROOT / relative
        self.assertTrue(path.is_file(), f"missing service packaging file: {relative}")
        return path.read_text(encoding="utf-8")

    def service(self):
        config = configparser.ConfigParser(interpolation=None, strict=True)
        config.optionxform = str
        config.read_string(self.read_file("systemd/codex-issue-worker.service"))
        return config

    def example_environment(self):
        return dict(line.split("=", 1) for line in self.read_file(".env.example").splitlines()
                    if line.strip() and not line.lstrip().startswith("#"))

    def test_example_documents_all_supported_settings_and_loads_safely(self):
        env = self.example_environment()
        self.assertEqual(set(env), {"GH_REPO", "AGENT", "TASK_LABEL", "IN_PROGRESS_LABEL",
                                    "MAX_MINUTES", "WORK_ROOT", "POLL_SECONDS", "AGENT_TMUX"})
        config = load_config(env)
        self.assertEqual(config.repo, "owner/repository")
        self.assertFalse(config.agent_tmux)
        self.assertGreater(config.max_minutes, 0)
        self.assertTrue(Path(config.work_root).is_absolute())

    def test_service_is_agent_user_scoped_and_loads_required_environment(self):
        config = self.service()
        service = config["Service"]
        self.assertEqual(config["Unit"]["ConditionUser"], "agent")
        self.assertEqual(Path(service["WorkingDirectory"]), ROOT)
        self.assertEqual(service["EnvironmentFile"],
                         "/home/agent/.config/codex-issue-worker/worker.env")
        self.assertEqual(config["Install"]["WantedBy"], "default.target")

    def test_user_service_inherits_manager_credentials_without_group_setup(self):
        service = self.service()["Service"]
        # Explicit credentials can trigger privileged supplementary-group setup
        # before ExecStart, failing with 216/GROUP in an unprivileged manager.
        for directive in ("User", "Group", "SupplementaryGroups", "DynamicUser"):
            with self.subTest(directive=directive):
                self.assertNotIn(directive, service)

    def test_service_bounds_restart_resources_and_stops_its_process_tree(self):
        service = self.service()["Service"]
        self.assertEqual(service["Restart"], "on-failure")
        self.assertGreaterEqual(int(service["RestartSec"]), 5)
        self.assertEqual(service["NoNewPrivileges"], "yes")
        self.assertEqual(service["KillMode"], "control-group")
        self.assertGreater(int(service["TimeoutStopSec"]), 0)
        self.assertEqual(service["UMask"], "0077")
        self.assertTrue(0 < int(service["CPUQuota"].rstrip("%")) <= 200)
        self.assertTrue(0 < int(service["MemoryMax"].rstrip("G")) <= 4)
        self.assertTrue(0 < int(service["TasksMax"]) <= 512)
        self.assertTrue(0 < int(service["LimitNOFILE"]) <= 65536)
        self.assertEqual(service["StandardOutput"], "journal")
        self.assertEqual(service["StandardError"], "journal")
        self.assertEqual(service["RuntimeDirectory"], "codex-issue-worker")
        self.assertIn("TMUX_TMPDIR=%t/codex-issue-worker", shlex.split(service["Environment"]))

    def test_packaged_start_command_runs_one_offline_cycle(self):
        service = self.service()["Service"]
        command = shlex.split(service["ExecStart"])
        self.assertIn("--dry-run", command)
        self.assertNotIn("--execute", command)
        env = self.example_environment()
        # No credentials or inherited environment; external CLIs cannot be found.
        env["PATH"] = "/nonexistent"
        result = subprocess.run(command, cwd=service["WorkingDirectory"], env=env,
                                capture_output=True, text=True, timeout=10)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout), {"dry_run": True, "results": []})
        self.assertEqual(result.stderr, "")

    def test_operations_cover_required_operator_actions(self):
        readme = self.read_file("README.md")
        self.assertIn("docs/operations.md", readme)
        operations = self.read_file("docs/operations.md")
        for required in ("gh auth login", "codex-task", "in-progress", "worker-failed",
                         "--dry-run", "--execute", "AGENT_TMUX=1", "tmux attach",
                         "events.jsonl", "agent.stdout.log", "agent.stderr.log", "task.log",
                         "systemctl --user stop", "systemctl --user status", "journalctl --user",
                         "回滚"):
            with self.subTest(required=required):
                self.assertIn(required, operations)

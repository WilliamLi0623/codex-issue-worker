import unittest

from src.worker.config import load_config
from src.worker.issues import branch_name, eligible_issue


class ConfigTests(unittest.TestCase):
    def test_requires_repository(self):
        with self.assertRaises(ValueError):
            load_config({})

    def test_defaults_are_safe(self):
        config = load_config({"GH_REPO": "WilliamLi0623/codex-issue-worker"})
        self.assertEqual(config.agent, "codex")
        self.assertEqual(config.task_label, "codex-task")
        self.assertEqual(config.in_progress_label, "in-progress")
        self.assertEqual(config.max_minutes, 120)

    def test_rejects_invalid_sandbox(self):
        for sandbox in ("", "unknown", "danger-full-access --extra"):
            with self.subTest(sandbox=sandbox):
                with self.assertRaisesRegex(ValueError, "AGENT_SANDBOX"):
                    load_config({"GH_REPO": "x/y", "AGENT_SANDBOX": sandbox})

    def test_rejects_unknown_agent(self):
        with self.assertRaises(ValueError):
            load_config({"GH_REPO": "x/y", "AGENT": "unknown"})


class IssueSelectionTests(unittest.TestCase):
    def test_only_open_codex_tasks_are_eligible(self):
        self.assertTrue(eligible_issue({"state": "OPEN", "labels": [{"name": "codex-task"}]}))
        self.assertFalse(eligible_issue({"state": "CLOSED", "labels": [{"name": "codex-task"}]}))
        self.assertFalse(eligible_issue({"state": "OPEN", "labels": [{"name": "in-progress"}]}))

    def test_branch_name_is_deterministic(self):
        self.assertEqual(branch_name(42), "worker/issue-42")


if __name__ == "__main__":
    unittest.main()

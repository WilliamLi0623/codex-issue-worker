import json
from pathlib import Path
import shutil
import sys
import tempfile
import threading
import time
import unittest


class ObservationTests(unittest.TestCase):
    def setUp(self):
        (Path(__file__).resolve().parents[1] / ".test-artifacts").mkdir(exist_ok=True)

    def test_missing_tmux_completion_keeps_partial_output(self):
        from unittest.mock import patch
        from src.worker.observation import run_agent
        from src.worker.runner import CommandResult

        directory = Path(tempfile.mkdtemp(prefix="observe-watchdog-", dir=Path(__file__).resolve().parents[1] / ".test-artifacts"))
        def launch(*args):
            (directory / "stdout").write_text("partial")
            (directory / "stderr").write_text("diagnostic")
            return CommandResult(0)

        with patch("src.worker.runner.SubprocessAdapter.run", side_effect=launch), \
                patch("src.worker.observation.time.monotonic", side_effect=[0, 20]):
            result = run_agent(["fake-agent"], directory, 1, directory / "stdout", directory / "stderr",
                               session_name="codex-task-watchdog", tmux=True)
        self.assertTrue(result.timed_out)
        self.assertEqual((result.stdout, result.stderr), ("partial", "diagnostic"))

    @unittest.skipUnless(shutil.which("tmux"), "tmux is unavailable")
    def test_tmux_live_pane_and_logs_are_visible_before_agent_finishes(self):
        import subprocess
        from src.worker.observation import run_agent

        directory = Path(tempfile.mkdtemp(prefix="observe live ", dir=Path(__file__).resolve().parents[1] / ".test-artifacts"))
        session = "codex-task-live-" + directory.name.split()[-1]
        output = []
        script = "import sys,time; print(sys.argv[1], flush=True); print('live error', file=sys.stderr, flush=True); time.sleep(1.5)"
        literal = "$(touch injected); 'quoted'"
        thread = threading.Thread(target=lambda: output.append(run_agent(
            [sys.executable, "-c", script, literal], directory, 4,
            directory / "stdout", directory / "stderr", session_name=session, tmux=True)))
        thread.start()
        try:
            deadline = time.monotonic() + 3
            while time.monotonic() < deadline:
                pane = subprocess.run(["tmux", "capture-pane", "-p", "-t", "=" + session + ":"],
                                      capture_output=True, text=True)
                if literal in pane.stdout and "live error" in pane.stdout:
                    break
                time.sleep(0.02)
            self.assertIn(literal, pane.stdout)
            self.assertIn("live error", pane.stdout)
            self.assertTrue(thread.is_alive())
            self.assertIn(literal, (directory / "stdout").read_text())
            self.assertEqual((directory / "stderr").read_text(), "live error\n")
            collision = run_agent([sys.executable, "-c", "raise SystemExit(0)"], directory,
                                  1, directory / "other-out", directory / "other-err",
                                  session_name=session, tmux=True)
            self.assertNotEqual(collision.returncode, 0)
        finally:
            thread.join(6)
        self.assertFalse(thread.is_alive())
        self.assertEqual(output[0].returncode, 0)
        self.assertFalse((directory / "injected").exists())

    @unittest.skipUnless(shutil.which("tmux"), "tmux is unavailable")
    def test_tmux_timeout_keeps_partial_logs(self):
        from src.worker.observation import run_agent

        directory = Path(tempfile.mkdtemp(prefix="observe-timeout-", dir=Path(__file__).resolve().parents[1] / ".test-artifacts"))
        result = run_agent([sys.executable, "-c", "import time; print('partial', flush=True); time.sleep(20)"],
                           directory, 0.2, directory / "stdout", directory / "stderr",
                           session_name="codex-task-" + directory.name, tmux=True)
        self.assertTrue(result.timed_out)
        self.assertEqual(result.stdout, "partial\n")

    def test_tmux_config_is_opt_in_and_validated(self):
        from src.worker.config import load_config

        self.assertFalse(load_config({"GH_REPO": "x/y"}).agent_tmux)
        self.assertTrue(load_config({"GH_REPO": "x/y", "AGENT_TMUX": "1"}).agent_tmux)
        with self.assertRaises(ValueError):
            load_config({"GH_REPO": "x/y", "AGENT_TMUX": "perhaps"})

    def test_tmux_preserves_separate_output_and_agent_exit_status(self):
        from src.worker.observation import run_agent

        root = Path(__file__).resolve().parents[1] / ".test-artifacts"
        directory = Path(tempfile.mkdtemp(prefix="observe-", dir=root))
        for tmux in (False, True):
            with self.subTest(tmux=tmux):
                if tmux and not shutil.which("tmux"):
                    self.skipTest("tmux is unavailable")
                result = run_agent(
                    [sys.executable, "-c", "import sys; print('out'); print('err', file=sys.stderr); sys.exit(3)"],
                    directory, 3, directory / "stdout", directory / "stderr",
                    session_name="codex-task-" + directory.name, tmux=tmux)
                self.assertEqual((result.returncode, result.timed_out), (3, False))
                self.assertEqual((directory / "stdout").read_text(), "out\n")
                self.assertEqual((directory / "stderr").read_text(), "err\n")

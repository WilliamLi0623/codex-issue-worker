"""Local agent output capture; the tmux child also owns the agent timeout."""

import json
import os
from pathlib import Path
import shlex
import signal
import subprocess
import sys
import tempfile
import threading
import time


def _capture(command, cwd, timeout, stdout_path, stderr_path, mirror=False):
    status = dict(returncode=127, timed_out=False)
    with stdout_path.open("wb") as out, stderr_path.open("wb") as err:
        try:
            process = subprocess.Popen(command, cwd=cwd, stdin=subprocess.DEVNULL,
                                       stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                       start_new_session=True)
        except OSError:
            err.write(b"could not start command\n")
            return status

        def copy(source, target, terminal):
            with source:
                while chunk := source.read1(65536):
                    target.write(chunk)
                    target.flush()
                    if mirror:
                        try:
                            terminal.buffer.write(chunk)
                            terminal.buffer.flush()
                        except OSError:
                            pass

        threads = [threading.Thread(target=copy, args=args) for args in
                   ((process.stdout, out, sys.stdout), (process.stderr, err, sys.stderr))]
        for thread in threads:
            thread.start()
        try:
            code = process.wait(timeout=timeout)
            status = dict(returncode=code, timed_out=False)
        except subprocess.TimeoutExpired:
            status = dict(returncode=124, timed_out=True)
        finally:
            # Descendants must not hold the log pipes open after the agent exits.
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            process.wait()
            for thread in threads:
                thread.join()
    return status


def run_agent(command, cwd, timeout, stdout_path, stderr_path, *, session_name, tmux=False):
    from .runner import CommandResult, SubprocessAdapter

    for path in (stdout_path, stderr_path):
        path.touch(mode=0o600)
    if not tmux:
        status = _capture(command, cwd, timeout, stdout_path, stderr_path)
    else:
        # A fresh private directory prevents stale completion files on retries.
        state = Path(tempfile.mkdtemp(prefix="tmux-", dir=stdout_path.parent))
        result_path = state / "result.json"
        args = [sys.executable, "-B", str(Path(__file__).resolve()), str(result_path),
                str(cwd), str(timeout), str(stdout_path), str(stderr_path), *command]
        launched = SubprocessAdapter().run(
            ["tmux", "new-session", "-d", "-s", session_name, "-c", str(cwd), shlex.join(args)],
            cwd, min(timeout, 10))
        if launched.returncode or launched.timed_out:
            stderr_path.write_text("could not start tmux session", encoding="utf-8")
            return CommandResult(launched.returncode, stderr="could not start tmux session",
                                 timed_out=launched.timed_out)
        deadline = time.monotonic() + timeout + 5
        while not result_path.exists():
            if time.monotonic() >= deadline:
                status = dict(returncode=124, timed_out=True)
                break
            time.sleep(0.05)
        else:
            status = json.loads(result_path.read_text())
    return CommandResult(stdout=stdout_path.read_text(encoding="utf-8", errors="replace"),
                         stderr=stderr_path.read_text(encoding="utf-8", errors="replace"), **status)


if __name__ == "__main__":
    result_path, cwd, timeout, stdout_path, stderr_path, *command = sys.argv[1:]
    status = _capture(command, Path(cwd), float(timeout), Path(stdout_path), Path(stderr_path), True)
    pending = Path(result_path).with_suffix(".pending")
    pending.write_text(json.dumps(status))
    pending.replace(result_path)

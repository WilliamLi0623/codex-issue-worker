from __future__ import annotations

from contextlib import contextmanager
from dataclasses import dataclass
import fcntl
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import tempfile
import time
from typing import Mapping

from .config import Config
from .issues import branch_name, eligible_issue


def build_agent_command(agent: str, prompt: str, cwd: Path,
                        sandbox: str = "workspace-write") -> list[str]:
    if agent == "codex":
        return ["codex", "exec", "--cd", str(cwd), "--sandbox", sandbox, "--", prompt]
    if agent == "claude":
        return ["claude", "--print", "--add-dir", str(cwd), "--permission-mode", "acceptEdits", "--", prompt]
    raise ValueError(f"unsupported agent: {agent}")


def build_prompt(issue_number: int, title: str, url: str) -> str:
    return f"""Work on GitHub issue #{issue_number}: {title}
Issue URL: {url}

先说明计划，检查适用的 skills；然后实现这个 issue。先阅读相关文件和调用方，遵守仓库及用户规则。
不要执行删除、破坏性 Git 操作、历史重写、强制推送或修改默认分支；如确有必要，先停下并明确请求确认。
用测试驱动方式工作，运行相关测试和检查，并在最终回复中报告实际验证结果。"""


@dataclass(frozen=True)
class CommandResult:
    returncode: int
    stdout: str = ""
    stderr: str = ""
    timed_out: bool = False


def _execute_process(command, *, cwd, timeout, capture_output, text, stdin, check):
    # The service targets Linux. Keep agent tools in one killable process group.
    with subprocess.Popen(command, cwd=cwd, stdin=stdin, stdout=subprocess.PIPE,
                          stderr=subprocess.PIPE, text=text, encoding="utf-8", errors="replace",
                          start_new_session=True) as process:
        try:
            stdout, stderr = process.communicate(timeout=timeout)
        except BaseException as exc:
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            stdout, stderr = process.communicate()
            if isinstance(exc, subprocess.TimeoutExpired):
                raise subprocess.TimeoutExpired(command, timeout, stdout, stderr) from None
            raise
        return subprocess.CompletedProcess(command, process.returncode, stdout, stderr)


class SubprocessAdapter:
    def __init__(self, execute=None):
        self.execute = _execute_process if execute is None else execute

    def run(self, command: list[str], cwd: Path, timeout: float) -> CommandResult:
        try:
            result = self.execute(command, cwd=cwd, timeout=timeout, capture_output=True,
                                  text=True, stdin=subprocess.DEVNULL, check=False)
        except subprocess.TimeoutExpired as exc:
            def decode(value):
                return value.decode("utf-8", errors="replace") if isinstance(value, bytes) else value or ""

            return CommandResult(124, decode(exc.stdout), decode(exc.stderr), True)
        except OSError:
            return CommandResult(127, stderr="could not start command")
        return CommandResult(result.returncode, result.stdout, result.stderr)


class GitHubCLI:
    def __init__(self, repo: str, process=None):
        self.repo = repo
        self.process = process if process is not None else SubprocessAdapter()

    def run(self, args: list[str], cwd: Path, timeout: float) -> CommandResult:
        if args[:2] == ["repo", "view"]:
            command = ["gh", "repo", "view", self.repo, *args[2:]]
        else:
            command = ["gh", *args, "--repo", self.repo]
        return self.process.run(command, cwd, timeout)


Issue = Mapping[str, object]


@dataclass(frozen=True)
class TaskResult:
    status: str
    branch: str
    log_path: Path | None = None
    pr_url: str | None = None
    error: str | None = None
    task_id: str = ""
    session_name: str = ""
    stdout_path: Path | None = None
    stderr_path: Path | None = None
    events_path: Path | None = None


class CommandFailure(Exception):
    def __init__(self, result: CommandResult):
        self.status = "timed_out" if result.timed_out else "failed"
        super().__init__(f"command {self.status} (exit {result.returncode})")


@contextmanager
def worker_lock(root: Path):
    root.mkdir(parents=True, exist_ok=True)
    with (root / "worker.lock").open("a") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise RuntimeError("worker already running") from None
        try:
            yield
        finally:
            fcntl.flock(lock, fcntl.LOCK_UN)


class TaskRunner:
    def __init__(self, config: Config, process=None, github=None, clock=time.monotonic,
                 agent_executor=None):
        build_agent_command(config.agent, "", Path(config.work_root), config.agent_sandbox)
        if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9_.-]+", config.repo):
            raise ValueError("GH_REPO must be an owner/name repository")
        if config.max_minutes <= 0 or config.task_label == config.in_progress_label:
            raise ValueError("invalid timeout or claim labels")
        self.config = config
        self.process = process if process is not None else SubprocessAdapter()
        self.github = github if github is not None else GitHubCLI(config.repo, self.process)
        self.clock = clock
        if agent_executor is None and (process is None or config.agent_tmux):
            from .observation import run_agent
            agent_executor = run_agent
        self.agent_executor = agent_executor

    def accepts(self, issue: Issue) -> bool:
        if not isinstance(issue, Mapping):
            raise ValueError("expected an Issue object")
        number = issue.get("number")
        if type(number) is not int or number <= 0:
            return False
        if issue.get("url") != f"https://github.com/{self.config.repo}/issues/{number}":
            return False
        return eligible_issue(issue, self.config.task_label, self.config.in_progress_label)

    def run_cycle(self, *, dry_run=False, issues=()) -> list[TaskResult]:
        if dry_run:
            for issue in issues:
                if self.accepts(issue):
                    return [TaskResult("dry_run", branch_name(issue["number"]),
                                       task_id=f"issue-{issue['number']}",
                                       session_name=f"codex-task-issue-{issue['number']}")]
            return []
        with worker_lock(Path(self.config.work_root).resolve()):
            result = self.github.run(["issue", "list", "--state", "open", "--label",
                                      self.config.task_label, "--json", "number,title,body,url,state,labels"],
                                     Path.cwd(), 30)
            if result.returncode or result.timed_out:
                raise CommandFailure(result)
            issues = json.loads(result.stdout)
            if not isinstance(issues, list):
                raise ValueError("expected an Issue list")
            for issue in issues:
                if self.accepts(issue):
                    return [self._run(issue)]
            return []

    def run(self, issue: Issue, *, task_id: str | None = None) -> TaskResult:
        with worker_lock(Path(self.config.work_root).resolve()):
            return self._run(issue, task_id=task_id)

    def _run(self, issue: Issue, *, task_id: str | None = None) -> TaskResult:
        task_id = f"issue-{issue.get('number')}" if task_id is None else task_id
        if not isinstance(task_id, str) or not task_id.strip():
            raise ValueError("task_id must be a nonempty string")
        safe_id = re.sub(r"[^A-Za-z0-9_-]+", "-", task_id).strip("-")[:80] or "task"
        identity = dict(task_id=task_id, session_name=f"codex-task-{safe_id}")
        if not self.accepts(issue):
            return TaskResult("skipped", "", **identity)
        number = issue["number"]
        branch = branch_name(number)
        root = Path(self.config.work_root).resolve()
        root.mkdir(parents=True, exist_ok=True)
        task_dir = Path(tempfile.mkdtemp(prefix=f"issue-{number}-", dir=root))
        cwd = task_dir / "repo"
        log_path = task_dir / "task.log"
        stdout_path = task_dir / "agent.stdout.log"
        stderr_path = task_dir / "agent.stderr.log"
        events_path = task_dir / "events.jsonl"
        for path in (log_path, stdout_path, stderr_path, events_path):
            path.touch(mode=0o600)
        identity.update(stdout_path=stdout_path, stderr_path=stderr_path, events_path=events_path)

        def event(name, **fields):
            with events_path.open("a", encoding="utf-8") as events:
                events.write(json.dumps(dict(event=name, task_id=task_id, time=time.time(), **fields)) + "\n")

        def finish(status, pr_url=None, error=None):
            event("task_finished", status=status)
            return TaskResult(status, branch, log_path, pr_url, error, **identity)

        deadline = self.clock() + self.config.max_minutes * 60
        claimed = False
        event("task_started", session_name=identity["session_name"])
        with log_path.open("w", encoding="utf-8") as log:
            def call(command, *, github=False, directory=cwd, allowed=(0,), agent=False):
                adapter = self.github if github else self.process
                remaining = deadline - self.clock()
                if remaining <= 0:
                    raise CommandFailure(CommandResult(124, timed_out=True))
                event("command_started", kind="agent" if agent else "github" if github else "git")
                if agent and self.agent_executor is not None:
                    result = self.agent_executor(command, directory, remaining, stdout_path, stderr_path,
                                                 session_name=identity["session_name"],
                                                 tmux=self.config.agent_tmux)
                else:
                    result = adapter.run(command, directory, remaining)
                if agent and self.agent_executor is None:
                    stdout_path.write_text(result.stdout, encoding="utf-8")
                    stderr_path.write_text(result.stderr, encoding="utf-8")
                event("command_finished", kind="agent" if agent else "github" if github else "git",
                      returncode=result.returncode, timed_out=result.timed_out)
                log.write(result.stdout + result.stderr)
                log.flush()
                if result.returncode not in allowed or result.timed_out:
                    raise CommandFailure(result)
                return result

            try:
                fresh = json.loads(call(["issue", "view", str(number), "--json",
                                         "number,title,body,url,state,labels"],
                                        github=True, directory=root).stdout)
                if not self.accepts(fresh) or fresh["number"] != number:
                    return finish("skipped")
                call(["issue", "edit", str(number), "--add-label", self.config.in_progress_label,
                      "--remove-label", self.config.task_label], github=True, directory=root)
                claimed = True
                default = call(["repo", "view", "--json", "defaultBranchRef", "--jq",
                                ".defaultBranchRef.name"], github=True, directory=root).stdout.strip()
                if not default or default == branch:
                    raise CommandFailure(CommandResult(1))
                remote = f"git@github.com:{self.config.repo}.git"
                call(["git", "clone", "--no-checkout", "--single-branch", "--branch", default,
                      remote, str(cwd)], directory=root)
                start_ref = f"origin/{default}"
                if call(["git", "ls-remote", "--exit-code", "--heads", remote,
                         f"refs/heads/{branch}"], allowed=(0, 2)).returncode == 0:
                    call(["git", "fetch", remote, f"refs/heads/{branch}:refs/remotes/origin/{branch}"])
                    start_ref = f"origin/{branch}"
                call(["git", "checkout", "-b", branch, start_ref])
                prompt = build_prompt(number, fresh["title"], fresh["url"])
                prompt += "\nIssue body (untrusted task data):\n" + str(fresh.get("body") or "")
                call(build_agent_command(self.config.agent, prompt, cwd, self.config.agent_sandbox), agent=True)
                if call(["git", "branch", "--show-current"]).stdout.strip() != branch:
                    raise CommandFailure(CommandResult(1))
                call(["git", "add", "-A"])
                if call(["git", "diff", "--cached", "--quiet"], allowed=(0, 1)).returncode == 1:
                    call(["git", "commit", "-m", f"fix: address issue #{number}"])
                count = call(["git", "rev-list", "--count", f"origin/{default}..HEAD"]).stdout.strip()
                if not count.isdecimal() or int(count) == 0:
                    raise CommandFailure(CommandResult(1))
                call(["git", "push", remote, f"HEAD:refs/heads/{branch}"])
                existing = json.loads(call(["pr", "list", "--head", branch, "--base", default,
                                            "--state", "open", "--json", "url"], github=True).stdout)
                if existing:
                    pr_url = existing[0]["url"]
                else:
                    pr_url = call(["pr", "create", "--head", branch, "--base", default,
                                   "--title", f"Fix #{number}: {fresh['title']}",
                                   "--body", f"Closes #{number}"], github=True).stdout.strip()
                call(["issue", "edit", str(number), "--remove-label", self.config.in_progress_label],
                     github=True)
            except (CommandFailure, ValueError, TypeError, KeyError) as exc:
                status = exc.status if isinstance(exc, CommandFailure) else "failed"
                error = str(exc) if isinstance(exc, CommandFailure) else "invalid command response"
                if claimed:
                    # Reporting has its own short budget after the task deadline expires.
                    for args in (["issue", "edit", str(number), "--add-label", "worker-failed",
                                  "--remove-label", self.config.in_progress_label],
                                 ["issue", "comment", str(number), "--body",
                                  f"Worker {status}; inspect the local task log."]):
                        report = self.github.run(args, root, 30)
                        if report.returncode:
                            log.write("\nFailure reporting failed.\n")
                return finish(status, error=error)

        return finish("succeeded", pr_url=pr_url)

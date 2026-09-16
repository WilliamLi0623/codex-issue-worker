from dataclasses import dataclass
import os
from typing import Mapping


@dataclass(frozen=True)
class Config:
    repo: str
    agent: str = "codex"
    task_label: str = "codex-task"
    in_progress_label: str = "in-progress"
    max_minutes: int = 120
    work_root: str = "/home/agent/data/tasks"
    poll_seconds: int = 60
    agent_tmux: bool = False


def load_config(env: Mapping[str, str] | None = None) -> Config:
    values = os.environ if env is None else env
    repo = values.get("GH_REPO", "").strip()
    if not repo or "/" not in repo:
        raise ValueError("GH_REPO must be an owner/name repository")
    agent = values.get("AGENT", "codex").strip().lower()
    if agent not in {"codex", "claude"}:
        raise ValueError("AGENT must be codex or claude")
    max_minutes = int(values.get("MAX_MINUTES", "120"))
    poll_seconds = int(values.get("POLL_SECONDS", "60"))
    if max_minutes <= 0 or poll_seconds <= 0:
        raise ValueError("MAX_MINUTES and POLL_SECONDS must be positive")
    agent_tmux = values.get("AGENT_TMUX", "0")
    if agent_tmux not in {"0", "1"}:
        raise ValueError("AGENT_TMUX must be 0 or 1")
    return Config(
        repo=repo,
        agent=agent,
        task_label=values.get("TASK_LABEL", "codex-task"),
        in_progress_label=values.get("IN_PROGRESS_LABEL", "in-progress"),
        max_minutes=max_minutes,
        work_root=values.get("WORK_ROOT", "/home/agent/data/tasks"),
        poll_seconds=poll_seconds,
        agent_tmux=agent_tmux == "1",
    )

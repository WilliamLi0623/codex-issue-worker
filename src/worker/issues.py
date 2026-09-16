from collections.abc import Mapping


def _label_names(issue: Mapping[str, object]) -> set[str]:
    labels = issue.get("labels", [])
    names: set[str] = set()
    for label in labels if isinstance(labels, list) else []:
        if isinstance(label, str):
            names.add(label)
        elif isinstance(label, Mapping) and isinstance(label.get("name"), str):
            names.add(label["name"])
    return names


def eligible_issue(issue: Mapping[str, object], task_label: str = "codex-task",
                   in_progress_label: str = "in-progress") -> bool:
    state = str(issue.get("state", "")).upper()
    labels = _label_names(issue)
    return state == "OPEN" and task_label in labels and in_progress_label not in labels


def branch_name(issue_number: int) -> str:
    if issue_number <= 0:
        raise ValueError("issue number must be positive")
    return f"worker/issue-{issue_number}"

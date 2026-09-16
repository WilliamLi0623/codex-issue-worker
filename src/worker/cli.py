"""Offline by default; --execute explicitly enables GitHub and agent operations."""

import argparse
import json
from pathlib import Path
import sys
import time

from .config import load_config
from .runner import CommandFailure, TaskRunner


def main(argv=None, *, env=None, process=None, github=None, stdout=None, sleep=time.sleep) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--execute", action="store_true", help="enable external operations")
    mode.add_argument("--dry-run", action="store_true", help="run one offline cycle (default)")
    parser.add_argument("--once", action="store_true", help="stop after one cycle")
    parser.add_argument("--issues-file", type=Path, help="offline Issue JSON list for dry-run")
    args = parser.parse_args(argv)
    output = sys.stdout if stdout is None else stdout
    try:
        config = load_config(env)
        issues = json.loads(args.issues_file.read_text(encoding="utf-8")) if args.issues_file else []
        if not isinstance(issues, list) or not all(isinstance(issue, dict) for issue in issues):
            raise ValueError("expected an Issue JSON list")
        runner = TaskRunner(config, process=process, github=github)
        while True:
            results = runner.run_cycle(dry_run=not args.execute, issues=issues)
            summary = {"dry_run": not args.execute,
                       "results": [{"status": result.status, "branch": result.branch}
                                   for result in results]}
            print(json.dumps(summary), file=output, flush=True)
            if not args.execute or args.once:
                return int(any(result.status in {"failed", "timed_out"} for result in results))
            sleep(config.poll_seconds)
    except (ValueError, OSError, RuntimeError, CommandFailure) as exc:
        error = str(exc) if isinstance(exc, (RuntimeError, CommandFailure)) else "invalid configuration or input"
        print(json.dumps({"error": error}), file=output)
        return 1
    except KeyboardInterrupt:
        return 130


if __name__ == "__main__":
    raise SystemExit(main())

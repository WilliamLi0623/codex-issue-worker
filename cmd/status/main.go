package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/WilliamLi0623/codex-issue-worker/internal/status"
)

const probeScript = `set -eu
service=codex-issue-worker.service
checkout=/home/agent/data/projects/codex-issue-worker
env_file=/home/agent/.config/codex-issue-worker/worker.env

env_value() {
    awk -F= -v key="$1" '$1 == key { sub(/^[^=]*=/, ""); print; exit }' "$env_file"
}

repo=$(env_value GH_REPO)
label=$(env_value TASK_LABEL)
work_root=$(env_value WORK_ROOT)
[ -n "$label" ] || label=codex-task
[ -n "$work_root" ] || work_root=/home/agent/data/tasks

printf '%s\n' SERVICE
systemctl --user show "$service" -p ActiveState --value
systemctl --user show "$service" -p SubState --value
systemctl --user show "$service" -p Result --value
systemctl --user show "$service" -p MainPID --value

printf '%s\n' DEPLOYMENT
git -C "$checkout" rev-parse --show-toplevel
git -C "$checkout" rev-parse HEAD

printf '%s\n' TASKS
if [ -d "$work_root" ]; then
    find "$work_root" -mindepth 1 -maxdepth 1 -type d -name 'issue-*' -printf '%f\n' | sort
fi

printf '%s\n%s\n' LABEL "$label"
printf '%s\n' QUEUE
gh issue list --repo "$repo" --state open --label "$label" --json number,title,url
`

func main() {
	host := flag.String("host", "", "SSH destination, for example agent@worker.example.com")
	flag.Parse()
	if strings.TrimSpace(*host) == "" {
		fmt.Fprintln(os.Stderr, "-host is required")
		os.Exit(2)
	}
	if err := run(context.Background(), *host); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, host string) error {
	cmd := exec.CommandContext(ctx, "ssh", host, "bash", "-s")
	cmd.Stdin = strings.NewReader(probeScript)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return fmt.Errorf("remote status probe failed: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return fmt.Errorf("remote status probe failed: %w", err)
	}
	snapshot, err := status.ParseProbe(string(output))
	if err != nil {
		return fmt.Errorf("parse remote status: %w", err)
	}
	fmt.Print(status.Format(snapshot))
	return nil
}

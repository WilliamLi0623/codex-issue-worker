package status

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"
)

type Service struct {
	ActiveState string
	SubState    string
	Result      string
	MainPID     string
}

type Deployment struct {
	Checkout string
	Commit   string
}

type QueueItem struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
}

type Snapshot struct {
	Service    Service
	Deployment Deployment
	Tasks      []string
	QueueLabel string
	Queue      []QueueItem
}

func ParseProbe(input string) (Snapshot, error) {
	sections := map[string][]string{}
	section := ""
	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "SERVICE" || line == "DEPLOYMENT" || line == "TASKS" || line == "LABEL" || line == "QUEUE" {
			section = line
			continue
		}
		if section == "" {
			if strings.TrimSpace(line) != "" {
				return Snapshot{}, fmt.Errorf("unexpected probe output %q", line)
			}
			continue
		}
		sections[section] = append(sections[section], line)
	}
	if err := scanner.Err(); err != nil {
		return Snapshot{}, fmt.Errorf("read probe output: %w", err)
	}

	service, err := fixedSection[Service](sections, "SERVICE", 4, func(lines []string) (Service, error) {
		return Service{ActiveState: lines[0], SubState: lines[1], Result: lines[2], MainPID: lines[3]}, nil
	})
	if err != nil {
		return Snapshot{}, err
	}
	deployment, err := fixedSection[Deployment](sections, "DEPLOYMENT", 2, func(lines []string) (Deployment, error) {
		return Deployment{Checkout: lines[0], Commit: lines[1]}, nil
	})
	if err != nil {
		return Snapshot{}, err
	}
	queueLines, ok := sections["QUEUE"]
	if !ok || len(queueLines) != 1 {
		return Snapshot{}, fmt.Errorf("QUEUE section must contain one JSON line")
	}
	var queue []QueueItem
	if err := json.Unmarshal([]byte(queueLines[0]), &queue); err != nil {
		return Snapshot{}, fmt.Errorf("parse queue: %w", err)
	}
	for _, item := range queue {
		if item.Number <= 0 {
			return Snapshot{}, fmt.Errorf("queue item has invalid number %d", item.Number)
		}
	}
	if _, ok := sections["TASKS"]; !ok {
		return Snapshot{}, fmt.Errorf("missing TASKS section")
	}
	label, ok := sections["LABEL"]
	if !ok || len(label) != 1 || strings.TrimSpace(label[0]) == "" {
		return Snapshot{}, fmt.Errorf("LABEL section must contain one non-empty line")
	}
	return Snapshot{Service: service, Deployment: deployment, Tasks: sections["TASKS"], QueueLabel: label[0], Queue: queue}, nil
}

func fixedSection[T any](sections map[string][]string, name string, count int, parse func([]string) (T, error)) (T, error) {
	lines, ok := sections[name]
	if !ok || len(lines) != count {
		var zero T
		return zero, fmt.Errorf("%s section must contain %d lines", name, count)
	}
	return parse(lines)
}

func Format(snapshot Snapshot) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Service: %s (%s), result=%s, pid=%s\n", snapshot.Service.ActiveState, snapshot.Service.SubState, snapshot.Service.Result, snapshot.Service.MainPID)
	fmt.Fprintf(&b, "Checkout: %s\nCommit: %s\n", snapshot.Deployment.Checkout, snapshot.Deployment.Commit)
	b.WriteString("Active task directories:\n")
	for _, task := range snapshot.Tasks {
		fmt.Fprintf(&b, "  %s\n", task)
	}
	label := snapshot.QueueLabel
	if label == "" {
		label = "codex-task"
	}
	fmt.Fprintf(&b, "Queue (%s):\n", label)
	for _, item := range snapshot.Queue {
		fmt.Fprintf(&b, "  #%d %s (%s)\n", item.Number, item.Title, item.URL)
	}
	return b.String()
}

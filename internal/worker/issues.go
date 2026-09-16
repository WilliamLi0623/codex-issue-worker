package worker

import (
	"encoding/json"

	"github.com/WilliamLi0623/codex-issue-worker/internal/issue"
)

func ParseIssues(data string) ([]issue.Issue, error) {
	var issues []issue.Issue
	err := json.Unmarshal([]byte(data), &issues)
	return issues, err
}

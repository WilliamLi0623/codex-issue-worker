package issue

import (
	"fmt"
	"strings"
)

type Issue struct {
	Number            int     `json:"number"`
	Title             string  `json:"title"`
	Body              string  `json:"body"`
	URL               string  `json:"url"`
	State             string  `json:"state"`
	AuthorAssociation string  `json:"authorAssociation"`
	Labels            []Label `json:"labels"`
}
type Label struct {
	Name string `json:"name"`
}

func Eligible(i Issue, taskLabel, inProgressLabel string) bool {
	if !Candidate(i, taskLabel, inProgressLabel) {
		return false
	}
	switch strings.ToUpper(i.AuthorAssociation) {
	case "OWNER", "MEMBER", "COLLABORATOR":
		return true
	default:
		return false
	}
}

func Candidate(i Issue, taskLabel, inProgressLabel string) bool {
	if strings.ToUpper(i.State) != "OPEN" {
		return false
	}
	hasTask, claimed := false, false
	for _, l := range i.Labels {
		if l.Name == taskLabel {
			hasTask = true
		}
		if l.Name == inProgressLabel {
			claimed = true
		}
	}
	return hasTask && !claimed
}
func BranchName(n int) string { return fmt.Sprintf("worker/issue-%d", n) }
func ValidateNumber(n int) error {
	if n <= 0 {
		return fmt.Errorf("issue number must be positive")
	}
	return nil
}

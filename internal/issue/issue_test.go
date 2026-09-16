package issue

import "testing"

func TestEligibilityAndBranch(t *testing.T) {
	open := Issue{Number: 42, State: "OPEN", Labels: []Label{{Name: "codex-task"}}}
	if !Eligible(open, "codex-task", "in-progress") {
		t.Fatal("expected eligible")
	}
	open.Labels = append(open.Labels, Label{Name: "in-progress"})
	if Eligible(open, "codex-task", "in-progress") {
		t.Fatal("claimed issue must be ineligible")
	}
	if got := BranchName(42); got != "worker/issue-42" {
		t.Fatalf("branch=%q", got)
	}
	if err := ValidateNumber(0); err == nil {
		t.Fatal("zero issue number accepted")
	}
}

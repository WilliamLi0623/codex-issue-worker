package issue

import "testing"

func TestEligibilityAndBranch(t *testing.T) {
	open := Issue{Number: 42, State: "OPEN", AuthorAssociation: "OWNER", Labels: []Label{{Name: "codex-task"}}}
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

func TestEligibleRequiresTrustedAuthorAssociation(t *testing.T) {
	trusted := []string{"OWNER", "MEMBER", "COLLABORATOR"}
	for _, association := range trusted {
		i := Issue{Number: 42, State: "OPEN", AuthorAssociation: association, Labels: []Label{{Name: "codex-task"}}}
		if !Eligible(i, "codex-task", "in-progress") {
			t.Errorf("association %q should be eligible", association)
		}
	}

	untrusted := []string{"", "CONTRIBUTOR", "FIRST_TIMER", "FIRST_TIME_CONTRIBUTOR", "NONE"}
	for _, association := range untrusted {
		i := Issue{Number: 42, State: "OPEN", AuthorAssociation: association, Labels: []Label{{Name: "codex-task"}}}
		if Eligible(i, "codex-task", "in-progress") {
			t.Errorf("association %q must not be eligible", association)
		}
	}
}

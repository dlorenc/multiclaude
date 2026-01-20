package pr

import (
	"testing"
)

func TestDecideRecovery_SupersededPR(t *testing.T) {
	analyzer := &Analyzer{}

	pr := &ClosedPR{
		Number:       1,
		Title:        "Test PR",
		HeadRefName:  "feature/test",
		Additions:    100,
		Deletions:    50,
		IsSuperseded: true,
	}

	decision := analyzer.DecideRecovery(pr)

	if decision.ShouldRecover {
		t.Errorf("Expected superseded PR to not be recovered")
	}
	if decision.Reason != "Superseded by a merged PR with similar work" {
		t.Errorf("Unexpected reason: %s", decision.Reason)
	}
}

func TestDecideRecovery_TrivialChanges(t *testing.T) {
	analyzer := &Analyzer{}

	pr := &ClosedPR{
		Number:            2,
		Title:             "Small fix",
		HeadRefName:       "fix/small",
		Additions:         5,
		Deletions:         3,
		HasReviewComments: false,
	}

	decision := analyzer.DecideRecovery(pr)

	if decision.ShouldRecover {
		t.Errorf("Expected trivial changes without review to not be recovered")
	}
}

func TestDecideRecovery_TrivialWithReview(t *testing.T) {
	analyzer := &Analyzer{}

	pr := &ClosedPR{
		Number:            3,
		Title:             "Small fix with review",
		HeadRefName:       "fix/reviewed",
		Additions:         10,
		Deletions:         5,
		HasReviewComments: true,
	}

	decision := analyzer.DecideRecovery(pr)

	if !decision.ShouldRecover {
		t.Errorf("Expected trivial changes with review comments to be recovered")
	}
}

func TestDecideRecovery_SubstantialChanges(t *testing.T) {
	analyzer := &Analyzer{}

	pr := &ClosedPR{
		Number:        4,
		Title:         "Big feature",
		HeadRefName:   "feature/big",
		Additions:     200,
		Deletions:     50,
		HasCIFailures: true,
	}

	decision := analyzer.DecideRecovery(pr)

	if !decision.ShouldRecover {
		t.Errorf("Expected substantial changes to be recovered even with CI failures")
	}
}

func TestDecideRecovery_MediumCIFailing(t *testing.T) {
	analyzer := &Analyzer{}

	pr := &ClosedPR{
		Number:            5,
		Title:             "Medium change",
		HeadRefName:       "feature/medium",
		Additions:         30,
		Deletions:         10,
		HasCIFailures:     true,
		HasReviewComments: false,
	}

	decision := analyzer.DecideRecovery(pr)

	if decision.ShouldRecover {
		t.Errorf("Expected medium changes with CI failures and no review to not be recovered")
	}
}

func TestClosedPRStruct(t *testing.T) {
	pr := ClosedPR{
		Number:      42,
		Title:       "Test PR",
		HeadRefName: "feature/test",
		Author:      "test-user",
		Additions:   100,
		Deletions:   50,
	}

	if pr.Number != 42 {
		t.Errorf("Expected Number to be 42, got %d", pr.Number)
	}
	if pr.Title != "Test PR" {
		t.Errorf("Expected Title to be 'Test PR', got %s", pr.Title)
	}
}

func TestRecoveryDecisionStruct(t *testing.T) {
	decision := RecoveryDecision{
		ShouldRecover: true,
		Reason:        "Test reason",
		PRNumber:      123,
		Title:         "Test Title",
		Branch:        "test-branch",
	}

	if !decision.ShouldRecover {
		t.Errorf("Expected ShouldRecover to be true")
	}
	if decision.PRNumber != 123 {
		t.Errorf("Expected PRNumber to be 123, got %d", decision.PRNumber)
	}
}

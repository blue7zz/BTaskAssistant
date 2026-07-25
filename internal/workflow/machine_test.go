package workflow

import "testing"

func TestValidateTransitionBlocksSkippedStage(t *testing.T) {
	err := ValidateTransition(TransitionRequest{From: StatusInbox, To: StatusApproved})
	if err == nil {
		t.Fatal("expected skipped transition to be blocked")
	}
}

func TestValidateTransitionRequiresRequirementConfirmation(t *testing.T) {
	err := ValidateTransition(TransitionRequest{
		From: StatusRequirements,
		To:   StatusApproved,
	})
	if err == nil {
		t.Fatal("expected unconfirmed requirements to be blocked")
	}
}

func TestValidateTransitionAllowsConfirmedRequirements(t *testing.T) {
	err := ValidateTransition(TransitionRequest{
		From:                  StatusRequirements,
		To:                    StatusApproved,
		RequirementsConfirmed: true,
	})
	if err != nil {
		t.Fatalf("expected confirmed requirements to pass, got %v", err)
	}
}

func TestValidateTransitionAllowsSingleBackwardStep(t *testing.T) {
	err := ValidateTransition(TransitionRequest{
		From: StatusReview,
		To:   StatusDevelopment,
	})
	if err != nil {
		t.Fatalf("expected manual rollback to pass, got %v", err)
	}
}


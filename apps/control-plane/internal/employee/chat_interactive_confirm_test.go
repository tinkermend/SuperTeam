package employee

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestChatInteractiveConfirmRequired(t *testing.T) {
	if !chatInteractiveConfirmRequired(1, "") {
		t.Fatal("skill selection should require confirm")
	}
	if !chatInteractiveConfirmRequired(0, "pause_at_gate") {
		t.Fatal("project pause ceiling should require confirm")
	}
	if chatInteractiveConfirmRequired(0, "") {
		t.Fatal("empty selection + no ceiling should not require confirm")
	}
	if chatInteractiveConfirmRequired(0, "full_auto") {
		t.Fatal("full_auto ceiling alone should not require confirm")
	}
}

func TestEnforceInteractiveLightConfirm(t *testing.T) {
	skill := uuid.New()
	err := enforceInteractiveLightConfirm(CreateDigitalEmployeeRunRequest{
		SkillIDs: []uuid.UUID{skill},
	}, "")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}

	err = enforceInteractiveLightConfirm(CreateDigitalEmployeeRunRequest{
		SkillIDs:             []uuid.UUID{skill},
		InteractiveConfirmed: true,
	}, "")
	if err != nil {
		t.Fatalf("confirmed: %v", err)
	}

	err = enforceInteractiveLightConfirm(CreateDigitalEmployeeRunRequest{
		SkillIDs: []uuid.UUID{skill},
		Metadata: map[string]any{"automation_rule_id": uuid.New().String()},
	}, "")
	if err != nil {
		t.Fatalf("automation bypass: %v", err)
	}
}

package inbox

import "testing"

// TestDecisionActionRegistryHandlerCoverage is the §4.4 CI assertion: every inbox
// action the platform can emit for a project decision must name a handler that is
// actually implemented. This is the structural guard against F1 (a button with no
// server handler behind it).
func TestDecisionActionRegistryHandlerCoverage(t *testing.T) {
	check := func(kind string, entries []registeredDecisionAction) {
		if len(entries) == 0 {
			t.Fatalf("decision kind %q declares no actions", kind)
		}
		seen := map[string]struct{}{}
		for _, entry := range entries {
			if entry.action.Key == "" || entry.action.Label == "" {
				t.Fatalf("decision kind %q has an action with empty key/label: %#v", kind, entry.action)
			}
			if _, dup := seen[entry.action.Key]; dup {
				t.Fatalf("decision kind %q has duplicate action key %q", kind, entry.action.Key)
			}
			seen[entry.action.Key] = struct{}{}
			if entry.handler == "" {
				t.Fatalf("decision kind %q action %q declares no handler", kind, entry.action.Key)
			}
			if _, ok := implementedDecisionHandlers[entry.handler]; !ok {
				t.Fatalf("decision kind %q action %q references unimplemented handler %q; wire it and add it to implementedDecisionHandlers", kind, entry.action.Key, entry.handler)
			}
		}
	}
	for kind, entries := range decisionActionRegistry {
		check(kind, entries)
	}
	check("<generic>", genericDecisionActions)
}

// TestDecisionActionsAreRegistryDriven asserts DecisionActions never emits an
// action outside the registry — the "能点⟺已登记" invariant. Special kinds emit
// exactly their registry entry; any other kind emits exactly the generic set.
func TestDecisionActionsAreRegistryDriven(t *testing.T) {
	for kind, entries := range decisionActionRegistry {
		got := DecisionActions(kind)
		if len(got) != len(entries) {
			t.Fatalf("DecisionActions(%q) returned %d actions, registry declares %d", kind, len(got), len(entries))
		}
		for i, entry := range entries {
			if got[i].Key != entry.action.Key || got[i].Label != entry.action.Label || got[i].Tone != entry.action.Tone || got[i].RequiresComment != entry.action.RequiresComment {
				t.Fatalf("DecisionActions(%q)[%d]=%#v does not match registry %#v", kind, i, got[i], entry.action)
			}
		}
	}
	// Unregistered kinds fall back to the generic set.
	generic := DecisionActions("plan_review")
	if len(generic) != len(genericDecisionActions) {
		t.Fatalf("generic DecisionActions returned %d actions, expected %d", len(generic), len(genericDecisionActions))
	}
	for i, entry := range genericDecisionActions {
		if generic[i].Key != entry.action.Key {
			t.Fatalf("generic DecisionActions[%d].Key=%q, expected %q", i, generic[i].Key, entry.action.Key)
		}
	}
}

// TestGenericDecisionActionsMatchDefaultActions pins the registry's generic set to
// DefaultActions(ItemTypeProjectDecision) so the two vocabularies can't drift.
func TestGenericDecisionActionsMatchDefaultActions(t *testing.T) {
	defaults := DefaultActions(ItemTypeProjectDecision)
	if len(genericDecisionActions) != len(defaults) {
		t.Fatalf("genericDecisionActions has %d entries, DefaultActions has %d", len(genericDecisionActions), len(defaults))
	}
	for i, def := range defaults {
		got := genericDecisionActions[i].action
		if got.Key != def.Key || got.Label != def.Label || got.Tone != def.Tone || got.RequiresComment != def.RequiresComment {
			t.Fatalf("genericDecisionActions[%d]=%#v does not match DefaultActions %#v", i, got, def)
		}
	}
}

func TestTaskHumanWaitFamilyOmitsNeedsMoreEvidence(t *testing.T) {
	// Wait-family cards are released only by approved/rejected. Emitting
	// needs_more_evidence would settle the inbox while the coordinator no-ops
	// and the task stays waiting_human (sister-F1 stranding path).
	kinds := []string{
		"project_task_clarification",
		"project_task_recovery",
		"project_task_runtime_recovery",
		"project_task_missing_context",
		"project_task_permission",
		"project_task_plan_invalid",
		"project_task_budget_approval",
		"project_task_human_wait",
	}
	for _, kind := range kinds {
		got := DecisionActions(kind)
		if len(got) != 2 {
			t.Fatalf("DecisionActions(%q) returned %d actions, want 2 (approved/rejected)", kind, len(got))
		}
		if got[0].Key != "approved" || got[1].Key != "rejected" {
			t.Fatalf("DecisionActions(%q)=%#v, want approved then rejected", kind, got)
		}
		for _, action := range got {
			if action.Key == "needs_more_evidence" {
				t.Fatalf("DecisionActions(%q) must not emit needs_more_evidence", kind)
			}
		}
		// Registry entries must point at the wait-release handler, not generic.
		entries := registeredActionsForDecision(kind)
		for _, entry := range entries {
			if entry.handler != handlerTaskHumanWaitRelease {
				t.Fatalf("%s action %q handler=%q, want %q", kind, entry.action.Key, entry.handler, handlerTaskHumanWaitRelease)
			}
		}
	}
}

func TestProjectTaskApprovalKeepsGenericEvidenceAction(t *testing.T) {
	// Gate risk approvals still use the generic vocabulary; do not fold
	// project_task_approval into the wait-family closed set.
	got := DecisionActions("project_task_approval")
	if len(got) != 3 || got[2].Key != "needs_more_evidence" {
		t.Fatalf("project_task_approval should keep generic 3-action set, got %#v", got)
	}
}

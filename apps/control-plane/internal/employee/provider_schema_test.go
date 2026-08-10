package employee

import "testing"

func TestAnnotateProviderSchemaViolations_tagsBrokenEnvelope(t *testing.T) {
	before := ProviderSchemaViolationCount()
	event := RuntimeCommandEventWriteback{
		EventType: "turn_error",
		Payload: map[string]any{
			"error": map[string]any{
				"schema_version": "provider.error.v1",
				"code":           "RATE_LIMIT",
				// missing family/message/retryable/provider_type
			},
		},
		Metadata: map[string]any{
			"schema_version": "provider.event.v1",
			"provider_type":  "claude-code",
		},
	}
	annotateProviderSchemaViolations(&event)
	sv, ok := event.Metadata["schema_violation"].(map[string]any)
	if !ok {
		t.Fatalf("expected schema_violation tag, got %#v", event.Metadata)
	}
	reasons, _ := sv["reasons"].([]string)
	if len(reasons) == 0 {
		// reasons may be []any after map insert — accept either
		if raw, ok := sv["reasons"].([]any); !ok || len(raw) == 0 {
			t.Fatalf("expected non-empty reasons, got %#v", sv["reasons"])
		}
	}
	if ProviderSchemaViolationCount() != before+1 {
		t.Fatalf("expected violation counter +1")
	}
}

func TestAnnotateProviderSchemaViolations_okEnvelopeNoTag(t *testing.T) {
	before := ProviderSchemaViolationCount()
	event := RuntimeCommandEventWriteback{
		EventType: "turn_error",
		Payload: map[string]any{
			"error": map[string]any{
				"schema_version": "provider.error.v1",
				"code":           "RATE_LIMIT",
				"family":         "transient_provider",
				"retryable":      true,
				"message":        "rate limit",
				"provider_type":  "claude-code",
			},
		},
		Metadata: map[string]any{
			"schema_version": "provider.event.v1",
			"provider_type":  "claude-code",
		},
	}
	annotateProviderSchemaViolations(&event)
	if _, ok := event.Metadata["schema_violation"]; ok {
		t.Fatalf("did not expect schema_violation, got %#v", event.Metadata["schema_violation"])
	}
	if ProviderSchemaViolationCount() != before {
		t.Fatalf("counter should not increase for valid envelope")
	}
}

func TestAnnotateProviderSchemaViolations_legacyWithoutSchemaNoTag(t *testing.T) {
	event := RuntimeCommandEventWriteback{
		EventType: "text_delta",
		Payload:   map[string]any{"text": "hi"},
	}
	annotateProviderSchemaViolations(&event)
	if event.Metadata != nil {
		if _, ok := event.Metadata["schema_violation"]; ok {
			t.Fatalf("legacy events must not be tagged")
		}
	}
}

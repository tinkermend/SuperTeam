package employee

import (
	"fmt"
	"log"
	"strings"
	"sync/atomic"
)

// S2 ingest tagging (provider semantic unification §4.5.1): structural checks
// against contracts/provider semantics. Failures never reject writeback; they
// annotate metadata.schema_violation and increment a process counter for ops.

var providerSchemaViolationCount atomic.Uint64

// ProviderSchemaViolationCount returns how many ingest events were tagged since process start.
func ProviderSchemaViolationCount() uint64 {
	return providerSchemaViolationCount.Load()
}

// annotateProviderSchemaViolations mutates event.Metadata when payload/metadata
// shapes look like provider contracts but fail required-field checks.
func annotateProviderSchemaViolations(event *RuntimeCommandEventWriteback) {
	if event == nil {
		return
	}
	var reasons []string
	if event.Payload != nil {
		reasons = append(reasons, validateProviderErrorEnvelope(event.Payload["error"])...)
		if event.EventType == "turn_completed" {
			reasons = append(reasons, validateTurnCompletedUsage(event.Payload["usage"])...)
		}
	}
	if event.Metadata != nil {
		if sv, ok := event.Metadata["schema_version"].(string); ok {
			sv = strings.TrimSpace(sv)
			if sv != "" && !strings.HasPrefix(sv, "provider.event.") && !strings.HasPrefix(sv, "provider.") {
				reasons = append(reasons, "metadata.schema_version_unknown:"+sv)
			}
		}
		if pt, ok := event.Metadata["provider_type"].(string); ok {
			pt = strings.TrimSpace(pt)
			if pt != "" && !isKnownProviderType(pt) {
				reasons = append(reasons, "metadata.provider_type_unknown:"+pt)
			}
		}
	}
	if len(reasons) == 0 {
		return
	}
	providerSchemaViolationCount.Add(1)
	if event.Metadata == nil {
		event.Metadata = map[string]any{}
	}
	event.Metadata["schema_violation"] = map[string]any{
		"reasons": reasons,
		"policy":  "s2_tag_only",
	}
	log.Printf(
		"provider schema_violation (s2 tag-only) event_type=%s reasons=%v total=%d",
		event.EventType,
		reasons,
		providerSchemaViolationCount.Load(),
	)
}

func isKnownProviderType(pt string) bool {
	switch pt {
	case "claude-code", "opencode", "codex":
		return true
	default:
		return false
	}
}

func validateProviderErrorEnvelope(raw any) []string {
	if raw == nil {
		return nil
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return []string{"error_not_object"}
	}
	sv, _ := obj["schema_version"].(string)
	sv = strings.TrimSpace(sv)
	// Only enforce when the payload claims the provider error schema.
	if sv != "" && sv != "provider.error.v1" {
		return []string{"error.schema_version_unsupported:" + sv}
	}
	if sv == "" {
		// Legacy fail events without envelope schema: no S2 tag.
		return nil
	}
	var reasons []string
	for _, key := range []string{"code", "family", "message", "provider_type"} {
		if strings.TrimSpace(fmt.Sprint(obj[key])) == "" || obj[key] == nil {
			reasons = append(reasons, "error.missing_"+key)
		}
	}
	if _, ok := obj["retryable"].(bool); !ok {
		reasons = append(reasons, "error.missing_retryable")
	}
	if pt, ok := obj["provider_type"].(string); ok && strings.TrimSpace(pt) != "" && !isKnownProviderType(strings.TrimSpace(pt)) {
		reasons = append(reasons, "error.provider_type_unknown:"+pt)
	}
	return reasons
}

func validateTurnCompletedUsage(raw any) []string {
	if raw == nil {
		return nil
	}
	if _, ok := raw.(map[string]any); !ok {
		return []string{"usage_not_object"}
	}
	return nil
}

package employee

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// S2 ingest tagging (provider semantic unification §4.5.1):
// validate provider ErrorEnvelope (and related shapes) with JSON Schema when
// available; fall back to structural checks. Failures never reject writeback —
// they annotate metadata.schema_violation and increment process counters.

var (
	providerSchemaViolationCount atomic.Uint64
	providerSchemaEngineOnce     sync.Once
	providerErrorSchema          *jsonschema.Schema
	providerSchemaEngineMode     = "structural" // or "jsonschema"
)

// ProviderSchemaViolationCount returns how many ingest events were tagged since process start.
func ProviderSchemaViolationCount() uint64 {
	return providerSchemaViolationCount.Load()
}

// ProviderSchemaEngineMode reports "jsonschema" when schemas loaded, else "structural".
func ProviderSchemaEngineMode() string {
	ensureProviderSchemaEngine()
	return providerSchemaEngineMode
}

func ensureProviderSchemaEngine() {
	providerSchemaEngineOnce.Do(func() {
		path := resolveProviderErrorSchemaPath()
		if path == "" {
			log.Printf("provider schema engine: structural fallback (provider-error.schema.json not found)")
			return
		}
		compiler := jsonschema.NewCompiler()
		f, err := os.Open(path)
		if err != nil {
			log.Printf("provider schema engine: open %s: %v (structural fallback)", path, err)
			return
		}
		defer f.Close()
		doc, err := jsonschema.UnmarshalJSON(f)
		if err != nil {
			log.Printf("provider schema engine: parse %s: %v (structural fallback)", path, err)
			return
		}
		const schemaURL = "https://superteam.local/contracts/provider/schemas/provider-error.schema.json"
		if err := compiler.AddResource(schemaURL, doc); err != nil {
			log.Printf("provider schema engine: AddResource: %v (structural fallback)", err)
			return
		}
		schema, err := compiler.Compile(schemaURL)
		if err != nil {
			log.Printf("provider schema engine: compile: %v (structural fallback)", err)
			return
		}
		providerErrorSchema = schema
		providerSchemaEngineMode = "jsonschema"
		log.Printf("provider schema engine: jsonschema loaded from %s", path)
	})
}

func resolveProviderErrorSchemaPath() string {
	candidates := []string{}
	if dir := strings.TrimSpace(os.Getenv("SUPERTEAM_PROVIDER_SCHEMAS_DIR")); dir != "" {
		candidates = append(candidates, filepath.Join(dir, "provider-error.schema.json"))
	}
	if root := strings.TrimSpace(os.Getenv("SUPERTEAM_REPO_ROOT")); root != "" {
		candidates = append(candidates, filepath.Join(root, "contracts", "provider", "schemas", "provider-error.schema.json"))
	}
	// cwd-relative (dev-services / monorepo root)
	candidates = append(candidates,
		filepath.Join("contracts", "provider", "schemas", "provider-error.schema.json"),
		filepath.Join("..", "..", "contracts", "provider", "schemas", "provider-error.schema.json"),
	)
	// relative to this source file (tests)
	if _, thisFile, _, ok := runtime.Caller(0); ok {
		root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", ".."))
		candidates = append(candidates, filepath.Join(root, "contracts", "provider", "schemas", "provider-error.schema.json"))
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}
	return ""
}

// annotateProviderSchemaViolations mutates event.Metadata when payload/metadata
// shapes look like provider contracts but fail schema/structural checks.
func annotateProviderSchemaViolations(event *RuntimeCommandEventWriteback) {
	if event == nil {
		return
	}
	ensureProviderSchemaEngine()
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
		"engine":  providerSchemaEngineMode,
	}
	log.Printf(
		"provider schema_violation (s2 tag-only) event_type=%s engine=%s reasons=%v total=%d",
		event.EventType,
		providerSchemaEngineMode,
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

	if providerErrorSchema != nil {
		// jsonschema v6 Validate expects unmarshaled JSON values.
		if err := providerErrorSchema.Validate(obj); err != nil {
			return []string{"error.jsonschema:" + truncateReason(err.Error())}
		}
		return nil
	}

	// Structural fallback when schema file is unavailable.
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

func truncateReason(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 240 {
		return s[:240] + "…"
	}
	return s
}

// ProviderContractMetrics is exposed on /health for ops dashboards.
type ProviderContractMetrics struct {
	SchemaViolationCount uint64 `json:"schema_violation_count"`
	SchemaEngine         string `json:"schema_engine"`
}

// SnapshotProviderContractMetrics returns process-local S2 counters.
func SnapshotProviderContractMetrics() ProviderContractMetrics {
	ensureProviderSchemaEngine()
	return ProviderContractMetrics{
		SchemaViolationCount: providerSchemaViolationCount.Load(),
		SchemaEngine:         providerSchemaEngineMode,
	}
}

// resetProviderSchemaEngineForTest clears the once-loader (tests only).
func resetProviderSchemaEngineForTest() {
	// Cannot fully reset sync.Once; tests should not rely on reloading mid-process.
	_ = json.Marshal
}

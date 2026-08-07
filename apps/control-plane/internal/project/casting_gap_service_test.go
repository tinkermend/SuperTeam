package project

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// stubDiscoverer returns a fixed suggestion for service-layer gate tests.
type stubDiscoverer struct {
	suggestion CastingGapSuggestion
	err        error
	calls      int
}

func (s *stubDiscoverer) DiscoverCastingGap(ctx context.Context, in CastingGapInput) (CastingGapSuggestion, error) {
	s.calls++
	if s.err != nil {
		return CastingGapSuggestion{}, s.err
	}
	return s.suggestion, nil
}

type stubRoleLister struct {
	rows []RoleVocabularyRow
}

func (s stubRoleLister) ListActiveRoleRows(ctx context.Context, tenantID uuid.UUID) ([]RoleVocabularyRow, error) {
	return s.rows, nil
}

func TestCastingGapDiscoverySummary(t *testing.T) {
	t.Parallel()
	if got := castingGapDiscoverySummary("needed", CastingGapSuggestion{RoleKey: "operator"}, "运维（operator）"); !strings.Contains(got, "运维（operator）") {
		t.Fatalf("needed: %q", got)
	}
	if got := castingGapDiscoverySummary("external", CastingGapSuggestion{}, ""); !strings.Contains(got, "词表外") {
		t.Fatalf("external: %q", got)
	}
	if got := castingGapDiscoverySummary("not_needed", CastingGapSuggestion{}, ""); !strings.Contains(got, "无需") {
		t.Fatalf("not_needed: %q", got)
	}
}

func TestCastingGapDiscoveryMaxPerDemandDefaults(t *testing.T) {
	t.Parallel()
	s := &Service{}
	if got := s.castingGapDiscoveryMaxPerDemand(context.Background(), uuid.New()); got != 3 {
		t.Fatalf("default max=%d want 3", got)
	}
}

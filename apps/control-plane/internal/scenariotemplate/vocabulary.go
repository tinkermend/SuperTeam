package scenariotemplate

import (
	"context"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// VocabularyEntry is a tenant-scoped capability key row: the shared vocabulary
// that scenario template role requirements (required_capabilities) and
// digital employee capability declarations both draw from. Scenario
// differences are registry inserts, never code enums.
type VocabularyEntry struct {
	Key         string
	Title       string
	Description string
	Status      string
}

// VocabularyRepository reads the tenant capability vocabulary registry.
type VocabularyRepository interface {
	ListVocabulary(ctx context.Context, tenantID uuid.UUID) ([]VocabularyEntry, error)
	ActiveKeys(ctx context.Context, tenantID uuid.UUID, keys []string) (map[string]bool, error)
}

// SetVocabularyRepository injects the capability vocabulary reader. Left
// unset, ValidateCapabilityKeys is a pass-through — same nil-optional
// convention as the package's other resolver/source setters.
func (s *Service) SetVocabularyRepository(repository VocabularyRepository) {
	s.vocabularyRepository = repository
}

// ValidateCapabilityKeys returns the subset of keys that are not registered
// as active in the tenant's capability vocabulary. Keys are trimmed and
// deduped before lookup; blank entries are skipped. All keys known returns
// nil. With no vocabulary repository injected, it passes through and
// returns (nil, nil).
func (s *Service) ValidateCapabilityKeys(ctx context.Context, tenantID uuid.UUID, keys []string) ([]string, error) {
	if s.vocabularyRepository == nil {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(keys))
	unique := make([]string, 0, len(keys))
	for _, raw := range keys {
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, key)
	}
	if len(unique) == 0 {
		return nil, nil
	}

	active, err := s.vocabularyRepository.ActiveKeys(ctx, tenantID, unique)
	if err != nil {
		return nil, err
	}

	var unknown []string
	for _, key := range unique {
		if !active[key] {
			unknown = append(unknown, key)
		}
	}
	sort.Strings(unknown)
	return unknown, nil
}

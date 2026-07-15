package scenariotemplate

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

type fakeVocabularyRepository struct {
	active map[string]bool
}

func (f *fakeVocabularyRepository) ListVocabulary(ctx context.Context, tenantID uuid.UUID) ([]VocabularyEntry, error) {
	entries := make([]VocabularyEntry, 0, len(f.active))
	for key, ok := range f.active {
		if !ok {
			continue
		}
		entries = append(entries, VocabularyEntry{Key: key, Title: key, Description: "", Status: "active"})
	}
	return entries, nil
}

func (f *fakeVocabularyRepository) ActiveKeys(ctx context.Context, tenantID uuid.UUID, keys []string) (map[string]bool, error) {
	result := make(map[string]bool, len(keys))
	for _, key := range keys {
		if f.active[key] {
			result[key] = true
		}
	}
	return result, nil
}

func TestValidateCapabilityKeysReportsUnknown(t *testing.T) {
	repo := &fakeVocabularyRepository{active: map[string]bool{"code_review": true}}
	svc := NewService(nil)
	svc.SetVocabularyRepository(repo)

	unknown, err := svc.ValidateCapabilityKeys(context.Background(), uuid.New(), []string{"code_review", "ghost"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(unknown) != 1 || unknown[0] != "ghost" {
		t.Fatalf("expected [ghost], got %v", unknown)
	}
}

func TestValidateCapabilityKeysNilRepoPasses(t *testing.T) {
	svc := NewService(nil)

	unknown, err := svc.ValidateCapabilityKeys(context.Background(), uuid.New(), []string{"anything"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if unknown != nil {
		t.Fatalf("expected nil, got %v", unknown)
	}
}

func TestValidateCapabilityKeysTrimsAndDedupes(t *testing.T) {
	repo := &fakeVocabularyRepository{active: map[string]bool{"code_review": true}}
	svc := NewService(nil)
	svc.SetVocabularyRepository(repo)

	unknown, err := svc.ValidateCapabilityKeys(context.Background(), uuid.New(), []string{" code_review ", "code_review"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if unknown != nil {
		t.Fatalf("expected nil, got %v", unknown)
	}
}

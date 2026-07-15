package scenariotemplate

import (
	"context"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type PgVocabularyRepository struct {
	q *queries.Queries
}

func NewPgVocabularyRepository(q *queries.Queries) VocabularyRepository {
	return &PgVocabularyRepository{q: q}
}

func (r *PgVocabularyRepository) ListVocabulary(ctx context.Context, tenantID uuid.UUID) ([]VocabularyEntry, error) {
	rows, err := r.q.ListCapabilityVocabulary(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	entries := make([]VocabularyEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, VocabularyEntry{
			Key:         row.VocabKey,
			Title:       row.Title,
			Description: row.Description,
			Status:      row.Status,
		})
	}
	return entries, nil
}

func (r *PgVocabularyRepository) ActiveKeys(ctx context.Context, tenantID uuid.UUID, keys []string) (map[string]bool, error) {
	rows, err := r.q.GetCapabilityVocabularyByKeys(ctx, queries.GetCapabilityVocabularyByKeysParams{
		TenantID:  tenantID,
		VocabKeys: keys,
	})
	if err != nil {
		return nil, err
	}
	active := make(map[string]bool, len(rows))
	for _, row := range rows {
		active[row.VocabKey] = true
	}
	return active, nil
}

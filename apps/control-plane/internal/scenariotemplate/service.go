package scenariotemplate

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) List(ctx context.Context, tenantID uuid.UUID) ([]ScenarioTemplate, error) {
	return s.repository.ListScenarioTemplates(ctx, tenantID)
}

func (s *Service) GetByKey(ctx context.Context, tenantID uuid.UUID, key string) (ScenarioTemplate, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return ScenarioTemplate{}, ErrScenarioTemplateNotFound
	}
	return s.repository.GetScenarioTemplateByKey(ctx, tenantID, key)
}

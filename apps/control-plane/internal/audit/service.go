package audit

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

type Event struct {
	ID           uuid.UUID `db:"id"`
	EventType    string    `db:"event_type"`
	ActorType    string    `db:"actor_type"`
	ActorID      string    `db:"actor_id"`
	ResourceType string    `db:"resource_type"`
	ResourceID   string    `db:"resource_id"`
	Action       string    `db:"action"`
	Details      string    `db:"details"`
	IPAddress    string    `db:"ip_address"`
	CreatedAt    time.Time `db:"created_at"`
}

type Repository interface {
	CreateEvent(ctx context.Context, event *Event) error
	ListEvents(ctx context.Context, limit, offset int) ([]*Event, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, errors.New("audit repository is required")
	}
	return &Service{repository: repository}, nil
}

func (s *Service) LogEvent(ctx context.Context, eventType, actorType, actorID, resourceType, resourceID, action string) error {
	event := &Event{
		EventType:    eventType,
		ActorType:    actorType,
		ActorID:      actorID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Action:       action,
		CreatedAt:    time.Now(),
	}
	return s.repository.CreateEvent(ctx, event)
}

func (s *Service) ListEvents(ctx context.Context, limit, offset int) ([]*Event, error) {
	return s.repository.ListEvents(ctx, limit, offset)
}

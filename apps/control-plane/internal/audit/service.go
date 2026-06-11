package audit

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type Event struct {
	ID           uuid.UUID      `db:"id"`
	TenantID     uuid.UUID      `db:"tenant_id"`
	EventType    string         `db:"event_type"`
	ActorType    string         `db:"actor_type"`
	ActorID      string         `db:"actor_id"`
	ResourceType string         `db:"resource_type"`
	ResourceID   string         `db:"resource_id"`
	Action       string         `db:"action"`
	Details      map[string]any `db:"details"`
	IPAddress    string         `db:"ip_address"`
	CreatedAt    time.Time      `db:"created_at"`
}

type Repository interface {
	CreateEvent(ctx context.Context, event *Event) error
	ListEvents(ctx context.Context, limit, offset int) ([]*Event, error)
	ListTeamEvents(ctx context.Context, tenantID, teamID uuid.UUID, limit, offset int) ([]*Event, error)
	ListResourceEvents(ctx context.Context, tenantID uuid.UUID, resourceType, resourceID string, limit, offset int) ([]*Event, error)
}

type PgRepository struct {
	q *queries.Queries
}

func NewPgRepository(q *queries.Queries) Repository {
	return &PgRepository{q: q}
}

func (r *PgRepository) CreateEvent(ctx context.Context, event *Event) error {
	details, err := json.Marshal(event.Details)
	if err != nil {
		return err
	}
	created, err := r.q.CreateAuditEvent(ctx, queries.CreateAuditEventParams{
		TenantID:     uuid.NullUUID{UUID: event.TenantID, Valid: event.TenantID != uuid.Nil},
		EventType:    event.EventType,
		ActorType:    event.ActorType,
		ActorID:      event.ActorID,
		ResourceType: textFromString(event.ResourceType),
		ResourceID:   textFromString(event.ResourceID),
		Action:       event.Action,
		Details:      details,
		IpAddress:    nil,
	})
	if err != nil {
		return err
	}
	*event = eventFromQuery(created)
	return nil
}

func (r *PgRepository) ListEvents(ctx context.Context, limit, offset int) ([]*Event, error) {
	events, err := r.q.ListAuditEvents(ctx, queries.ListAuditEventsParams{
		Offset: int32(offset),
		Limit:  int32(limit),
	})
	if err != nil {
		return nil, err
	}
	return eventsFromQuery(events), nil
}

func (r *PgRepository) ListTeamEvents(ctx context.Context, tenantID, teamID uuid.UUID, limit, offset int) ([]*Event, error) {
	events, err := r.q.ListTeamAuditEvents(ctx, queries.ListTeamAuditEventsParams{
		TenantID: tenantID,
		TeamID:   teamID,
		Offset:   int32(offset),
		Limit:    int32(limit),
	})
	if err != nil {
		return nil, err
	}
	return eventsFromQuery(events), nil
}

func (r *PgRepository) ListResourceEvents(ctx context.Context, tenantID uuid.UUID, resourceType, resourceID string, limit, offset int) ([]*Event, error) {
	events, err := r.q.ListAuditEvents(ctx, queries.ListAuditEventsParams{
		ResourceType: textFromString(resourceType),
		ResourceID:   textFromString(resourceID),
		Offset:       int32(offset),
		Limit:        int32(limit),
	})
	if err != nil {
		return nil, err
	}
	return filterEventsByTenant(eventsFromQuery(events), tenantID), nil
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

func (s *Service) ListTeamEvents(ctx context.Context, tenantID, teamID uuid.UUID, limit, offset int) ([]*Event, error) {
	return s.repository.ListTeamEvents(ctx, tenantID, teamID, limit, offset)
}

func (s *Service) ListProjectEvents(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int) ([]*Event, error) {
	if tenantID == uuid.Nil {
		return nil, errors.New("tenant id is required")
	}
	if projectID == uuid.Nil {
		return nil, errors.New("project id is required")
	}
	return s.repository.ListResourceEvents(ctx, tenantID, "project", projectID.String(), limit, offset)
}

func filterEventsByTenant(events []*Event, tenantID uuid.UUID) []*Event {
	filtered := make([]*Event, 0, len(events))
	for _, event := range events {
		if event != nil && event.TenantID == tenantID {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func eventsFromQuery(events []queries.AuditEvent) []*Event {
	items := make([]*Event, 0, len(events))
	for _, event := range events {
		item := eventFromQuery(event)
		items = append(items, &item)
	}
	return items
}

func eventFromQuery(event queries.AuditEvent) Event {
	return Event{
		ID:           event.ID,
		TenantID:     event.TenantID,
		EventType:    event.EventType,
		ActorType:    event.ActorType,
		ActorID:      event.ActorID,
		ResourceType: stringFromText(event.ResourceType),
		ResourceID:   stringFromText(event.ResourceID),
		Action:       event.Action,
		Details:      mapFromJSON(event.Details),
		IPAddress:    ipAddressString(event),
		CreatedAt:    timeFromTimestamptz(event.CreatedAt),
	}
}

func mapFromJSON(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return map[string]any{}
	}
	if value == nil {
		return map[string]any{}
	}
	return value
}

func textFromString(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func stringFromText(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func timeFromTimestamptz(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func ipAddressString(event queries.AuditEvent) string {
	if event.IpAddress == nil {
		return ""
	}
	return event.IpAddress.String()
}

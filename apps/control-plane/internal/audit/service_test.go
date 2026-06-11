package audit

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

type memoryRepository struct {
	events []*Event
}

func (m *memoryRepository) CreateEvent(ctx context.Context, event *Event) error {
	event.ID = uuid.New()
	m.events = append(m.events, event)
	return nil
}

func (m *memoryRepository) ListEvents(ctx context.Context, limit, offset int) ([]*Event, error) {
	return paginateEvents(m.events, limit, offset), nil
}

func (m *memoryRepository) ListTeamEvents(ctx context.Context, tenantID, teamID uuid.UUID, limit, offset int) ([]*Event, error) {
	return m.ListResourceEvents(ctx, tenantID, "team", teamID.String(), limit, offset)
}

func (m *memoryRepository) ListResourceEvents(ctx context.Context, tenantID uuid.UUID, resourceType, resourceID string, limit, offset int) ([]*Event, error) {
	filtered := make([]*Event, 0, len(m.events))
	for _, event := range m.events {
		if event.TenantID == tenantID && event.ResourceType == resourceType && event.ResourceID == resourceID {
			filtered = append(filtered, event)
		}
	}
	return paginateEvents(filtered, limit, offset), nil
}

func paginateEvents(events []*Event, limit, offset int) []*Event {
	if offset >= len(events) || limit <= 0 {
		return []*Event{}
	}
	end := offset + limit
	if end > len(events) {
		end = len(events)
	}
	return events[offset:end]
}

func TestNewServiceRequiresRepository(t *testing.T) {
	if _, err := NewService(nil); err == nil {
		t.Fatal("expected nil repository to fail")
	}
}

func TestNewServiceAcceptsRepository(t *testing.T) {
	service, err := NewService(&memoryRepository{})
	if err != nil {
		t.Fatalf("expected service: %v", err)
	}
	if service == nil {
		t.Fatal("expected service")
	}
}

func TestLogEvent(t *testing.T) {
	repo := &memoryRepository{}
	service, _ := NewService(repo)

	err := service.LogEvent(context.Background(), "user.login", "user", "user123", "session", "sess456", "login")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(repo.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(repo.events))
	}

	event := repo.events[0]
	if event.EventType != "user.login" {
		t.Errorf("expected event type user.login, got %s", event.EventType)
	}
	if event.ActorID != "user123" {
		t.Errorf("expected actor id user123, got %s", event.ActorID)
	}
}

func TestListEvents(t *testing.T) {
	repo := &memoryRepository{}
	service, _ := NewService(repo)

	service.LogEvent(context.Background(), "event1", "user", "u1", "res", "r1", "action1")
	service.LogEvent(context.Background(), "event2", "user", "u2", "res", "r2", "action2")
	service.LogEvent(context.Background(), "event3", "user", "u3", "res", "r3", "action3")

	events, err := service.ListEvents(context.Background(), 2, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}

	events, err = service.ListEvents(context.Background(), 2, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

func TestListProjectEventsFiltersByProjectResource(t *testing.T) {
	tenantID := uuid.New()
	otherTenantID := uuid.New()
	projectID := uuid.New()
	otherProjectID := uuid.New()
	repo := &memoryRepository{events: []*Event{
		{
			ID:           uuid.New(),
			TenantID:     tenantID,
			EventType:    "project.created",
			ResourceType: "project",
			ResourceID:   projectID.String(),
			Action:       "project.create",
		},
		{
			ID:           uuid.New(),
			TenantID:     tenantID,
			EventType:    "project.created",
			ResourceType: "project",
			ResourceID:   otherProjectID.String(),
			Action:       "project.create",
		},
		{
			ID:           uuid.New(),
			TenantID:     otherTenantID,
			EventType:    "project.created",
			ResourceType: "project",
			ResourceID:   projectID.String(),
			Action:       "project.create",
		},
		{
			ID:           uuid.New(),
			TenantID:     tenantID,
			EventType:    "project.created",
			ResourceType: "task",
			ResourceID:   projectID.String(),
			Action:       "task.create",
		},
	}}
	service, _ := NewService(repo)

	events, err := service.ListProjectEvents(context.Background(), tenantID, projectID, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one project event, got %d", len(events))
	}
	if events[0].TenantID != tenantID || events[0].ResourceType != "project" || events[0].ResourceID != projectID.String() {
		t.Fatalf("expected project resource event, got %#v", events[0])
	}

	if _, err := service.ListProjectEvents(context.Background(), tenantID, uuid.Nil, 10, 0); err == nil {
		t.Fatal("expected nil project id to fail")
	}
}

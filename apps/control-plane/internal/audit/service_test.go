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
	if offset >= len(m.events) {
		return []*Event{}, nil
	}
	end := offset + limit
	if end > len(m.events) {
		end = len(m.events)
	}
	return m.events[offset:end], nil
}

func (m *memoryRepository) ListTeamEvents(ctx context.Context, tenantID, teamID uuid.UUID, limit, offset int) ([]*Event, error) {
	return m.ListEvents(ctx, limit, offset)
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

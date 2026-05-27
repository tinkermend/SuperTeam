package approval

import "testing"

func TestNewServiceRequiresRepository(t *testing.T) {
	if _, err := NewService(nil); err == nil {
		t.Fatal("expected nil repository to fail")
	}
}

func TestNewServiceAcceptsRepository(t *testing.T) {
	service, err := NewService(memoryRepository{})
	if err != nil {
		t.Fatalf("expected service: %v", err)
	}
	if service == nil {
		t.Fatal("expected service")
	}
}

type memoryRepository struct{}

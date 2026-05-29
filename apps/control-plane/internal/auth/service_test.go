package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type mockRepo struct {
	users         map[string]*User
	runtimeTokens map[string]*RuntimeToken
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		users:         make(map[string]*User),
		runtimeTokens: make(map[string]*RuntimeToken),
	}
}

func (m *mockRepo) CreateUser(ctx context.Context, username, passwordHash string) (*User, error) {
	user := &User{
		ID:           int64(len(m.users) + 1),
		Username:     username,
		PasswordHash: passwordHash,
		Status:       "active",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	m.users[username] = user
	return user, nil
}

func (m *mockRepo) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	user, ok := m.users[username]
	if !ok {
		return nil, errors.New("user not found")
	}
	return user, nil
}

func (m *mockRepo) CreateRuntimeToken(ctx context.Context, nodeID, tokenHash string, expiresAt time.Time) error {
	m.runtimeTokens[nodeID] = &RuntimeToken{
		NodeID:    nodeID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}
	return nil
}

func (m *mockRepo) GetRuntimeTokenByNodeID(ctx context.Context, nodeID string) (*RuntimeToken, error) {
	token, ok := m.runtimeTokens[nodeID]
	if !ok {
		return nil, errors.New("token not found")
	}
	return token, nil
}

func TestNewService(t *testing.T) {
	if _, err := NewService(nil); err == nil {
		t.Fatal("expected error with nil repo")
	}
	if _, err := NewService(newMockRepo()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateUser(t *testing.T) {
	svc, _ := NewService(newMockRepo())
	user, err := svc.CreateUser(context.Background(), "test", "password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Username != "test" {
		t.Errorf("expected username test, got %s", user.Username)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("password")); err != nil {
		t.Error("password hash mismatch")
	}
}

func TestAuthenticateUser(t *testing.T) {
	svc, _ := NewService(newMockRepo())
	svc.CreateUser(context.Background(), "test", "password")

	user, err := svc.AuthenticateUser(context.Background(), "test", "password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Username != "test" {
		t.Errorf("expected username test, got %s", user.Username)
	}

	if _, err := svc.AuthenticateUser(context.Background(), "test", "wrong"); err != ErrInvalidCredentials {
		t.Error("expected invalid credentials error")
	}
}

func TestGenerateRuntimeToken(t *testing.T) {
	svc, _ := NewService(newMockRepo())
	expiresAt := time.Now().Add(24 * time.Hour)
	token, err := svc.GenerateRuntimeToken(context.Background(), "node1", expiresAt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token")
	}
}

func TestValidateRuntimeToken(t *testing.T) {
	svc, _ := NewService(newMockRepo())
	expiresAt := time.Now().Add(24 * time.Hour)
	token, _ := svc.GenerateRuntimeToken(context.Background(), "node1", expiresAt)

	if err := svc.ValidateRuntimeToken(context.Background(), "node1", token); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if err := svc.ValidateRuntimeToken(context.Background(), "node1", "invalid"); err != ErrInvalidToken {
		t.Error("expected invalid token error")
	}
}

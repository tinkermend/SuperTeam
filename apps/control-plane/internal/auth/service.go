package auth

import (
	"context"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Repository interface {
	CreateUser(ctx context.Context, username, passwordHash string) (*User, error)
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	CreateRuntimeToken(ctx context.Context, nodeID, tokenHash string, expiresAt time.Time) error
	GetRuntimeTokenByNodeID(ctx context.Context, nodeID string) (*RuntimeToken, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) (*Service, error) {
	if repo == nil {
		return nil, errors.New("repository is required")
	}
	return &Service{repo: repo}, nil
}

func (s *Service) CreateUser(ctx context.Context, username, password string) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return s.repo.CreateUser(ctx, username, string(hash))
}

func (s *Service) AuthenticateUser(ctx context.Context, username, password string) (*User, error) {
	user, err := s.repo.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	if user.Status != "active" {
		return nil, ErrUserDisabled
	}
	return user, nil
}

func (s *Service) GenerateRuntimeToken(ctx context.Context, nodeID string, expiresAt time.Time) (string, error) {
	token, err := GenerateToken()
	if err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(token), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	if err := s.repo.CreateRuntimeToken(ctx, nodeID, string(hash), expiresAt); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) ValidateRuntimeToken(ctx context.Context, nodeID, token string) error {
	rt, err := s.repo.GetRuntimeTokenByNodeID(ctx, nodeID)
	if err != nil {
		return ErrInvalidToken
	}
	if time.Now().After(rt.ExpiresAt) {
		return ErrInvalidToken
	}
	if err := bcrypt.CompareHashAndPassword([]byte(rt.TokenHash), []byte(token)); err != nil {
		return ErrInvalidToken
	}
	return nil
}

package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type PgRepository struct {
	q *queries.Queries
}

func NewPgRepository(q *queries.Queries) Repository {
	return &PgRepository{q: q}
}

func (r *PgRepository) CreateUser(ctx context.Context, username, passwordHash string) (*User, error) {
	user, err := r.q.CreateUser(ctx, queries.CreateUserParams{
		Username:     username,
		PasswordHash: passwordHash,
		Status:       UserStatusActive,
	})
	if err != nil {
		return nil, err
	}
	return toDomainUser(user), nil
}

func (r *PgRepository) ListUsers(ctx context.Context, filter ListUsersFilter) ([]*User, error) {
	rows, err := r.q.ListUsers(ctx, queries.ListUsersParams{
		Q: pgtype.Text{
			String: filter.Q,
			Valid:  filter.Q != "",
		},
		Status: pgtype.Text{
			String: filter.Status,
			Valid:  filter.Status != "",
		},
		Offset: filter.Offset,
		Limit:  filter.Limit,
	})
	if err != nil {
		return nil, err
	}
	users := make([]*User, 0, len(rows))
	for _, row := range rows {
		users = append(users, toDomainUser(row))
	}
	return users, nil
}

func (r *PgRepository) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	user, err := r.q.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	return toDomainUser(user), nil
}

func (r *PgRepository) GetUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	user, err := r.q.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return toDomainUser(user), nil
}

func (r *PgRepository) UpdateUserStatus(ctx context.Context, userID uuid.UUID, status string) (*User, error) {
	user, err := r.q.UpdateUser(ctx, queries.UpdateUserParams{
		ID:     userID,
		Status: status,
	})
	if err != nil {
		return nil, err
	}
	return toDomainUser(user), nil
}

func (r *PgRepository) UpdateUserPassword(ctx context.Context, userID uuid.UUID, passwordHash string) (*User, error) {
	user, err := r.q.UpdateUserPassword(ctx, queries.UpdateUserPasswordParams{
		ID:           userID,
		PasswordHash: passwordHash,
	})
	if err != nil {
		return nil, err
	}
	return toDomainUser(user), nil
}

func toDomainUser(user queries.AuthUser) *User {
	return &User{
		ID:           user.ID,
		Username:     user.Username,
		PasswordHash: user.PasswordHash,
		Status:       user.Status,
		CreatedAt:    user.CreatedAt.Time,
		UpdatedAt:    user.UpdatedAt.Time,
	}
}

func (r *PgRepository) CreateRuntimeToken(ctx context.Context, nodeID, tokenHash string, expiresAt time.Time) error {
	_, err := r.q.CreateRuntimeToken(ctx, queries.CreateRuntimeTokenParams{
		NodeID:    nodeID,
		TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	return err
}

func (r *PgRepository) GetRuntimeTokenByNodeID(ctx context.Context, nodeID string) (*RuntimeToken, error) {
	token, err := r.q.GetRuntimeTokenByNodeID(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	return &RuntimeToken{
		ID:        token.ID,
		NodeID:    token.NodeID,
		TokenHash: token.TokenHash,
		ExpiresAt: token.ExpiresAt.Time,
		CreatedAt: token.CreatedAt.Time,
	}, nil
}

func (r *PgRepository) CreateSession(ctx context.Context, session *Session, tokenHash string) error {
	created, err := r.q.CreateSession(ctx, queries.CreateSessionParams{
		UserID:    session.UserID,
		TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{
			Time:  session.ExpiresAt,
			Valid: true,
		},
		LastSeenAt: pgtype.Timestamptz{
			Time:  session.LastSeenAt,
			Valid: true,
		},
		ClientIp: pgtype.Text{
			String: session.ClientIP,
			Valid:  session.ClientIP != "",
		},
		UserAgent: pgtype.Text{
			String: session.UserAgent,
			Valid:  session.UserAgent != "",
		},
	})
	if err == nil {
		session.ID = created.ID
		session.ExpiresAt = created.ExpiresAt.Time
		session.LastSeenAt = created.LastSeenAt.Time
	}
	return err
}

func (r *PgRepository) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, error) {
	session, err := r.q.GetSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	return &Session{
		ID:         session.ID,
		UserID:     session.UserID,
		ExpiresAt:  session.ExpiresAt.Time,
		LastSeenAt: session.LastSeenAt.Time,
		ClientIP:   session.ClientIp.String,
		UserAgent:  session.UserAgent.String,
	}, nil
}

func (r *PgRepository) DeleteSession(ctx context.Context, tokenHash string) error {
	return r.q.DeleteSessionByTokenHash(ctx, tokenHash)
}

func (r *PgRepository) UpdateSessionLastSeen(ctx context.Context, tokenHash string, lastSeenAt time.Time) error {
	_, err := r.q.UpdateSessionLastSeen(ctx, queries.UpdateSessionLastSeenParams{
		TokenHash: tokenHash,
		LastSeenAt: pgtype.Timestamptz{
			Time:  lastSeenAt,
			Valid: true,
		},
	})
	return err
}

func (r *PgRepository) CreateLoginLog(ctx context.Context, params CreateLoginLogParams) error {
	_, err := r.q.CreateWebLoginLog(ctx, queries.CreateWebLoginLogParams{
		EventType: params.EventType,
		UserID:    nullUUID(params.UserID),
		Username:  params.Username,
		SessionID: nullUUID(params.SessionID),
		ClientIp: pgtype.Text{
			String: params.ClientIP,
			Valid:  params.ClientIP != "",
		},
		UserAgent: pgtype.Text{
			String: params.UserAgent,
			Valid:  params.UserAgent != "",
		},
		Result: params.Result,
		FailureReason: pgtype.Text{
			String: params.FailureReason,
			Valid:  params.FailureReason != "",
		},
	})
	return err
}

func (r *PgRepository) ListLoginLogs(ctx context.Context, filter ListLoginLogsFilter) ([]LoginLog, error) {
	rows, err := r.q.ListWebLoginLogs(ctx, queries.ListWebLoginLogsParams{
		Offset: filter.Offset,
		Limit:  filter.Limit,
	})
	if err != nil {
		return nil, err
	}
	logs := make([]LoginLog, 0, len(rows))
	for _, row := range rows {
		logs = append(logs, toDomainLoginLog(row))
	}
	return logs, nil
}

func (r *PgRepository) CreateOperationLog(ctx context.Context, params CreateOperationLogParams) error {
	_, err := r.q.CreateWebOperationLog(ctx, queries.CreateWebOperationLogParams{
		UserID: nullUUID(params.UserID),
		Username: pgtype.Text{
			String: params.Username,
			Valid:  params.Username != "",
		},
		Module: params.Module,
		ResourceType: pgtype.Text{
			String: params.ResourceType,
			Valid:  params.ResourceType != "",
		},
		ResourceID: pgtype.Text{
			String: params.ResourceID,
			Valid:  params.ResourceID != "",
		},
		Action: params.Action,
		Result: params.Result,
		ClientIp: pgtype.Text{
			String: params.ClientIP,
			Valid:  params.ClientIP != "",
		},
		UserAgent: pgtype.Text{
			String: params.UserAgent,
			Valid:  params.UserAgent != "",
		},
	})
	return err
}

func nullUUID(value *uuid.UUID) uuid.NullUUID {
	if value == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *value, Valid: true}
}

func toDomainLoginLog(log queries.WebLoginLog) LoginLog {
	var userID *uuid.UUID
	if log.UserID.Valid {
		id := log.UserID.UUID
		userID = &id
	}
	var sessionID *uuid.UUID
	if log.SessionID.Valid {
		id := log.SessionID.UUID
		sessionID = &id
	}
	return LoginLog{
		ID:            log.ID,
		EventType:     log.EventType,
		UserID:        userID,
		Username:      log.Username,
		SessionID:     sessionID,
		ClientIP:      log.ClientIp.String,
		UserAgent:     log.UserAgent.String,
		Result:        log.Result,
		FailureReason: log.FailureReason.String,
		CreatedAt:     log.CreatedAt.Time,
	}
}

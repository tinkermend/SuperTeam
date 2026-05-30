package auth

import (
	"context"
	"time"

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
	})
	if err != nil {
		return nil, err
	}
	return toDomainUser(user), nil
}

func (r *PgRepository) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	user, err := r.q.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	return toDomainUser(user), nil
}

func (r *PgRepository) GetUserByID(ctx context.Context, id int64) (*User, error) {
	user, err := r.q.GetUserByID(ctx, id)
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
	_, err := r.q.CreateSession(ctx, queries.CreateSessionParams{
		ID:        session.ID,
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
		UserID: pgtype.Int8{
			Int64: valueOrZero(params.UserID),
			Valid: params.UserID != nil,
		},
		Username: params.Username,
		SessionID: pgtype.Text{
			String: params.SessionID,
			Valid:  params.SessionID != "",
		},
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

func valueOrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func toDomainLoginLog(log queries.WebLoginLog) LoginLog {
	var userID *int64
	if log.UserID.Valid {
		id := log.UserID.Int64
		userID = &id
	}
	return LoginLog{
		ID:            log.ID,
		EventType:     log.EventType,
		UserID:        userID,
		Username:      log.Username,
		SessionID:     log.SessionID.String,
		ClientIP:      log.ClientIp.String,
		UserAgent:     log.UserAgent.String,
		Result:        log.Result,
		FailureReason: log.FailureReason.String,
		CreatedAt:     log.CreatedAt.Time,
	}
}

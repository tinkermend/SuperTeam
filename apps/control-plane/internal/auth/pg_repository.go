package auth

import (
	"context"
	"encoding/json"
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

func (r *PgRepository) CreateUser(ctx context.Context, input CreateUserRecordInput) (*User, error) {
	avatar := normalizeUserAvatarConfig(input.Username, UserAvatarConfig{})
	avatarOptions, err := json.Marshal(avatar.Options)
	if err != nil {
		return nil, err
	}
	user, err := r.q.CreateUser(ctx, queries.CreateUserParams{
		Username: input.Username,
		DisplayName: pgtype.Text{
			String: input.DisplayName,
			Valid:  input.DisplayName != "",
		},
		Email: pgtype.Text{
			String: input.Email,
			Valid:  input.Email != "",
		},
		PasswordHash: input.PasswordHash,
		Status:       UserStatusActive,
		AvatarAssetID: pgtype.Text{
			String: input.AvatarAssetID,
			Valid:  input.AvatarAssetID != "",
		},
	})
	if err != nil {
		return nil, err
	}
	user, err = r.q.UpdateUserAvatar(ctx, queries.UpdateUserAvatarParams{
		ID:             user.ID,
		AvatarProvider: avatar.Provider,
		AvatarStyle:    avatar.Style,
		AvatarSeed: pgtype.Text{
			String: avatar.Seed,
			Valid:  avatar.Seed != "",
		},
		AvatarOptions: avatarOptions,
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

func (r *PgRepository) UpdateUserProfile(ctx context.Context, userID uuid.UUID, input UpdateUserProfileInput) (*User, error) {
	user, err := r.q.UpdateUser(ctx, queries.UpdateUserParams{
		ID: userID,
		DisplayName: pgtype.Text{
			String: input.DisplayName,
			Valid:  true,
		},
		Email: pgtype.Text{
			String: input.Email,
			Valid:  true,
		},
	})
	if err != nil {
		return nil, err
	}
	avatarOptions, err := json.Marshal(input.Avatar.Options)
	if err != nil {
		return nil, err
	}
	user, err = r.q.UpdateUserAvatar(ctx, queries.UpdateUserAvatarParams{
		ID:             user.ID,
		AvatarProvider: input.Avatar.Provider,
		AvatarStyle:    input.Avatar.Style,
		AvatarSeed: pgtype.Text{
			String: input.Avatar.Seed,
			Valid:  input.Avatar.Seed != "",
		},
		AvatarOptions: avatarOptions,
	})
	if err != nil {
		return nil, err
	}
	return toDomainUser(user), nil
}

func toDomainUser(user queries.AuthUser) *User {
	avatar := normalizeUserAvatarConfig(user.Username, UserAvatarConfig{
		Provider: user.AvatarProvider,
		Style:    user.AvatarStyle,
		Seed:     user.AvatarSeed.String,
		Options:  userAvatarOptions(user.AvatarOptions),
	})
	return &User{
		ID:            user.ID,
		Username:      user.Username,
		DisplayName:   user.DisplayName.String,
		Email:         user.Email.String,
		PasswordHash:  user.PasswordHash,
		Status:        user.Status,
		Avatar:        avatar,
		AvatarAssetID: user.AvatarAssetID.String,
		CreatedAt:     user.CreatedAt.Time,
		UpdatedAt:     user.UpdatedAt.Time,
	}
}

func userAvatarOptions(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var options map[string]any
	if err := json.Unmarshal(raw, &options); err != nil || options == nil {
		return map[string]any{}
	}
	return options
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
		UserID: nullUUID(filter.UserID),
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

func (r *PgRepository) ReplaceUserProjectTeamScopes(ctx context.Context, tenantID, userID, grantedByUserID uuid.UUID, teamIDs []uuid.UUID) ([]UserProjectTeamScopeSummary, error) {
	if err := r.q.RevokeUserProjectTeamScopes(ctx, queries.RevokeUserProjectTeamScopesParams{
		TenantID: tenantID,
		UserID:   userID,
		TeamIds:  teamIDs,
	}); err != nil {
		return nil, err
	}
	for _, teamID := range teamIDs {
		if _, err := r.q.UpsertUserProjectTeamScope(ctx, queries.UpsertUserProjectTeamScopeParams{
			TenantID: tenantID,
			UserID:   userID,
			TeamID:   teamID,
			GrantedByUserID: uuid.NullUUID{
				UUID:  grantedByUserID,
				Valid: grantedByUserID != uuid.Nil,
			},
		}); err != nil {
			return nil, err
		}
	}
	return r.ListUserProjectTeamScopes(ctx, tenantID, userID)
}

func (r *PgRepository) ListUserProjectTeamScopes(ctx context.Context, tenantID, userID uuid.UUID) ([]UserProjectTeamScopeSummary, error) {
	rows, err := r.q.ListUserProjectTeamScopeSummaries(ctx, queries.ListUserProjectTeamScopeSummariesParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return nil, err
	}
	items := make([]UserProjectTeamScopeSummary, 0, len(rows))
	for _, row := range rows {
		items = append(items, toDomainUserProjectTeamScopeSummary(row))
	}
	return items, nil
}

func (r *PgRepository) CanUseTeamForProject(ctx context.Context, tenantID, userID, teamID uuid.UUID) (bool, error) {
	return r.q.UserHasActiveProjectTeamScope(ctx, queries.UserHasActiveProjectTeamScopeParams{
		TenantID: tenantID,
		UserID:   userID,
		TeamID:   teamID,
	})
}

func nullUUID(value *uuid.UUID) uuid.NullUUID {
	if value == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *value, Valid: true}
}

func toDomainUserProjectTeamScopeSummary(row queries.ListUserProjectTeamScopeSummariesRow) UserProjectTeamScopeSummary {
	return UserProjectTeamScopeSummary{
		ID:              row.ID,
		TenantID:        row.TenantID,
		UserID:          row.UserID,
		TeamID:          row.TeamID,
		Status:          row.Status,
		GrantedByUserID: uuidPtr(row.GrantedByUserID),
		RevokedAt:       timePtr(row.RevokedAt),
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
		Team: UserProjectTeamScopeTeamSummary{
			ID:                   row.TeamID,
			Slug:                 row.Slug,
			Name:                 row.Name,
			Status:               row.TeamStatus,
			DigitalEmployeeCount: row.DigitalEmployeeCount,
			CurrentRevision:      int32Ptr(row.CurrentRevision),
			PendingDraftCount:    row.PendingDraftCount,
			GovernanceStatus:     row.GovernanceStatus,
			RiskSummary:          row.RiskSummary,
			HumanOwners:          userProjectTeamOwners(row.HumanOwners),
		},
	}
}

func userProjectTeamOwners(raw []byte) []UserProjectTeamScopeOwnerSummary {
	if len(raw) == 0 {
		return []UserProjectTeamScopeOwnerSummary{}
	}
	var rows []struct {
		ID             uuid.UUID      `json:"id"`
		Username       string         `json:"username"`
		DisplayName    string         `json:"display_name"`
		Email          string         `json:"email"`
		Status         string         `json:"status"`
		AvatarProvider string         `json:"avatar_provider"`
		AvatarStyle    string         `json:"avatar_style"`
		AvatarSeed     string         `json:"avatar_seed"`
		AvatarOptions  map[string]any `json:"avatar_options"`
		AvatarAssetID  string         `json:"avatar_asset_id"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return []UserProjectTeamScopeOwnerSummary{}
	}
	owners := make([]UserProjectTeamScopeOwnerSummary, 0, len(rows))
	for _, row := range rows {
		owners = append(owners, UserProjectTeamScopeOwnerSummary{
			ID:          row.ID,
			Username:    row.Username,
			DisplayName: row.DisplayName,
			Email:       row.Email,
			Status:      row.Status,
			Avatar: normalizeUserAvatarConfig(row.Username, UserAvatarConfig{
				Provider: row.AvatarProvider,
				Style:    row.AvatarStyle,
				Seed:     row.AvatarSeed,
				Options:  row.AvatarOptions,
			}),
			AvatarAssetID: row.AvatarAssetID,
		})
	}
	return owners
}

func uuidPtr(value uuid.NullUUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	return &value.UUID
}

func timePtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

func int32Ptr(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	return &value.Int32
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

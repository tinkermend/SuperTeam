package tenant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func (r *PgRepository) CreateTeam(ctx context.Context, params CreateTeamParams) (TeamRecord, error) {
	metadata, err := jsonbFromMap(params.Metadata, "metadata")
	if err != nil {
		return TeamRecord{}, err
	}
	team, err := r.q.CreateTenantTeam(ctx, queries.CreateTenantTeamParams{
		TenantID:         params.TenantID,
		Slug:             params.Slug,
		Name:             params.Name,
		Status:           string(params.Status),
		HumanOwnerUserID: nullUUIDFromPtr(params.HumanOwnerUserID),
		Metadata:         metadata,
	})
	if err != nil {
		return TeamRecord{}, err
	}
	return teamRecordFromQuery(team)
}

func (r *PgRepository) ListTeams(ctx context.Context, params ListTeamsParams) ([]TeamRecord, error) {
	teams, err := r.q.ListTenantTeams(ctx, queries.ListTenantTeamsParams{
		TenantID: params.TenantID,
		Status:   textFromTeamStatus(params.Status),
		Offset:   params.Offset,
		Limit:    params.Limit,
	})
	if err != nil {
		return nil, err
	}
	records := make([]TeamRecord, 0, len(teams))
	for _, team := range teams {
		record, err := teamRecordFromQuery(team)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (r *PgRepository) GetTeam(ctx context.Context, tenantID, teamID uuid.UUID) (TeamRecord, error) {
	team, err := r.q.GetTenantTeam(ctx, queries.GetTenantTeamParams{
		ID:       teamID,
		TenantID: tenantID,
	})
	if err != nil {
		return TeamRecord{}, mapNoRows(err)
	}
	return teamRecordFromQuery(team)
}

func (r *PgRepository) CreateTeamConfigRevision(ctx context.Context, params CreateTeamConfigRevisionParams) (TeamConfigRevisionRecord, error) {
	constitution, err := jsonbFromMap(params.Constitution, "constitution")
	if err != nil {
		return TeamConfigRevisionRecord{}, err
	}
	capabilityPolicy, err := jsonbFromMap(params.CapabilityPolicy, "capability_policy")
	if err != nil {
		return TeamConfigRevisionRecord{}, err
	}
	contextPolicy, err := jsonbFromMap(params.ContextPolicy, "context_policy")
	if err != nil {
		return TeamConfigRevisionRecord{}, err
	}
	approvalPolicy, err := jsonbFromMap(params.ApprovalPolicy, "approval_policy")
	if err != nil {
		return TeamConfigRevisionRecord{}, err
	}
	artifactContract, err := jsonbFromMap(params.ArtifactContract, "artifact_contract")
	if err != nil {
		return TeamConfigRevisionRecord{}, err
	}
	internalCollaborationPolicy, err := jsonbFromMap(params.InternalCollaborationPolicy, "internal_collaboration_policy")
	if err != nil {
		return TeamConfigRevisionRecord{}, err
	}
	runtimeScopePolicy, err := jsonbFromMap(params.RuntimeScopePolicy, "runtime_scope_policy")
	if err != nil {
		return TeamConfigRevisionRecord{}, err
	}

	revision, err := r.q.CreateTenantTeamConfigRevision(ctx, queries.CreateTenantTeamConfigRevisionParams{
		TenantID:                    params.TenantID,
		TeamID:                      params.TeamID,
		RevisionNumber:              params.RevisionNumber,
		Constitution:                constitution,
		CapabilityPolicy:            capabilityPolicy,
		ContextPolicy:               contextPolicy,
		ApprovalPolicy:              approvalPolicy,
		ArtifactContract:            artifactContract,
		InternalCollaborationPolicy: internalCollaborationPolicy,
		RuntimeScopePolicy:          runtimeScopePolicy,
		HumanOwnerUserID:            nullUUIDFromPtr(params.HumanOwnerUserID),
		Status:                      string(params.Status),
		ApprovedBy:                  nullUUIDFromPtr(params.ApprovedBy),
		ApprovedAt:                  timestamptzFromPtr(params.ApprovedAt),
	})
	if err != nil {
		return TeamConfigRevisionRecord{}, err
	}
	return configRevisionRecordFromQuery(revision)
}

func (r *PgRepository) GetTeamConfigRevision(ctx context.Context, tenantID, revisionID uuid.UUID) (TeamConfigRevisionRecord, error) {
	revision, err := r.q.GetTenantTeamConfigRevision(ctx, queries.GetTenantTeamConfigRevisionParams{
		ID:       revisionID,
		TenantID: tenantID,
	})
	if err != nil {
		return TeamConfigRevisionRecord{}, mapNoRows(err)
	}
	return configRevisionRecordFromQuery(revision)
}

func (r *PgRepository) GetCurrentTeamConfigRevision(ctx context.Context, tenantID, teamID uuid.UUID) (TeamConfigRevisionRecord, error) {
	revision, err := r.q.GetCurrentTenantTeamConfigRevision(ctx, queries.GetCurrentTenantTeamConfigRevisionParams{
		TenantID: tenantID,
		TeamID:   teamID,
	})
	if err != nil {
		return TeamConfigRevisionRecord{}, mapNoRows(err)
	}
	return configRevisionRecordFromQuery(revision)
}

func (r *PgRepository) GetNextTeamConfigRevisionNumber(ctx context.Context, tenantID, teamID uuid.UUID) (int32, error) {
	nextRevision, err := r.q.GetNextTenantTeamConfigRevisionNumber(ctx, queries.GetNextTenantTeamConfigRevisionNumberParams{
		TenantID: tenantID,
		TeamID:   teamID,
	})
	if err != nil {
		return 0, err
	}
	return nextRevision, nil
}

func teamRecordFromQuery(team queries.TenantTeam) (TeamRecord, error) {
	metadata, err := mapFromJSONB(team.Metadata, "metadata")
	if err != nil {
		return TeamRecord{}, err
	}
	return TeamRecord{
		ID:               team.ID,
		TenantID:         team.TenantID,
		Slug:             team.Slug,
		Name:             team.Name,
		Status:           TeamStatus(team.Status),
		HumanOwnerUserID: uuidPtrFromNull(team.HumanOwnerUserID),
		Metadata:         metadata,
		CreatedAt:        timeFromTimestamptz(team.CreatedAt),
		UpdatedAt:        timeFromTimestamptz(team.UpdatedAt),
	}, nil
}

func configRevisionRecordFromQuery(revision queries.TenantTeamConfigRevision) (TeamConfigRevisionRecord, error) {
	constitution, err := mapFromJSONB(revision.Constitution, "constitution")
	if err != nil {
		return TeamConfigRevisionRecord{}, err
	}
	capabilityPolicy, err := mapFromJSONB(revision.CapabilityPolicy, "capability_policy")
	if err != nil {
		return TeamConfigRevisionRecord{}, err
	}
	contextPolicy, err := mapFromJSONB(revision.ContextPolicy, "context_policy")
	if err != nil {
		return TeamConfigRevisionRecord{}, err
	}
	approvalPolicy, err := mapFromJSONB(revision.ApprovalPolicy, "approval_policy")
	if err != nil {
		return TeamConfigRevisionRecord{}, err
	}
	artifactContract, err := mapFromJSONB(revision.ArtifactContract, "artifact_contract")
	if err != nil {
		return TeamConfigRevisionRecord{}, err
	}
	internalCollaborationPolicy, err := mapFromJSONB(revision.InternalCollaborationPolicy, "internal_collaboration_policy")
	if err != nil {
		return TeamConfigRevisionRecord{}, err
	}
	runtimeScopePolicy, err := mapFromJSONB(revision.RuntimeScopePolicy, "runtime_scope_policy")
	if err != nil {
		return TeamConfigRevisionRecord{}, err
	}
	return TeamConfigRevisionRecord{
		ID:                          revision.ID,
		TenantID:                    revision.TenantID,
		TeamID:                      revision.TeamID,
		RevisionNumber:              revision.RevisionNumber,
		Constitution:                constitution,
		CapabilityPolicy:            capabilityPolicy,
		ContextPolicy:               contextPolicy,
		ApprovalPolicy:              approvalPolicy,
		ArtifactContract:            artifactContract,
		InternalCollaborationPolicy: internalCollaborationPolicy,
		RuntimeScopePolicy:          runtimeScopePolicy,
		HumanOwnerUserID:            uuidPtrFromNull(revision.HumanOwnerUserID),
		Status:                      TeamConfigRevisionStatus(revision.Status),
		ApprovedBy:                  uuidPtrFromNull(revision.ApprovedBy),
		ApprovedAt:                  timePtrFromTimestamptz(revision.ApprovedAt),
		CreatedAt:                   timeFromTimestamptz(revision.CreatedAt),
		UpdatedAt:                   timeFromTimestamptz(revision.UpdatedAt),
	}, nil
}

func mapNoRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func nullUUIDFromPtr(value *uuid.UUID) uuid.NullUUID {
	if value == nil || *value == uuid.Nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *value, Valid: true}
}

func uuidPtrFromNull(value uuid.NullUUID) *uuid.UUID {
	if !value.Valid || value.UUID == uuid.Nil {
		return nil
	}
	copied := value.UUID
	return &copied
}

func textFromTeamStatus(status TeamStatus) pgtype.Text {
	if status == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: string(status), Valid: true}
}

func timestamptzFromPtr(value *time.Time) pgtype.Timestamptz {
	if value == nil || value.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func timePtrFromTimestamptz(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time.UTC()
	return &t
}

func timeFromTimestamptz(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}

func jsonbFromMap(value map[string]any, field string) ([]byte, error) {
	encoded, err := json.Marshal(cloneMap(value))
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", field, err)
	}
	return encoded, nil
}

func mapFromJSONB(raw []byte, field string) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("decode %s: %w", field, err)
	}
	if decoded == nil {
		return map[string]any{}, nil
	}
	return decoded, nil
}

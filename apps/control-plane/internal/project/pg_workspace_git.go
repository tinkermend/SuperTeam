package project

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type workspaceGitDueProject struct {
	ID                   uuid.UUID
	TenantID             uuid.UUID
	DirectoryName        string
	PrimaryRuntimeNodeID uuid.UUID
	SampledAt            *time.Time
	IsClean              *bool
	SampleError          string
	InflightAt           *time.Time
}

type workspaceGitStore interface {
	GetProjectWorkspaceGitSnapshot(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectWorkspaceGitStatus, error)
	ListProjectWorkspaceGitSnapshots(ctx context.Context, tenantID uuid.UUID, projectIDs []uuid.UUID) (map[uuid.UUID]*ProjectWorkspaceGitStatus, error)
	UpsertProjectWorkspaceGitSnapshotSuccess(ctx context.Context, tenantID, projectID uuid.UUID, status ProjectWorkspaceGitStatus) error
	MarkProjectWorkspaceGitSnapshotFailed(ctx context.Context, tenantID, projectID uuid.UUID, sampleError string, attemptedAt time.Time) error
	MarkProjectWorkspaceGitProbeInflight(ctx context.Context, tenantID, projectID uuid.UUID, commandID string, inflightAt time.Time) error
	ListProjectsDueForWorkspaceGitSample(ctx context.Context, staleBefore, idleStaleBefore, inflightStaleBefore time.Time, limit int32) ([]workspaceGitDueProject, error)
}

func (r *PgRepository) GetProjectWorkspaceGitSnapshot(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectWorkspaceGitStatus, error) {
	row, err := r.q.GetProjectWorkspaceGitSnapshot(ctx, queries.GetProjectWorkspaceGitSnapshotParams{
		TenantID:  tenantID,
		ProjectID: projectID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	status := workspaceGitStatusFromRow(row)
	return &status, nil
}

func (r *PgRepository) ListProjectWorkspaceGitSnapshots(ctx context.Context, tenantID uuid.UUID, projectIDs []uuid.UUID) (map[uuid.UUID]*ProjectWorkspaceGitStatus, error) {
	out := make(map[uuid.UUID]*ProjectWorkspaceGitStatus, len(projectIDs))
	if len(projectIDs) == 0 {
		return out, nil
	}
	rows, err := r.q.ListProjectWorkspaceGitSnapshots(ctx, queries.ListProjectWorkspaceGitSnapshotsParams{
		TenantID:   tenantID,
		ProjectIds: projectIDs,
	})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		status := workspaceGitStatusFromRow(row)
		copied := status
		out[row.ProjectID] = &copied
	}
	return out, nil
}

func (r *PgRepository) UpsertProjectWorkspaceGitSnapshotSuccess(ctx context.Context, tenantID, projectID uuid.UUID, status ProjectWorkspaceGitStatus) error {
	entries, err := json.Marshal(status.UncommittedEntries)
	if err != nil {
		return err
	}
	if len(status.UncommittedEntries) == 0 {
		entries = []byte("[]")
	}
	sampledAt := time.Now().UTC()
	if status.SampledAt != nil && !status.SampledAt.IsZero() {
		sampledAt = status.SampledAt.UTC()
	}
	return r.q.UpsertProjectWorkspaceGitSnapshotSuccess(ctx, queries.UpsertProjectWorkspaceGitSnapshotSuccessParams{
		TenantID:             tenantID,
		ProjectID:            projectID,
		IsGitRepo:            boolPtr(status.IsGitRepo),
		IsClean:              boolPtr(status.IsClean),
		HeadCommit:           textOrNull(strings.TrimSpace(status.HeadCommit)),
		CurrentBranch:        textOrNull(strings.TrimSpace(status.CurrentBranch)),
		Detached:             status.Detached,
		RepoState:            textOrNull(string(status.RepoState)),
		UncommittedCount:     int32(status.UncommittedCount),
		UncommittedEntries:   entries,
		UncommittedTruncated: status.UncommittedTruncated,
		UncommittedOmitted:   int32(status.UncommittedOmitted),
		SampledAt:            pgtype.Timestamptz{Time: sampledAt, Valid: true},
		SampledRuntimeNodeID: nullUUID(status.SampledRuntimeNodeID),
		SampledNodeID:        textOrNull(strings.TrimSpace(status.SampledNodeID)),
	})
}

func (r *PgRepository) MarkProjectWorkspaceGitSnapshotFailed(ctx context.Context, tenantID, projectID uuid.UUID, sampleError string, attemptedAt time.Time) error {
	if attemptedAt.IsZero() {
		attemptedAt = time.Now().UTC()
	}
	return r.q.MarkProjectWorkspaceGitSnapshotFailed(ctx, queries.MarkProjectWorkspaceGitSnapshotFailedParams{
		TenantID:    tenantID,
		ProjectID:   projectID,
		SampleError: strings.TrimSpace(sampleError),
		AttemptedAt: pgtype.Timestamptz{Time: attemptedAt.UTC(), Valid: true},
	})
}

func (r *PgRepository) MarkProjectWorkspaceGitProbeInflight(ctx context.Context, tenantID, projectID uuid.UUID, commandID string, inflightAt time.Time) error {
	if inflightAt.IsZero() {
		inflightAt = time.Now().UTC()
	}
	return r.q.MarkProjectWorkspaceGitProbeInflight(ctx, queries.MarkProjectWorkspaceGitProbeInflightParams{
		TenantID:          tenantID,
		ProjectID:         projectID,
		InflightAt:        pgtype.Timestamptz{Time: inflightAt.UTC(), Valid: true},
		InflightCommandID: strings.TrimSpace(commandID),
	})
}

func (r *PgRepository) ListProjectsDueForWorkspaceGitSample(ctx context.Context, staleBefore, idleStaleBefore, inflightStaleBefore time.Time, limit int32) ([]workspaceGitDueProject, error) {
	if limit <= 0 {
		limit = workspaceGitSampleBatchLimit
	}
	rows, err := r.q.ListProjectsDueForWorkspaceGitSample(ctx, queries.ListProjectsDueForWorkspaceGitSampleParams{
		InflightStaleBefore: pgtype.Timestamptz{Time: inflightStaleBefore.UTC(), Valid: true},
		IdleStaleBefore:     pgtype.Timestamptz{Time: idleStaleBefore.UTC(), Valid: true},
		StaleBefore:         pgtype.Timestamptz{Time: staleBefore.UTC(), Valid: true},
		LimitCount:          limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]workspaceGitDueProject, 0, len(rows))
	for _, row := range rows {
		if !row.PrimaryRuntimeNodeID.Valid || row.PrimaryRuntimeNodeID.UUID == uuid.Nil {
			continue
		}
		item := workspaceGitDueProject{
			ID:                   row.ID,
			TenantID:             row.TenantID,
			DirectoryName:        strings.TrimSpace(row.DirectoryName),
			PrimaryRuntimeNodeID: row.PrimaryRuntimeNodeID.UUID,
			SampledAt:            ptrTime(row.SampledAt),
			IsClean:              ptrBool(row.IsClean),
			SampleError:          strings.TrimSpace(row.SampleError.String),
			InflightAt:           ptrTime(row.InflightAt),
		}
		if !row.SampleError.Valid {
			item.SampleError = ""
		}
		out = append(out, item)
	}
	return out, nil
}

func workspaceGitStatusFromRow(row queries.ProjectWorkspaceGitSnapshot) ProjectWorkspaceGitStatus {
	status := ProjectWorkspaceGitStatus{
		Applicable:           row.IsGitRepo.Valid && row.IsGitRepo.Bool,
		IsGitRepo:            ptrBool(row.IsGitRepo),
		IsClean:              ptrBool(row.IsClean),
		HeadCommit:           strings.TrimSpace(row.HeadCommit.String),
		CurrentBranch:        strings.TrimSpace(row.CurrentBranch.String),
		Detached:             row.Detached,
		RepoState:            ProjectWorkspaceGitRepoState(strings.TrimSpace(row.RepoState.String)),
		UncommittedCount:     int(row.UncommittedCount),
		UncommittedTruncated: row.UncommittedTruncated,
		UncommittedOmitted:   int(row.UncommittedOmitted),
		SampledAt:            ptrTime(row.SampledAt),
		SampledRuntimeNodeID: ptrUUID(row.SampledRuntimeNodeID),
		SampledNodeID:        strings.TrimSpace(row.SampledNodeID.String),
		SampleError:          strings.TrimSpace(row.SampleError.String),
		LastAttemptAt:        ptrTime(row.LastAttemptAt),
		RefreshPending:       row.InflightAt.Valid && !row.InflightAt.Time.IsZero(),
		InflightAt:           ptrTime(row.InflightAt),
	}
	if !row.HeadCommit.Valid {
		status.HeadCommit = ""
	}
	if !row.CurrentBranch.Valid {
		status.CurrentBranch = ""
	}
	if !row.RepoState.Valid {
		status.RepoState = ""
	}
	if !row.SampledNodeID.Valid {
		status.SampledNodeID = ""
	}
	if !row.SampleError.Valid {
		status.SampleError = ""
	}
	if len(row.UncommittedEntries) > 0 {
		var entries []ProjectWorkspaceGitFileEntry
		if err := json.Unmarshal(row.UncommittedEntries, &entries); err == nil {
			status.UncommittedEntries = entries
		}
	}
	return status
}

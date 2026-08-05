package project

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

// PgCastingRepository implements CastingRepository against sqlc queries.
type PgCastingRepository struct {
	q  *queries.Queries
	db projectTransactionBeginner
}

func NewPgCastingRepository(q *queries.Queries, db ...projectTransactionBeginner) *PgCastingRepository {
	var beginner projectTransactionBeginner
	if len(db) > 0 {
		beginner = db[0]
	}
	return &PgCastingRepository{q: q, db: beginner}
}

func (r *PgCastingRepository) ListProjectCastings(ctx context.Context, tenantID, projectID uuid.UUID, templateKey *string) ([]CastingEntry, error) {
	var key pgtype.Text
	if templateKey != nil && strings.TrimSpace(*templateKey) != "" {
		key = pgtype.Text{String: strings.TrimSpace(*templateKey), Valid: true}
	}
	rows, err := r.q.ListProjectPlaybookCastings(ctx, queries.ListProjectPlaybookCastingsParams{
		TenantID:            tenantID,
		ProjectID:           projectID,
		ScenarioTemplateKey: key,
	})
	if err != nil {
		return nil, err
	}
	out := make([]CastingEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, castingFromRow(row))
	}
	return out, nil
}

func (r *PgCastingRepository) ReplaceProjectCasting(ctx context.Context, tenantID, projectID, actorUserID uuid.UUID, templateKey string, assignments []CastingAssignment) ([]CastingEntry, error) {
	run := func(q *queries.Queries) ([]CastingEntry, error) {
		if err := q.DeleteProjectPlaybookCastingsByTemplate(ctx, queries.DeleteProjectPlaybookCastingsByTemplateParams{
			TenantID:            tenantID,
			ProjectID:           projectID,
			ScenarioTemplateKey: templateKey,
		}); err != nil {
			return nil, err
		}
		out := make([]CastingEntry, 0, len(assignments))
		for _, a := range assignments {
			row, err := q.InsertProjectPlaybookCasting(ctx, queries.InsertProjectPlaybookCastingParams{
				ID:                  uuid.New(),
				TenantID:            tenantID,
				ProjectID:           projectID,
				ScenarioTemplateKey: templateKey,
				RoleKey:             a.RoleKey,
				DigitalEmployeeID:   a.DigitalEmployeeID,
				CastByUserID:        actorUserID,
			})
			if err != nil {
				return nil, err
			}
			out = append(out, castingFromRow(row))
		}
		return out, nil
	}
	if r.db == nil {
		return run(r.q)
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin casting replace transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	out, err := run(r.q.WithTx(tx))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit casting replace transaction: %w", err)
	}
	return out, nil
}

func (r *PgCastingRepository) CountCastingsForEmployee(ctx context.Context, tenantID, projectID, employeeID uuid.UUID) (int, error) {
	n, err := r.q.CountProjectPlaybookCastingsForEmployee(ctx, queries.CountProjectPlaybookCastingsForEmployeeParams{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DigitalEmployeeID: employeeID,
	})
	return int(n), err
}

func (r *PgCastingRepository) EnsureDigitalEmployeeMember(ctx context.Context, tenantID, projectID, employeeID uuid.UUID, displayName string) error {
	members, err := r.q.ListProjectMembers(ctx, queries.ListProjectMembersParams{
		TenantID:  tenantID,
		ProjectID: projectID,
	})
	if err != nil {
		return err
	}
	for _, m := range members {
		if m.PrincipalType == string(PrincipalTypeDigitalEmployee) && m.PrincipalID == employeeID && m.Status == "active" {
			return nil
		}
	}
	display := pgtype.Text{}
	if strings.TrimSpace(displayName) != "" {
		display = pgtype.Text{String: strings.TrimSpace(displayName), Valid: true}
	}
	_, err = r.q.CreateProjectMember(ctx, queries.CreateProjectMemberParams{
		TenantID:            tenantID,
		ProjectID:           projectID,
		PrincipalType:       string(PrincipalTypeDigitalEmployee),
		PrincipalID:         employeeID,
		ProjectRole:         string(ProjectRoleExecutor),
		DisplayNameSnapshot: display,
		Status:              "active",
		Settings:            []byte("{}"),
	})
	return err
}

func castingFromRow(row queries.ProjectPlaybookCasting) CastingEntry {
	return CastingEntry{
		ID:                  row.ID,
		TenantID:            row.TenantID,
		ProjectID:           row.ProjectID,
		ScenarioTemplateKey: row.ScenarioTemplateKey,
		RoleKey:             row.RoleKey,
		DigitalEmployeeID:   row.DigitalEmployeeID,
		CastByUserID:        row.CastByUserID,
		CreatedAt:           pgTimestamptz(row.CreatedAt),
		UpdatedAt:           pgTimestamptz(row.UpdatedAt),
	}
}

func pgTimestamptz(ts pgtype.Timestamptz) time.Time {
	if !ts.Valid {
		return time.Time{}
	}
	return ts.Time
}

package rolevocab

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

// roleKeyPattern: 下划线小写，与批一能力词表一致。
var roleKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// CastingCascade disables a vocabulary role and cascades castings in one transaction.
type CastingCascade interface {
	// DisableRoleWithCascade deletes castings for the role and applies the patch
	// (status→disabled and any title/description) in one DB transaction, then notifies.
	DisableRoleWithCascade(ctx context.Context, req PatchRequest) (Entry, error)
}

type Service struct {
	q              *queries.Queries
	castingCascade CastingCascade
}

func NewService(q *queries.Queries) *Service {
	return &Service{q: q}
}

func (s *Service) SetCastingCascade(c CastingCascade) {
	s.castingCascade = c
}

func (s *Service) List(ctx context.Context, tenantID uuid.UUID) ([]Entry, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	rows, err := s.q.ListRoleVocabulary(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(rows))
	for _, row := range rows {
		out = append(out, entryFromRow(row))
	}
	return out, nil
}

func (s *Service) ListActive(ctx context.Context, tenantID uuid.UUID) ([]Entry, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	rows, err := s.q.ListActiveRoleVocabulary(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(rows))
	for _, row := range rows {
		out = append(out, entryFromRow(row))
	}
	return out, nil
}

// ActiveKeys returns the subset of keys that are registered and active.
func (s *Service) ActiveKeys(ctx context.Context, tenantID uuid.UUID, keys []string) (map[string]bool, error) {
	unique := normalizeKeys(keys)
	if len(unique) == 0 {
		return map[string]bool{}, nil
	}
	rows, err := s.q.GetActiveRoleVocabularyByKeys(ctx, queries.GetActiveRoleVocabularyByKeysParams{
		TenantID: tenantID,
		RoleKeys: unique,
	})
	if err != nil {
		return nil, err
	}
	active := make(map[string]bool, len(rows))
	for _, row := range rows {
		active[row.RoleKey] = true
	}
	return active, nil
}

// UnknownKeys returns keys not registered as active.
func (s *Service) UnknownKeys(ctx context.Context, tenantID uuid.UUID, keys []string) ([]string, error) {
	unique := normalizeKeys(keys)
	if len(unique) == 0 {
		return nil, nil
	}
	active, err := s.ActiveKeys(ctx, tenantID, unique)
	if err != nil {
		return nil, err
	}
	var unknown []string
	for _, key := range unique {
		if !active[key] {
			unknown = append(unknown, key)
		}
	}
	return unknown, nil
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (Entry, error) {
	if req.TenantID == uuid.Nil {
		return Entry{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	key := strings.TrimSpace(req.RoleKey)
	if !roleKeyPattern.MatchString(key) {
		return Entry{}, fmt.Errorf("%w: role_key must be lowercase snake_case (got %q)", ErrInvalidInput, key)
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return Entry{}, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = StatusActive
	}
	if status != StatusActive && status != StatusDisabled {
		return Entry{}, fmt.Errorf("%w: status must be active or disabled", ErrInvalidInput)
	}
	row, err := s.q.CreateRoleVocabulary(ctx, queries.CreateRoleVocabularyParams{
		ID:          uuid.New(),
		TenantID:    req.TenantID,
		RoleKey:     key,
		Title:       title,
		Description: pgtype.Text{String: strings.TrimSpace(req.Description), Valid: true},
		Status:      pgtype.Text{String: status, Valid: true},
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Entry{}, fmt.Errorf("%w: role_key %q already exists", ErrConflict, key)
		}
		return Entry{}, err
	}
	return entryFromRow(row), nil
}

// GetReferences returns who would be affected if roleKey were disabled:
// templates whose current spec lists the role, employees holding it, and
// existing casting rows. roleKey must exist (any status); missing → ErrNotFound.
func (s *Service) GetReferences(ctx context.Context, tenantID uuid.UUID, roleKey string) (References, error) {
	if tenantID == uuid.Nil {
		return References{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	key := strings.TrimSpace(roleKey)
	if key == "" {
		return References{}, fmt.Errorf("%w: role_key is required", ErrInvalidInput)
	}
	if _, err := s.q.GetRoleVocabularyByKey(ctx, queries.GetRoleVocabularyByKeyParams{
		TenantID: tenantID,
		RoleKey:  key,
	}); err != nil {
		if errorsIsNoRows(err) {
			return References{}, ErrNotFound
		}
		return References{}, err
	}

	templates, err := s.q.ListScenarioTemplatesReferencingRole(ctx, queries.ListScenarioTemplatesReferencingRoleParams{
		TenantID: tenantID,
		RoleKey:  key,
	})
	if err != nil {
		return References{}, err
	}
	employees, err := s.q.ListEmployeesHoldingRole(ctx, queries.ListEmployeesHoldingRoleParams{
		TenantID: tenantID,
		RoleKey:  key,
	})
	if err != nil {
		return References{}, err
	}
	castingCount, err := s.q.CountCastingsForRole(ctx, queries.CountCastingsForRoleParams{
		TenantID: tenantID,
		RoleKey:  key,
	})
	if err != nil {
		return References{}, err
	}

	out := References{
		ScenarioTemplates: make([]TemplateRef, 0, len(templates)),
		Employees:         make([]EmployeeRef, 0, len(employees)),
		EmployeeCount:     len(employees),
		CastingCount:      int(castingCount),
	}
	for _, t := range templates {
		out.ScenarioTemplates = append(out.ScenarioTemplates, TemplateRef{Key: t.TemplateKey, Name: t.Name})
	}
	for _, e := range employees {
		out.Employees = append(out.Employees, EmployeeRef{ID: e.ID, Name: e.Name})
	}
	return out, nil
}

func (s *Service) Patch(ctx context.Context, req PatchRequest) (Entry, error) {
	if req.TenantID == uuid.Nil {
		return Entry{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	key := strings.TrimSpace(req.RoleKey)
	if key == "" {
		return Entry{}, fmt.Errorf("%w: role_key is required", ErrInvalidInput)
	}
	var title, description, status pgtype.Text
	if req.Title != nil {
		t := strings.TrimSpace(*req.Title)
		if t == "" {
			return Entry{}, fmt.Errorf("%w: title must not be blank", ErrInvalidInput)
		}
		title = pgtype.Text{String: t, Valid: true}
	}
	if req.Description != nil {
		description = pgtype.Text{String: strings.TrimSpace(*req.Description), Valid: true}
	}
	disabling := false
	if req.Status != nil {
		st := strings.TrimSpace(*req.Status)
		if st != StatusActive && st != StatusDisabled {
			return Entry{}, fmt.Errorf("%w: status must be active or disabled", ErrInvalidInput)
		}
		status = pgtype.Text{String: st, Valid: true}
		disabling = st == StatusDisabled
	}

	if disabling {
		refs, err := s.GetReferences(ctx, req.TenantID, key)
		if err != nil {
			return Entry{}, err
		}
		if refs.CastingCount > 0 && !req.ConfirmImpact {
			return Entry{}, &ErrCastingImpactRequiresConfirm{CastingCount: refs.CastingCount, References: refs}
		}
		// Same transaction: casting deletes + vocabulary status/title/description.
		if refs.CastingCount > 0 && s.castingCascade != nil {
			return s.castingCascade.DisableRoleWithCascade(ctx, req)
		}
	}

	row, err := s.q.UpdateRoleVocabulary(ctx, queries.UpdateRoleVocabularyParams{
		TenantID:    req.TenantID,
		RoleKey:     key,
		Title:       title,
		Description: description,
		Status:      status,
	})
	if err != nil {
		if errorsIsNoRows(err) {
			return Entry{}, ErrNotFound
		}
		return Entry{}, err
	}
	return entryFromRow(row), nil
}

func entryFromRow(row queries.RoleVocabulary) Entry {
	return Entry{
		ID:          row.ID,
		TenantID:    row.TenantID,
		RoleKey:     row.RoleKey,
		Title:       row.Title,
		Description: row.Description,
		Status:      row.Status,
		CreatedAt:   pgTime(row.CreatedAt),
		UpdatedAt:   pgTime(row.UpdatedAt),
	}
}

func pgTime(ts pgtype.Timestamptz) time.Time {
	if !ts.Valid {
		return time.Time{}
	}
	return ts.Time
}

func normalizeKeys(keys []string) []string {
	seen := make(map[string]struct{}, len(keys))
	out := make([]string, 0, len(keys))
	for _, raw := range keys {
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key") || strings.Contains(msg, "unique constraint") || strings.Contains(msg, "uq_role_vocabulary")
}

func errorsIsNoRows(err error) bool {
	return err != nil && (err == pgx.ErrNoRows || strings.Contains(err.Error(), "no rows"))
}


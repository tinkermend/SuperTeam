package capability

import (
	"context"
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

func (r *PgRepository) CreateCredential(ctx context.Context, req CreateCredentialStoreRequest) (Credential, error) {
	if err := r.requireQueries(); err != nil {
		return Credential{}, err
	}
	credential, err := r.q.CreateUserCredential(ctx, queries.CreateUserCredentialParams{
		TenantID:       req.TenantID,
		UserID:         req.UserID,
		Name:           req.Name,
		CredentialType: string(req.CredentialType),
		EncryptedValue: req.EncryptedValue,
		LastFour:       req.LastFour,
		Metadata:       nil,
	})
	if err != nil {
		return Credential{}, err
	}
	return credentialFromQuery(credential), nil
}

func (r *PgRepository) ListCredentials(ctx context.Context, req ListCredentialsRequest) ([]Credential, error) {
	if err := r.requireQueries(); err != nil {
		return nil, err
	}
	rows, err := r.q.ListUserCredentials(ctx, queries.ListUserCredentialsParams{
		TenantID:       req.TenantID,
		UserID:         req.UserID,
		CredentialType: textFromCredentialType(req.CredentialType),
	})
	if err != nil {
		return nil, err
	}
	credentials := make([]Credential, 0, len(rows))
	for _, row := range rows {
		credentials = append(credentials, credentialFromQuery(row))
	}
	return credentials, nil
}

func (r *PgRepository) GetCredential(ctx context.Context, req ResolveCredentialRequest) (Credential, error) {
	if err := r.requireQueries(); err != nil {
		return Credential{}, err
	}
	credential, err := r.q.GetUserCredential(ctx, queries.GetUserCredentialParams{
		TenantID: req.TenantID,
		UserID:   req.UserID,
		ID:       req.CredentialID,
	})
	if err != nil {
		return Credential{}, mapNoRows(err)
	}
	return credentialFromQuery(credential), nil
}

func (r *PgRepository) CreateTeamMCPServer(ctx context.Context, req CreateTeamMCPServerRequest) (MCPServer, error) {
	if err := r.requireQueries(); err != nil {
		return MCPServer{}, err
	}
	server, err := r.q.CreateTeamMCPServer(ctx, queries.CreateTeamMCPServerParams{
		TenantID:     req.TenantID,
		TeamID:       req.TeamID,
		Name:         req.Name,
		Url:          req.URL,
		CredentialID: nullUUIDFromPtr(req.CredentialID),
		Metadata:     nil,
		CreatedBy:    nullUUIDFromValue(req.UserID),
	})
	if err != nil {
		return MCPServer{}, err
	}
	return teamMCPServerFromQuery(server), nil
}

func (r *PgRepository) ListTeamMCPServers(ctx context.Context, req TeamScopedRequest) ([]MCPServer, error) {
	if err := r.requireQueries(); err != nil {
		return nil, err
	}
	rows, err := r.q.ListTeamMCPServers(ctx, queries.ListTeamMCPServersParams{
		TenantID: req.TenantID,
		TeamID:   req.TeamID,
	})
	if err != nil {
		return nil, err
	}
	servers := make([]MCPServer, 0, len(rows))
	for _, row := range rows {
		servers = append(servers, teamMCPServerFromListRow(row))
	}
	return servers, nil
}

func (r *PgRepository) DeleteTeamMCPServer(ctx context.Context, req DeleteTeamMCPServerRequest) error {
	if err := r.requireQueries(); err != nil {
		return err
	}
	return r.q.DeleteTeamMCPServer(ctx, queries.DeleteTeamMCPServerParams{
		TenantID: req.TenantID,
		TeamID:   req.TeamID,
		ID:       req.ServerID,
	})
}

func (r *PgRepository) CreateEmployeeMCPBinding(ctx context.Context, req CreateEmployeeMCPBindingRequest) (MCPServer, error) {
	if err := r.requireQueries(); err != nil {
		return MCPServer{}, err
	}
	server, err := r.q.CreateDigitalEmployeeMCPBinding(ctx, queries.CreateDigitalEmployeeMCPBindingParams{
		TenantID:          req.TenantID,
		DigitalEmployeeID: req.DigitalEmployeeID,
		Name:              req.Name,
		Url:               req.URL,
		CredentialID:      nullUUIDFromPtr(req.CredentialID),
		Metadata:          nil,
		CreatedBy:         nullUUIDFromValue(req.UserID),
	})
	if err != nil {
		return MCPServer{}, err
	}
	return employeeMCPServerFromQuery(server), nil
}

func (r *PgRepository) ListEmployeeMCPBindings(ctx context.Context, req EmployeeScopedRequest) ([]MCPServer, error) {
	if err := r.requireQueries(); err != nil {
		return nil, err
	}
	rows, err := r.q.ListDigitalEmployeeMCPBindings(ctx, queries.ListDigitalEmployeeMCPBindingsParams{
		TenantID:          req.TenantID,
		DigitalEmployeeID: req.DigitalEmployeeID,
	})
	if err != nil {
		return nil, err
	}
	servers := make([]MCPServer, 0, len(rows))
	for _, row := range rows {
		servers = append(servers, employeeMCPServerFromListRow(row))
	}
	return servers, nil
}

func (r *PgRepository) DeleteEmployeeMCPBinding(ctx context.Context, req DeleteEmployeeMCPBindingRequest) error {
	if err := r.requireQueries(); err != nil {
		return err
	}
	return r.q.DeleteDigitalEmployeeMCPBinding(ctx, queries.DeleteDigitalEmployeeMCPBindingParams{
		TenantID:          req.TenantID,
		DigitalEmployeeID: req.DigitalEmployeeID,
		ID:                req.BindingID,
	})
}

func (r *PgRepository) ListEffectiveMCPServers(ctx context.Context, req EmployeeScopedRequest) ([]MCPServer, error) {
	if err := r.requireQueries(); err != nil {
		return nil, err
	}
	rows, err := r.q.ListEffectiveMCPServersForEmployee(ctx, queries.ListEffectiveMCPServersForEmployeeParams{
		TenantID:          req.TenantID,
		DigitalEmployeeID: req.DigitalEmployeeID,
	})
	if err != nil {
		return nil, err
	}
	servers := make([]MCPServer, 0, len(rows))
	for _, row := range rows {
		servers = append(servers, effectiveMCPServerFromRow(row))
	}
	return servers, nil
}

func (r *PgRepository) requireQueries() error {
	if r == nil || r.q == nil {
		return fmt.Errorf("%w: postgres queries are required", ErrInvalidInput)
	}
	return nil
}

func credentialFromQuery(row queries.UserCredential) Credential {
	return Credential{
		ID:             row.ID,
		TenantID:       row.TenantID,
		UserID:         row.UserID,
		Name:           row.Name,
		CredentialType: CredentialType(row.CredentialType),
		EncryptedValue: row.EncryptedValue,
		LastFour:       row.LastFour,
		Status:         row.Status,
		DisabledAt:     timeFromTimestamptz(row.DisabledAt),
		DeletedAt:      timeFromTimestamptz(row.DeletedAt),
		CreatedAt:      timeFromTimestamptz(row.CreatedAt),
		UpdatedAt:      timeFromTimestamptz(row.UpdatedAt),
	}
}

func teamMCPServerFromQuery(row queries.TeamMcpServer) MCPServer {
	return MCPServer{
		ID:           row.ID,
		TenantID:     row.TenantID,
		TeamID:       &row.TeamID,
		Name:         row.Name,
		URL:          row.Url,
		CredentialID: uuidPtrFromNull(row.CredentialID),
		Status:       row.Status,
		SourceScope:  "team",
		Inherited:    true,
		CreatedBy:    uuidPtrFromNull(row.CreatedBy),
		DisabledAt:   timeFromTimestamptz(row.DisabledAt),
		DeletedAt:    timeFromTimestamptz(row.DeletedAt),
		CreatedAt:    timeFromTimestamptz(row.CreatedAt),
		UpdatedAt:    timeFromTimestamptz(row.UpdatedAt),
	}
}

func teamMCPServerFromListRow(row queries.ListTeamMCPServersRow) MCPServer {
	return MCPServer{
		ID:                 row.ID,
		TenantID:           row.TenantID,
		TeamID:             &row.TeamID,
		Name:               row.Name,
		URL:                row.Url,
		CredentialID:       uuidPtrFromNull(row.CredentialID),
		CredentialName:     row.CredentialName,
		CredentialType:     CredentialType(row.CredentialType),
		CredentialLastFour: row.CredentialLastFour,
		Status:             row.Status,
		SourceScope:        "team",
		Inherited:          true,
		CreatedBy:          uuidPtrFromNull(row.CreatedBy),
		DisabledAt:         timeFromTimestamptz(row.DisabledAt),
		DeletedAt:          timeFromTimestamptz(row.DeletedAt),
		CreatedAt:          timeFromTimestamptz(row.CreatedAt),
		UpdatedAt:          timeFromTimestamptz(row.UpdatedAt),
	}
}

func employeeMCPServerFromQuery(row queries.DigitalEmployeeMcpBinding) MCPServer {
	return MCPServer{
		ID:                row.ID,
		TenantID:          row.TenantID,
		DigitalEmployeeID: &row.DigitalEmployeeID,
		Name:              row.Name,
		URL:               row.Url,
		CredentialID:      uuidPtrFromNull(row.CredentialID),
		Status:            row.Status,
		SourceScope:       "employee",
		Inherited:         false,
		CreatedBy:         uuidPtrFromNull(row.CreatedBy),
		DisabledAt:        timeFromTimestamptz(row.DisabledAt),
		DeletedAt:         timeFromTimestamptz(row.DeletedAt),
		CreatedAt:         timeFromTimestamptz(row.CreatedAt),
		UpdatedAt:         timeFromTimestamptz(row.UpdatedAt),
	}
}

func employeeMCPServerFromListRow(row queries.ListDigitalEmployeeMCPBindingsRow) MCPServer {
	return MCPServer{
		ID:                 row.ID,
		TenantID:           row.TenantID,
		DigitalEmployeeID:  &row.DigitalEmployeeID,
		Name:               row.Name,
		URL:                row.Url,
		CredentialID:       uuidPtrFromNull(row.CredentialID),
		CredentialName:     row.CredentialName,
		CredentialType:     CredentialType(row.CredentialType),
		CredentialLastFour: row.CredentialLastFour,
		Status:             row.Status,
		SourceScope:        "employee",
		Inherited:          false,
		CreatedBy:          uuidPtrFromNull(row.CreatedBy),
		DisabledAt:         timeFromTimestamptz(row.DisabledAt),
		DeletedAt:          timeFromTimestamptz(row.DeletedAt),
		CreatedAt:          timeFromTimestamptz(row.CreatedAt),
		UpdatedAt:          timeFromTimestamptz(row.UpdatedAt),
	}
}

func effectiveMCPServerFromRow(row queries.ListEffectiveMCPServersForEmployeeRow) MCPServer {
	return MCPServer{
		ID:                 row.ID,
		TenantID:           row.TenantID,
		TeamID:             uuidPtrFromNull(row.TeamID),
		DigitalEmployeeID:  &row.DigitalEmployeeID,
		Name:               row.Name,
		URL:                row.Url,
		CredentialID:       uuidPtrFromNull(row.CredentialID),
		CredentialName:     row.CredentialName,
		CredentialType:     CredentialType(row.CredentialType),
		CredentialLastFour: row.CredentialLastFour,
		Status:             row.Status,
		SourceScope:        row.SourceScope,
		Inherited:          row.Inherited,
		CreatedAt:          timeFromTimestamptz(row.CreatedAt),
		UpdatedAt:          timeFromTimestamptz(row.UpdatedAt),
	}
}

func textFromCredentialType(value CredentialType) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: string(value), Valid: true}
}

func nullUUIDFromPtr(value *uuid.UUID) uuid.NullUUID {
	if value == nil || *value == uuid.Nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *value, Valid: true}
}

func nullUUIDFromValue(value uuid.UUID) uuid.NullUUID {
	if value == uuid.Nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: value, Valid: true}
}

func uuidPtrFromNull(value uuid.NullUUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	return &value.UUID
}

func timeFromTimestamptz(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func mapNoRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

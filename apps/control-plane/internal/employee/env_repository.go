package employee

import (
	"context"

	"github.com/google/uuid"
)

type EnvironmentVariableRepository interface {
	ListEnvironmentVariables(ctx context.Context, req ListEnvironmentVariablesRequest) ([]EnvironmentVariableRecord, error)
	UpsertEnvironmentVariable(ctx context.Context, req UpsertEnvironmentVariableStoreRequest) (EnvironmentVariableRecord, error)
	DeleteEnvironmentVariable(ctx context.Context, req DeleteEnvironmentVariableRequest) error
	ListRuntimeEnvironmentVariables(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]EnvironmentVariableRecord, error)
}

type UpsertEnvironmentVariableStoreRequest struct {
	TenantID          uuid.UUID
	TeamID            *uuid.UUID
	DigitalEmployeeID uuid.UUID
	Name              string
	EncryptedValue    string
	EncryptionKeyID   string
	ValueFingerprint  string
	Sensitive         bool
	UpdatedBy         *uuid.UUID
}

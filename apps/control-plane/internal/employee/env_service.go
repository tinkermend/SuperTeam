package employee

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (s *Service) SetEnvironmentCodec(codec *EnvironmentValueCodec) {
	s.envCodec = codec
}

func (s *Service) ListEnvironmentVariables(ctx context.Context, req ListEnvironmentVariablesRequest) ([]EnvironmentVariableSummary, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	if _, err := s.repository.GetDigitalEmployee(ctx, req.TenantID, req.DigitalEmployeeID); err != nil {
		return nil, fmt.Errorf("get digital employee: %w", err)
	}
	records, err := s.repository.ListEnvironmentVariables(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("list environment variables: %w", err)
	}
	summaries := make([]EnvironmentVariableSummary, 0, len(records))
	for _, record := range records {
		summaries = append(summaries, environmentSummaryFromRecord(record))
	}
	return summaries, nil
}

func (s *Service) UpsertEnvironmentVariable(ctx context.Context, req UpsertEnvironmentVariableRequest) (EnvironmentVariableSummary, error) {
	if req.TenantID == uuid.Nil {
		return EnvironmentVariableSummary{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return EnvironmentVariableSummary{}, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	name, err := normalizeEnvName(req.Name)
	if err != nil {
		return EnvironmentVariableSummary{}, err
	}
	employee, err := s.repository.GetDigitalEmployee(ctx, req.TenantID, req.DigitalEmployeeID)
	if err != nil {
		return EnvironmentVariableSummary{}, fmt.Errorf("get digital employee: %w", err)
	}
	record, err := s.upsertEncryptedEnvironmentVariable(ctx, s.repository, UpsertEnvironmentVariableStoreInput{
		TenantID:          req.TenantID,
		TeamID:            employee.TeamID,
		DigitalEmployeeID: req.DigitalEmployeeID,
		Name:              name,
		Value:             req.Value,
		Sensitive:         req.Sensitive,
		UpdatedBy:         req.ActorUserID,
	})
	if err != nil {
		return EnvironmentVariableSummary{}, err
	}
	return environmentSummaryFromRecord(record), nil
}

func (s *Service) DeleteEnvironmentVariable(ctx context.Context, req DeleteEnvironmentVariableRequest) error {
	if req.TenantID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	name, err := normalizeEnvName(req.Name)
	if err != nil {
		return err
	}
	if _, err := s.repository.GetDigitalEmployee(ctx, req.TenantID, req.DigitalEmployeeID); err != nil {
		return fmt.Errorf("get digital employee: %w", err)
	}
	req.Name = name
	if err := s.repository.DeleteEnvironmentVariable(ctx, req); err != nil {
		return fmt.Errorf("delete environment variable: %w", err)
	}
	return nil
}

func (s *Service) ListRuntimeEnvironmentVariablesForRuntime(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]RuntimeEnvironmentVariablePayload, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if digitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	if s.envCodec == nil {
		return nil, fmt.Errorf("%w: environment encryption codec is required", ErrInvalidInput)
	}
	records, err := s.repository.ListRuntimeEnvironmentVariables(ctx, tenantID, digitalEmployeeID)
	if err != nil {
		return nil, fmt.Errorf("list runtime environment variables: %w", err)
	}
	payload := make([]RuntimeEnvironmentVariablePayload, 0, len(records))
	for _, record := range records {
		value, err := s.envCodec.Decrypt(record.Name, EnvironmentEncryptedValue{
			KeyID:          record.EncryptionKeyID,
			EncryptedValue: record.EncryptedValue,
			Fingerprint:    record.ValueFingerprint,
		})
		if err != nil {
			return nil, fmt.Errorf("decrypt environment variable %s: %w", record.Name, err)
		}
		payload = append(payload, RuntimeEnvironmentVariablePayload{
			Name:      record.Name,
			Value:     value,
			Sensitive: record.Sensitive,
		})
	}
	return payload, nil
}

type UpsertEnvironmentVariableStoreInput struct {
	TenantID          uuid.UUID
	TeamID            *uuid.UUID
	DigitalEmployeeID uuid.UUID
	Name              string
	Value             string
	Sensitive         bool
	UpdatedBy         *uuid.UUID
}

func (s *Service) upsertEncryptedEnvironmentVariable(ctx context.Context, repository Repository, input UpsertEnvironmentVariableStoreInput) (EnvironmentVariableRecord, error) {
	if s.envCodec == nil {
		return EnvironmentVariableRecord{}, fmt.Errorf("%w: environment encryption codec is required", ErrInvalidInput)
	}
	encrypted, err := s.envCodec.Encrypt(input.Name, input.Value)
	if err != nil {
		return EnvironmentVariableRecord{}, fmt.Errorf("encrypt environment variable: %w", err)
	}
	record, err := repository.UpsertEnvironmentVariable(ctx, UpsertEnvironmentVariableStoreRequest{
		TenantID:          input.TenantID,
		TeamID:            input.TeamID,
		DigitalEmployeeID: input.DigitalEmployeeID,
		Name:              input.Name,
		EncryptedValue:    encrypted.EncryptedValue,
		EncryptionKeyID:   encrypted.KeyID,
		ValueFingerprint:  encrypted.Fingerprint,
		Sensitive:         input.Sensitive,
		UpdatedBy:         input.UpdatedBy,
	})
	if err != nil {
		return EnvironmentVariableRecord{}, fmt.Errorf("upsert environment variable: %w", err)
	}
	return record, nil
}

func normalizeEnvName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !envNamePattern.MatchString(name) {
		return "", fmt.Errorf("%w: invalid environment variable name", ErrInvalidInput)
	}
	return name, nil
}

func environmentSummaryFromRecord(record EnvironmentVariableRecord) EnvironmentVariableSummary {
	return EnvironmentVariableSummary{
		ID:                record.ID,
		TenantID:          record.TenantID,
		TeamID:            record.TeamID,
		DigitalEmployeeID: record.DigitalEmployeeID,
		Name:              record.Name,
		Configured:        record.EncryptedValue != "",
		Fingerprint:       record.ValueFingerprint,
		Sensitive:         record.Sensitive,
		Status:            record.Status,
		UpdatedAt:         record.UpdatedAt,
	}
}

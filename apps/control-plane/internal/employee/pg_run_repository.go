package employee

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type PgRunRepository struct {
	q *queries.Queries
}

func NewPgRunRepository(q *queries.Queries) DigitalEmployeeRunRepository {
	return &PgRunRepository{q: q}
}

func (r *PgRunRepository) GetRunPreflight(ctx context.Context, tenantID, employeeID uuid.UUID) (RunPreflight, error) {
	preflight, err := r.q.GetDigitalEmployeeRunPreflight(ctx, queries.GetDigitalEmployeeRunPreflightParams{
		DigitalEmployeeID: employeeID,
		TenantID:          tenantID,
	})
	if err != nil {
		return RunPreflight{}, mapNoRows(err)
	}

	return runPreflightFromQuery(preflight)
}

func runPreflightFromQuery(preflight queries.GetDigitalEmployeeRunPreflightRow) (RunPreflight, error) {
	if !preflight.TeamID.Valid {
		return RunPreflight{}, fmt.Errorf("%w: digital employee team_id is required for run preflight", ErrInvalidInput)
	}

	runtimeSelector, err := mapFromJSONB(preflight.RuntimeSelector, "runtime_selector")
	if err != nil {
		return RunPreflight{}, err
	}
	sessionPolicy, err := mapFromJSONB(preflight.SessionPolicy, "session_policy")
	if err != nil {
		return RunPreflight{}, err
	}
	workspacePolicy, err := mapFromJSONB(preflight.WorkspacePolicy, "workspace_policy")
	if err != nil {
		return RunPreflight{}, err
	}

	return RunPreflight{
		TenantID:                   preflight.TenantID,
		TeamID:                     preflight.TeamID.UUID,
		DigitalEmployeeID:          preflight.DigitalEmployeeID,
		DigitalEmployeeStatus:      DigitalEmployeeStatus(preflight.DigitalEmployeeStatus),
		ExecutionInstanceID:        preflight.ExecutionInstanceID,
		ExecutionStatus:            ExecutionInstanceStatus(preflight.ExecutionStatus),
		RuntimeNodeID:              preflight.RuntimeNodeID,
		NodeID:                     preflight.NodeID,
		ProviderType:               preflight.ProviderType,
		AgentHomeDir:               preflight.AgentHomeDir,
		RuntimeSelector:            runtimeSelector,
		SessionPolicy:              sessionPolicy,
		WorkspacePolicy:            workspacePolicy,
		HasApprovedEffectiveConfig: preflight.HasApprovedEffectiveConfig,
		ProviderHealthy:            preflight.ProviderHealthy,
	}, nil
}

func (r *PgRunRepository) GetActiveRun(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployeeRun, error) {
	run, err := r.q.GetActiveDigitalEmployeeRun(ctx, queries.GetActiveDigitalEmployeeRunParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return digitalEmployeeRunFromQuery(run), nil
}

func (r *PgRunRepository) GetRun(ctx context.Context, tenantID, employeeID, runID uuid.UUID) (*DigitalEmployeeRun, error) {
	run, err := r.q.GetDigitalEmployeeRun(ctx, queries.GetDigitalEmployeeRunParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		RunID:             runID,
	})
	if err != nil {
		return nil, mapNoRows(err)
	}
	return digitalEmployeeRunFromQuery(run), nil
}

func (r *PgRunRepository) GetRunByCommandID(ctx context.Context, tenantID uuid.UUID, commandID string) (*DigitalEmployeeRun, error) {
	run, err := r.q.GetDigitalEmployeeRunByCommandID(ctx, queries.GetDigitalEmployeeRunByCommandIDParams{
		TenantID:  tenantID,
		CommandID: commandID,
	})
	if err != nil {
		return nil, mapNoRows(err)
	}
	return digitalEmployeeRunFromQuery(run), nil
}

func (r *PgRunRepository) ListRuns(ctx context.Context, tenantID, employeeID uuid.UUID, limit, offset int32) ([]*DigitalEmployeeRun, error) {
	runs, err := r.q.ListDigitalEmployeeRuns(ctx, queries.ListDigitalEmployeeRunsParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		Offset:            offset,
		Limit:             limit,
	})
	if err != nil {
		return nil, err
	}

	out := make([]*DigitalEmployeeRun, 0, len(runs))
	for _, run := range runs {
		out = append(out, digitalEmployeeRunFromQuery(run))
	}
	return out, nil
}

func (r *PgRunRepository) CreateRun(ctx context.Context, req CreateRunRecordRequest) (*DigitalEmployeeRun, error) {
	if req.TeamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required for digital employee run", ErrInvalidInput)
	}

	params, err := jsonBytesFromMap(req.Params, "params")
	if err != nil {
		return nil, err
	}

	row, err := r.q.CreateDigitalEmployeeTaskRun(ctx, queries.CreateDigitalEmployeeTaskRunParams{
		IdempotencyKey:         textFromPtr(req.IdempotencyKey),
		IdempotencyFingerprint: textFromPtr(req.IdempotencyFingerprint),
		TenantID:               req.TenantID,
		DigitalEmployeeID:      req.DigitalEmployeeID,
		TeamID:                 req.TeamID,
		Title:                  req.Title,
		Description:            textFromPtr(req.Description),
		Priority:               req.Priority,
		ProviderType:           req.ProviderType,
		CreatorID:              nullUUIDFromPtr(req.CreatorID),
		TargetNodeID:           req.TargetNodeID,
		WorkspacePath:          textFromPtr(req.WorkspacePath),
		Params:                 params,
		RiskLevel:              textFromPtr(req.RiskLevel),
		NodeID:                 req.NodeID,
		RuntimeNodeID:          req.RuntimeNodeID,
		ProviderSessionID:      textFromPtr(req.ProviderSessionID),
		RunStatus:              string(req.RunStatus),
		CommandID:              req.CommandID,
		ExecutionInstanceID:    req.ExecutionInstanceID,
		TimeoutSec:             int4FromPtr(req.TimeoutSec),
		GraceSec:               int4FromPtr(req.GraceSec),
	})
	if err != nil {
		return nil, mapCreateRunError(err, req)
	}

	return r.GetRun(ctx, req.TenantID, req.DigitalEmployeeID, row.RunID)
}

func mapCreateRunError(err error, req CreateRunRecordRequest) error {
	if errors.Is(err, pgx.ErrNoRows) && req.IdempotencyKey != nil {
		return fmt.Errorf("%w: idempotency fingerprint mismatch", ErrConflict)
	}
	return err
}

func (r *PgRunRepository) UpdateRunStatus(ctx context.Context, req UpdateRunStatusRequest) (*DigitalEmployeeRun, error) {
	result, err := nullableJSONBytesFromMap(req.Result, "result")
	if err != nil {
		return nil, err
	}
	diagnostic, err := nullableJSONBytesFromMap(req.Diagnostic, "diagnostic")
	if err != nil {
		return nil, err
	}
	workProducts, err := nullableJSONBytesFromWorkProducts(req.WorkProducts, "work_products")
	if err != nil {
		return nil, err
	}
	sessionState, err := nullableJSONBytesFromMap(req.SessionState, "session_state")
	if err != nil {
		return nil, err
	}

	run, err := r.q.UpdateDigitalEmployeeRunStatus(ctx, queries.UpdateDigitalEmployeeRunStatusParams{
		Status:                    string(req.Status),
		Result:                    result,
		ErrorMessage:              textFromPtr(req.ErrorMessage),
		Diagnostic:                diagnostic,
		LogRef:                    textFromPtr(req.LogRef),
		RawResultRef:              textFromPtr(req.RawResultRef),
		WorkProducts:              workProducts,
		SessionState:              sessionState,
		ErrorCode:                 textFromPtr(req.ErrorCode),
		ErrorFamily:               textFromPtr(req.ErrorFamily),
		ExitCode:                  int4FromPtr(req.ExitCode),
		Signal:                    textFromPtr(req.Signal),
		ProviderSessionExternalID: textFromPtr(req.ProviderSessionExternalID),
		TenantID:                  req.TenantID,
		RunID:                     req.RunID,
	})
	if err != nil {
		return nil, mapNoRows(err)
	}
	return digitalEmployeeRunFromQuery(run), nil
}

func (r *PgRunRepository) CreateTaskEventIfAbsent(ctx context.Context, req CreateRunEventRecordRequest) error {
	payload, err := jsonBytesFromMap(redactRuntimeEventPayload(req.Payload), "payload")
	if err != nil {
		return err
	}
	metadata, err := jsonBytesFromMap(req.Metadata, "metadata")
	if err != nil {
		return err
	}

	_, err = r.q.CreateTaskEventIfAbsent(ctx, queries.CreateTaskEventIfAbsentParams{
		TenantID:       req.TenantID,
		TaskID:         req.TaskID,
		RunID:          req.RunID,
		EventType:      req.EventType,
		SequenceNumber: req.SequenceNumber,
		Payload:        payload,
		CommandID:      textFromPtr(req.CommandID),
		RawEventRef:    textFromPtr(req.RawEventRef),
		LogRef:         textFromPtr(req.LogRef),
		Metadata:       metadata,
	})
	return err
}

func (r *PgRunRepository) UpsertProviderSession(ctx context.Context, req UpsertProviderSessionRequest) (uuid.UUID, error) {
	sessionParams, err := jsonBytesFromMap(req.SessionParams, "session_params")
	if err != nil {
		return uuid.Nil, err
	}
	sessionState, err := jsonBytesFromMap(req.SessionState, "session_state")
	if err != nil {
		return uuid.Nil, err
	}
	metadata, err := jsonBytesFromMap(req.Metadata, "metadata")
	if err != nil {
		return uuid.Nil, err
	}

	session, err := r.q.UpsertProviderSessionByExternalID(ctx, queries.UpsertProviderSessionByExternalIDParams{
		TenantID:            req.TenantID,
		ProviderSessionID:   req.ProviderSessionID,
		DigitalEmployeeID:   req.DigitalEmployeeID,
		ExecutionInstanceID: req.ExecutionInstanceID,
		RuntimeNodeID:       req.RuntimeNodeID,
		ProviderType:        req.ProviderType,
		Status:              req.Status,
		Recoverable:         req.Recoverable,
		SessionDisplayID:    textFromPtr(req.SessionDisplayID),
		SessionParams:       sessionParams,
		SessionState:        sessionState,
		LastSequenceNumber:  req.LastSequenceNumber,
		LastCommandID:       textFromPtr(req.LastCommandID),
		LastRunID:           nullUUIDFromPtr(req.LastRunID),
		LastErrorFamily:     textFromPtr(req.LastErrorFamily),
		Metadata:            metadata,
	})
	if err != nil {
		return uuid.Nil, err
	}
	return session.ID, nil
}

func (r *PgRunRepository) CreateProviderSessionEventIfAbsent(ctx context.Context, req CreateProviderSessionEventRecordRequest) error {
	payload, err := jsonBytesFromMap(redactRuntimeEventPayload(req.Payload), "payload")
	if err != nil {
		return err
	}
	sessionStatePatch, err := jsonBytesFromMap(req.SessionStatePatch, "session_state_patch")
	if err != nil {
		return err
	}
	metadata, err := jsonBytesFromMap(req.Metadata, "metadata")
	if err != nil {
		return err
	}

	_, err = r.q.CreateProviderSessionEventIfAbsent(ctx, queries.CreateProviderSessionEventIfAbsentParams{
		EventType:           req.EventType,
		SequenceNumber:      req.SequenceNumber,
		Payload:             payload,
		RequestID:           textFromPtr(req.RequestID),
		CommandID:           textFromPtr(req.CommandID),
		RawEventRef:         textFromPtr(req.RawEventRef),
		LogRef:              textFromPtr(req.LogRef),
		SessionStatePatch:   sessionStatePatch,
		Metadata:            metadata,
		ProviderSessionUuid: req.ProviderSessionUUID,
		TenantID:            req.TenantID,
	})
	return err
}

func (r *PgRunRepository) CreateCommandReceipt(ctx context.Context, req CreateRuntimeCommandReceiptRequest) error {
	payload, err := jsonBytesFromMap(req.Payload, "payload")
	if err != nil {
		return err
	}
	_, err = r.q.CreateRuntimeCommandReceipt(ctx, queries.CreateRuntimeCommandReceiptParams{
		TenantID:      req.TenantID,
		CommandID:     req.CommandID,
		CommandType:   req.CommandType,
		RuntimeNodeID: req.RuntimeNodeID,
		NodeID:        req.NodeID,
		ResourceType:  req.ResourceType,
		ResourceID:    req.ResourceID,
		Status:        req.Status,
		Payload:       payload,
		DispatchedAt:  timestamptzFromPtr(req.DispatchedAt),
	})
	return err
}

func (r *PgRunRepository) GetCommandReceipt(ctx context.Context, tenantID uuid.UUID, commandID string) (*RuntimeCommandReceipt, error) {
	receipt, err := r.q.GetRuntimeCommandReceiptByCommandID(ctx, queries.GetRuntimeCommandReceiptByCommandIDParams{
		TenantID:  tenantID,
		CommandID: commandID,
	})
	if err != nil {
		return nil, mapNoRows(err)
	}
	return runtimeCommandReceiptFromQuery(receipt), nil
}

func (r *PgRunRepository) UpdateCommandReceipt(ctx context.Context, req UpdateRuntimeCommandReceiptRequest) (*RuntimeCommandReceipt, error) {
	result, err := nullableJSONBytesFromMap(req.Result, "result")
	if err != nil {
		return nil, err
	}

	receipt, err := r.q.UpdateRuntimeCommandReceiptStatus(ctx, queries.UpdateRuntimeCommandReceiptStatusParams{
		Status:       req.Status,
		Result:       result,
		ErrorMessage: textFromPtr(req.ErrorMessage),
		TenantID:     req.TenantID,
		CommandID:    req.CommandID,
	})
	if err != nil {
		return nil, mapNoRows(err)
	}
	return runtimeCommandReceiptFromQuery(receipt), nil
}

func runStatusFromString(value string) DigitalEmployeeRunStatus {
	return DigitalEmployeeRunStatus(value)
}

func jsonMapFromBytes(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{"_decode_error": err.Error()}
	}
	if out == nil {
		return map[string]any{}
	}
	return out
}

func redactRuntimeEventPayload(payload map[string]any) map[string]any {
	blocked := map[string]struct{}{
		"authorization": {},
		"password":      {},
		"secret":        {},
		"token":         {},
		"access_token":  {},
		"refresh_token": {},
		"api_key":       {},
		"apikey":        {},
		"private_key":   {},
	}
	return redactMap(payload, blocked)
}

func redactMap(payload map[string]any, blocked map[string]struct{}) map[string]any {
	if payload == nil {
		return map[string]any{}
	}
	redacted := make(map[string]any, len(payload))
	for key, value := range payload {
		if _, ok := blocked[strings.ToLower(key)]; ok {
			redacted[key] = "[redacted]"
			continue
		}
		redacted[key] = redactValue(value, blocked)
	}
	return redacted
}

func redactValue(value any, blocked map[string]struct{}) any {
	switch typed := value.(type) {
	case map[string]any:
		return redactMap(typed, blocked)
	case []any:
		redacted := make([]any, len(typed))
		for i, item := range typed {
			redacted[i] = redactValue(item, blocked)
		}
		return redacted
	case []map[string]any:
		redacted := make([]map[string]any, len(typed))
		for i, item := range typed {
			redacted[i] = redactMap(item, blocked)
		}
		return redacted
	default:
		return value
	}
}

func digitalEmployeeRunFromQuery(run queries.TaskRun) *DigitalEmployeeRun {
	return &DigitalEmployeeRun{
		ID:                        run.ID,
		TenantID:                  run.TenantID,
		TaskID:                    run.TaskID,
		DigitalEmployeeID:         uuidFromNull(run.DigitalEmployeeID),
		ExecutionInstanceID:       uuidFromNull(run.ExecutionInstanceID),
		RuntimeNodeID:             uuidFromNull(run.RuntimeNodeID),
		NodeID:                    run.NodeID,
		CommandID:                 stringFromText(run.CommandID),
		ProviderType:              stringFromText(run.ProviderType),
		ProviderSessionID:         stringPtrFromText(run.ProviderSessionID),
		ProviderSessionExternalID: stringPtrFromText(run.ProviderSessionExternalID),
		Status:                    runStatusFromString(run.Status),
		Result:                    jsonMapFromBytes(run.Result),
		Diagnostic:                jsonMapFromBytes(run.Diagnostic),
		LogRef:                    stringPtrFromText(run.LogRef),
		RawResultRef:              stringPtrFromText(run.RawResultRef),
		WorkProducts:              workProductsFromBytes(run.WorkProducts),
		SessionState:              jsonMapFromBytes(run.SessionState),
		ErrorMessage:              stringPtrFromText(run.ErrorMessage),
		ErrorCode:                 stringPtrFromText(run.ErrorCode),
		ErrorFamily:               stringPtrFromText(run.ErrorFamily),
		ExitCode:                  int32PtrFromInt4(run.ExitCode),
		Signal:                    stringPtrFromText(run.Signal),
		TimedOut:                  run.TimedOut,
		IdempotencyKey:            stringPtrFromText(run.IdempotencyKey),
		IdempotencyFingerprint:    stringPtrFromText(run.IdempotencyFingerprint),
		TimeoutSec:                int32PtrFromInt4(run.TimeoutSec),
		GraceSec:                  int32PtrFromInt4(run.GraceSec),
		StartedAt:                 timeFromTimestamptz(run.StartedAt),
		CompletedAt:               timePtrFromTimestamptz(run.CompletedAt),
		FinishedAt:                timePtrFromTimestamptz(run.FinishedAt),
		CreatedAt:                 timeFromTimestamptz(run.CreatedAt),
		UpdatedAt:                 timeFromTimestamptz(run.UpdatedAt),
	}
}

func runtimeCommandReceiptFromQuery(receipt queries.RuntimeCommandReceipt) *RuntimeCommandReceipt {
	return &RuntimeCommandReceipt{
		ID:            receipt.ID,
		TenantID:      receipt.TenantID,
		CommandID:     receipt.CommandID,
		CommandType:   receipt.CommandType,
		RuntimeNodeID: receipt.RuntimeNodeID,
		NodeID:        receipt.NodeID,
		ResourceType:  receipt.ResourceType,
		ResourceID:    receipt.ResourceID,
		Status:        receipt.Status,
		Payload:       jsonMapFromBytes(receipt.Payload),
		Result:        jsonMapFromBytes(receipt.Result),
		ErrorMessage:  stringPtrFromText(receipt.ErrorMessage),
		DispatchedAt:  timePtrFromTimestamptz(receipt.DispatchedAt),
		CompletedAt:   timePtrFromTimestamptz(receipt.CompletedAt),
		CreatedAt:     timeFromTimestamptz(receipt.CreatedAt),
		UpdatedAt:     timeFromTimestamptz(receipt.UpdatedAt),
	}
}

func jsonBytesFromMap(value map[string]any, field string) ([]byte, error) {
	if value == nil {
		value = map[string]any{}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", field, err)
	}
	return encoded, nil
}

func nullableJSONBytesFromMap(value map[string]any, field string) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	return jsonBytesFromMap(value, field)
}

func nullableJSONBytesFromWorkProducts(value []WorkProduct, field string) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", field, err)
	}
	return encoded, nil
}

func workProductsFromBytes(raw []byte) []WorkProduct {
	if len(raw) == 0 {
		return []WorkProduct{}
	}
	var out []WorkProduct
	if err := json.Unmarshal(raw, &out); err != nil {
		return []WorkProduct{}
	}
	if out == nil {
		return []WorkProduct{}
	}
	return out
}

func int4FromPtr(value *int32) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *value, Valid: true}
}

func int32PtrFromInt4(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	copied := value.Int32
	return &copied
}

func uuidFromNull(value uuid.NullUUID) uuid.UUID {
	if !value.Valid {
		return uuid.Nil
	}
	return value.UUID
}

func stringFromText(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func stringFromMap(value map[string]any, key string) string {
	item, ok := value[key]
	if !ok {
		return ""
	}
	text, _ := item.(string)
	return text
}

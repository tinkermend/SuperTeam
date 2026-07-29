package project

import (
	"context"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/systemconfig"
)

// ProjectTaskMaxAttempts bounds match systemconfig KeyProjectTaskDefaultMaxAttempts.
const (
	ProjectTaskMaxAttemptsMin     int32 = 1
	ProjectTaskMaxAttemptsMax     int32 = 5
	ProjectTaskMaxAttemptsDefault int32 = 3 // keep in sync with systemconfig registry default
)

// DefaultProjectTaskMaxAttempts returns the code/registry fallback when system
// config is unavailable (tests, unwired readers).
func DefaultProjectTaskMaxAttempts() int32 {
	return int32(systemconfig.DefaultFor(systemconfig.KeyProjectTaskDefaultMaxAttempts))
}

// ClampProjectTaskMaxAttempts forces a positive attempt budget into [1,5].
func ClampProjectTaskMaxAttempts(value int32) int32 {
	if value < ProjectTaskMaxAttemptsMin {
		return ProjectTaskMaxAttemptsMin
	}
	if value > ProjectTaskMaxAttemptsMax {
		return ProjectTaskMaxAttemptsMax
	}
	return value
}

// EffectiveProjectTaskMaxAttempts resolves the attempt budget for a task:
// explicit task.max_attempts (if >0) wins; otherwise platformDefault (system
// config); otherwise registry default 3. Never returns the old silent "1".
func EffectiveProjectTaskMaxAttempts(taskMax *int32, platformDefault int32) int32 {
	if taskMax != nil && *taskMax > 0 {
		return ClampProjectTaskMaxAttempts(*taskMax)
	}
	if platformDefault > 0 {
		return ClampProjectTaskMaxAttempts(platformDefault)
	}
	return ClampProjectTaskMaxAttempts(DefaultProjectTaskMaxAttempts())
}

// ResolvePlatformDefaultMaxAttempts reads tenant system config when available.
func ResolvePlatformDefaultMaxAttempts(ctx context.Context, reader systemconfig.Reader, tenantID uuid.UUID) int32 {
	if reader == nil || tenantID == uuid.Nil {
		return DefaultProjectTaskMaxAttempts()
	}
	return ClampProjectTaskMaxAttempts(int32(reader.Int64(ctx, tenantID, systemconfig.KeyProjectTaskDefaultMaxAttempts)))
}

// normalizeMaxAttemptsForCreate returns a non-nil clamped pointer to persist on insert.
func normalizeMaxAttemptsForCreate(explicit *int32, platformDefault int32) *int32 {
	value := EffectiveProjectTaskMaxAttempts(explicit, platformDefault)
	return &value
}

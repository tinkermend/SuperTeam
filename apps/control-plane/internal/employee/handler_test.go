package employee

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestGetDigitalEmployeeEffectiveConfigReturnsNotFoundWhenNoApprovedConfig verifies the
// error-mapping contract that GetDigitalEmployeeEffectiveConfig relies on for its only
// error branch: ErrNotFound from Service.GetCurrentEffectiveConfig must surface as HTTP 404.
//
// A full handler-level test (constructing HTTPHandler with a fake HandlerService and
// authorizer) is not attempted here: no existing test harness in this package builds the
// full HTTPHandler dependency graph (authorizer + service + runService), and standing one
// up is out of scope for this change. The behavior this handler adds beyond existing,
// already-tested plumbing (employeeIDFromRequest, authorizeDigitalEmployeeManagement,
// serviceFromRequest, writeJSON, effectiveConfigResponseFromDomain) is exactly this
// writeHandlerError(ErrNotFound) -> 404 mapping, which is what this test pins down.
func TestGetDigitalEmployeeEffectiveConfigReturnsNotFoundWhenNoApprovedConfig(t *testing.T) {
	require.Equal(t, http.StatusNotFound, httpStatusForHandlerError(ErrNotFound))
}

func httpStatusForHandlerError(err error) int {
	rec := httptest.NewRecorder()
	writeHandlerError(rec, err)
	return rec.Code
}

// TestEffectiveConfigResponseFromDomainHandlesNilTeamConfigRevisionID guards against a
// nil-pointer panic: team-less digital employees can have an approved effective config
// with a nil TeamConfigRevisionID (see teamConfigRevisionIDPtr), and this is the shared
// response formatter used by both ApproveDigitalEmployeeEffectiveConfig and the new
// GetDigitalEmployeeEffectiveConfig handler. Calling .String() directly on the nil
// *uuid.UUID pointer previously panicked; it must instead render as an empty string.
func TestEffectiveConfigResponseFromDomainHandlesNilTeamConfigRevisionID(t *testing.T) {
	now := time.Now().UTC()
	effectiveConfig := &DigitalEmployeeEffectiveConfig{
		ID:                       uuid.New(),
		TenantID:                 uuid.New(),
		DigitalEmployeeID:        uuid.New(),
		TeamConfigRevisionID:     nil,
		EmployeeConfigRevisionID: uuid.New(),
		EffectiveConfig:          map[string]any{},
		ValidationResult:         map[string]any{},
		Status:                   EffectiveConfigStatusApproved,
		CreatedAt:                now,
		UpdatedAt:                now,
	}

	require.NotPanics(t, func() {
		response := effectiveConfigResponseFromDomain(effectiveConfig)
		require.Equal(t, "", response.TeamConfigRevisionID)
	})
}

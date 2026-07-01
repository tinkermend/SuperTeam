// Package platform holds zero-dependency platform-wide constants shared across
// domain packages (auth, runtime, task, ...). It must not import other internal
// packages so it can serve as a common leaf without creating import cycles.
package platform

import "github.com/google/uuid"

// DefaultTenantID is the platform bootstrap tenant assigned before multi-tenant
// onboarding is configured. This value is the single source of truth for the Go
// side. It MUST stay in sync with the SQL literal
// '00000000-0000-0000-0000-000000000001' used as COALESCE/DEFAULT fallback in
// internal/storage/queries/*.sql and the corresponding migrations.
var DefaultTenantID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

package employee

import (
	"strings"
	"testing"

	"github.com/superteam/control-plane/internal/storage/queries"
)

func TestRuntimeProviderQueriesDoNotRequireLegacyTeamProviderPolicy(t *testing.T) {
	for name, query := range map[string]string{
		"create_options": queries.ListRuntimeProviderOptionsForDigitalEmployeeCreate,
	} {
		normalized := strings.Join(strings.Fields(query), " ")
		if strings.Contains(normalized, "allowed_provider_types") {
			t.Fatalf("%s query must not require legacy team allowed_provider_types policy", name)
		}
		if strings.Contains(normalized, "provider_outside_team_policy") {
			t.Fatalf("%s query must not emit legacy provider_outside_team_policy disabled reason", name)
		}
	}
}

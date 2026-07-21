package queries

import (
	"strings"
	"testing"
)

func TestGeneratedSchedulingSkillCountQueryMatchesEffectiveSkillPrecedence(t *testing.T) {
	normalized := strings.Join(strings.Fields(GetDigitalEmployeeSchedulingSkillCounts), " ")
	if !strings.Contains(normalized, "FROM target_employee te JOIN team_skill_bindings stb") {
		t.Fatal("scheduling skill count query must count team bindings as inherited skills")
	}
	if !strings.Contains(normalized, "WHERE NOT EXISTS ( SELECT 1 FROM team_skill_bindings inherited_binding") {
		t.Fatal("scheduling skill count query must suppress personal duplicate skills using team bindings")
	}
	if strings.Contains(normalized, "WHERE NOT EXISTS ( SELECT 1 FROM personal_skills ps") {
		t.Fatal("scheduling skill count query must not suppress inherited skills using personal bindings")
	}
}

func TestRuntimeProviderQueriesDoNotRequireLegacyTeamProviderPolicy(t *testing.T) {
	for name, query := range map[string]string{
		"create_options": ListRuntimeProviderOptionsForDigitalEmployeeCreate,
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

package queries

import (
	"strings"
	"testing"
)

func TestGeneratedSchedulingSkillCountQueryMatchesEffectiveSkillPrecedence(t *testing.T) {
	normalized := strings.Join(strings.Fields(GetDigitalEmployeeSchedulingSkillCounts), " ")
	if !strings.Contains(normalized, "FROM target_employee te JOIN skill_team_bindings stb") {
		t.Fatal("scheduling skill count query must count team bindings as inherited skills")
	}
	if !strings.Contains(normalized, "WHERE NOT EXISTS ( SELECT 1 FROM skill_team_bindings inherited_binding") {
		t.Fatal("scheduling skill count query must suppress personal duplicate skills using team bindings")
	}
	if strings.Contains(normalized, "WHERE NOT EXISTS ( SELECT 1 FROM personal_skills ps") {
		t.Fatal("scheduling skill count query must not suppress inherited skills using personal bindings")
	}
}

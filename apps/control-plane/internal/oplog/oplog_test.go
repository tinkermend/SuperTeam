package oplog

import "testing"

func TestModuleForAudit(t *testing.T) {
	t.Parallel()
	cases := []struct {
		eventType string
		action    string
		module    string
		ok        bool
	}{
		{"team_management", "team.create", ModuleTeams, true},
		{"team_management", "team.skill.bind", ModuleSkills, true},
		{"digital_employee_management", "digital_employee.delete", ModuleEmployees, true},
		{"project_management", "project.delete", ModuleProjects, true},
		{"system_config", "system_config.update", ModuleSystemConfig, true},
		{"scenario_template", "scenario_template.publish", ModuleScenarioTemplates, true},
		{"digital_employee_run_created", "employee.run.create", "", false},
	}
	for _, tc := range cases {
		module, ok := ModuleForAudit(tc.eventType, tc.action)
		if ok != tc.ok || module != tc.module {
			t.Fatalf("%s/%s: got (%q,%v) want (%q,%v)", tc.eventType, tc.action, module, ok, tc.module, tc.ok)
		}
	}
}

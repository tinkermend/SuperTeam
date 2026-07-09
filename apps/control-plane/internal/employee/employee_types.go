package employee

// customAgentEmployeeTypeDefinition is the sentinel "blank custom" digital
// employee type. It is never persisted to digital_employee_templates —
// unlike every other type, it has no default role, skills, or policies for
// a user to configure; it exists purely so the create-employee wizard can
// offer a fully custom starting point.
func customAgentEmployeeTypeDefinition() EmployeeTypeDefinition {
	return EmployeeTypeDefinition{
		Type:                     "custom_agent",
		Label:                    "自定义数字员工",
		Description:              "由用户直接定义职责定位、能力扩展、治理策略和执行器类型的自定义数字员工。",
		DefaultRole:              "",
		RecommendedSkills:        []string{},
		RecommendedMCPServers:    []string{},
		RecommendedProviderTypes: []string{"codex", "opencode", "claude-code"},
		PersonaMemoryMarkdown:    "",
		CapabilityBindings:       map[string]any{},
		BudgetPolicy:             map[string]any{},
		DefaultApprovalPolicy:    map[string]any{},
		Metadata: map[string]any{
			"creation_mode": "blank_custom",
			"system_type":   true,
		},
	}
}

func cloneStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func cloneEmployeeTypeMap(values map[string]any) map[string]any {
	if values == nil {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = cloneEmployeeTypeValue(value)
	}
	return cloned
}

func cloneEmployeeTypeValue(value any) any {
	switch typed := value.(type) {
	case []string:
		return cloneStringSlice(typed)
	case []any:
		cloned := make([]any, len(typed))
		for i, item := range typed {
			cloned[i] = cloneEmployeeTypeValue(item)
		}
		return cloned
	case map[string]any:
		return cloneEmployeeTypeMap(typed)
	default:
		return typed
	}
}

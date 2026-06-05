package employee

import "strings"

var defaultEmployeeTypeDefinitions = []EmployeeTypeDefinition{
	{
		Type:                     "database_admin",
		Label:                    "数据库管理",
		Description:              "负责数据库运行维护、性能诊断、备份恢复、变更执行和数据安全检查。",
		DefaultRole:              "database_admin",
		RecommendedSkills:        []string{"database-troubleshooting", "sql-review", "backup-restore", "performance-tuning"},
		RecommendedMCPServers:    []string{"postgres-readonly", "mysql-readonly"},
		RecommendedProviderTypes: []string{"codex", "opencode"},
		DefaultCapabilitySelection: map[string]any{
			"enabled_skills":         []string{"database-troubleshooting", "sql-review"},
			"enabled_mcp_servers":    []string{"postgres-readonly"},
			"enabled_provider_types": []string{"codex"},
		},
		DefaultContextPolicyOverride: map[string]any{
			"sources": []string{"runbook", "monitoring", "database_schema"},
		},
		DefaultApprovalPolicy: map[string]any{
			"min_risk_for_human":          "high",
			"write_actions_require_human": true,
		},
	},
	{
		Type:                     "devops_engineer",
		Label:                    "DevOps 运维",
		Description:              "负责运行环境、发布流水线、故障处置、基础设施变更和可观测性排查。",
		DefaultRole:              "devops_engineer",
		RecommendedSkills:        []string{"incident-diagnosis", "release-operations", "runtime-troubleshooting", "observability-analysis"},
		RecommendedMCPServers:    []string{"kubernetes-readonly", "prometheus-readonly", "grafana-readonly"},
		RecommendedProviderTypes: []string{"codex", "opencode"},
		DefaultCapabilitySelection: map[string]any{
			"enabled_skills":         []string{"incident-diagnosis", "runtime-troubleshooting"},
			"enabled_mcp_servers":    []string{"prometheus-readonly"},
			"enabled_provider_types": []string{"codex"},
		},
		DefaultContextPolicyOverride: map[string]any{
			"sources": []string{"runbook", "monitoring", "deployment_logs"},
		},
		DefaultApprovalPolicy: map[string]any{
			"min_risk_for_human":          "high",
			"write_actions_require_human": true,
		},
	},
	{
		Type:                     "frontend_engineer",
		Label:                    "前端开发",
		Description:              "负责 Web 控制台界面开发、交互实现、前端状态管理和页面问题诊断。",
		DefaultRole:              "frontend_engineer",
		RecommendedSkills:        []string{"frontend-implementation", "ui-regression-check", "accessibility-check", "playwright-verification"},
		RecommendedMCPServers:    []string{"browser"},
		RecommendedProviderTypes: []string{"codex", "opencode"},
		DefaultCapabilitySelection: map[string]any{
			"enabled_skills":         []string{"frontend-implementation", "ui-regression-check"},
			"enabled_provider_types": []string{"codex"},
		},
		DefaultContextPolicyOverride: map[string]any{
			"sources": []string{"design", "frontend_code", "browser_logs"},
		},
		DefaultApprovalPolicy: map[string]any{
			"min_risk_for_human": "medium",
		},
	},
	{
		Type:                     "backend_engineer",
		Label:                    "后端开发",
		Description:              "负责控制平面后端服务、API 契约、业务逻辑、数据访问和服务端测试。",
		DefaultRole:              "backend_engineer",
		RecommendedSkills:        []string{"backend-implementation", "api-contract-check", "database-query-review", "go-test-verification"},
		RecommendedMCPServers:    []string{"postgres-readonly"},
		RecommendedProviderTypes: []string{"codex", "opencode"},
		DefaultCapabilitySelection: map[string]any{
			"enabled_skills":         []string{"backend-implementation", "api-contract-check"},
			"enabled_provider_types": []string{"codex"},
		},
		DefaultContextPolicyOverride: map[string]any{
			"sources": []string{"api_contracts", "backend_code", "database_design"},
		},
		DefaultApprovalPolicy: map[string]any{
			"min_risk_for_human": "medium",
		},
	},
	{
		Type:                     "fullstack_engineer",
		Label:                    "全栈开发",
		Description:              "负责跨前端、后端和契约的端到端功能实现、联调和回归验证。",
		DefaultRole:              "fullstack_engineer",
		RecommendedSkills:        []string{"frontend-implementation", "backend-implementation", "api-contract-check", "end-to-end-verification"},
		RecommendedMCPServers:    []string{"browser", "postgres-readonly"},
		RecommendedProviderTypes: []string{"codex", "opencode"},
		DefaultCapabilitySelection: map[string]any{
			"enabled_skills":         []string{"frontend-implementation", "backend-implementation"},
			"enabled_provider_types": []string{"codex"},
		},
		DefaultContextPolicyOverride: map[string]any{
			"sources": []string{"design", "api_contracts", "backend_code", "frontend_code"},
		},
		DefaultApprovalPolicy: map[string]any{
			"min_risk_for_human": "medium",
		},
	},
	{
		Type:                     "implementation_engineer",
		Label:                    "实施工程师",
		Description:              "负责客户侧部署配置、环境核对、能力接入、交付验证和问题闭环。",
		DefaultRole:              "implementation_engineer",
		RecommendedSkills:        []string{"environment-check", "connector-configuration", "delivery-verification", "customer-runbook-update"},
		RecommendedMCPServers:    []string{"http-connector"},
		RecommendedProviderTypes: []string{"codex", "opencode"},
		DefaultCapabilitySelection: map[string]any{
			"enabled_skills":         []string{"environment-check", "delivery-verification"},
			"enabled_provider_types": []string{"codex"},
		},
		DefaultContextPolicyOverride: map[string]any{
			"sources": []string{"customer_profile", "runbook", "deployment_notes"},
		},
		DefaultApprovalPolicy: map[string]any{
			"min_risk_for_human":          "high",
			"write_actions_require_human": true,
		},
	},
	{
		Type:                     "general_engineer",
		Label:                    "通用工程执行",
		Description:              "负责边界清晰、低风险的通用工程任务、资料整理、代码检查和验证执行。",
		DefaultRole:              "general_engineer",
		RecommendedSkills:        []string{"code-reading", "test-execution", "artifact-preparation", "technical-summary"},
		RecommendedProviderTypes: []string{"codex", "opencode"},
		DefaultCapabilitySelection: map[string]any{
			"enabled_skills":         []string{"code-reading", "test-execution"},
			"enabled_provider_types": []string{"codex"},
		},
		DefaultContextPolicyOverride: map[string]any{
			"sources": []string{"task_context", "repository"},
		},
		DefaultApprovalPolicy: map[string]any{
			"min_risk_for_human": "medium",
		},
	},
}

func DefaultEmployeeTypeDefinitions() []EmployeeTypeDefinition {
	definitions := make([]EmployeeTypeDefinition, 0, len(defaultEmployeeTypeDefinitions))
	for _, definition := range defaultEmployeeTypeDefinitions {
		definitions = append(definitions, cloneEmployeeTypeDefinition(definition))
	}
	return definitions
}

func EmployeeTypeDefinitionByType(value string) (EmployeeTypeDefinition, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return EmployeeTypeDefinition{}, false
	}
	for _, definition := range defaultEmployeeTypeDefinitions {
		if definition.Type == normalized {
			return cloneEmployeeTypeDefinition(definition), true
		}
	}
	return EmployeeTypeDefinition{}, false
}

func cloneEmployeeTypeDefinition(definition EmployeeTypeDefinition) EmployeeTypeDefinition {
	return EmployeeTypeDefinition{
		Type:                         definition.Type,
		Label:                        definition.Label,
		Description:                  definition.Description,
		DefaultRole:                  definition.DefaultRole,
		RecommendedSkills:            cloneStringSlice(definition.RecommendedSkills),
		RecommendedMCPServers:        cloneStringSlice(definition.RecommendedMCPServers),
		RecommendedProviderTypes:     cloneStringSlice(definition.RecommendedProviderTypes),
		DefaultCapabilitySelection:   cloneEmployeeTypeMap(definition.DefaultCapabilitySelection),
		DefaultContextPolicyOverride: cloneEmployeeTypeMap(definition.DefaultContextPolicyOverride),
		DefaultApprovalPolicy:        cloneEmployeeTypeMap(definition.DefaultApprovalPolicy),
		Metadata:                     cloneEmployeeTypeMap(definition.Metadata),
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

CREATE TABLE digital_employee_templates (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  type VARCHAR(64) NOT NULL,
  label VARCHAR(128) NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  default_role VARCHAR(128) NOT NULL DEFAULT '',
  recommended_skills JSONB NOT NULL DEFAULT '[]',
  recommended_mcp_servers JSONB NOT NULL DEFAULT '[]',
  recommended_provider_types JSONB NOT NULL DEFAULT '[]',
  default_capability_selection JSONB NOT NULL DEFAULT '{}',
  default_context_policy_override JSONB NOT NULL DEFAULT '{}',
  default_approval_policy JSONB NOT NULL DEFAULT '{}',
  metadata JSONB NOT NULL DEFAULT '{}',
  status VARCHAR(16) NOT NULL DEFAULT 'active',
  is_system BOOLEAN NOT NULL DEFAULT false,
  deleted_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT digital_employee_templates_status_check CHECK (status IN ('active', 'disabled'))
);

CREATE UNIQUE INDEX digital_employee_templates_tenant_type_key
  ON digital_employee_templates (tenant_id, type) WHERE deleted_at IS NULL;

CREATE INDEX digital_employee_templates_tenant_status_idx
  ON digital_employee_templates (tenant_id, status) WHERE deleted_at IS NULL;

INSERT INTO digital_employee_templates (
  tenant_id, type, label, description, default_role,
  recommended_skills, recommended_mcp_servers, recommended_provider_types,
  default_capability_selection, default_context_policy_override, default_approval_policy,
  metadata, status, is_system
) VALUES
(
  '00000000-0000-0000-0000-000000000001', 'database_admin', '数据库管理',
  '负责数据库运行维护、性能诊断、备份恢复、变更执行和数据安全检查。', 'database_admin',
  '["database-troubleshooting","sql-review","backup-restore","performance-tuning"]',
  '["postgres-readonly","mysql-readonly"]',
  '["codex","opencode"]',
  '{"enabled_skills":["database-troubleshooting","sql-review"],"enabled_mcp_servers":["postgres-readonly"],"enabled_provider_types":["codex"]}',
  '{"sources":["runbook","monitoring","database_schema"]}',
  '{"min_risk_for_human":"high","write_actions_require_human":true}',
  '{}', 'active', true
),
(
  '00000000-0000-0000-0000-000000000001', 'devops_engineer', 'DevOps 运维',
  '负责运行环境、发布流水线、故障处置、基础设施变更和可观测性排查。', 'devops_engineer',
  '["incident-diagnosis","release-operations","runtime-troubleshooting","observability-analysis"]',
  '["kubernetes-readonly","prometheus-readonly","grafana-readonly"]',
  '["codex","opencode"]',
  '{"enabled_skills":["incident-diagnosis","runtime-troubleshooting"],"enabled_mcp_servers":["prometheus-readonly"],"enabled_provider_types":["codex"]}',
  '{"sources":["runbook","monitoring","deployment_logs"]}',
  '{"min_risk_for_human":"high","write_actions_require_human":true}',
  '{}', 'active', true
),
(
  '00000000-0000-0000-0000-000000000001', 'security_engineer', '安全工程',
  '负责安全评审、漏洞分析、权限与配置风险检查、应急处置和修复建议。', 'security_engineer',
  '["security-review","vulnerability-analysis","permission-audit","incident-response"]',
  '["postgres-readonly","http-connector"]',
  '["codex","opencode"]',
  '{"enabled_skills":["security-review","vulnerability-analysis"],"enabled_provider_types":["codex"]}',
  '{"sources":["security_policy","audit_logs","repository"]}',
  '{"min_risk_for_human":"high","write_actions_require_human":true}',
  '{}', 'active', true
),
(
  '00000000-0000-0000-0000-000000000001', 'qa_engineer', '测试工程',
  '负责测试计划、用例设计、自动化验证、缺陷复现、回归检查和验收证据整理。', 'qa_engineer',
  '["test-planning","test-automation","bug-reproduction","regression-verification"]',
  '["browser"]',
  '["codex","opencode"]',
  '{"enabled_skills":["test-planning","regression-verification"],"enabled_provider_types":["codex"]}',
  '{"sources":["requirements","test_reports","browser_logs"]}',
  '{"min_risk_for_human":"medium"}',
  '{}', 'active', true
),
(
  '00000000-0000-0000-0000-000000000001', 'frontend_engineer', '前端开发',
  '负责 Web 控制台界面开发、交互实现、前端状态管理和页面问题诊断。', 'frontend_engineer',
  '["frontend-implementation","ui-regression-check","accessibility-check","playwright-verification"]',
  '["browser"]',
  '["codex","opencode"]',
  '{"enabled_skills":["frontend-implementation","ui-regression-check"],"enabled_provider_types":["codex"]}',
  '{"sources":["design","frontend_code","browser_logs"]}',
  '{"min_risk_for_human":"medium"}',
  '{}', 'active', true
),
(
  '00000000-0000-0000-0000-000000000001', 'backend_engineer', '后端开发',
  '负责控制平面后端服务、API 契约、业务逻辑、数据访问和服务端测试。', 'backend_engineer',
  '["backend-implementation","api-contract-check","database-query-review","go-test-verification"]',
  '["postgres-readonly"]',
  '["codex","opencode"]',
  '{"enabled_skills":["backend-implementation","api-contract-check"],"enabled_provider_types":["codex"]}',
  '{"sources":["api_contracts","backend_code","database_design"]}',
  '{"min_risk_for_human":"medium"}',
  '{}', 'active', true
),
(
  '00000000-0000-0000-0000-000000000001', 'fullstack_engineer', '全栈开发',
  '负责跨前端、后端和契约的端到端功能实现、联调和回归验证。', 'fullstack_engineer',
  '["frontend-implementation","backend-implementation","api-contract-check","end-to-end-verification"]',
  '["browser","postgres-readonly"]',
  '["codex","opencode"]',
  '{"enabled_skills":["frontend-implementation","backend-implementation"],"enabled_provider_types":["codex"]}',
  '{"sources":["design","api_contracts","backend_code","frontend_code"]}',
  '{"min_risk_for_human":"medium"}',
  '{}', 'active', true
),
(
  '00000000-0000-0000-0000-000000000001', 'implementation_engineer', '实施工程师',
  '负责客户侧部署配置、环境核对、能力接入、交付验证和问题闭环。', 'implementation_engineer',
  '["environment-check","connector-configuration","delivery-verification","customer-runbook-update"]',
  '["http-connector"]',
  '["codex","opencode"]',
  '{"enabled_skills":["environment-check","delivery-verification"],"enabled_provider_types":["codex"]}',
  '{"sources":["customer_profile","runbook","deployment_notes"]}',
  '{"min_risk_for_human":"high","write_actions_require_human":true}',
  '{}', 'active', true
),
(
  '00000000-0000-0000-0000-000000000001', 'general_engineer', '通用工程执行',
  '负责边界清晰、低风险的通用工程任务、资料整理、代码检查和验证执行。', 'general_engineer',
  '["code-reading","test-execution","artifact-preparation","technical-summary"]',
  '[]',
  '["codex","opencode"]',
  '{"enabled_skills":["code-reading","test-execution"],"enabled_provider_types":["codex"]}',
  '{"sources":["task_context","repository"]}',
  '{"min_risk_for_human":"medium"}',
  '{}', 'active', true
)
ON CONFLICT DO NOTHING;

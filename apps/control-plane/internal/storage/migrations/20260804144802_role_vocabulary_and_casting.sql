-- 批二：角色词表 + 员工角色多值 + 项目×剧本编制
-- 角色 = 编制单位（who）；能力词表 = 提示（what）。剧本 roles[].key 与员工角色绑定共用本词表。

CREATE TABLE role_vocabulary (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants (id),
    role_key TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status VARCHAR NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'disabled')),
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_role_vocabulary_key_not_blank CHECK (btrim(role_key) <> ''),
    CONSTRAINT uq_role_vocabulary_tenant_key UNIQUE (tenant_id, role_key)
);

CREATE INDEX idx_role_vocabulary_tenant_status
    ON role_vocabulary (tenant_id, status)
    WHERE deleted_at IS NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_role_vocabulary_updated_at'
    ) THEN
        CREATE TRIGGER update_role_vocabulary_updated_at
        BEFORE UPDATE ON role_vocabulary
        FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;

COMMENT ON TABLE role_vocabulary IS '租户级角色词表：剧本 roles[].key 与员工角色绑定共用，插行即扩展';
COMMENT ON COLUMN role_vocabulary.role_key IS '角色键稳定标识（下划线小写，如 code_reviewer），租户内唯一';
COMMENT ON COLUMN role_vocabulary.title IS '角色显示名（中文）';
COMMENT ON COLUMN role_vocabulary.status IS '角色状态：active/disabled';

-- 默认租户种子：覆盖内置 4 个剧本的全部角色（含 operator，故意不绑员工）
INSERT INTO role_vocabulary (id, tenant_id, role_key, title, description, status) VALUES
('00000000-0000-0000-0000-000000000601'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'developer', '开发', '实现功能与代码变更', 'active'),
('00000000-0000-0000-0000-000000000602'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'reviewer', '审查', '独立评审变更，只审不改', 'active'),
('00000000-0000-0000-0000-000000000603'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'tester', '测试', '编写并执行测试以验证行为', 'active'),
('00000000-0000-0000-0000-000000000604'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'collector', '采集', '采集日志、指标等运行证据', 'active'),
('00000000-0000-0000-0000-000000000605'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'analyst', '分析', '分析运维证据并形成结论', 'active'),
('00000000-0000-0000-0000-000000000606'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'diagnostician', '诊断', '故障分诊与根因定位', 'active'),
('00000000-0000-0000-0000-000000000607'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'operator', '修复实施', '实施故障修复与变更操作', 'active'),
('00000000-0000-0000-0000-000000000608'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'verifier', '验证', '独立验证修复效果', 'active'),
('00000000-0000-0000-0000-000000000609'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'researcher', '调研', '搜集资料与事实', 'active'),
('00000000-0000-0000-0000-000000000610'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'writer', '撰写', '撰写调研报告成稿', 'active')
ON CONFLICT (tenant_id, role_key) DO NOTHING;

-- 员工角色多值关联表（按 role_key 查人走索引）
CREATE TABLE digital_employee_roles (
    tenant_id UUID NOT NULL,
    digital_employee_id UUID NOT NULL REFERENCES digital_employees (id) ON DELETE CASCADE,
    role_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (digital_employee_id, role_key),
    CONSTRAINT ck_digital_employee_roles_key_not_blank CHECK (btrim(role_key) <> '')
);

CREATE INDEX idx_digital_employee_roles_tenant_role
    ON digital_employee_roles (tenant_id, role_key);

COMMENT ON TABLE digital_employee_roles IS '数字员工可兼多角色；匹配与编制候选读此表，不读 digital_employees.role';
COMMENT ON COLUMN digital_employee_roles.role_key IS '引用 role_vocabulary.role_key（同命名空间，应用层校验 active）';

-- 旧 role 列降级为显示用自述标签，读路径不再参与匹配
COMMENT ON COLUMN digital_employees.role IS '显示用自述标签（自由文本）。匹配/编制/可达收口一律读 digital_employee_roles，禁止再用本列做匹配。';

-- 项目 × 剧本编制：一角色一人；同一员工可占多角色（兼任）
CREATE TABLE project_playbook_casting (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    scenario_template_key TEXT NOT NULL,
    role_key TEXT NOT NULL,
    digital_employee_id UUID NOT NULL REFERENCES digital_employees (id),
    cast_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_project_playbook_casting_template_key_not_blank CHECK (btrim(scenario_template_key) <> ''),
    CONSTRAINT ck_project_playbook_casting_role_key_not_blank CHECK (btrim(role_key) <> ''),
    CONSTRAINT uq_project_playbook_casting_role UNIQUE (project_id, scenario_template_key, role_key)
);

CREATE INDEX idx_project_playbook_casting_tenant_project
    ON project_playbook_casting (tenant_id, project_id);

CREATE INDEX idx_project_playbook_casting_employee
    ON project_playbook_casting (tenant_id, digital_employee_id);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_project_playbook_casting_updated_at'
    ) THEN
        CREATE TRIGGER update_project_playbook_casting_updated_at
        BEFORE UPDATE ON project_playbook_casting
        FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;

COMMENT ON TABLE project_playbook_casting IS '项目×剧本编制：每个角色定一人，必须由人操作（cast_by_user_id 留痕）';
COMMENT ON COLUMN project_playbook_casting.cast_by_user_id IS '编制操作人；回答「谁批准这个人上这一仗」';
COMMENT ON COLUMN project_playbook_casting.digital_employee_id IS '被编制的数字员工；写入时若未在成员池则同事务入池';

-- 团队宪法版本化（spec 2026-07-26-team-configuration-console-design §5.3，D1 接通 / D9 仅文本注入）。
--
-- 此前团队宪法是 tenant_teams.constitution 上的一块 jsonb：整块覆盖、无版本、无变更
-- 说明、无历史，且 hard_rules 全仓没有任何执行侧消费者（存了不用）。P3 把它接进派发
-- 链，同时补上"改了什么、谁改的、为什么改"的可追溯性。
--
-- 设计取舍：tenant_teams.constitution 保留为**当前生效快照**，读路径（员工创建基线、
-- 派发注入）不变；本表只承载版本历史。回滚 = 以旧内容创建新版本，不改写历史。
CREATE TABLE IF NOT EXISTS team_constitution_revisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    team_id UUID NOT NULL,
    revision_number INTEGER NOT NULL,
    -- rules: [{id, text, category}]，category ∈ 禁止/必须/需审批（服务端注册校验，
    -- 不在 DB 层写死枚举，便于后续扩展分类）。
    rules JSONB NOT NULL DEFAULT '[]'::jsonb,
    change_note TEXT NOT NULL DEFAULT '',
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_team_constitution_revisions_revision_positive CHECK (revision_number > 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_team_constitution_revisions_team_revision
    ON team_constitution_revisions(tenant_id, team_id, revision_number);

CREATE INDEX IF NOT EXISTS idx_team_constitution_revisions_team
    ON team_constitution_revisions(tenant_id, team_id, revision_number DESC);

COMMENT ON TABLE team_constitution_revisions IS '团队宪法版本历史；当前生效内容仍在 tenant_teams.constitution';
COMMENT ON COLUMN team_constitution_revisions.rules IS '规则条目数组 [{id,text,category}]';
COMMENT ON COLUMN team_constitution_revisions.change_note IS '变更说明，保存时必填';

-- 回填：把现有的 hard_rules 收成 revision 1，避免"已有内容却没有任何版本"的空档。
-- 旧数据没有 category，统一按「必须」归类（原语义就是硬性规则）。
INSERT INTO team_constitution_revisions (tenant_id, team_id, revision_number, rules, change_note, created_at)
SELECT
    tt.tenant_id,
    tt.id,
    1,
    COALESCE(
        (
            SELECT jsonb_agg(
                jsonb_build_object(
                    'id', gen_random_uuid()::text,
                    'text', rule,
                    'category', 'must'
                )
            )
            FROM jsonb_array_elements_text(tt.constitution->'hard_rules') AS rule
            WHERE btrim(rule) <> ''
        ),
        '[]'::jsonb
    ),
    '迁移自旧版硬性规则',
    NOW()
FROM tenant_teams tt
WHERE tt.deleted_at IS NULL
  AND jsonb_typeof(tt.constitution->'hard_rules') = 'array'
  AND jsonb_array_length(tt.constitution->'hard_rules') > 0
ON CONFLICT (tenant_id, team_id, revision_number) DO NOTHING;

-- 同步把结构化 rules 写回当前生效快照，让读路径一次性拿到 hard_rules 与 rules 两种
-- 形态：hard_rules 保留给既有读者（员工创建基线），rules 供新的编辑/注入路径使用。
UPDATE tenant_teams tt
SET constitution = tt.constitution || jsonb_build_object('rules', r.rules)
FROM team_constitution_revisions r
WHERE r.tenant_id = tt.tenant_id
  AND r.team_id = tt.id
  AND r.revision_number = 1
  AND tt.deleted_at IS NULL
  AND NOT (tt.constitution ? 'rules');

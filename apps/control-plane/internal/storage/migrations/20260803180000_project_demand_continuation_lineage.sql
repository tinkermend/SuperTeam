-- 同单接续血缘（spec 2026-08-01-demand-continuation-design §4.1）。
--
-- 接续产出的是**新 demand**（demand 状态单调不可回退，原单终态永不改写），
-- 血缘靠本列串起来：一单的用户身份是这条链，不是单行。
--
-- 用真列而不是塞 source_refs JSONB：链遍历要走递归 CTE 且必须能走索引；
-- source_type 是渠道枚举（manual/github/ticket/document/log/automation），
-- 语义是"需求从哪来"，与血缘正交，不得挪用——接续单同样可能来自任一渠道。
--
-- 单亲：一个 demand 至多接续一个前序，不做合并式接续。
ALTER TABLE project_demands
    ADD COLUMN continues_demand_id UUID REFERENCES project_demands (id);

COMMENT ON COLUMN project_demands.continues_demand_id IS
    'Demand this one continues (single parent). NULL for a chain head. Chain traversal is indexed by idx_project_demands_tenant_continues.';

-- 部分索引：绝大多数 demand 是链头（列为 NULL），没必要为它们建索引条目。
CREATE INDEX idx_project_demands_tenant_continues
    ON project_demands (tenant_id, continues_demand_id)
    WHERE continues_demand_id IS NOT NULL;

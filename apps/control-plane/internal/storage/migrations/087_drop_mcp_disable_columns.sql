-- 087: MCP 注册表/绑定放弃"禁用"生命周期（用户拍板：场景少、正在跑的任务也停不
-- 掉，只承认创建/删除两态）。
--
-- 这批 status/disabled_at 是只建了读侧过滤、从未有写入路径的半成品：全库常量
-- 'active'。删除后软删口径统一为 deleted_at 单列；员工删除级联同步收敛（不再
-- 顺手写 status='disabled'）。删除的下游同步依赖既有机制天然成立：effective
-- 投影 JOIN 注册表按 deleted_at 过滤 + 派发指纹(cmv1)按投影计算 + MCP 配置
-- 每 run 一次性重投影——注册表项删除后，员工下一次运行的配置即不再包含它。
--
-- 不动：runtime_capabilities.status（runtime 能力同步真实写入）、
-- runtime_nodes.disabled_at（节点归档在用）、
-- digital_employee_environment_variables.status（删除级联在写）。

ALTER TABLE mcp_servers
    DROP COLUMN status,
    DROP COLUMN disabled_at;
ALTER TABLE team_mcp_bindings
    DROP COLUMN status,
    DROP COLUMN disabled_at;
ALTER TABLE digital_employee_mcp_bindings_v2
    DROP COLUMN status,
    DROP COLUMN disabled_at;
ALTER TABLE project_mcp_bindings
    DROP COLUMN status,
    DROP COLUMN disabled_at;
-- runtime_capabilities.disabled_at 有两个依赖对象，先改写再删列：
-- ① 就绪视图（047 版）去掉 rc.disabled_at 谓词重建；
-- ② 执行可用性部分索引去掉 disabled_at IS NULL 谓词重建。
CREATE OR REPLACE VIEW digital_employee_runtime_readiness AS
WITH latest_runs AS (
    SELECT DISTINCT ON (tr.tenant_id, tr.digital_employee_id)
        tr.tenant_id,
        tr.digital_employee_id,
        tr.status
    FROM task_runs tr
    JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
    WHERE tr.digital_employee_id IS NOT NULL
      AND t.deleted_at IS NULL
    ORDER BY tr.tenant_id, tr.digital_employee_id, tr.updated_at DESC, tr.created_at DESC
),
employee_config_states AS (
    SELECT DISTINCT ON (decr.tenant_id, decr.digital_employee_id)
        decr.tenant_id,
        decr.digital_employee_id,
        decr.id AS effective_config_id,
        CASE
            WHEN decr.status = 'active' AND decr.archived_at IS NULL THEN 'approved'
            WHEN decr.status IN ('draft', 'pending_approval') AND decr.archived_at IS NULL THEN 'pending_approval'
            WHEN decr.status = 'archived' OR decr.archived_at IS NOT NULL THEN 'stale'
            ELSE COALESCE(NULLIF(BTRIM(decr.status), ''), 'missing')
        END::text AS governance_status
    FROM digital_employee_config_revisions decr
    ORDER BY decr.tenant_id, decr.digital_employee_id, decr.revision_number DESC, decr.updated_at DESC
),
provider_capabilities AS (
    SELECT DISTINCT ON (rc.tenant_id, rc.runtime_node_id, rc.provider_type)
        rc.tenant_id,
        rc.runtime_node_id,
        rc.provider_type,
        rc.available,
        rc.status,
        rc.health_status
    FROM runtime_capabilities rc
    WHERE rc.capability_type = 'provider'
      AND rc.archived_at IS NULL
    ORDER BY rc.tenant_id, rc.runtime_node_id, rc.provider_type, rc.last_seen_at DESC NULLS LAST, rc.updated_at DESC
)
SELECT
    de.tenant_id,
    de.id AS digital_employee_id,
    (
        de.status IN ('ready', 'active')
        AND COALESCE(dei.status, 'missing')::text IN ('ready', 'active')
        AND ecs.effective_config_id IS NOT NULL
        AND dei.runtime_node_id IS NOT NULL
        AND rn.status = 'online'
        AND rn.disabled_at IS NULL
        AND rn.archived_at IS NULL
        AND NULLIF(BTRIM(COALESCE(dei.agent_home_dir, '')), '') IS NOT NULL
        AND COALESCE(ecs.governance_status, 'missing')::text = 'approved'
        AND COALESCE(pc.available, false)::boolean = true
        AND COALESCE(pc.status, '')::text = 'healthy'
        AND COALESCE(pc.health_status, '')::text = 'healthy'
    )::boolean AS is_runtime_ready
FROM digital_employees de
LEFT JOIN digital_employee_execution_instances dei
  ON dei.tenant_id = de.tenant_id
 AND dei.digital_employee_id = de.id
 AND dei.deleted_at IS NULL
LEFT JOIN runtime_nodes rn
  ON rn.id = dei.runtime_node_id
 AND rn.tenant_id = dei.tenant_id
LEFT JOIN provider_capabilities pc
  ON pc.tenant_id = dei.tenant_id
 AND pc.runtime_node_id = dei.runtime_node_id
 AND pc.provider_type = dei.provider_type
LEFT JOIN employee_config_states ecs
  ON ecs.tenant_id = de.tenant_id
 AND ecs.digital_employee_id = de.id
WHERE de.deleted_at IS NULL;

DROP INDEX IF EXISTS idx_runtime_capabilities_execution_available;
CREATE INDEX idx_runtime_capabilities_execution_available
    ON runtime_capabilities(tenant_id, runtime_node_id, provider_type, health_status)
    WHERE capability_type = 'provider' AND available = true AND archived_at IS NULL;

ALTER TABLE runtime_capabilities
    DROP COLUMN disabled_at;

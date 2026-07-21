-- 外科手术版 dei 退役(A+D):
-- 1) 重建 digital_employee_runtime_readiness:判据改为身份+治理+租户级 provider
--    可用性,不再依赖员工级 digital_employee_execution_instances 绑定。
-- 2) 清空 dei 数据行(表保留,物理 DROP 另开拆表版)。
-- 写路径已在应用层封口(PUT execution-instance 删除);总览 execution_summary
-- 改取 task_runs 真实落点见 sqlc 查询变更。

CREATE OR REPLACE VIEW digital_employee_runtime_readiness AS
WITH employee_config_states AS (
    SELECT DISTINCT ON (decr.tenant_id, decr.digital_employee_id)
        decr.tenant_id,
        decr.digital_employee_id,
        CASE
            WHEN decr.status = 'active' AND decr.archived_at IS NULL THEN 'approved'
            WHEN decr.status IN ('draft', 'pending_approval') AND decr.archived_at IS NULL THEN 'pending_approval'
            WHEN decr.status = 'archived' OR decr.archived_at IS NOT NULL THEN 'stale'
            ELSE COALESCE(NULLIF(BTRIM(decr.status), ''), 'missing')
        END::text AS governance_status
    FROM digital_employee_config_revisions decr
    ORDER BY decr.tenant_id, decr.digital_employee_id, decr.revision_number DESC, decr.updated_at DESC
),
available_provider_types AS (
    SELECT DISTINCT
        rc.tenant_id,
        rc.provider_type
    FROM runtime_capabilities rc
    JOIN runtime_nodes rn
      ON rn.id = rc.runtime_node_id
     AND rn.tenant_id = rc.tenant_id
    WHERE rc.capability_type = 'provider'
      AND rc.archived_at IS NULL
      AND rc.available = true
      AND rc.status = 'healthy'
      AND rc.health_status = 'healthy'
      AND rn.status = 'online'
      AND rn.disabled_at IS NULL
      AND rn.archived_at IS NULL
)
SELECT
    de.tenant_id,
    de.id AS digital_employee_id,
    (
        de.status IN ('ready', 'active')
        AND COALESCE(ecs.governance_status, 'missing')::text = 'approved'
        AND EXISTS (
            SELECT 1
            FROM available_provider_types apt
            WHERE apt.tenant_id = de.tenant_id
              AND apt.provider_type = de.provider_type
        )
    )::boolean AS is_runtime_ready
FROM digital_employees de
LEFT JOIN employee_config_states ecs
  ON ecs.tenant_id = de.tenant_id
 AND ecs.digital_employee_id = de.id
WHERE de.deleted_at IS NULL;

COMMENT ON VIEW digital_employee_runtime_readiness IS
'数字员工 runtime 就绪读视图:身份 ready/active + 治理 approved + 租户内存在在线 healthy provider;不再依赖员工级 dei 绑定。协调线程主路径已改走项目级 placement,本视图仅遗留兼容。';

DELETE FROM digital_employee_execution_instances;

-- 068_evidence_grounding.sql
-- 证据地基:artifact 血缘列、attempt 内幂等约束、evidence 状态约束、删除 v0 遗留 task_artifacts。
-- 见 docs/superpowers/specs/2026-07-09-evidence-grounding-artifact-collection-design.md(§4.5、§8 修订)

-- 1) v0 任务系统遗留表,0 行、无 handler 引用(sqlc 死查询同步删除)。
DROP TABLE IF EXISTS task_artifacts;

-- 2) artifact 血缘:同一任务下多员工/多次 attempt 各自成行,去重发生在存储层(内容寻址),血缘保留在关系层。
ALTER TABLE project_artifact_refs
    ADD COLUMN attempt_id UUID,
    ADD COLUMN digital_employee_id UUID;

COMMENT ON COLUMN project_artifact_refs.attempt_id IS '产出该 artifact 的 project_task_attempt;项目级/人工上传的 artifact 为 NULL。无 FK,attempt 生命周期独立。';
COMMENT ON COLUMN project_artifact_refs.digital_employee_id IS '产出者数字员工;与 attempt_id 同源的反范式冗余,便于验收界面按员工过滤。';

CREATE INDEX idx_project_artifact_refs_tenant_project_attempt
    ON project_artifact_refs (tenant_id, project_id, attempt_id);

-- 幂等边界 = attempt 内:同一 attempt 的 result 重复提交不产生重复行;
-- 不同 attempt 即使内容字节一致(同 checksum)也各自保留一行(血缘)。
CREATE UNIQUE INDEX uq_project_artifact_refs_attempt_checksum
    ON project_artifact_refs (tenant_id, project_task_id, attempt_id, checksum)
    WHERE attempt_id IS NOT NULL AND project_task_id IS NOT NULL;

ALTER TABLE project_artifact_refs
    ADD CONSTRAINT chk_project_artifact_refs_checksum
        CHECK (checksum IS NULL OR checksum = '' OR checksum ~ '^[a-f0-9]{64}$');

-- 3) evidence 状态不允许空串(双写路径的历史产物先回填再约束)。
UPDATE project_evidence_refs SET verification_status = 'submitted' WHERE verification_status = '';

ALTER TABLE project_evidence_refs
    ADD CONSTRAINT chk_project_evidence_refs_verification_status
        CHECK (verification_status <> '');

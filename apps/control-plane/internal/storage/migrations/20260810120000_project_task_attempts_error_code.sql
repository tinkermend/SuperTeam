-- Provider 语义统一 Phase 4：稳定 error_code 落 attempt 行，便于跨 Provider 统计与告警
-- （code 此前只在事件 payload JSON 里，扫库代价高；family 开放字符串不足以精确定位）。

ALTER TABLE project_task_attempts
    ADD COLUMN IF NOT EXISTS error_code VARCHAR(100);

CREATE INDEX IF NOT EXISTS idx_project_task_attempts_error_code
    ON project_task_attempts (tenant_id, error_code, finished_at DESC)
    WHERE error_code IS NOT NULL;

COMMENT ON COLUMN project_task_attempts.error_code IS 'Provider ErrorEnvelope.code（UPPER_SNAKE 稳定机器码，如 RATE_LIMIT、BUDGET_FUSE）；可选；历史行不回填';

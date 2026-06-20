-- 025_skill_archive_storage.sql
-- 技能包存储重构：zip 整包存对象存储，删除逐文件存储和在线编辑，移除 installed/available 状态

-- 1. skills 表增加 archive 元数据列
ALTER TABLE skills
    ADD COLUMN archive_object_ref TEXT,
    ADD COLUMN archive_filename VARCHAR(255),
    ADD COLUMN archive_size_bytes BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN archive_checksum_sha256 VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN archive_file_count INTEGER NOT NULL DEFAULT 0;

COMMENT ON COLUMN skills.archive_object_ref IS '技能 zip 包在对象存储的 URI，例如 s3://bucket/skills/.../xxx.zip';
COMMENT ON COLUMN skills.archive_filename IS '上传时原始 zip 文件名';
COMMENT ON COLUMN skills.archive_size_bytes IS 'zip 包字节数';
COMMENT ON COLUMN skills.archive_checksum_sha256 IS 'zip 包 SHA256 校验值，用于 Runtime 下发完整性校验';
COMMENT ON COLUMN skills.archive_file_count IS 'zip 包内文件总数';

-- 2. 移除 installed/available 状态概念：删除依赖 status 的索引
DROP INDEX IF EXISTS idx_skills_tenant_status_updated;

-- 3. 清理种子数据（diagnose/tdd 没有 archive，无法在新模型下分发）
DELETE FROM skill_team_bindings
WHERE skill_id IN ('00000000-0000-0000-0000-000000000301'::uuid, '00000000-0000-0000-0000-000000000302'::uuid);
DELETE FROM skill_agent_bindings
WHERE skill_id IN ('00000000-0000-0000-0000-000000000301'::uuid, '00000000-0000-0000-0000-000000000302'::uuid);
DELETE FROM skills
WHERE id IN ('00000000-0000-0000-0000-000000000301'::uuid, '00000000-0000-0000-0000-000000000302'::uuid);

-- 4. 删除 status 列
ALTER TABLE skills DROP COLUMN status;

-- 5. 添加替代索引（按更新时间列出技能）
CREATE INDEX IF NOT EXISTS idx_skills_tenant_updated
    ON skills(tenant_id, updated_at DESC)
    WHERE deleted_at IS NULL;

-- 6. 删除 skill_files 表（不再逐文件存储）
DROP TABLE IF EXISTS skill_files;

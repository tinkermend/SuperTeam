-- Migration: project_multiple_human_owners
-- Description: 项目支持多个平级人类负责人。新增 human_owner_user_ids 数组并从
-- 现有单值 human_owner_user_id 回填；本迁移保留旧标量列(双写过渡期,由服务端
-- 维护 scalar = ids[0]),待所有读路径切换到数组后再单独迁移 drop。
-- 设计见 docs/superpowers/specs/2026-07-20-project-multiple-human-owners.md

ALTER TABLE projects
    ADD COLUMN human_owner_user_ids UUID[] NOT NULL DEFAULT '{}'::uuid[];

UPDATE projects
SET human_owner_user_ids = ARRAY[human_owner_user_id]
WHERE human_owner_user_id IS NOT NULL
  AND cardinality(human_owner_user_ids) = 0;

COMMENT ON COLUMN projects.human_owner_user_ids IS '项目人类负责人ID集合(平级,至少一个;任一可审批/验收/兜底路由)';

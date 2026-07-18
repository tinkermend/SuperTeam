-- 团队说明是团队的职责与协作范围摘要，面向团队目录、详情和创建流程展示。
-- 空字符串表示存量团队尚未补充说明；长度由列定义和服务层共同约束。

ALTER TABLE tenant_teams
    ADD COLUMN description VARCHAR(280) NOT NULL DEFAULT '';

COMMENT ON COLUMN tenant_teams.description IS '团队说明，描述职责、服务对象或协作边界，最长 280 字符';

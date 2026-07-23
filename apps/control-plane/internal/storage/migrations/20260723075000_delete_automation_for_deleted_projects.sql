-- 删项目时自动化规则此前未级联:存量已删项目上的规则/触发记录清掉。
-- Temporal Schedule 由重启后应用侧或本机 temporal CLI 收尾(见 CHANGELOG)。

DELETE FROM automation_fires
WHERE rule_id IN (
  SELECT ar.id
  FROM automation_rules ar
  JOIN projects p ON p.id = ar.project_id
  WHERE p.deleted_at IS NOT NULL
);

DELETE FROM automation_rules ar
USING projects p
WHERE p.id = ar.project_id
  AND p.deleted_at IS NOT NULL;

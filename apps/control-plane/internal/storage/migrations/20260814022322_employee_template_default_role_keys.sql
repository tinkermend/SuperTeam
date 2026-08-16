-- 员工模板默认剧本角色：创建数字员工时写入 digital_employee_roles，与角色词表对齐。
-- default_role 仍为展示用自述标签，不参与编制匹配。

ALTER TABLE digital_employee_templates
  ADD COLUMN default_role_keys JSONB NOT NULL DEFAULT '[]'::jsonb;

COMMENT ON COLUMN digital_employee_templates.default_role_keys IS
  '创建员工时默认绑定的 role_vocabulary.role_key 列表；空数组表示不预绑剧本角色';

-- 系统标准模板：与种子角色词表 reviewer / tester 对齐
UPDATE digital_employee_templates
SET default_role_keys = '["reviewer"]'::jsonb
WHERE type = 'standard_code_reviewer'
  AND deleted_at IS NULL
  AND (default_role_keys = '[]'::jsonb OR default_role_keys IS NULL);

UPDATE digital_employee_templates
SET default_role_keys = '["tester"]'::jsonb
WHERE type = 'standard_tester'
  AND deleted_at IS NULL
  AND (default_role_keys = '[]'::jsonb OR default_role_keys IS NULL);

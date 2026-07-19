-- 头像独占：每个头像同时只能被一个在册数字员工持有（用户拍板，2026-07-19）。
-- 历史遗留数据里存在多员工共用同一头像（dev 库 engineer-m-01 曾被 12 个员工共用），
-- 先做数据修复再建唯一索引：
--   ① 每个（tenant, 头像）保留创建最早的持有者；
--   ② 其余员工按创建顺序改派目录内空闲头像（写 metadata.avatar_asset_id + avatar.id，
--      读路径按 id 查内置目录取展示数据，无需在此回填 label/url）；
--   ③ 空闲头像不足时兜底剥离多余声明（留空走前端哈希兜底展示，不占独占名额）；
--   ④ 建 partial 唯一索引：软删（deleted_at）后头像自动释放回库，可被重新选用。
-- 服务层已有创建前校验（409），索引是并发抢占的最终防线。

-- ① + ②：把重复/未知声明改派为目录内空闲头像。
WITH catalog(asset_id, ord) AS (
    VALUES
        ('engineer-m-01', 1), ('engineer-m-02', 2), ('engineer-m-03', 3), ('engineer-m-04', 4),
        ('engineer-m-05', 5), ('engineer-m-06', 6), ('engineer-m-07', 7), ('engineer-m-08', 8),
        ('engineer-m-09', 9), ('engineer-m-10', 10), ('engineer-m-11', 11), ('engineer-m-12', 12),
        ('engineer-m-13', 13), ('engineer-m-14', 14), ('engineer-m-15', 15), ('engineer-m-16', 16),
        ('engineer-m-17', 17), ('engineer-m-18', 18),
        ('engineer-f-01', 19), ('engineer-f-02', 20), ('engineer-f-03', 21), ('engineer-f-04', 22),
        ('engineer-f-05', 23), ('engineer-f-06', 24), ('engineer-f-07', 25), ('engineer-f-08', 26),
        ('engineer-f-09', 27), ('engineer-f-10', 28), ('engineer-f-11', 29), ('engineer-f-12', 30),
        ('engineer-f-13', 31), ('engineer-f-14', 32)
),
claims AS (
    SELECT id, tenant_id, created_at,
           COALESCE(metadata->>'avatar_asset_id', metadata->'avatar'->>'id') AS asset_id
    FROM digital_employees
    WHERE deleted_at IS NULL
),
keepers AS (
    SELECT DISTINCT ON (tenant_id, asset_id) id, tenant_id, asset_id
    FROM claims
    WHERE asset_id IN (SELECT asset_id FROM catalog)
    ORDER BY tenant_id, asset_id, created_at ASC, id ASC
),
needs AS (
    SELECT c.id, c.tenant_id, c.created_at
    FROM claims c
    WHERE NOT EXISTS (SELECT 1 FROM keepers k WHERE k.id = c.id)
      AND c.asset_id IS NOT NULL
),
free_assets AS (
    SELECT t.tenant_id, cat.asset_id,
           ROW_NUMBER() OVER (PARTITION BY t.tenant_id ORDER BY cat.ord) AS rn
    FROM (SELECT DISTINCT tenant_id FROM needs) t
    JOIN catalog cat ON NOT EXISTS (
        SELECT 1 FROM keepers k
        WHERE k.tenant_id = t.tenant_id AND k.asset_id = cat.asset_id
    )
),
ranked_needs AS (
    SELECT id, tenant_id,
           ROW_NUMBER() OVER (PARTITION BY tenant_id ORDER BY created_at ASC, id ASC) AS rn
    FROM needs
),
assignments AS (
    SELECT rn_needs.id, fa.asset_id
    FROM ranked_needs rn_needs
    JOIN free_assets fa ON fa.tenant_id = rn_needs.tenant_id AND fa.rn = rn_needs.rn
)
UPDATE digital_employees de
SET metadata = COALESCE(de.metadata, '{}'::jsonb)
        || jsonb_build_object('avatar_asset_id', a.asset_id, 'avatar', jsonb_build_object('id', a.asset_id)),
    updated_at = NOW()
FROM assignments a
WHERE de.id = a.id;

-- ③ 兜底：空闲头像不足时仍残留的重复声明，剥离其头像 key（不占独占名额）。
WITH claims AS (
    SELECT id, tenant_id, created_at,
           COALESCE(metadata->>'avatar_asset_id', metadata->'avatar'->>'id') AS asset_id
    FROM digital_employees
    WHERE deleted_at IS NULL
      AND COALESCE(metadata->>'avatar_asset_id', metadata->'avatar'->>'id') IS NOT NULL
),
keepers AS (
    SELECT DISTINCT ON (tenant_id, asset_id) id
    FROM claims
    ORDER BY tenant_id, asset_id, created_at ASC, id ASC
)
UPDATE digital_employees de
SET metadata = COALESCE(de.metadata, '{}'::jsonb) - 'avatar_asset_id' - 'avatar',
    updated_at = NOW()
FROM claims c
WHERE de.id = c.id
  AND NOT EXISTS (SELECT 1 FROM keepers k WHERE k.id = de.id);

-- ④ 唯一索引：在册员工内头像独占；软删行不占名额（删除即释放回库）。
CREATE UNIQUE INDEX uq_digital_employees_avatar_asset
    ON digital_employees (tenant_id, COALESCE(metadata->>'avatar_asset_id', metadata->'avatar'->>'id'))
    WHERE deleted_at IS NULL
      AND COALESCE(metadata->>'avatar_asset_id', metadata->'avatar'->>'id') IS NOT NULL;

COMMENT ON INDEX uq_digital_employees_avatar_asset IS '数字员工头像独占：在册员工一人一头像，软删释放';

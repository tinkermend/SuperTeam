# UUID-first 数据库重建说明

SuperTeam 早期环境采用重写初始 schema 的方式引入 UUID 主键。以下命令会删除当前开发库数据，只能用于确认没有保留价值的本地或远端开发环境。

## 本地重建

1. 确认当前连接信息：

```bash
sed -n '1,220p' doc/database/conn_info.md
```

2. 可选备份：

```bash
pg_dump "$DATABASE_URL" > /tmp/superteam-before-uuid-rebuild.sql
```

3. 删除并重建 schema：

```bash
psql "$DATABASE_URL" -c 'DROP SCHEMA IF EXISTS superteam CASCADE; CREATE SCHEMA superteam;'
```

4. 重新执行迁移：

```bash
make -C apps/control-plane migrate-up DATABASE_URL="$DATABASE_URL"
```

如果连接串的 `search_path` 指向预先创建的 `superteam` schema，Atlas 会认为数据库非空。早期重建场景应使用：

```bash
cd apps/control-plane
atlas migrate apply \
  --dir file://internal/storage/migrations \
  --url "$DATABASE_URL" \
  --revisions-schema atlas_schema_revisions \
  --allow-dirty
```

如修改过迁移文件，先重新生成 Atlas 校验和：

```bash
cd apps/control-plane
rm -f internal/storage/migrations/atlas.sum
atlas migrate hash --dir file://internal/storage/migrations
```

5. 检查主键类型：

```sql
SELECT table_name, column_name, data_type, column_default
FROM information_schema.columns
WHERE table_schema = current_schema()
  AND column_name = 'id'
ORDER BY table_name;
```

所有 SuperTeam 自有表的 `id` 都应是 `uuid`，并通过 `gen_random_uuid()` 默认生成。

# Control Plane 数据库迁移

本目录是 Control Plane 唯一的生产迁移路径，由 [Atlas](https://atlasgo.io) 管理。
`atlas.hcl`（`env "local"`）、`apps/control-plane/Makefile` 的 `migrate-*` 目标、以及
`scripts/dev-services.sh` 的启动钩子都指向这里。历史上曾存在的 `apps/control-plane/migrations/`
空占位目录已删除，不要再新建同名目录。

## 完整性

`atlas.sum` 是迁移目录的校验和清单，条目数必须与 `*.sql` 文件数一致。任何一次
`atlas migrate apply/status/validate` 都会先校验 `atlas.sum`；不一致会直接报错。
新增迁移后必须运行 `atlas migrate hash`（或 `atlas migrate new`）更新 `atlas.sum`。

## 编号缺口

编号 `042` 是**有意跳过**的（git 全历史中从未存在该文件，非回滚丢失）。Atlas 按
`atlas.sum` 的顺序执行，不依赖编号连续性，因此缺口无功能影响。看到 041 → 043 属正常。

## 常用命令（在 `apps/control-plane/` 下）

```bash
# 应用迁移到目标库（需要 DATABASE_URL）
make migrate-up DATABASE_URL=postgres://...

# 查看当前版本 / 待应用数量
make migrate-status DATABASE_URL=postgres://...

# 回滚 N 步
make migrate-down DATABASE_URL=postgres://... STEPS=1

# 完整性 + 可重放校验（在一次性 dev 库上重放全部迁移，不碰真实库）
# 标准 CI 有 docker 守护进程时可直接 make migrate-validate；
# 本地用 podman 等时覆盖 DEV_URL：
make migrate-validate DEV_URL=postgres://user:pass@localhost:5432/dev?sslmode=disable
```

> `atlas migrate lint`（破坏性变更静态分析）自 Atlas v0.38 起需要 Atlas Pro 登录，
> 未纳入免费 CI 命令；如已有 Pro 账号可自行 `atlas login` 后使用。

## 何时执行迁移（避免 schema 漂移）

- **本地开发**：`scripts/dev-services.sh start|restart control-plane` 会在启动 Control Plane
  前自动执行 `atlas migrate apply`；迁移失败则中止启动。可用 `SUPERTEAM_DEV_SKIP_MIGRATIONS=1` 跳过。
- **部署 / 生产**：部署流水线必须在启动新版本 Control Plane **之前**运行
  `make migrate-up`（或等价的 `atlas migrate apply`）。Control Plane 进程本身不在启动时
  自动应用迁移（无 `go:embed` 迁移、无 boot-time apply），因此这一步不能省略，否则新代码会
  跑在旧 schema 上。建议在 CI 合并门禁中加 `make migrate-validate` 作为完整性检查。

# 收件箱筛选栏方案 B 落地概览

## 完成内容

将收件箱筛选栏的「状态」筛选从**强制必填**改为**可选**，新增「所有」中性态选项，使三个筛选维度（状态/类型/风险）行为一致——均有中性默认值 + 可清除。

### 改动范围（跨前端 + 后端 + 测试）

**后端 (Go + SQL)**
- `inbox.sql` — ListInboxItems + CountInboxItems 的 `sqlc.arg('status')` → `sqlc.narg('status')` + IS NULL 模式
- `inbox.sql.go` — sqlc 重新生成，Status 参数 `string` → `pgtype.Text`
- `types.go` — ListItemsRequest.Status: `Status` → `*Status`
- `handler.go` — 仅在 status 非空非 "all" 时设置
- `service.go` — 移除 `req.Status == "" → StatusOpen` 默认逻辑，nil = 返回所有状态
- `pg_repository.go` — 新增 `textFromStatusPtr` 辅助函数

**前端 (TypeScript)**
- `inbox-shell.tsx` — statusOptions 增加「所有」选项，类型 `InboxStatus | "all"`，默认值 `filters.status ?? "all"`
- `index.test.tsx` — 更新状态选项测试

**测试**
- Go: 适配 `*Status` 类型 + 新增 `TestServiceListItemsNilStatusReturnsAllStatuses`
- Web: 19/19 inbox 测试通过

**原型**
- `docs/prototypes/inbox-filter-bar-option-a.html` — 方案 B 可交互原型

### 验证状态
- Go build: OK
- Go inbox 测试: 全通过
- Web typecheck: inbox 零错误
- Web inbox 测试: 19/19 通过
- 真实端到端验证: 待完成（需 dev-services 运行）

# 04 · 阶段 A — 立法与门禁骨架

## 目标

在**不改业务运行时行为**的前提下：

1. 把制度从「Button 或 V3Button 都行」改为唯一公共 API 方向（可先写目标态，alias 窗口写明）。
2. 挂上可执行的 import / 命名护栏（测试或脚本），防止迁移期间漂移加剧。
3. 在 DESIGN / actions / tokens 增加「迁移进行中」指针到本目录。

## 非目标

- 不 rename 组件
- 不改 token 名
- 不改 features 业务代码（除为 guard 所必需的 allowlist 注释，尽量为零）

## 前置

- `03` 决策前置已满足（至少 Q 已勾选；若只先合文档指针，可放宽，但 guard 严格模式需 Q 完成）

## 任务清单

### A1. 规范指针

- [ ] `DESIGN.md`「事实源 / 落地策略」增加迁移入口链接与状态 Draft→In Progress
- [ ] `docs/design-system/actions.md`：删除「主按钮可用 Button 或 V3Button」；改为：
  - 目标：业务仅使用 `@/components/superteam` 的 `Button`（迁移期可暂用 `V3Button` alias）
  - `components/ui/button` 为内部 primitive，业务禁止新增引用
- [ ] `docs/design-system/tokens.md`：注明 token 去前缀迁移见本目录；现行代码事实源仍是 `theme.css` 的现名，直到阶段 E
- [ ] `docs/design-system/principles.md` 或 surfaces：补「禁止版本前缀进永久 API」

### A2. Import Guard（推荐 vitest，对齐 status-labels.guard）

新建例如：`apps/web/src/lib/design-import.guard.test.ts`

规则草案：

1. 扫描 `apps/web/src/features/**`、`apps/web/src/routes/**`
2. 禁止静态导入：
   - `@/components/ui/button`
   - `@/components/ui/badge`
   - `@/components/ui/card`
3. **阶段 A 模式选择（开放，建议 A2a）**：
   - **A2a 基线冻结**：当前违规写入 allowlist 快照；**只失败于新增违规路径**（防止扩大）
   - **A2b 严格**：立即失败所有违规——会倒逼与 D 同时做，**不推荐 A 单独上**

推荐 **A2a**：allowlist = 当前 inventory 的文件集合；测试比较 set 相等或 only-new。

### A3. 命名 Guard（可选 A 阶段，可 B 后加强）

- 禁止 features 新增 `V3[A-Z]` 类型导入？迁移期反而还在用 → **B 完成前不要禁 V3**
- 可增加：禁止 features 新文件出现 `from '@/components/ui/button'`（与 A2a 合并）

### A4. verify-design-system

- [ ] 脚本增加对本迁移 README 存在性检查（可选）
- [ ] 暂不删除对 `v3` 字符串的旧校验，避免假红；在 F 再改

### A5. 开发者通知

- [ ] 按 `03` §8 模板在团队频道/TODO 告知（若适用）
- [ ] 根 `TODO.md` **不要**把整个迁移塞成延后项；这是活跃迁移

## 交付物

- 文档条文 PR
- guard 测试 PR（可与文档同 PR）
- allowlist 文件或 inline 列表（若 A2a）

## 验证

- [ ] `pnpm --filter @superteam/web test` 含 guard 通过
- [ ] 故意在 feature 新增一行 ui/button import → guard 红（手工验证一次）
- [ ] 无生产行为变化

## 完成定义

- 制度不再鼓励双轨
- 双轨**扩大**被自动拦住
- 映射表路径被现行文档引用

## 回滚

Revert 文档 + 删除 guard 测试即可。

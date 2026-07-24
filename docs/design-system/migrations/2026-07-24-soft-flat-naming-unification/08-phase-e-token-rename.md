# 08 · 阶段 E — Token / 工具类去 `v3-` 前缀

## 警告

**本阶段是全迁移中风险最高、diff 最大的阶段。**  
必须：双挂 → 验证 → codemod → 验证 → 删旧名。  
禁止：直接查找替换删旧名一条龙。

## 目标

- CSS 变量、`@theme`、Tailwind 类、`var(--v3-*)`、全局 `.v3-*` class 全部过渡到 `02` 映射名
- 色值/阴影公式 **零 intentional 变化**

## 前置

- Q5（**已决 A 真值合并**）、Q6、Q7 已确认
- 阶段 B 完成（避免组件与 token 同时海量冲突）；D 建议完成或冻结 features 大改

## 子阶段

### E0. 冻结与基线

- [ ] 刷新 inventory
- [ ] 记录当前关键截图或依赖现有 vitest screenshots 作为基线
- [ ] 确认无并行 PR 大改 theme.css

### E1. 双挂（只加不删）

**Q5=A 操作要点**：

- 对重叠面：以 Soft-Flat 现行值为唯一真源，写入 `--card` / `--background`（及 dark 对应），然后 `--v3-card: var(--card)` 等旧名别名双挂。
- 业务目标类名：`bg-v3-card`→`bg-card`，`bg-v3-bg`→`bg-background`（codemod 在 E2）。
- 独有语义（ink/line/ok/…）：新名承载真值，旧 `--v3-*` 别名双挂；**不要**再引入 `--surface` 第二套白底。


在 `theme.css` 的 `:root` 与 `.dark`：

1. 用**新名**承载真值（把现 `--v3-brand: #2f5fff` 改为 `--brand: #2f5fff`）
2. 立即 `--v3-brand: var(--brand)`
3. `@theme inline` **同时**暴露：
   - `--color-brand: var(--brand)`
   - `--color-v3-brand: var(--brand)`（旧）
4. shell/aurora/signature/layout 凡未进 `@theme` 的，至少 CSS 变量双挂

验证 E1：

- [ ] 不改任何 TSX class 的情况下，页面视觉不变
- [ ] typecheck/test 仍绿

### E2. Codemod 类名与 var()（按族拆 PR）

建议顺序（每族可独立 PR）：

1. **ink / line**（最高频，视觉敏感）
2. **brand**
3. **semantic colors**（ok/warn/danger/info/artifact/mute）
4. **surface/card/bg**
5. **radius/shadow**
6. **layout/metric var()**
7. **shell var() + index.css 选择器**
8. **signature / aurora**
9. **全局 class** `.v3-glass` → `.glass` 等
10. **data-slot** 若 B 未做则此处做完

Codemod 范围：

- 包含：`apps/web/src/**/*.{ts,tsx,css}`
- 排除：`docs/prototypes/**`、`node_modules`、本迁移文档中的「现名」说明可保留

每族验证：

- [ ] `rg` 旧 token 在 src 中归零（该族）
- [ ] tests 绿
- [ ] 抽样页面

### E3. 删除旧名

单独 PR：

- 删除所有 `--v3-*` 定义与 `@theme` 旧暴露
- 删除 `.v3-glass` 旧 class 名
- 全量 `rg '\bv3-|--v3-' apps/web/src` → 0
- 更新 `verify-design-system.mjs` 期望字符串
- 更新 `tokens.md` 全文到新名（**不改 hex**）

### E4. 幽灵 token 处理

- 盘点 `shadow-v3-soft` 等未定义类：删除使用或补定义到新名体系（需在 PR 说明）

## 验证（阶段完成）

- [ ] typecheck
- [ ] web test 全量
- [ ] verify:design-system
- [ ] 人工：light + dark 各一关键页
- [ ] `rg` 清零命令见 `10`

## 回滚

- E2 单族：revert 该 PR；E1 双挂仍在则安全
- E3 删旧名后回滚成本高：依赖 git revert 整 PR，且需确保无新代码已只写新名

## 严禁

- 借机修改 hex / 阴影数值「微调观感」
- 在 E2 同一 PR 做功能开发
- 无双挂直接删旧变量

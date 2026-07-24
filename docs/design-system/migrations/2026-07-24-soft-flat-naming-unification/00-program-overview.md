# 00 · 项目总览

## 1. 背景

SuperTeam Web 在引入 Soft-Flat 设计基线时，用 **「v3」** 作为版本/批次标签：

- 组件：`V3Button`、`V3Table`、`V3EmptyState`…
- Token：`--v3-brand`、`text-v3-ink`、`rounded-v3-card`…
- 文档：`v3 Soft-Flat`、`design-direction-v3`…

同时历史上保留 shadcn/ui 模板组件（`components/ui/button|card|badge`），与 `components/superteam` 项目组件**平行存在**。结果：

1. 版本号永久进入日常 API，读起来像未完成迁移。
2. `text-v3-ink` 等类名易与 Tailwind 大版本号混淆。
3. 业务面可在 `Button` 与 `V3Button` 间随意选择，视觉持续漂移。
4. `docs/design-system/actions.md` 甚至明文允许两者并存，从制度上固化了双轨。

## 2. 目标（Goals）

| ID | 目标 | 可观测结果 |
| --- | --- | --- |
| G1 | 公共组件 API 无 `V3` 前缀 | `V3Button` 等标识符在 `apps/web/src` 业务与公共导出中为 0（过渡 alias 窗口除外） |
| G2 | 设计 token / 工具类无 `v3-` 版本前缀 | 生产 class 使用 `text-ink` / `bg-brand` 等；旧名仅短暂兼容后删除 |
| G3 | 单一 Button 视觉事实源 | `buttonVariants`（或等价）只定义一次；ui 与 superteam 共用 |
| G4 | 业务 import 边界清晰 | `features/**` + `routes/**` 不得直接 import `ui/button|badge|card` |
| G5 | 文档语言统一为 Soft-Flat | `DESIGN.md` 与 `docs/design-system/*` 不再把「v3」当现行 API 名 |
| G6 | 可回归、可回滚 | 每阶段有 verify 清单与回滚步骤 |

## 3. 非目标（Non-goals）

- 重做配色、间距体系或信息架构
- 修复 Glass Tier / 深色硬编码 / 视口断点等其它 UI 债
- 建立 monorepo 级 `packages/ui` 发布物
- 修改 Control Plane / Runtime / 契约
- 要求历史原型 HTML、旧 plan/spec 全文去 V3（仅现行规范必须更新）

## 4. 设计原则（实施时不可违背）

1. **语义名优先，版本号禁止进永久 API**  
   允许历史叙述保留「曾用 v3 批次引入 Soft-Flat」。
2. **先对齐实现，再删兼容**  
   任何改名必须先有 alias 或双挂，再 codemod，再删旧名；禁止「只删不挂」。
3. **视觉无 intentional change**  
   截图/观感允许因 token 解析路径变化产生的 0 差异；不允许借机改圆角、色值、高度。
4. **shadcn 仍是底层胶水，不是业务设计 API**  
   Radix 复合组件可留在 `components/ui`；业务默认入口是 `@/components/superteam`。
5. **小步 PR，显式路径**  
   共享 worktree：禁止 `git add -A`；禁止无关分支切换。
6. **映射表是唯一命名真相**  
   实现中发现映射表遗漏 → **先改文档再改代码**，不得 PR 内即兴起名。

## 5. 终态架构（逻辑）

```text
┌─────────────────────────────────────────────┐
│  features / routes（业务）                    │
│  import { Button, SoftCard, StatusPill, … } │
│         from "@/components/superteam"         │
└───────────────────┬─────────────────────────┘
                    │
┌───────────────────▼─────────────────────────┐
│  components/superteam（公共设计 API）          │
│  Button, SoftCard, WorkSurface, …           │
│  唯一 buttonVariants / 语义组件实现            │
└───────────────────┬─────────────────────────┘
                    │ 可复用同一 variants
┌───────────────────▼─────────────────────────┐
│  components/ui（内部 primitive / Radix 胶水）  │
│  dialog, sidebar, calendar, 内部 Button…     │
│  features 默认禁止直连 button/badge/card      │
└───────────────────┬─────────────────────────┘
                    │
┌───────────────────▼─────────────────────────┐
│  styles/theme.css + index.css                │
│  --brand / --ink / …（无 v3 前缀）            │
│  @theme 暴露 color-brand, radius-card, …     │
└─────────────────────────────────────────────┘
```

## 6. 阶段总图

| 阶段 | 名称 | 是否改视觉 | 是否大面积 touch features | 依赖 |
| --- | --- | --- | --- | --- |
| A | 立法 + 门禁骨架 | 否 | 否 | 人类确认开放决策 |
| B | 组件 API 去 V3 | 否（纯改名） | 是（机械 rename） | A |
| C | Button 单一样式源 | 原则上否* | 少 | B（或与 B 同 PR 仅限 superteam+ui） |
| D | 业务 import 清零 | 应接近否 | 是（按域切片） | C |
| E | Token 去 v3 前缀 | 原则上否* | 是（最大） | B 完成；建议 D 完成或并行严格隔离 |
| F | 文档与 verify | 否 | 否 | 随 A–E 增量；终验在 E 后 |

\*阶段 C/E 若发现 Button 尺寸/圆角两套不一致，**允许且仅允许**把 ui/Button 对齐到当前 Soft-Flat（V3Button）规格；必须在 PR 说明「对齐项清单」，并跑相关截图测试。这不是改版，是消歧。

## 7. 成功标准（项目级 Done）

- [ ] `apps/web/src` 中无业务/公共导出的 `V3[A-Z]` 组件名（测试与迁移脚本除外，或仅测 deprecated 警告）
- [ ] 生产路径无新增 `text-v3-*` / `bg-v3-*` / `--v3-*` 引用（旧名已删或仅存在于 changelog 历史）
- [ ] `buttonVariants` 单一定义；superteam `Button` 与 ui 内部按钮同源
- [ ] import guard：features/routes 引用 `ui/button|badge|card` → CI/测试失败
- [ ] `DESIGN.md` + `docs/design-system/{actions,tokens,surfaces,...}.md` 现行 API 无「V3 组件」称呼
- [ ] `corepack pnpm --filter @superteam/web typecheck` 通过
- [ ] `corepack pnpm --filter @superteam/web test`（或迁移约定的子集 + 终验全量）通过
- [ ] `corepack pnpm verify:design-system`（若脚本已更新）通过
- [ ] 人类抽检：登录、项目列表、员工创建、收件箱、一页密集表，无样式回归

## 8. 建议 PR 切片（人类确认后可调整）

1. `docs only`：本目录方案（当前）
2. `phase-a`：规范条文 + guard 测试（可先 warn 或只扫新增）
3. `phase-b`：superteam 导出改名 + 全仓组件标识符 codemod + 测试
4. `phase-c`：buttonVariants 抽取与对齐
5. `phase-d-1…n`：按 teams / projects / employees / errors / … 清 ui/button
6. `phase-e-1`：token 双挂（新名=旧值，旧名=var(新名)）
7. `phase-e-2…n`：按 token 族 codemod class
8. `phase-e-final`：删除旧 token 名
9. `phase-f`：文档终态 + verify 脚本

## 9. 角色与审批

| 角色 | 职责 |
| --- | --- |
| 人类 Owner | 勾选 `11-human-review-checklist`；阶段门禁放行；争议命名一锤定音 |
| 实现 Agent/开发 | 严格按 phase 文档执行；发现映射外名称先停 |
| Reviewer | 检查是否夹带视觉改版、是否 `git add -A`、是否跳过验证 |

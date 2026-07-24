# Soft-Flat 命名去版本化 + 组件双轨收敛

> **状态**：草案（Draft）——仅文档，**禁止开工实现**，直至人类在 `11-human-review-checklist.md` 全部勾选确认。  
> **日期**：2026-07-24  
> **范围**：`apps/web` 设计语言命名、项目级组件 API、Button 双实现、import 边界；不改业务行为与视觉意图。  
> **风格终态名**：Soft-Flat（不再称为 v3 风格）

## 读法（按顺序）

| 顺序 | 文档 | 用途 |
| --- | --- | --- |
| 1 | [00-program-overview.md](./00-program-overview.md) | 目标、非目标、原则、阶段总图、成功标准 |
| 2 | [01-inventory-and-scope.md](./01-inventory-and-scope.md) | 现状盘点快照（开工前需重跑刷新） |
| 3 | [02-naming-map.md](./02-naming-map.md) | **完整映射表**（组件 / token / class / data-slot） |
| 4 | [03-prerequisites-and-risks.md](./03-prerequisites-and-risks.md) | 前置条件、冲突、风险、回滚、并发约束 |
| 5 | [04-phase-a-governance.md](./04-phase-a-governance.md) | 阶段 A：立法与门禁（仍可不迁业务） |
| 6 | [05-phase-b-component-rename.md](./05-phase-b-component-rename.md) | 阶段 B：组件 API 去 `V3` 前缀 |
| 7 | [06-phase-c-button-single-source.md](./06-phase-c-button-single-source.md) | 阶段 C：Button 单一样式源 |
| 8 | [07-phase-d-import-boundaries.md](./07-phase-d-import-boundaries.md) | 阶段 D：业务面 import 边界与清零 |
| 9 | [08-phase-e-token-rename.md](./08-phase-e-token-rename.md) | 阶段 E：CSS/Tailwind token 去 `v3-` |
| 10 | [09-phase-f-docs-and-history.md](./09-phase-f-docs-and-history.md) | 阶段 F：文档 / verify 脚本 / 历史目录策略 |
| 11 | [10-verification-and-rollback.md](./10-verification-and-rollback.md) | 每阶段验证矩阵与回滚手册 |
| 12 | [11-human-review-checklist.md](./11-human-review-checklist.md) | **人工审核清单**（你的签字入口） |
| 13 | [12-self-review-log.md](./12-self-review-log.md) | 方案自检记录（起草方复检） |
| 14 | [13-decisions-log.md](./13-decisions-log.md) | 审核对话决策记录（含 Q5 白话说明） |

## 一句话目标

把迁移动词 **「V3」** 从设计语言中移除，使 Soft-Flat 成为无名号的唯一基线；同时消灭 **Button / Card / Badge 双轨实现**，让业务代码只有一条公共设计 API。

## 明确不做（本迁移）

- 不做视觉改版（颜色、间距、布局算法不借机重设计）
- 不引入 `V4` / `St` / `Ds` 等新版本前缀作为永久方案
- 不搬迁到空的 `packages/ui`
- 不重写历史 CHANGELOG 正文里的旧叙述（可只在文首加注）
- 不强制改名 `docs/prototypes/design-direction-v3/` 路径（历史参考，见阶段 F）
- 不在本迁移中解决 Glass Tier 滥用、深色硬编码、容器断点等其它 UI 债（可另开迁移）

## 推荐执行节奏（人类确认后）

```text
A 立法门禁 → B 组件改名(+alias) → C Button 同源 → D 业务 import 清零
        ↘（可与 D 部分并行准备）E token 别名双挂 → codemod → 删旧名 → F 文档
```

**禁止**把 B+C+D+E 塞进同一个巨型 PR。  
**禁止**在共享 worktree 用 `git add -A`；只显式路径暂存本迁移文件。

## 文档维护约定

- 本目录是本迁移的**唯一方案入口**；实现 PR 描述必须链接到对应 phase 文档。
- 盘点数字以 `01-inventory` 为准；开工当日必须重跑 inventory 命令并更新「刷新日期」。
- 映射表以 `02-naming-map` 为准；实现不得口头另起名字。
- 开放决策未在 `11-human-review-checklist` 勾选前，**不得进入实现**。

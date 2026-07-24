# 07 · 阶段 D — 业务 import 边界与清零

## 目标

1. `features/**`、`routes/**` 对 `ui/button|badge|card` 引用 → **0**
2. `superteam` 内部不再为了按钮去引 `ui/button`
3. Guard 从 A2a allowlist 模式升级为 **严格 0 违规**
4. data-table **不**改为 superteam Button（Q4 已决）；继续 ui/button 白名单（须 C 同源）

## 前置

- 阶段 B：业务可 `import { Button } from '@/components/superteam'`
- 阶段 C：ui 与 superteam 按钮视觉一致（减少替换时观感跳变）

## 切片建议（每片独立 PR）

| 切片 | 路径 | 动作 |
| --- | --- | --- |
| D0 | `components/superteam/user-search-select.tsx`, `team-icon-picker.tsx` | ui/Button → superteam Button |
| D1 | `features/errors/*` | 同上 |
| D2 | `features/teams/components/create-team-*.tsx`, `team-management-toolbar.tsx` | 同上 |
| D3 | `features/projects/components/create-project/**`, `submit-demand-dialog.tsx` | 同上 |
| D4 | `features/employees/**` 残留 button/badge/card | Button→Button；Badge→StatusPill/Chip；Card→SoftCard |
| D5 | `features/users/components/create-user-drawer.tsx` | Button |
| D6 | `features/runtime/index.tsx`, `task-launches/.../prompt-template-dialog.tsx` | Button；注意是否混用两套 |
| D7 | badge/card 扫尾 | skills/upload 等 |
| D8 | Guard 严格化 | 删除 allowlist |
| D9 | **取消必达**（Q4 已决）：data-table 保持 ui/button 白名单；仅当另开清洁 PR 时再做 | 已决 |

## 替换规则

| 从 | 到 | 注意 |
| --- | --- | --- |
| `import { Button } from '@/components/ui/button'` | `import { Button } from '@/components/superteam'` | variant：`default`→`primary` |
| `variant="default"` | `variant="primary"` | |
| `variant="destructive"` | `variant="danger"` | |
| `variant="secondary"` | 多金为 `outline` 或 `ghost`（逐处看层级） | 禁止静默丢层级 |
| `size="lg"` | 无 lg 时 → default 或 className 补 | 映射表外要记录 |
| `Badge` 状态义 | `StatusPill` | tone 映射 |
| `Badge` 计数/过滤 | `Chip` | |
| `Card/CardHeader/...` | `SoftCard` + 内部结构 | 勿套多余阴影 |

## 验证（每一切片）

- [ ] typecheck
- [ ] 相关 feature 测试
- [ ] 该域手动点主按钮/取消/危险按钮
- [ ] `rg` 该切片路径无 ui/button

## 完成定义

- features/routes 三者 import 为 0
- guard 严格模式永久开启
- D0 完成（superteam 自洽）

## 回滚

按切片 revert。

## 特殊点：非状态 Badge

`components/superteam/team-role.tsx` 使用 `ui/badge` 表达**角色标签**（owner/admin/…），不是运行状态。

- **不要**一律改成 `StatusPill`（会与状态语义混淆）。
- 优先：保留轻量 `Badge` 但改为基于 Soft-Flat token 的项目级 `RoleBadge`（已有 `TeamRoleBadge` 则可只换底层实现），或 `Chip` 非 active 样式。
- 实现 PR 需人工看一眼团队成员列表角色展示。

## 风险

- Dialog footer 里 Button asChild + Link 组合
- 混用文件（既有 V3Button 又有 ui Button）替换后重复 import 要合并

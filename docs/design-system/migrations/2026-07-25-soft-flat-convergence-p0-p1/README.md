# Soft-Flat 收敛债 · P0 护栏 + P1 机械替换

日期：2026-07-25  
范围：features/routes 导入边界止血，以及 **tabs / alert / toast** 同构替换。  
不做：Dialog/Sheet Soft 化、EntityCard、Stepper 大改（属 P2）。

## 目标

| 波次 | 完成定义 |
| --- | --- |
| **P0** | `design-import.guard` 对 features/routes 禁止 `ui/tabs`、`ui/alert`、`ui/badge`（既有 button/card 保留）；禁止业务直引 `sonner`（须走 `notify*`）。allowlist 空则必须 0 命中。 |
| **P1-tabs** | features 内 0 处 `ui/tabs`；改用 `SoftTabs*`（或已是 `PageTabs` 的页面级条保持）。 |
| **P1-alert** | features 内 0 处 `ui/alert`；改用 `Callout`。 |
| **P1-toast** | features 内 0 处 `from "sonner"`；改用 `notifySuccess` / `notifyError` 等。 |

## 替换对照

| 旧 | 新 |
| --- | --- |
| `Tabs/List/Trigger/Content` from `ui/tabs` | `SoftTabs/SoftTabsList/SoftTabsTrigger/SoftTabsContent` |
| `Alert` + Title/Description | `Callout tone title description` 或 `children` |
| `toast.success/error` from `sonner` | `notifySuccess` / `notifyError` |

## 白名单（刻意不动）

- `components/data-table/**`、`components/ui/**` 内部 primitive
- `components/superteam/team-role.tsx` 等非 features 路径的 badge（若有）不在本护栏扫描集
- `ui/dialog` / `ui/sheet`：**本波次不禁**（P2）

## 验证

```bash
corepack pnpm --filter @superteam/web test -- design-import.guard
corepack pnpm --filter @superteam/web typecheck
# 相关域测试按需：permissions runtime projects automations workflows staff-gap human-gate
```

## 回滚

单 commit 可逆；护栏与替换同提交，避免「只上护栏全红」窗口。


## 落地状态（2026-07-25）

- [x] P0 `design-import.guard` 扩展 tabs/alert/sonner
- [x] P1-tabs：6 文件 → SoftTabs*
- [x] P1-alert：features Alert → Callout（含 HumanGateCallout）
- [x] P1-toast：3 文件 → notify*
- [x] permissions 测试 data-slot 对齐 soft-tabs-list
- 验证：design-import.guard / typecheck / 相关域测试通过

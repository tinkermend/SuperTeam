# 06 · 阶段 C — Button 单一样式源

## 目标

消灭 **两套按钮 class 实现**，只保留一份 variants 真源。

## 现状问题（依据）

- `superteam`：`V3Button`/`Button` 自建 `buttonBase` + `buttonVariant` + `buttonSize`（h-10、rounded-xl、primary/outline/ghost/danger/glass）
- `ui/button.tsx`：CVA `buttonVariants`（default/secondary/…、sm 全圆角 pill、h-9…）
- 二者**不是**互相包装

## 目标结构

```text
components/superteam/button-variants.ts  (或 components/ui/button-variants.ts)
  export const buttonVariants = cva(...)  // Soft-Flat 规格为唯一真源
  export type ButtonVariant / ButtonSize

components/superteam/primitives.tsx
  Button → 使用 buttonVariants；data-slot=app-button

components/ui/button.tsx
  Button → 使用同一 buttonVariants
  // variant 名兼容：default ≡ primary（映射）
```

## 规格对齐清单（需 Q9 批准）

以 **当前 Soft-Flat（原 V3Button）** 为准对齐 ui：

| 项 | Soft-Flat | 旧 ui（示例） | 对齐后 |
| --- | --- | --- | --- |
| 主 variant 名 | `primary` | `default` | 双接受，内部同 style |
| 高度 default | h-10 | h-9 | h-10 |
| 高度 sm | h-8 | h-8 rounded-full | h-8 **rounded-xl**（取消 pill） |
| 圆角 | rounded-xl | 部分 full | rounded-xl |
| danger | soft 红底 | solid destructive | Soft-Flat soft danger；`destructive` 别名可映射 |
| glass | 有 | 无 | 保留；ui 可导出但不推广到 data-table |
| secondary/link | 无 | 有 | 可保留映射到 outline/ghost 或独立轻量 style，**不得**分叉主路径视觉 |

**允许的 intentional 差异归零**；不允许顺带改 brand 色值。

## 任务清单

- [ ] 抽出 `buttonVariants` 单文件
- [ ] superteam `Button` 改用它
- [ ] `ui/button` 改用它 + variant 兼容层
- [ ] 更新 `ui/button` 相关测试 / alert-dialog 观感抽检
- [ ] `docs/design-system/actions.md` 写明唯一真源路径
- [ ] 确认 `forwardRef` / `asChild` / disabled / loading 行为不回归

## 验证

- [ ] typecheck + 相关 component tests
- [ ] 手动或浏览器：data-table 工具条按钮 vs 页面主按钮圆角/高度一致
- [ ] 若有 screenshot：更新须在 PR 列出「对齐项」而非 silent

## 完成定义

- 仓库内仅一处定义主按钮视觉 class 集合
- `rg` 无第二套平行 `buttonBase`/`buttonVariant` 对象

## 回滚

Revert；风险低于 E。

## 不做

- 不强制本阶段清 features 的 ui/button import（D 做）
- 不改 Badge/Card 实现（可另小 PR 将 Card 外壳 class 与 SoftCard 对齐，但不阻塞 C）

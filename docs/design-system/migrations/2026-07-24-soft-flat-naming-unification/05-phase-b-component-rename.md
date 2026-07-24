# 05 · 阶段 B — 组件 API 去 `V3` 前缀

## 目标

- 公共导出使用无版本前缀名（`Button`、`EmptyState`…）
- 旧名 `V3*` 作为 **deprecated alias** 保留完整窗口（见 Q8）
- **不改** CSS token 名与 `v3-*` Tailwind 类（留 E）
- **本阶段可改** `data-slot`（推荐与组件名一并去 v3，测试同 PR）；若担心面过大，可拆 B1 标识符 / B2 data-slot

## 前置

- 阶段 A 完成
- Q1–Q4、Q8、Q3 已确认
- inventory 已刷新

## 推荐子阶段

### B1. 定义侧改名 + alias

文件：`v3-components.tsx` → 目标文件名（`primitives.tsx` 等）

1. 将 function/const/type 改为新名（按 `02` 表）
2. 底部：
   ```ts
   export const V3Button = Button
   export type V3Tone = Tone
   // …所有旧名
   ```
3. 更新 `index.ts` export 路径
4. 单测文件改 import；**增加**一组断言：新旧 export 引用相等

### B2. 全仓标识符 codemod（`apps/web/src`）

顺序建议（避免部分替换）：

1. 长名优先：`V3PermissionDenied`、`V3ToolbarSearch`、`V3ButtonVariant`…
2. 再短名：`V3Button`、`V3Td`…
3. **禁止**替换字符串字面量中的业务文案；仅 TS/TSX 标识符
4. 工具：`jscodeshift` / `ts-morph` / 审慎 `rg`+perl；人工 diff 抽样

### B3. data-slot 同步（若本阶段做）

1. 组件内 `data-slot="v3-…"` → 新名
2. `index.css` 选择器
3. 所有测试 querySelector
4. layout：`v3-authenticated-shell` 等

### B4. 不在本阶段做

- 删除 alias
- 改 `--v3-*`
- 改 feature 的 `ui/button` 引用（阶段 D）
- 合并 button 样式源（阶段 C，可紧随 B 但最好第二 PR）

## 验证

- [ ] typecheck 绿
- [ ] web test 绿（含截图：B1 若只改 JS 名通常截图不变；B3 改 slot 不变像素）
- [ ] `rg '\bV3Button\b' apps/web/src`：仅 alias 定义处与 deprecated 测试（若业务已全部 codemod 则为 0 业务引用）
- [ ] 抽查：从 `@/components/superteam` 可 `import { Button, EmptyState, SoftCard }`

## 完成定义

- 新名成为默认写法
- 旧名仍可用但不在新代码出现（靠 review + 后续删 alias）

## 回滚

Revert PR。alias 策略使下游半迁移分支仍可能工作——若已 codemod 全仓，回滚需整 PR revert。

## 风险注意

- `Button` 与自动导入可能错误解析到 `ui/button`：依赖路径 `@/components/superteam` 显式 import，避免 barrel 混乱
- IDE 自动 import 教唆：阶段 A 文档写明正确路径

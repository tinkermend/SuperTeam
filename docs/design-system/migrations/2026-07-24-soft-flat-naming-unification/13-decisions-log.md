# 13 · 决策记录（人类对话增量）

> 由审核对话写入；回写到 `02` / `11` 的对应项。

## 2026-07-24

### Q9 — 阶段 C 将 ui/Button 几何对齐 Soft-Flat

- **人类决定**：**批准**
- **含义**：允许/要求阶段 C 以原 V3Button 规格为唯一真源（如 default 高 h-10、`rounded-xl`、sm 取消 pill 全圆角等，细节见 `06-phase-c-button-single-source.md`）
- **约束**：只做消歧对齐，不改 brand 色值；截图若红须在 PR 列出对齐项

### Q4 — data-table 是否改为 superteam `Button`

- **决定（Agent 评估后代选）**：**本迁移不强制**；data-table（及 calendar/sidebar/alert-dialog 等基础设施）**长期允许** `import { Button } from '@/components/ui/button'`，前提是阶段 C 已 **样式同源**。
- **评估摘要**：
  - 触面：`components/data-table` 6 文件，约 15 处 Button；variant 含 `ghost` / `outline` / `default` / `secondary` / `icon` / `sm`
  - 阶段 C 后：换 import **几乎不改变视觉**，只改路径与 variant 别名（`default`→`primary`、`secondary` 需兼容层）
  - 收益：叙事略干净；**不**再消除第二套视觉（视觉已在 C 消除）
  - 成本：D 阶段噪音、secondary 映射、与 Radix 复合组件（alert-dialog 用 `buttonVariants`）不一致的 import 故事
  - 结论：基础设施继续走 `ui/button`（内部 primitive）更清晰；业务 `features/routes` 仍禁止 ui/button
- **可选后续**（非本迁移 DoD）：E 结束后另开清洁 PR 统一 import，不阻塞主线

### Q5 — card/bg 与 shadcn token 关系

- **人类决定**：**选项 A — 真值合并**（2026-07-24）
- **含义**：
  - `--v3-card` / `--v3-bg` 等与 shadcn 重叠的表面，合并为同一真值；业务 class 目标为 `bg-card` / `bg-background`（及 foreground 等既有 shadcn 名，若适用）
  - Soft-Flat 独有 token（`ink` / `line` / `ok` / `brand-soft` 等）去 `v3-` 前缀为语义名，不另造平行「第二套白底」
  - 合并时以 Soft-Flat 现行值为准写入唯一真源，shadcn 变量 `var(--…)` 互指；**禁止借机改审美 hex**
- 白话说明仍见本节附录（供后人理解）。

#### 附录：Q5 白话说明

现在系统里对「卡片白底 / 页面底」有**两套名字**说同一类东西：

| | shadcn 老名字 | Soft-Flat（现 v3）名字 |
| --- | --- | --- |
| 页面底 | `--background` → 类名 `bg-background` | `--v3-bg` → `bg-v3-bg` |
| 卡片白底 | `--card` → `bg-card` | `--v3-card` → `bg-v3-card` |

去 `v3-` 前缀时，不能只是把 `--v3-card` 改成 `--card` 就完事——因为 **`--card` 已经存在**。要先定两者关系：

**选项 A — 真值合并（推荐）**

- 世界上只留**一个**卡片色、一个页面底色。
- 例如：`--card` 与（原）`--v3-card` 指向同一值；业务里原来的 `bg-v3-card` 改成已经存在的 `bg-card`。
- 原 `--v3-ink`、`--v3-line`、`--v3-ok` 等 shadcn **没有**的，去前缀成 `--ink` / `text-ink` 等新语义名。
- **优点**：类名更少、和 shadcn 不打架、和 tokens.md「能叠 shadcn 就叠」一致。  
- **缺点**：codemod 时 `bg-v3-card`→`bg-card` 要确认两边色值本来就该一致（本迁移禁止借机改色，合并前核对 light/dark 是否已一致或可接受以 Soft-Flat 为准写一处真值）。

**选项 B — 保持两套表面名**

- shadcn 继续 `--card` / `bg-card`。
- 原 v3 改成例如 `--surface` / `bg-surface`（不叫 card）。
- **优点**：零合并争议。  
- **缺点**：长期两套「白底」名字（`bg-card` vs `bg-surface`），双轨感残留在 token 层。

**选项 C — 全部独立新名，连 shadcn 也不共用**

- 不推荐：最吵，迁移最大。

人类需在 A / B（或其它）上勾选一次。

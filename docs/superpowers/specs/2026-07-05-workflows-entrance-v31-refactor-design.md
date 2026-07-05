# 流程编排首页（/workflows entrance）v3.1 语义+密度重构设计

日期：2026-07-05 · 状态：已实施。第一版为"表格+统计筛选条"；用户反馈"只剩一个数据表格不像企业级平台、且流程编排不该用表格维度展现"后，最终形态改为**流水线运行流**（用户在选项中确认），并同步完成详情页词表清理。

## 最终形态（v2 · 流水线运行流）

- 保留 WorkSurface 壳 + 头部统计筛选条（全部/进行中/等待人工/阻断/已完成，V3Chip 可点筛选）。
- 数据本体从 `V3Table` 改为分诊分组运行列表（`role="list"`）：**需要介入**（阻断置顶+等待人工）→ **进行中**（running/planning）→ **已完成** → **其他**；组头为灰底细条+语义圆点+计数。
- 每行是一条"流水线运行条"：状态 pill + 标题 + 项目名 + 高价值徽标一行；第二行 **mini 节点链**（已完成=brand、运行=info、等待人工=warn、阻断=danger、待执行=空心，节点 >14 退化为等宽分段条，total=0 显示"待规划"虚线）+ `n/m` + 非零计数；第三行摘要（阻断类才用 danger 圆点）。阻断行带左 danger accent bar + 极淡红底。整行可点进详情，标题 Link 承载键盘焦点。
- 共享 `workflowStatusTone`：planning 从 warn 降为 mute（规划中是协调线程正常工作态）；entrance 本地重复的 tone 映射删除，统一走共享函数。

## 详情页词表清理（同批完成）

- 编排头 IconTile 不再按状态换色（info/warn → brand）；去掉与需求摘要栏重复的状态 pill；运行/等待人工/阻塞计数 pill 零值不渲染。
- 任务节点卡：「等待人工决策/需要人工审批」pill 从 ok 绿改为 warn；风险 pill 按级别取色（high→danger、medium→warn、low 不渲染）。
- 决策附件节点从绿色系（border/handle/icon 全 ok）改为中性卡 + brand handle；阶段标签图标 artifact 紫→ink-3，硬编码 `bg-white/92`（深色模式 bug）→ `bg-v3-card/92`。

---

以下为第一版（表格形态）设计记录，结构与降噪原则仍然有效，仅数据本体容器被流水线运行流取代：

## 背景与问题

实测现状（24 个实例，1728px 全页截图）：

1. **行级暖色滥用**：`workflowRowTone` 把所有 `waiting_human` 实例整行点亮 warn 软底，24 行里 16 行发光，违反 DESIGN.md「默认安静，例外强调」，真正的阻断行反而不突出。
2. **Signature 卡白字不可见**：`text-white/80` 写在浅色 signature 面上，"流程实例"标签和说明文字直接消失。
3. **KPI 带占首屏**：signature 卡 + 3 张 V3MetricCard 占约 190px 高，其数值与表格本身重复。
4. **行内噪音**：每行 3 个"运/人/阻"计数芯片（多数为 0）、按状态换色的 IconTile、`low` 优先级 pill、彩色摘要图标——多重状态编码并存，违反 v3.1「一行最多 1 个语义色状态编码」「类别图标不按状态换色」。
5. **真 bug**：`features/workflows/index.tsx` 的 dispatch blocker banner 用 `border-v3-warning/25 bg-v3-warning/5`，`v3-warning` token 不存在，样式静默落空。

## 范围

只改 `apps/web/src/features/workflows/` 的 entrance 路径（`workflow-entrance.tsx`、`workflow-shell.tsx`、`index.tsx` 两个 banner 的 token bug）与配套测试。detail 页（编排画布）、API、数据结构不动。不改全局 token 与公共组件。

## 设计

### 1. KPI 带 → 统计筛选条（密度试点核心动作）

删除 SignatureCard + 3 张 V3MetricCard。WorkSurface 头部改为一行：

- 左：标题「流程实例」+ 同步中/刷新失败 pill（沿用现有逻辑）。
- 右（或同行 wrap）：V3Chip 统计筛选条——`全部 n` / `运行中 n` / `等待人工 n` / `阻断 n` / `已完成 n`，点击筛选表格，active 态用现有 V3Chip 蓝柔底。
- 计数口径：运行中 = status running；等待人工 = status waiting_human 或 planning；阻断 = blocked_nodes>0 或 failed/cancelled；已完成 = completed。分类互斥优先级：阻断 > 等待人工 > 运行中 > 已完成 > 其他（其他仅计入「全部」）。

### 2. 行级降噪（v3.1 词表落地）

- 行 tone：只有「阻断」类（blocked_nodes>0 / failed / cancelled）给 danger 行；waiting_human 不再整行点亮。
- 删除行内 IconTile；状态唯一由 StatusPill 表达。
- 「人工/阻断」三芯片列改为灰字内联计数，只显示非零项（如 `运行 1 · 人工 2`），阻断>0 时该项用 `text-v3-danger-text`；全零显示 `—`。
- 摘要列去彩色图标：阻塞摘要前置 danger 小圆点，其余无装饰，灰字 line-clamp-1。
- badge 只显示有信息量的：priority/risk tone 为 mute 的（low 等）不渲染；SLA 沿用 breached 逻辑。

### 3. 密度

- 进度列：删「已完成」标签行与对勾图标，只留细进度条 + `3/3`（tabular-nums）。
- 标题 line-clamp-1，project 名保持次行 truncate。
- 整行可点击进入详情（cursor-pointer + onClick navigate），标题保留 Link 承载键盘焦点，符合「整对象可选中」。
- 行高目标 ≤56px。

### 4. 顺手修复

- `index.tsx`：`v3-warning` → `v3-warn`（两处 class）。
- `workflow-shell.tsx`：页头 iconTone `info` → `brand`（类别图标退出状态色）。

## 错误与状态

加载/错误/空态逻辑不变（V3StateSurface 族）。筛选后为空时显示轻量空态「该筛选下暂无实例」。

## 测试与验证

- 更新 `features/workflows/index.test.tsx` 中受影响断言；`corepack pnpm --filter ./apps/web run test -- workflows` 定向跑。
- 浏览器实开 `/workflows`（浅+深）对比验证：默认安静、阻断行唯一点亮、首屏以列表为主、筛选条可用、无横向溢出。

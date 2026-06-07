# 团队管理页面 → 卡片网格布局改造分析

## 参考图片

![目标布局](file:///Users/wangpei/.gemini/antigravity-cli/brain/9eb0b349-f4dd-4c04-bf2e-ba838ad322f2/uploaded_media_1780818072229.jpg)

## 当前实现 vs 目标布局

| 维度 | 当前实现 | 目标布局 |
|------|---------|---------|
| **数据展示** | 10 列横向表格（Table） | 3 列卡片网格（Grid） |
| **信息密度** | 每行一条团队，展示全部字段 | 每张卡片一个团队，突出负责人 + 代表成员 |
| **顶部统计** | 无 | 汇总统计栏（团队数 / Agent 数 / 代表成员数） |
| **成员预览** | 仅数字（member_count） | 头像堆叠 + 溢出计数（`+3`） |
| **负责人** | 一行文字 | 大头像 + 姓名 + 职位 |
| **操作入口** | 行尾更多菜单 | 底部「查看完整部门 →」链接 |

## 改动评估

### ✅ 可以实现，改动量为**中等**

预计新增/修改 **2-3 个文件**，不影响现有路由、API 或业务逻辑。

### 具体改动项

| # | 改动 | 文件 | 说明 |
|---|------|------|------|
| 1 | **新建卡片网格组件** | `team-card-grid.tsx`（新文件） | 核心工作量。包含：统计栏、3 列 Grid、团队卡片（图标 + 名称 + 成员数 + 级别标签 + 负责人区 + 代表成员头像堆叠 + 底部链接）。使用现有 `TeamIconTile` 和 `UserIdentityAvatar` 组件。 |
| 2 | **修改列表页主入口** | [index.tsx](file:///Users/wangpei/src/singe/SuperTeam/apps/web/src/features/teams/index.tsx) | 把 `<TeamListTable>` 替换为 `<TeamCardGrid>`；可以做视图切换（表格/卡片）保留两种模式。 |
| 3 | **可选：API 层补充** | [teams.ts](file:///Users/wangpei/src/singe/SuperTeam/apps/web/src/lib/api/teams.ts) | 如果需要在卡片上展示「代表成员头像」，当前 `TeamListItem` 只有 `member_count` 数字，**没有成员头像列表**。有两种方案（见下文）。 |

### 代表成员头像的数据来源

图片中每张卡片底部有一组成员头像堆叠。当前 API 的 `TeamListItem` 只返回 `member_count: number`，**不包含成员列表**。

**方案 A：前端额外请求（推荐，后端不改）**
- 对每个团队调用已有的 `listTeamMembers(options, teamId)` 获取成员
- 在卡片渲染时只取前 6 个成员头像，显示溢出计数
- 用 `useQueries` 并行请求，加 staleTime 缓存
- 优点：后端零改动；缺点：团队多时请求数多

**方案 B：后端在 listTeamSummaries 中嵌入 `representative_members`**
- 后端在 `TeamListItem` 中新增 `representative_members: TeamMemberPreview[]` 字段
- 每个 preview 只包含 `user_id`, `display_name`, `avatar`
- 优点：一次请求拿到全部数据；缺点：需要改后端 API 和 SQL

> [!TIP]
> 第一阶段用方案 A 快速上线，后续再优化为方案 B。

### 关于「级别标签（L1/L2...）」

图片中每张卡片右上角有 L1、L2 等级别标签。当前 `TeamListItem` 没有这个字段。有两种处理方式：
1. **利用已有字段**：用 `current_revision`（治理版本号）或 `governance_status` 映射
2. **序号代替**：用列表渲染的序号作为显示编号（L1 = 第1个团队）
3. **新增字段**：在 `metadata` 中存储团队级别

### 工作量估算

| 任务 | 估时 |
|------|------|
| 新建 `TeamCardGrid` 组件 + 样式 | ~2 小时 |
| 修改 `index.tsx` 接入 | ~15 分钟 |
| 代表成员头像数据获取（方案 A） | ~30 分钟 |
| 分页/空状态/加载态/错误态适配 | ~30 分钟 |
| 可选：表格/卡片视图切换 | ~30 分钟 |
| **合计** | **~3.5-4 小时** |

## 需要你确认的问题

1. **是否保留表格视图？** 做成表格/卡片双视图切换，还是直接替换为卡片？
2. **代表成员头像**：先用方案 A（前端多请求）还是先改后端？
3. **L1/L2 级别标签**：用序号、治理版本号还是新增字段？
4. **数字员工 AI 标记**：图片中成员头像上有蓝色「AI」标记——是否需要区分人类成员和数字员工？

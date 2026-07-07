# 数字员工创建页 UI 重设计 · 布局优化 + 视觉升级

## 背景

当前 `apps/web/src/features/employees/create.tsx` 存在两层问题：
- **视觉层**：8 项设计违规（原生 table、硬编码色、缺失 Tier A 玻璃等）
- **布局层**：4 步线性向导结构低效，信息断裂，侧栏浪费
- 步骤进度条缺乏节点/网络母题
- 头像选择器是开发级占位 UI
- 全局等权布局无信息层级

## 布局结构问题（深层）

当前 4 步线性向导存在以下结构性缺陷：

1. **preflight 和 confirm 是只读展示步**：不产生新决策，打断配置流
2. **嵌套层级过深**：外层 4 步 × 内层 4 子 Tab = 8 次切换才能完成创建
3. **步骤间信息断裂**：第 3 步配置时看不到模板完整信息和预检详细结果
4. **右侧 340px 侧栏浪费**：只放静态预检面板，无实时反馈
5. **创建模式选择占整列**：第 1 步左侧 260px 只放 4 个模式选项（2 个还禁用）

## 布局优化方案：4 步线性 → 2 步主从画布

**第 1 步：选择起点**（轻量入口）
- 顶部 segmented 切换创建模式（模板/空白），不占整列
- 主体模板网格卡片选择
- 选完直接进入配置，无独立预检步骤

**第 2 步：配置画布**（主从详情布局）
- **左侧主区**：分层区块（非线性 Tab）
  - 区块 1：身份画像（必填核心，默认展开）
  - 区块 2：能力配置（模板带入可调，默认展开）
  - 区块 3：高级策略（默认折叠）
  - 每个区块可独立展开/折叠，用户按自己节奏配置
- **右侧持续面板**（360px）：实时反馈区
  - 模板来源 diff：默认值 vs 用户修改值
  - 实时治理校验：每选一个能力立即检查 allow-list
  - 预检状态：creation_checks 实时结果（替代独立 preflight 步骤）
  - Payload 预览：CreateDigitalEmployeeInput JSON 实时生成（替代独立 confirm 步骤）
  - 创建按钮：底部固定，满足条件即可创建

**取消的步骤**：
- ❌ preflight → 变成右侧面板的实时校验区
- ❌ confirm → 变成右侧面板的 payload 预览 + 底部创建按钮

**优化收益**：
1. 8 次切换 → 2 步 + 分层区块，认知路径缩短 60%
2. preflight/confirm 不再打断配置流
3. 右侧侧栏从静态展示 → 动态校验 + diff + payload
4. 配置时始终可见模板默认值和治理冲突

---

## 四套原型

### 0. 布局优化版（推荐）`employee-create-optimized-layout.html`

**设计意图**：整合布局结构优化 + 极光玻璃视觉。2 步主从画布 + 分层区块 + 实时反馈侧栏。

**布局特征**：
- 2 步流程（选择起点 → 配置画布），取消 preflight/confirm 独立步骤
- 左侧分层可折叠区块（身份/能力/高级策略），非线性 Tab
- 右侧 360px 实时反馈面板（模板 diff + 治理校验 + 预检状态 + payload 预览）
- 底部 sticky 操作栏（保存草稿 + 创建按钮）
- 能力 chip 带治理状态（allowed/filtered 实时标注）
- 字段带"已修改"标记（对比模板默认值）

**视觉特征**：极光玻璃沉浸（同极光玻璃版）

**适用场景**：如果你认同布局也需要优化，不只是换皮肤。这是推荐方案。

### 1. 极光玻璃沉浸版 `employee-create-aurora-glass.html`

**设计意图**：Tier A 标准实现。创建向导是单一主动作的入口画布，按 DESIGN.md 分类应使用极光玻璃沉浸层。

**视觉特征**：
- 青/紫/品牌蓝三束低透明光晕极光底（`--v3-aurora-bg` 同源色值）
- 半透明玻璃面板（`rgba(255,255,255,0.55)` + `backdrop-filter: blur(22px) saturate(140%)`）
- 节点式步骤进度条（圆形节点 + 连接线，已完成段渐变蓝）
- 签名头像选择区（虚线边框 + 选中发光环 + 对勾角标 + hover scale）
- 玻璃内嵌字段（`rgba(255,255,255,0.9)` 背景 + 品牌蓝 focus ring）
- 浮动装饰光斑（3 个 `position:fixed` orb，`pointer-events:none`）

**适用场景**：如果你想要创建向导有仪式感和签名感，像 task-launch 极光版那样的沉浸体验。

**Token 对齐**：全部使用 `--v3-aurora-*` 系列 token 的同源色值。

---

### 2. 指挥中枢版 `employee-create-command-nexus.html`

**设计意图**：深色控制台风格，高密度数据表达，强调可审计性和工程感。参考 `workflow-orchestration-command-nexus.html`。

**视觉特征**：
- 深色背景（`#0a0e1a`）+ 极淡网格底纹（40px 方格）
- 终端式步骤进度条（`font-family: mono` + `select_template` / `preflight_check` 命令式命名）
- 高密度模板表格（sticky header + uppercase 列头 + 左侧 accent bar 选中态）
- JSON payload 预览侧栏（代码高亮 + 等宽字体 + API 路径注释）
- 全大写 STATUS PILL（`PASSED` / `WARNING` / `DEFAULT`）
- session ID 标识（`session: emp-create-7f3a2b`）

**适用场景**：如果你偏好 Linear/Vercel 那种深色高密度工程感，强调可审计性和数据透明度。

**Token 对齐**：深色端使用 `--v3-*` dark 变体的同源色值派生。

---

### 3. 柔和卡片版 `employee-create-soft-clarity.html`

**设计意图**：Tier B 风格但严格遵循 v3 Soft-Flat 规范。参考 `templates.tsx` 的正确实现，是"最安全、最规范"的方案。

**视觉特征**：
- 浅色极光底（`--v3-shell-background` 同源微渐变 + 品牌蓝/绿 radial）
- SoftCard 白卡（`--v3-card` + `--v3-r-card` 22px 圆角 + `--v3-shadow` 弥散阴影）
- IconTile 图标容器（`--v3-brand-soft` 底 + 品牌蓝图标）
- 签名头像选择区（`--v3-signature-surface` + `--v3-signature-wash` 渐变 + `--v3-signature-border`）
- StatusPill 语义状态胶囊（ok/warn/brand/mute 全 tone 覆盖）
- 清晰三级层级：页面头卡 → 主工作区 → 侧栏摘要
- V3Button 风格按钮（`--v3-brand-grad` 渐变主按钮 + ghost 次按钮）

**适用场景**：如果你想要最稳妥的 v3 规范对齐，与项目其他页面（templates.tsx、employees index）视觉完全一致。

**Token 对齐**：100% 使用 `--v3-*` token，零硬编码色值。

---

## 数据真实性

三套原型均基于真实 API 数据模型（`apps/web/src/lib/api/employees.ts`）：

| 字段 | 数据来源 |
|------|----------|
| employee_type / template | `DigitalEmployeeCreateOptions.employee_types[]` |
| creation_checks | `DigitalEmployeeCreateOptions.creation_checks[]` |
| recommended_skills | `DigitalEmployeeTypeOption.recommended_skills` |
| recommended_mcp_servers | `DigitalEmployeeTypeOption.recommended_mcp_servers` |
| recommended_provider_types | `DigitalEmployeeTypeOption.recommended_provider_types` |
| runtime_provider_options | `DigitalEmployeeCreateOptions.runtime_provider_options[]` |
| avatar_assets | `listDigitalEmployeeAvatarAssets()` |
| team_config (allow-list) | `DigitalEmployeeCreateOptions.team_config` |
| policy_defaults | `DigitalEmployeeCreateOptions.policy_defaults` |

**POST payload** 对应 `CreateDigitalEmployeeInput`：
- `team_id`, `employee_type`, `name`, `avatar_asset_id`, `role`, `description`, `risk_level`
- `provider_type`, `capability_selection`, `context_policy`, `approval_policy`

---

## v3 规范对齐检查

| 检查项 | 极光玻璃版 | 指挥中枢版 | 柔和卡片版 |
|--------|-----------|-----------|-----------|
| 容器组件（SoftCard/WorkSurface） | 玻璃面板 | 深色 panel | SoftCard ✅ |
| Token 覆盖率 | aurora-* 同源 | dark v3-* 同源 | --v3-* 100% ✅ |
| 圆角体系 | 24px/16px | 12px/8px | 22px/14px (--v3-r-*) ✅ |
| 阴影体系 | 玻璃 inset + pop | 深色 panel shadow | --v3-shadow ✅ |
| StatusPill tone | ok/warn/brand | ok/warn/brand/mute | ok/warn/brand/mute ✅ |
| IconTile | 玻璃图标 | icon-tile 方块 | brand-soft icon-tile ✅ |
| 步骤进度母题 | 节点+连接线 | 终端式命令 | 节点+连接线 |
| 头像选择器 | 签名 focal + 发光 | 紧凑 + 选中角标 | 签名 focal + wash 渐变 |
| Tier 分类 | Tier A ✅ | 自定义深色 | Tier B ✅ |
| 暗色模式支持 | 极光自带 | 原生深色 | token 自动切换 ✅ |

---

## 推荐

- **如果你想要仪式感和签名体验** → 选极光玻璃版（与 task-launch 极光版一脉相承）
- **如果你偏好工程感和高密度** → 选指挥中枢版（深色控制台风格）
- **如果你想要最稳妥的规范对齐** → 选柔和卡片版（与 templates.tsx 完全一致，落地风险最低）

三套原型均展示了第 3 步（完成配置）的完整界面，包含身份画像、能力配置、头像选择和侧栏摘要。选定方向后可扩展其余 3 步。

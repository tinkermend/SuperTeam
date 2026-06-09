# 2026-06-09-dual-layer-skill-management-design

## 1. 背景与目标 (Background & Goals)

SuperTeam 需要一套更精细的治理模型，将“团队公共能力”与“个人特有能力”进行解耦。

### 核心设计哲学
- **团队管工具**：团队负责统筹通用技能 (Skills) 和 MCP 服务器。
- **个人管灵魂**：数字员工个人负责定义人格 (Personality)、宪法 (Constitution) 和私有工具。
- **物理隔离**：各层级界面职责明确，互不干扰。

## 2. 逻辑架构 (Architecture)

### 2.1 治理层级 (Governance Layers)

| 层级 | 管理内容 | 权限需求 | UI 表现 |
| :--- | :--- | :--- | :--- |
| **团队 (Team)** | 公共技能、公共 MCP | Team Admin/Owner | 资源列表、市场选择器 |
| **员工 (Employee)** | 宪法/人格、个人技能、个人 MCP | Employee Owner | MD 编辑器、个人能力设置 |
| **个人凭据池 (User)** | Auth Token (API Keys) | 个人 | 凭据管理列表 |

### 2.2 逻辑合并规则
- **技能/MCP**：`最终工具 = 团队强制 (只读) + 个人特有 (可编辑)`。
- **宪法 (Constitution)**：仅采用个人层级的配置，不从团队继承。
- **凭据 (Auth)**：MCP 的 Token 引用自“个人凭据池”，支持跨员工复用。

## 3. 页面设计方案 (UI Design)

### 3.1 团队管理页 - [治理与能力]
- **模块 A：公共技能 (Skills)**
  - 支持从技能市场安装技能到团队。
  - 显示技能卡片，一旦安装，该团队下所有员工强制获得，不可取消。
- **模块 B：公共 MCP (MCP Servers)**
  - 配置远程 MCP HTTP 地址及 Auth Token。
  - 用于全团队共享的外部能力。

### 3.2 数字员工详情页 - [核心配置]
- **Tab 1: 宪法/人格 (Instructions)**
  - **组件**：文件列表 + Markdown 编辑器（参考技能管理编辑器样式）。
  - **文件**：支持新建/编辑 `AGENTS.md`, `SOUL.md` 等。
- **Tab 2: 能力设置 (Capabilities)**
  - **个人技能区**：可自行从市场安装特有技能。
  - **个人 MCP 区**：填写远程 URL，Auth Token 通过下拉框关联“个人凭据池”。
  - **继承预览区**：底部以只读形式列出团队下发的技能/MCP。

## 4. 数据模型变更 (Data Schema)

### 4.1 凭据管理
```sql
CREATE TABLE user_credentials (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    user_id UUID NOT NULL,
    name TEXT NOT NULL,
    credential_type TEXT NOT NULL, -- 'mcp_token'
    encrypted_value TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

### 4.2 个人 MCP 绑定
```sql
CREATE TABLE digital_employee_mcp_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    employee_id UUID NOT NULL,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    credential_id UUID REFERENCES user_credentials(id),
    status TEXT DEFAULT 'active'
);
```

## 5. 运行时逻辑 (Runtime Logic)

### 5.1 技能加载
1. 获取 `team_id` 下的所有 `skill_team_bindings`。
2. 获取 `employee_id` 下的所有 `skill_agent_bindings`。
3. 合并去重后推送到 Runtime Agent。

### 5.2 MCP Token 注入
1. 在调用 MCP 接口前，后端根据 `credential_id` 解密 Token。
2. 将 Token 注入到 HTTP 请求头：`Authorization: Bearer <TOKEN>`。

## 6. 后续演进 (Evolution)
- **权限校验**：后续接入 OpenFGA，通过 `team.capability.manage` 和 `employee.capability.edit` 进行细粒度控制。
- **记忆管理**：将 `MEMORY.md` 接入向量数据库，实现长期记忆自动载入。

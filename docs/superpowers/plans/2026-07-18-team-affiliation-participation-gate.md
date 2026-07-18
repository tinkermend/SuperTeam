# 数字员工团队归属参与门禁实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 落地用户拍板的产品规则（2026-07-18）：数字员工可以无团队创建（候岗态合法），但**参与项目必须先归属团队**——项目成员写入、协调器派发池、chat 锚定三条链全部立服务端门禁；同时补齐归队管理能力（已有团队收编无归属员工 + 员工换队），让候岗员工有产品内的归队路径。

**Architecture:** 现状是"团队优先"只活在创建项目向导 UI 里，服务端从未有此不变量，且三处有意相反实现：`validateMembers`（project/service.go:7124）只查角色/类型；协调器借调闸门显式注释 "No owning team → always eligible"（projectcoordination/project_store.go:203-205）；一键补员（staff-gap-dialog.tsx:132-159）创建无团队员工直接塞进项目成员。本计划把不变量立在服务端（唯一堵住所有入口的位置），协调器闸门语义翻转，chat 按用户裁定校验**项目成员资格**（不止团队归属）。归队 API 复用已有 `BindDigitalEmployeeToTeam` sqlc（自带 `team_id IS NULL` 防抢人）；换队新增 sqlc。删团队维持整队置空（用户裁定：候岗是合法状态，靠归队入口再收编）。

**用户已裁定（不再讨论）：** chat 收口=校验项目成员资格；删团队=维持置空；归队能力=团队侧收编+员工侧换队，UI 只做团队详情页最小收编入口，换队本期 API-only。存量已清（07-18 直接改库，dev 库 7 名无归属员工已归默认团队，真实 API 复验 unassigned=[]）。

**Tech Stack:** Go (chi + sqlc + Temporal activity) / OpenAPI / React + TanStack Router + vitest browser。无数据库迁移（不加 NOT NULL 约束——候岗态合法，规则是应用层参与门禁；`digital_employees.team_id` 可空语义不变）。

**关键实现事实（已勘察）：**
- sqlc `queries` 包跨模块共享，project pg_repository 可直接调用新查询；project.Service 对可选能力走类型断言模式（先例 `ProjectTeamScopeAuthorizer`，service.go:144），新校验依此模式做窄接口，测试 fake 不实现即自然跳过，真实 pg 仓储必实现。
- chat 校验器已有适配器链：employee run_service 的 `chatAnchorValidator` → `project.ChatAnchorProjectValidatorAdapter`（chat_anchor_validator_adapter.go:35），扩展该接口即可，改动面在两个包+测试 fake（run_service_test.go:2369）。
- 收编 SQL `BindDigitalEmployeeToTeam`（queries/tenant_team.sql:1-9）带 `AND team_id IS NULL`，天然只收候岗员工；现仅 `CreateTeamWithInitialMembers`（tenant/pg_repository.go:131）可达。
- web `createDigitalEmployee` 已支持可选 `team_id`（lib/api/employees.ts），补员对话框只是没传。
- 项目自身团队：`projects.team_id`（创建时取 `draft.sourceTeamIds[0]`），补员默认归入它。
- chat-panel 已有项目选择器（chat-panel.tsx:382 Select）与 projectId prop，员工过滤只需叠加项目成员查询。
- 协调器 `lendingEligibleEmployeeIDs`（project_store.go:186）当前 `lending == nil || ownTeamID == nil` 时整体放行（返回 nil = 不过滤）——teamless 门禁**不得**依赖 lending 服务与项目团队存在性，需独立于借调闸门生效。它在 `LoadProjectCoordinationSnapshot`（activity，非 workflow.go），无 Temporal 回放确定性问题。

## Global Constraints

- 根级命令 `corepack pnpm <script>`；Go 测试 `cd apps/control-plane && go test ./internal/...`（定向包）；Web 测试 `corepack pnpm --filter @superteam/web test -- --no-file-parallelism`；禁止 `npx playwright` / `npx vitest`。
- 契约改动：`contracts/control-plane/openapi.yaml` → `corepack pnpm generate:control-plane` + `corepack pnpm verify:contracts`；TS 客户端手写在 `apps/web/src/lib/api/`。
- 前端改动前读 `DESIGN.md`；内部跳转仅 TanStack Router `Link`/`navigate`。
- 服务启停 `scripts/dev-services.sh`（主 checkout）；`restart control-plane` 自动跑迁移。
- 共享 checkout 有并发会话：开发走隔离 worktree 分支 `feat/team-affiliation-gate`，合并用 ref 手术，不切主 checkout 分支。
- 每任务收尾提交，commit 尾行 `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。
- 收尾走 `$superteam-completion-check`；合并 main 后基于 main 真实 E2E 通过才可删分支。

---

### Task 1: 服务端不变量 —— 项目成员写入门禁

**Files:**
- Modify: `apps/control-plane/internal/storage/queries/employee_execution.sql`（新查询）+ 生成物
- Modify: `apps/control-plane/internal/project/service.go`、`pg_repository.go`、`handler.go`、`errors.go`（或错误定义所在文件）
- Modify: `apps/control-plane/internal/project/service_test.go`、`handler_test.go`

**Interfaces:**
- Produces: sqlc `ListDigitalEmployeeTeamAssignments(tenant_id, ids uuid[]) → (id, team_id)`；窄接口 `MemberTeamAssignmentResolver`（project 包，pg_repository 实现）；typed error `ErrTeamlessProjectMember`（HTTP 400，报错消息含违规员工 id/名单）。

- [ ] **Step 1:** 新增 sqlc 查询（`WHERE tenant_id=$1 AND id = ANY($2) AND deleted_at IS NULL`，返回 id+team_id），`corepack pnpm generate:control-plane` 若该生成走 make 则用 `make -C apps/control-plane sqlc-generate`（以仓库脚本实际为准）。
- [ ] **Step 2:** project 包定义窄接口 + pg_repository 实现；`CreateProject` 与 `ReplaceProjectMembers` 在 `validateMembers` 通过后、落库前：收集 `principal_type=digital_employee` 的 id，resolver 存在则批量查——任一员工不存在或 `team_id IS NULL` → `ErrTeamlessProjectMember`。resolver 不存在（测试 fake）跳过，与 `ProjectTeamScopeAuthorizer` 先例一致。
- [ ] **Step 3:** handler 错误映射 400 + 中文错误消息（前端直接展示）。
- [ ] **Step 4:** 单测：teamless 成员被拒（Create/Replace 双入口）、有团队通过、不存在员工被拒、纯人类成员不触发查询；`go test ./internal/project/`。

### Task 2: 协调器派发池 —— teamless 从 always-eligible 翻转为 skipped

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
- Modify: `apps/control-plane/internal/project/`（若需新增事件类型常量）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`

**Interfaces:**
- Produces: teamless 候选被排除出 executor pool + 审计事件（`reason_code: "teamless_employee_skipped"`，复用 lending-skip 事件形态或新增 `ProjectEventTeamlessEmployeeSkipped`）。

- [ ] **Step 1:** 重构 `LoadProjectCoordinationSnapshot` 的候选过滤：teamless 门禁独立前置——用已有 `ResolveEmployeeTeams`（lending 依赖存在时）或新增独立 resolver 取员工团队；`!hasTeam || team == uuid.Nil` → skipped（不再 eligible）。注意三个原放行条件（lending nil / ownTeamID nil / 查询错误 fail-open）只对**借调**闸门维持 fail-open，teamless 门禁在能取到团队数据时必须生效；取不到数据（错误）保持 fail-open 并记事件，理由同借调：不 strand 规划，权威门禁在 Task 1 成员写入层。
- [ ] **Step 2:** `recordLendingSkips` 或新函数为 teamless skip 落事件（`digital_employee_id` + reason，不带假 team_id）。
- [ ] **Step 3:** 单测盖住：teamless 成员被 skip + 事件、own-team 成员照常、借调逻辑不回归、全员被 skip 时触发既有 `no_plannable_digital_employee` 阻塞事件；`go test ./internal/workflow/projectcoordination/`。

### Task 3: chat 锚定 —— 校验项目成员资格

**Files:**
- Modify: `apps/control-plane/internal/employee/run_service.go`（`chatAnchorValidator` 接口）
- Modify: `apps/control-plane/internal/project/chat_anchor_validator_adapter.go` + `_test.go`
- Modify: `apps/control-plane/internal/employee/run_service_test.go`（fake 扩展）

**Interfaces:**
- Produces: 接口方法 `ValidateChatParticipant(ctx, tenantID, projectID, digitalEmployeeID) error`——校验该员工是项目的 `active` `digital_employee` 成员；违规 → `ErrInvalidInput` 包装、HTTP 400 中文消息。

- [ ] **Step 1:** 扩展接口与适配器：adapter 走 project service 的成员列表（复用 `ListProjectMembers` 仓储），校验 principal 匹配 + type=digital_employee + status=active。
- [ ] **Step 2:** `createChatRun` 在 `ValidateChatAnchorProject` 后调用新校验。
- [ ] **Step 3:** 更新 run_service_test fake 与用例（成员通过/非成员拒/inactive 拒）；`go test ./internal/employee/ ./internal/project/`。

### Task 4: 归队管理 API —— 团队侧收编 + 员工侧换队 + 契约

**Files:**
- Modify: `apps/control-plane/internal/api/server.go`（两条路由）
- Modify: `apps/control-plane/internal/tenant/handler.go`、`service.go`、`repository.go`、`pg_repository.go`（收编）
- Modify: `apps/control-plane/internal/employee/handler.go`、`service.go`、`pg_repository.go` + `queries/tenant_team.sql`（换队新查询）
- Modify: `contracts/control-plane/openapi.yaml`
- Modify: `apps/web/src/lib/api/teams.ts`、`apps/web/src/lib/api/employees.ts`（TS 客户端）
- Tests: tenant/employee 模块单测 + api 路由测试

**Interfaces:**
- Produces: `POST /api/v1/teams/{teamId}/digital-employees` body `{digital_employee_id}`——收编候岗员工（authz `ActionTeamUpdate`；复用 `BindDigitalEmployeeToTeam`，已归属者报 400"已有归属"）；`PUT /api/v1/digital-employees/{employeeId}/team` body `{team_id}`——换队/首次归队（authz 沿用员工管理动作；新 sqlc `ReassignDigitalEmployeeTeam` 无 `team_id IS NULL` 条件；校验目标团队存在且 active）。两者均落审计事件。

- [ ] **Step 1:** 收编：tenant 模块 service+repository 暴露单员工 bind（事务内 bind + 审计），handler + 路由 + 单测（收编成功/已归属 400/员工不存在 404/跨租户拒）。
- [ ] **Step 2:** 换队：新 sqlc 查询 + employee 模块 service/handler + 目标团队存在性校验 + 审计 + 单测（换队成功/首次归队成功/团队不存在 400/团队 disabled 400）。换队副作用（agent home dir 按 TeamID 键、团队技能/MCP 继承变化）在 handler 注释与 openapi description 中写明。
- [ ] **Step 3:** openapi.yaml 两条 path + `corepack pnpm generate:control-plane` + `corepack pnpm verify:contracts`。
- [ ] **Step 4:** TS 客户端 `bindTeamDigitalEmployee` / `reassignDigitalEmployeeTeam` + api 单测。

### Task 5: 前端 —— 补员带队 + chat 成员过滤 + 团队详情收编入口

**Files:**
- Modify: `apps/web/src/features/projects/components/staff-gap-dialog.tsx` + `.test.tsx`
- Modify: `apps/web/src/features/task-launches/components/chat-panel.tsx` + 相关测试
- Modify: `apps/web/src/features/teams/components/team-overview-tab.tsx`（或团队详情等价位置）+ 测试

**先读 `DESIGN.md`。**

- [ ] **Step 1:** 补员：dialog 拿项目 `team_id`（页面已有项目详情则传 prop，没有则 dialog 内查一次）传给 `createDigitalEmployee`；项目无团队时回退现状（不带 team_id——Task 1 会把后续成员写入拒掉，此时 dialog 显式报"项目未绑定团队，无法补员"而不是创建后半程失败）。更新测试断言 payload 含 team_id。
- [ ] **Step 2:** chat：员工下拉先选项目、按 `listProjectMembers(projectId)` 的 digital_employee principal 过滤全租户员工列表；未选项目时员工下拉禁用占位"请先选择项目"。空成员项目显式空态。更新测试。
- [ ] **Step 3:** 团队详情最小收编入口：候岗员工列表（`assignment=unassigned`）+ "收编进本团队"按钮 → 新 API；成功后 invalidate 团队员工与候岗查询。样式遵循 DESIGN.md 与既有 tab 形态。
- [ ] **Step 4:** `corepack pnpm --filter @superteam/web test -- --no-file-parallelism`（堆崩用分块串行）；`corepack pnpm verify:web`。

### Task 6: 合并 + 真实端到端 GATE（默认完成条件）

先 `verify:control-plane` + `verify:web` 全绿 → ref 手术合并 main → 主 checkout `dev-services.sh restart control-plane web` → 基于 main 验证：

- [ ] **G1 成员门禁:** API 造一个无团队员工，`PUT /projects/{id}/members` 塞成员 → 400；同员工经换队 API 归队后重试 → 200。
- [ ] **G2 派发池:** 直插 DB 造"已是项目成员但 team_id 置空"的存量态员工 → 触发协调（真实 Temporal）→ 事件流出现 teamless skip 事件且该员工不入 executor pool。
- [ ] **G3 chat:** 浏览器（codex chrome plug）任务中心 chat：未选项目时员工下拉禁用；选项目后仅项目成员可选；API 直调非成员员工发起 chat → 400。
- [ ] **G4 补员:** 浏览器走 planning_gap 一键补员 → 新员工带项目团队 → 成员写入成功 → 重规划触发；DB 复核 team_id 非空。
- [ ] **G5 归队:** 浏览器团队详情收编候岗员工 → 员工出现在团队且候岗列表消失；API 换队 → team_id 变更 + 审计事件。
- [ ] 阻塞则标记并停在阻塞态，不得以未验证状态交付；全过后按分支收尾规范删分支/worktree，更新 CHANGELOG 与 memory。

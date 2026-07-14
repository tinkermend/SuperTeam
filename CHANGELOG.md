# Changelog

All notable changes to the SuperTeam Control Plane project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

- 2026-07-14 15:19：布局宪法 P1（分支 feat/layout-constitution，spec 2026-07-14-layout-constitution-design.md）：`theme.css` 新增 `--v3-layout-*`/`--v3-metric-*` 宽度 token（三档：contained 1280 / wide 1680 / canvas 不限）；`Main` 加 `width` 档位属性（默认行为不变，存量 32 处调用零影响）；superteam 新增布局基元 `MasterDetailLayout`（rail 340/420 两档、`@container/master-detail` 容器断点折叠）与 `MetricGrid`（KPI 卡 208–336px 有界，flex-wrap 先压缩后换行——grid auto-fit 按 max 计列数会提前孤行，实测弃用）；`layout-density.md` 升级为可执行规范（宽度档位/断点口径/主从布局/指标带四章），DESIGN.md 概念表登记；试点迁移项目管理页（`Main width="wide"`、主从 420 栅格与 KPI 带换基元、右栏 sticky 改容器变体）。验证：layout/main/projects 单测 54/54 绿；verify:design-system 绿；真实浏览器（admin 登录真实 Control Plane）1280/1536/2000 三视口实测——2000 下内容 1680 居中、双列 1208+420、KPI 336×4、右栏 sticky；1536 下 KPI 307×4 单行；1280 下容器 1008<@5xl 正确落单列；三档均无横向溢出。：新建团队页数字员工库分页调整为每页展示 5 位；分页回归覆盖第 6 位员工进入下一页。验证：`create-team-page.test.tsx` 8/8。
- 2026-07-14 12:24：移除新建团队页底部操作栏的半透明底色与背景模糊，操作栏现在直接融入页面极光背景，同时保留顶部边线与按钮层级。验证：`git diff --check`。
- 2026-07-14 12:21：新建团队页改为受限视口工作区：页头与底部操作栏保持可见，配置画布与确认内容在中间区域滚动；因此无论桌面屏幕高度如何，取消、上一步和下一步/确认创建都固定在内容区底部。验证：`create-team-page.test.tsx` 8/8、串行 Web 测试 696/696、Web typecheck/build；Web 服务重启后 `/teams/new` 返回 200。标准 `verify:web` 的并行测试仍在 `route.fulfill` 内存回收异常处中断；浏览器真实 UI 检查仍受本机浏览器控制运行时初始化 `process` 冲突阻断。
- 2026-07-14 12:16：新建团队「团队画布」右侧数字员工库固定为 42rem 高度，并改为每页 4 位的本地分页；搜索时自动回到第一页，已选员工仍会从候选项中移除，避免员工多时将下一步与取消操作推到首屏外。验证：`create-team-page.test.tsx` 7/7、串行 Web 测试 695/695、Web typecheck/build；浏览器真实 UI 检查仍受本机浏览器控制运行时初始化 `process` 冲突阻断。
- 2026-07-14 12:10：新建团队页改为「团队画布」工作面：左侧为团队名片与人类负责人，中部用负责人→数字员工的关系画布呈现已选结构，右侧以实底员工库搜索并加入未归属员工；保留既有两步确认、多人负责人、创建接口与元数据契约。新增回归覆盖负责人和数字员工分别进入画布及 POST 负载。验证：`create-team-page.test.tsx` 6/6、Web typecheck；Web 服务重启后 `/teams/new` 返回 200 并确认 Vite 正在加载新组件。浏览器真实 UI 检查待本机浏览器控制运行时恢复（初始化受 `process` 冲突阻断）。

- 2026-07-13 17:50：新建团队 UI 打磨——数字员工列表改用 `EmployeeAvatar`；向导收敛为两步（配置团队=身份+负责人/数字员工同屏 → 确认并创建），去掉空壳权限步与单独身份页的稀疏布局。验证：`create-team-page.test.tsx`；浏览器 `/teams/new`。

- 2026-07-13 12:33：场景模板注册表 P1（分支 feat/scenario-template-p1，spec 2026-07-13-scenario-template-registry-design.md）：迁移 058 建 `scenario_templates`（5 种子含 generic 兜底）+ `projects.scenario_template_key`；只读 API GET /scenario-templates(+/{key})（authz scenario_template.read）；项目创建绑定 key（注册表校验 active）；规划快照装载模板内容注入 planner prompt（骨架实例化指令），`ValidateRouteDecisionPlan` 强制 plan.template_key == 绑定 key（template_key 首次有消费方）；template_key 经 PlanRevisionPayload 落库并在计划确认卡显示；Web 新增「场景模板」目录页（流程能力组）与项目创建模板下拉。顺手修复：web typecheck 两处预存在破损（workflows fixture 缺 execution_summaries、constitution tab unknown 取长）；五处过期测试断言对齐现实（5a8804fa 侧栏软化样式值、未提交的嵌套日志菜单期望、runtime/trace-panel 卡片数）。验证：verify:contracts/control-plane/runtime-agent 绿；web 679/679 绿（--no-file-parallelism；并行模式存在预existing 的 vitest browser route.fulfill 堆回收崩溃，待另行排查）；真实 E2E（2026-07-13 14:20 回填）：注册表 API/页面 5 种子可见可展开；绑 ops_analysis 项目的规划骨架逐字实例化（produces/required_inputs/depends_on）并全链执行（collect 交付真实 df -h 输出、analyze 经 upstream_results 消费、handoff.verified×2）、payload template_key 落库且计划卡显示；未绑项目回归正常；创建表单下拉列全模板。首轮规划暴露两问题已修（骨架任务必填字段 prompt 加固；置信度阈值用项目级 policy 旋钮）。附带发现调度韧性缺口：CP 重启瞬断命令通道时 DispatchProjectTask 超时放弃、queued 孤儿 attempt 无恢复（项目 12579692，待立项）。

- 2026-07-13 10:59：交接契约执行闭环 P1（分支 feat/handoff-loop-p1，spec 2026-07-13-handoff-contract-execution-loop-design.md）：① 派发注入——两条派发路径把**直接 blocker** 的最新 result（summary 4KB 截断 + deliverables + evidence/artifact refs）作为 upstream_results 注入员工 prompt 与 execution_context_packet，prompt 同时携带本任务 produces 与 deliverables 义务；② 图校验收紧——required_inputs 必须由直接 blocker 生产（原为任意祖先，一条边=一份交接），fan-in 靠显式建边；③ result_contract 新增 deliverables（Go+Rust 双侧，Rust 强类型 struct 补字段防静默丢）；④ 生产者侧履约核对——completed 结果缺 produces 声明项即校验失败走既有 rejected+waitHuman，通过则平台追加 handoff_fulfillment verification 并写 handoff.verified/unfulfilled ledger 事件。顺手修复 main 上 api 包测试红（5488e39c 验证码默认关闭后 routeLogin 仍强制解码验证码图）。验证：verify:control-plane + verify:runtime-agent 全绿；真实 E2E（项目 46e7206f「Handoff-E2E 20260713」，真实 claude-code×2 员工 + local-dev-node）三场景全过——①链式：任务 A 交付 secret_code=1783911737，任务 B 派工单 packet 含 upstream_results，B 复用该值译出中文大写且未重跑命令；②缺项拒绝：员工按测试指令交付错名 deliverable → validation_errors=[handoff_deliverable_missing:handoff_test_result]、任务停 waiting_human 未判 completed、handoff.unfulfilled 事件+澄清决策生成；③fan-in：汇总任务 packet 含 2 条 upstream_results（kernel_name+work_dir）并合并成结论；handoff.verified 事件 6 条，contract verification 首次有平台核对数据。附带观察（预存在，非本次回归）：并行任务撞"单员工单活跃 run"限制时派发失败进 recovery，人批准恢复后重试仍撞同一冲突，任务滞留 waiting_human（4dee7323，待另行立项）。

- 2026-07-13 01:46：Provider transcript 捕获 spec 收尾（分支 `feat/transcript-capture-completion`）：① env 值脱敏接线——`redact_with_environment` 原为死代码，ledger excerpt 现携带 provider 进程环境快照脱敏；② 本地 `raw.jsonl`/`events.jsonl` 创建 mode 0600；③ raw 分段新增 30s 时间轮转（原只按 8MB，安静 run 崩溃即丢全部已缓冲 raw）；④ 修复 main 上 `provider_exit_test.rs` 编译失败（8cd076c4 漏改 raw_sink 参数）。验证：`corepack pnpm verify:runtime-agent` 全绿；**Phase 2 真实 E2E 首次通过**（项目 `febd08af`「Transcript-E2E 20260713」真实 claude-code + 真实 TOS）：ledger tool_started/tool_completed 齐备、真实失败命令 `is_error=true`（attempt `25d17c60`）、TOS `raw.part-0001/0002 + manifest` 逐段与总量 sha256 三方一致（分段实算=manifest=attempt 行回写 `61d787b1…`）、raw 91 行全可解析含 3 个 tool_result、`sk-test…` 假 token ledger/Web 已脱敏而 raw 原样 9 处、Web「执行证据链」渲染工具调用/结果节点且失败标红。TOS Object Lock/版本控制核实为**支持但 bucket 未开启**。E2E 发现两缺陷（spec §8 已记录，待立项）：派发 `project conflict` 后 attempt 缺 run 绑定致 provider 事件静默不入 ledger（attempt `53809434`）；模型 prose（text_delta/summary）不脱敏。

- 2026-07-11 16:09：项目管理新增项目软删除（`GET .../delete-preview` + `DELETE .../projects/{id}`、活跃执行硬阻断、待审批警告后清理、先终止 Temporal coordinator 再同事务级联软删与审计、详情头归档+删除）。验证：迁移 057 已 apply；真实 API smoke（项目 0febfc92-1c6b-4588-96f9-299a69151017）——allowed_actions 含 project.archive/project.delete、preview can_delete=true、DELETE 204、GET 404、列表不可见；Temporal `WORKFLOW_EXECUTION_STATUS_TERMINATED`；`audit_events.action=project.delete`。证据 `.scratch/smoke/project-delete-summary.txt` / `.scratch/smoke/project-delete-preview.json`；浏览器详情头可见「归档项目」「删除项目」，删除确认框含名称确认与 preview 提示；并修复 overview 覆盖导致 allowed_actions 丢失。

- 2026-07-11 16:01：Console 状态文案通用化：扩展 `apps/web/src/lib/status-labels.ts` 共享码表与域包装（`teamStatusLabel`/`governanceStatusLabel`/`employeeStatusLabel`/`projectStatusLabel`）；团队管理去掉裸英文 status 与本地中文 map；项目生命周期三处本地表收敛到共享导出。验证：定向 Vitest status-labels+teams 30/30、touched project components 4/4；worktree Web(:3000)+CP(:8081, captchaEnabled=false) 浏览器确认团队「活跃/就绪/已禁用」、项目「运行中/验收中」中文展示。

- 2026-07-11 15:56：登录图形验证码默认改为关闭：`auth.captchaEnabled` 与 `auth.Service` 默认 `false`，仅当配置或 `AUTH_CAPTCHA_ENABLED=true` 时开启；避免各分支未携带本地 `config.yaml` 时端到端登录被验证码阻断。验证：`go test ./apps/control-plane/internal/auth ./apps/control-plane/internal/config`；重启 Control Plane(:8081) 后 `GET /api/auth/captcha` 返回 `{"enabled":false}`，`POST /api/auth/login` 不带验证码以 `admin/admin` 返回 200 且 `/api/auth/me` 有效。

- 2026-07-11 15:10：工作对象列表时间字段与排序：`DESIGN.md` 新增「时间可见 / 默认新近优先」规则；OpenAPI 与 Control Plane 暴露 `ProjectTask.created_at/updated_at`；项目任务/审批与流程河道组内按时间倒序并展示相对时间。验证：`verify:contracts`、`go test ./internal/project`、定向 Vitest 29 通过；重启 CP(:8081)+Web(:3000) 后 cookie 登录 API 确认 tasks 含时间且 `updated_at` 倒序、decisions 含 `created_at/resolved_at`；浏览器确认任务 Tab「更新」列、审批「决议 …前」、流程编排「创建 …前」与组内新近优先。

- 2026-07-11 12:32：Plan 6 会话血缘续接修复：FindProviderSessionForTaskRoot 放宽为可续接 recoverable 的 active/idle/completed 会话（上游完成后补做/返工派发仍能命中同一 provider_session_id）。验证：定向 go test TestPgRunRepositoryFindProviderSessionForTaskRoot 通过；真实 E2E 证据 .scratch/smoke/plan6-t5-resume-fix-20260711-121427/evidence.json（supplement 派发 metadata.provider_session_id == owner 已完成会话）。

- 2026-07-11 12:02：Plan 8 删除派发闸门中结构上不可能失败的 `tool.binding` / `tool.authorization` / `tool.available` 检查及整条死代码链（`PreDispatchToolSnapshot`、adapter `requiredTools`/`effectiveMCPServerNames`/`gateCapabilityReader`、workflow `mergePreDispatchToolSnapshot` 与 metadata 读取）；`capability.Service.ListEffectiveMCPServers` 与 `/effective-mcp-servers` 保留。验证：`corepack pnpm verify:control-plane` 通过；本 worktree 启停 Control Plane(:8081) 后对真实任务 `da4afe31-…` 跑 `RunPreDispatchGate` 落库，最新 `project_task_dispatch_gate_results.checks` 不含任何 `tool.*` key（证据 `.scratch/smoke/plan8-gate/evidence.json`）。

- 2026-07-11 11:40：Plan 6 会话血缘范围收紧分层门禁 + 真实 E2E（Task 5）。迁移 056 为 `provider_sessions` 加 `project_task_root_id`（血缘根，nullable 向后兼容）+ `FindProviderSessionForTaskRoot(tenant, employee, root)` 查询（仅命中 `status='active'`）；派发路径 `StartProjectTaskRun` 解析血缘根（`revision_root_task_id` 一跳，否则任务自身）并在可复用时把命中的 `provider_session_id` 注入 runtime payload metadata；runtime `non_empty_session_id` 改为优先读 `metadata.provider_session_id`（此前只读 `session_policy`，Task 3 的注入是死码）；写回路径 `UpsertProviderSessionByExternalID` 补齐 `project_task_root_id` 落库（`EXCLUDED` 非空则覆盖，回填历史 null）。验证：`corepack pnpm verify:control-plane` 通过；`corepack pnpm verify:runtime-agent` 中 `cargo test`（含预置 `tests/provider_exit_test.rs`）在本分支与 `main`/`main-integration` 上均因该测试文件未随 `run()` trait 新签名更新而编译失败（`git diff main...HEAD` 对该文件与 `providers/mod.rs` 均为空，确认为既有债务、非本任务引入）；范围内 `cargo test --lib` 79/79 通过。真实 E2E（本 worktree 独立重启 Control Plane :8081 + Runtime Agent，均加载迁移 056 与本分支代码）：创建项目+需求，LLM 规划出 owner→downstream（produces/required_inputs=load_test_report_t5），owner 由真实 Runtime/Claude Code dispatch 后组织完成；downstream 真实 claim 后通过 `POST .../project-task-attempts/{id}/result` 注入 blocked{missing_inputs}，触发 `CreateUpstreamSupplementTasks` 派生补做任务（真实 dispatch 并完成）。DB 确认 `project_tasks`：补做任务 `revision_of_task_id`=owner 任务、`plan_iteration=1`、`assigned_digital_employee_id`=owner 员工；`provider_sessions.project_task_root_id` 在补做任务的新会话行上正确等于 owner 任务根 id（Task 1/2 血缘归属基建验证通过）。**但会话级复用未在本次真实链路触发**：owner 任务与补做任务各自产生了不同的 `provider_session_id`（未共享同一会话）——根因是 owner 的原始会话在补做任务派发前已经因 `Complete()` 写回把 `status` 更新为 `completed`（`ON CONFLICT` 按 `last_sequence_number` 更新 status，completed 序号更高会覆盖 active），而 `FindProviderSessionForTaskRoot` 只匹配 `status='active'` 行，导致标准"上游先完成、下游后发现缺失"路径下复用查找必然落空；`recoverable` 列因 `ON CONFLICT` SET 列表未覆盖该字段停留在初始 `true`，不影响上述判断。此发现已如实记录，留待人类判断是否需要放宽复用窗口（如接受 completed-but-recoverable 会话）或调整设计预期。证据 `.scratch/smoke/plan6-t5-20260711-112321/evidence.json`。

- 2026-07-11：删除返工/延展的「失败指纹熔断」（`revisionFailureFingerprint` + `repeatedRevisionFailure`），终止性统一由与内容无关的计数上限承担（返工 `revisionMaxAttempts`、延展 `max_plan_iterations`）。理由：指纹能被任意内容扰动击穿，天然不能承担 liveness 保证——只在封闭词表字段（`missing_inputs`）上可靠，而那里计数上限本就必然触发；在模型自由列表字段（`requested_changes`）上换措辞即失效。指纹是伪保证，保留只会让人误以为它是安全阀。`repeatedRevisionFailure` 与 `revisionBudgetExhausted` 返回值完全相同（都是 `Exhausted: true` → 同一条人类评审路径），故删除只改变升级时机（一律走到计数上限），不改变升级类型或去向。改动：`project_store.go` 删两函数 + `revisionPlannerMetadata` 不再写 `revision_failure_fingerprint`（`priorRevisionTasks` 保留，`revisionBudgetExhausted` 仍用）；删 5 个相关测试；spec §4.8 加勘误（2026-07-11）并同步 §4.6/验证清单引用。验证：`go build ./internal/...` + `go test ./internal/project/ ./internal/workflow/projectcoordination/` 通过。删除的仅是既有升级路径的早退分支，存活路径（计数上限 → 人类）在 Plan 5 Task B4 已真实 E2E 验证且行为不变，未引入新运行路径。

- 2026-07-11 02:12：Plan 5 图扩展（A1–B3 上游补做/plan_iteration 熔断）分层门禁 + 真实 E2E 补测（Task B4），并修复三处只有真实链路才会暴露的缺陷：(1) `routeNonCompletedProjectTaskAttemptResult` 缺少 `blocked_resolvable_upstream` 分支，真实 HTTP 结果提交永远走不到 `CreateUpstreamSupplementTasks`（单测用内存仓库绕过了信号路由，未覆盖）——补上无需人类等待、直接信号协调工作流的分支；(2) `project_task_results.chk_project_task_results_decision` check 约束建表时（034）早于 `blocked_resolvable_upstream` 决策值引入，导致该决策的每次真实落库都违反约束——新增迁移 055 补齐约束值；(3) `CreateUpstreamSupplementTasks` 把补做任务的 `planned_task_key` 直接复制 owner 任务的 key，与同一 `coordination_job_id` 下的 `uq_project_tasks_coordination_planned_key` 唯一索引冲突——改为按补做轮次派生唯一 key（`upstreamSupplementTaskKey`，仿照既有 `revisionTaskKey` 模式）。验证：`corepack pnpm verify:control-plane` 通过（含新增单测 `TestSubmitProjectTaskAttemptResultBlockedResolvableUpstreamSignalsCoordinatorWithoutHumanWait`）；`atlas migrate apply` 在联调库应用迁移 054/055，`information_schema.columns`/`pg_constraint` 确认 `project_tasks.plan_iteration` 列与新 `chk_project_task_results_decision` 约束值均生效；`make -C apps/control-plane migrate-validate` 因本地沙箱 podman machine 无法建立 SSH 连接未能跑通（尝试重启 podman machine 两次均在 socket 连接阶段失败），以上真实 `atlas migrate apply` + DB 内省作为替代证据。真实 E2E（本 worktree 独立启停 Control Plane :8081 + Runtime Agent，均从本 worktree 加载迁移 054/055 与代码）：通过真实 API 创建两位数字员工、项目（`coordination_policy.max_plan_iterations=2`）与需求，LLM 规划出 a→b 依赖（a.produces=[load_test_report]，b.input_requirements.required_inputs=[load_test_report]）；a 由 Runtime 真实 dispatch/claim 后通过 `POST .../project-task-attempts/{id}/result` 提交 completed，b 同样真实 dispatch 后提交 blocked{missing_inputs:[load_test_report]}；DB 确认新增补做任务 `assigned_digital_employee_id`=a 的员工（非 b 的）、`revision_of_task_id`=a、`plan_iteration=1`。熔断轮次单独起一组 c→d 需求复现：对同一下游任务 d 连续 3 次提交同一 `missing_inputs` 的 blocked 结果，第 1/2 次分别产出 `plan_iteration=1/2` 的补做任务，第 3 次 `CreateUpstreamSupplementTasks` 返回 Exhausted 并触发 `RequestProjectTaskIterationExhaustedReview`，产生 `decision_type=project_task_iteration_exhausted` 的人类决策请求（id 见证据文件）。清理：smoke 项目已归档，遗留的真实 Claude Code 补做子进程已确认父进程为本 worktree runtime-agent 后终止。证据 `.scratch/smoke/plan5-b4-20260711-021230/evidence.json`。

- 2026-07-11 00:54：计划级验收判据：PlanRevisionPayload/RouteDecisionPlan 新增 plan_acceptance_criteria（id/statement/satisfied_by→task_key）；落库校验拒绝空/未知 satisfier（acceptance_criterion_has_no_satisfier / satisfied_by_task_not_found）；planner 提示词产出判据；评审上下文透传；计划确认面板展示「调度顺序」与「验收判据」。验证：corepack pnpm verify:control-plane；focused web test project-operational-detail.test.tsx；真实链路证据 .scratch/smoke/plan4-acceptance-criteria-retry-20260711-005223-10714/evidence.json（判据 satisfied_by ⊆ 真实 task keys）。浏览器 UI 已在本 worktree Web+CP（captchaEnabled=false + VITE_CONTROL_PLANE_URL=:8081）验证「调度顺序」「验收判据」可见；证据 .scratch/smoke/plan4-ui-browser-20260711-010058/meta.json。

- 2026-07-11 00:12：计划内部引用完整性：任务新增 produces 与 input_requirements.required_inputs；落库前校验祖先可达与 produces 键唯一；自由字段迁入 planner_metadata.planner_notes。验证：corepack pnpm verify:control-plane；拒绝路径 go test -run TestValidateRouteDecisionPlanRejects；真实 E2E 证据 .scratch/smoke/plan3-referential-integrity-20260710T161026Z/evidence.json （上游 produces=[cpu_memory_metrics]，下游 required_inputs 引用之，input_requirements 仅含 required_inputs）。

- 2026-07-10 23:09：计划期能力控制流退役并引入 selection_confidence：scoreCapabilities 不再因虚构能力名 HardFail/归零；ApplyPlanningProfileScores 不再因 MissingCapabilities 强制审批；ValidateRouteDecisionPlan 删除空 required_capabilities 与能力拒计划规则；planner 输出 selection_confidence，低于 coordination_policy 阈值返回 no_suitable_employee；另退役计划期 tool_requirements HardFail（MCP 由 Runtime 物化为项目 mcp.json，不经计划词表选人）。验证：corepack pnpm verify:control-plane；真实 E2E 证据 .scratch/smoke/plan2-retire-capability-toolfix-20260710T150607Z-2072afb0/evidence.json （missing_capabilities 非空、selection_score=80、low risk 且 requires_human_approval=false）。

- 2026-07-10 17:48：拆除派发假闸门与散文熔断指纹：EvaluatePreDispatchGate 不再产出 capability.match / capability.hard_missing；能力快照构造器停止从 planner 输出取能力；PreDispatchCapabilitySnapshot 收缩为 Required/Matched；revisionFailureFingerprint 只取 status + 排序后的 RequestedChanges；planner 提示词不再把 missing_capabilities 绑到人类审批。验证：corepack pnpm verify:control-plane 通过；真实链路用本 worktree Control Plane(:8081) + 既有 Runtime/Web，创建空 external_capabilities 员工并批准计划后，任务 d97bd715-91f1-4732-a576-636ced5a7cf4 闸门 passed 且 checks 不含 capability.match，missing_capabilities 非空，任务越过能力闸门完成执行后进入 waiting_human(acceptance_required)；证据 .scratch/smoke/task6-remove-fictional-gate-20260710T094031Z/evidence-final.json。

- 2026-07-10 02:08 完成数字员工配置破坏性重构：删除旧个人治理配置字段，改为人格记忆、能力绑定和预算策略，并通过真实创建与 Runtime 投影链路验证。

- 2026-07-09 04:11：新增数字员工可审计删除：Control Plane 提供 `DELETE /api/v1/digital-employees/{employeeId}`，删除前阻断 `queued/dispatching/running/cancelling` 运行与 `queued/running/in_progress` 项目任务；成功路径软删除员工、当前执行实例、环境变量、MCP/技能/配置/工作目录投影并清理项目员工节点亲和，保留历史运行/项目任务/工件/审计，同时写入 `digital_employee.delete` 审计和清理候选。Web 员工详情页按 `allowed_actions` 展示危险删除按钮，要求输入员工名称确认，409 时展示阻断项。验证：`go test ./apps/control-plane/internal/authz -run 'TestDBAuthorizerEmployee(ActionsUseBusinessActionSurface|OwnerCanUsePersonalEmployeeActions)' -count=1`、`go test ./apps/control-plane/internal/employee -run 'TestServiceDeleteDigitalEmployee' -count=1`、`go test ./apps/control-plane/internal/api -run 'Test(DeleteDigitalEmployeeRoute|GetDigitalEmployeeIncludesAllowedDeleteAction|EmployeeRoutesUseAuthzActions)' -count=1`、`go test ./apps/control-plane/internal/employee ./apps/control-plane/internal/api ./apps/control-plane/internal/authz -count=1`、`corepack pnpm generate:control-plane`、`corepack pnpm verify:contracts`、`corepack pnpm --filter ./apps/web run test -- src/lib/api/client.test.ts src/lib/api/employees.test.ts src/features/employees/components/employee-detail-header.test.tsx src/features/employees/detail.test.tsx`、`make -C apps/control-plane generate-sqlc`、`git diff --check` 通过；真实链路重启 Control Plane(:8081)+Web(:3000) 后，`admin/admin` 登录真实接口，插入临时数字员工后 `GET` 返回 200、`DELETE` 返回 204、删除后 `GET` 返回 404，DB 确认 `status=disabled`、`deleted_at` 非空且 `audit_events.action=digital_employee.delete` 含 `cleanup_candidates`，Chrome 打开临时员工详情页确认真实页面显示“删除员工”按钮和二次确认弹窗且确认按钮默认禁用。已知：`corepack pnpm --filter ./apps/web run typecheck` 当前被既有无关错误阻断（`team-constitution-tab.tsx` 的 unknown hard_rules、`workflows/index.test.tsx` fixture 缺 `execution_summaries`），本次未将 typecheck 作为通过项。

- 2026-07-08 02:13：重构数字员工创建页：删除独立“配置预检”步骤，将创建流程收敛为“创建方式 / 完成配置 / 确认创建”；空白自定义不再展示员工类型和描述输入，前端内部提交 `custom_agent`；团队治理缺失或团队未允许 `custom_agent` 时显示业务阻断，不暴露 422；风险等级显示为 `低 / 中 / 高 / 严重`，Provider 类型改为必选项并使用 Codex/OpenCode/Claude Code 标签；创建请求移除 Runtime 节点必填语义，团队继承能力只读展示且不会作为员工扩展能力重复提交。验证：`go test ./apps/control-plane/internal/employee ./apps/control-plane/internal/api`、`corepack pnpm verify:contracts`、`corepack pnpm --filter ./apps/web run test -- src/lib/api/employees.test.ts src/features/employees/create.test.tsx`、`corepack pnpm --filter ./apps/web run typecheck`、`git diff --check` 通过；真实链路使用当前 worktree Control Plane(:18082)+Web(:3002) 连接真实开发库并允许 3002 CORS，`admin/admin` 登录成功，`GET /api/v1/digital-employees/create-options?team_id=00000000-0000-0000-0000-000000000101` 返回 200 且 Provider 候选可用，浏览器打开 `/employees/new` 确认无独立“配置预检”节点、风险中文展示、身份页不展示员工类型/描述、空白自定义选择未治理团队显示“团队治理未启用/先不归属团队创建”业务阻断且无 422。

- 2026-07-07 16:39：内置数字员工头像库新增 12 张 AI 生成头像（8 男 4 女）：补齐 `engineer-m-11` 至 `engineer-m-18`、`engineer-f-11` 至 `engineer-f-14` 的 512/256 WebP 资产、设计源图归档、Control Plane 内置头像定义和 Web 本地头像库。验证：`go test ./apps/control-plane/internal/avatar ./apps/control-plane/internal/employee ./apps/control-plane/internal/api`、`corepack pnpm --filter ./apps/web run test -- src/lib/api/employees.test.ts`、`corepack pnpm --filter ./apps/web run typecheck` 通过；重启 Control Plane(:8081) 与 Web(:3000) 后，`admin/admin` 真实登录，`GET /api/v1/digital-employee-avatar-assets` 返回 200 且 32 个头像，新增 8 个 male 与 4 个 female ID 均存在，Web 静态路径 `/images/digital-employee-avatars/engineer-m-18-256.webp` 返回 `200 image/webp` 且为 256x256。已知：`corepack pnpm --filter ./apps/web run test` 全量 Web 测试在 Vitest/Playwright harness 层抛 `route.fulfill: The object has been collected to prevent unbounded heap growth`，未进入头像相关断言。

- 2026-07-07 04:57：补齐数字员工创建到项目调度就绪的闭环：Control Plane 新增员工调度就绪 read model 与 `GET /api/v1/digital-employees/{employeeId}/scheduling-readiness`，聚合员工状态、生效配置、技能/MCP/环境变量事实，并保持项目 Runtime readiness 作为真实执行来源；OpenAPI、生成代码与 Web API client 同步；创建页将运行配置文案收敛为 Provider 偏好，明确 Runtime 节点在项目运行准备中决定；员工详情页新增“项目调度就绪度”面板、展示检查项/能力计数/缺失项和项目入口，并降低直接“开始任务”入口视觉优先级。验证：`go test ./apps/control-plane/internal/employee ./apps/control-plane/internal/api`、`corepack pnpm verify:contracts`、`corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx src/features/employees/detail.test.tsx src/features/employees/components/employee-detail-header.test.tsx`、`corepack pnpm --filter ./apps/web run typecheck`、`git diff --check` 通过；真实链路重启 Control Plane(:8081) 与 Web(:3000) 后，`admin/admin` 登录真实接口，`GET /api/v1/digital-employees/6ac6e7b8-3111-400e-926e-5cda9ed9cd14/scheduling-readiness` 返回 200 且 `ready_for_project_scheduling=true`、`project_execution_source=project_runtime_readiness`，Chrome 打开 `/employees/6ac6e7b8-3111-400e-926e-5cda9ed9cd14` 确认“项目调度就绪度”面板、项目运行准备文案、`进入项目` 链接和 outline “开始任务”按钮均为真实页面渲染，浏览器 error 日志为空。

- 2026-07-06 16:41：落地 UX 架构重设计三批 Web 改造：项目详情升级为 `概览 / 任务 / 工件 / 审批 / 预算 / 验收 / 配置` 工作中枢并补齐需求、员工、成员、运行总览链接；运行总览支持 `employee/project` query 与员工/Runtime 跳转，Runtime 页支持 `node` query；收件箱和审批中心统一项目决策深链为 `/projects/{projectId}?tab=approval&focus={decisionId}`，审批弹窗展示风险、来源项目/任务/审批请求和上下文摘要；`/approvals` 从占位页改为真实 inbox 驱动审批中心；侧栏重组为工作台、协作对象、流程能力、治理平台四组并保持 `/tasks` 不暴露。验证：`corepack pnpm --filter ./apps/web run test -- src/features/inbox/index.test.tsx src/features/approvals/index.test.tsx src/features/projects/components/project-operational-detail.test.tsx`（23/23）通过；真实链路使用当前 worktree Web(:13001)+临时 Control Plane(:18082，连接真实开发库且允许 13001 CORS)，登录 `admin/admin` 打开 `/approvals` 确认真实 inbox 审批数据而非占位页，点击项目审批链接进入项目详情审批 Tab 并高亮目标 decision，`/run-overview?employee=f4bd1d90-bb7a-4d16-9bd1-78c76274ea08` 默认选中员工且显示 `/employees/...` 与 `/runtime?node=local-dev-node` 链接，`/runtime?node=local-dev-node` 展示该节点真实事件；对低风险 smoke 审批 `Create smoke-note-success.txt` 提交同意后真实 API 返回 `resolved`，审批中心开放数从 13 变 12 且列表移除该项。

- 2026-07-06 14:58：数字员工创建页开放「空白自定义」路径：创建入口支持选择现有员工类型生成空白草稿，模板模式继续使用模板默认能力/治理，空白模式不注入推荐技能、MCP、外部能力、上下文覆盖或审批覆盖，并在提交时写入 `metadata.creation_mode=blank_custom`；预检对空白模式放行真实 `capability_policy`/兼容 `capability_boundary` 的零能力边界阻断，但保留模板模式和其他 blocked checks 的阻断语义。验证：`corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx`（38/38）、`git diff --check` 通过；真实链路使用 feature worktree Web(:3000)+Control Plane(:8081，`--config apps/control-plane/config/config.yaml` 且 captcha disabled) 和真实 admin 会话打开 `/employees/new`，确认左侧「空白自定义」可用、选择「数据库管理」后真实 `GET /api/v1/digital-employees/create-options` 仍返回 `capability_policy` blocked/`技能 0 · MCP 0 · 外部能力 0`，但空白模式预检按钮可继续并进入「员工画像蓝图」；3002 无监听。

- 2026-07-06 10:03：收件箱首页从「表格+右侧详情」两栏重构为「左列表+中详情+右操作面板」三栏决策控制台，对齐 `docs/prototypes/inbox-decision-console.html` 原型。InboxShell 拆为三栏：左栏 WorkSurface 紧凑列表（风险 accent bar + IconTile + 标题 + 风险 pill + 两行摘要 + 来源·节点 + 相对时间）、中栏 InboxDetailPanel（KI 编号 + 标题 pills + 为什么需要处理 dl + 过程记录时间线 + 关联引用）、右栏 InboxActionPanel sticky（已等待时长环 + 动作矩阵 + 快速跳转）；顶部指标从 3 卡扩为 4 卡，新增「等待最久」基于 `items[].created_at` 前端计算。数据真实性审计修正 5 处装饰占位：第 4 指标由「平均响应」改为「等待最久」(max(now-created_at))、KI 编号由编造业务号改为 `item_type`+`source_id` 前 8 位、过程记录由 3 步编造时间戳改为 created_at/last_activity_at 两真实点+当前状态、证据文件列表由编造文件名改为 `source_*_id` 关联引用跳转、右栏 SLA 倒计时由无来源改为基于 created_at 的已等待时长（InboxItem 无 sla 字段）。验证：`corepack pnpm --filter ./apps/web run typecheck`、`corepack pnpm --filter ./apps/web run test -- src/features/inbox/index.test.tsx`（17/17）、`git diff --check` 通过；源码核对 5 处修正全部落实（grep 确认无 DB_MIGRATION_PROD/平均响应/SLA 倒计时/artifacts[]/process_records 残留）；真实链路 curl Web(:3000)/inbox 返回 200 + 正常 SPA HTML、inbox-shell.tsx 与 inbox-item-list.tsx 经 vite 即时编译返回 200（无编译错误）、Control Plane(:8081)/api/v1/inbox/items 与 /inbox/badge 返回 401（接口真实存在，认证拦截非 mock）；浏览器视觉渲染验证因环境无 Playwright/codex chrome plug 未做，需人工浏览器复核三栏布局渲染。

- 2026-07-06 00:09：运行总览落地为真实数据驱动的三层地图：Web 通过现有数字员工 overview 与团队列表接口轮询渲染 2.5D 团队办公区、固定 10 工位、数字员工头像、楼层切换、侧栏详情和表格视图；Control Plane 在团队创建初始数字员工和数字员工创建入队时增加每团队 10 人服务层容量守卫。验证：`corepack pnpm --filter ./apps/web run test -- run-overview runtime-overview-adapter`、`corepack pnpm --filter ./apps/web run typecheck`、`go test ./apps/control-plane/internal/tenant ./apps/control-plane/internal/employee`、`git diff --check` 通过；真实 Chrome 连接当前 worktree Web(:3000)+Control Plane(:8081) 打开 `/run-overview`，确认真实接口数据渲染 6 个团队 workspace、60 个工位和员工头像，页面不是占位页。

- 2026-07-05 23:32：落地 v3.1 语义色板并按新词表重构流程编排首页。theme.css 语义色改为 OKLCH 公式派生（浅端 solid=oklch(0.60 0.15 H)，深端 L=0.75；danger 唯一例外 C=0.175），info 从撞品牌蓝的 H263 迁到青色 H215，artifact 迁 H305 仅作类别色，新增 `--v3-*-text` 文字层专用于 soft 底文字（对比度从 2.89~4.48:1 提到 ≥6.3:1），ink-2/ink-3 加深；StatusPill 文字改用 text 层 + semibold、圆点用 solid。流程编排首页删除 signature/指标卡带，最终形态为"流水线运行流"：头部可点击统计筛选条（全部/进行中/等待人工/阻断/已完成）+ 分诊分组运行列表（需要介入/进行中/已完成/其他），每行含状态 pill、mini 节点链（已完成 brand/运行 info/等待人工 warn/阻断 danger/待执行空心，>14 节点退化分段条）、非零计数与摘要，阻断行左 danger accent bar，整行可点进详情；行级 warn 染色、状态换色 IconTile、零值计数芯片、mute 档 badge 全部移除；共享 workflowStatusTone 的 planning 从 warn 降为 mute。详情页同批词表清理：编排头 IconTile 固定 brand、去重复状态 pill、零值计数 pill 不渲染、任务节点"等待人工决策"从 ok 绿改 warn、风险 pill 按级别取色、决策附件节点去绿色系、阶段标签修 artifact 紫与硬编码白底（深色模式 bug）；修复 `v3-warning` 不存在 token 的静默失效。同步 `DESIGN.md`（状态色/类别色分家、彩色文字预算）与 `docs/design-system/tokens.md`；色板对照原型存 `docs/prototypes/palette-v3-semantic-comparison.html`，设计 spec 存 `docs/superpowers/specs/2026-07-05-workflows-entrance-v31-refactor-design.md`。验证：`node docs/design-system/verify-design-system.mjs`、`corepack pnpm --filter ./apps/web run test -- v3-components`（18/18）、`-- workflows`（42/42）、`git diff --check` 通过；真实浏览器连接 Web(:3000)+Control Plane(:8081) 打开 `/projects`、`/employees`、`/workflows`（浅+深色）确认新色板与新首页渲染真实数据，筛选 chip 过滤（阻断=2 行）与整行点击 TanStack 路由跳转 `/workflows/{demandId}` 正常；流水线版复验 24 实例按需要介入 13/进行中 5/已完成 4/其他 2 分组、节点链与"待规划"态渲染正确、详情页画布节点卡语义色正确。

- 2026-07-05 22:34：项目管理页面改为更紧凑的企业级控制面风格：首页保留真实项目队列与风险识别逻辑，压缩风险汇总、筛选条和表格行距，风险行改为左侧语义条而非整行大面积染色；新建项目页改为“新建项目工作台”，步骤条和主表单收紧，创建前审阅改为字段表格。验证：`corepack pnpm --filter ./apps/web run test -- src/features/projects/index.test.tsx`、`git diff --check` 通过；真实浏览器连接 Web(:3000)+Control Plane(:8081) 打开 `/projects` 与 `/projects/new`，确认紧凑队列、审阅表、真实接口数据加载正常，新建页不渲染项目队列。
- 2026-07-05 21:27：项目管理首页队列表格补充固定列宽与长文本溢出策略：表格改为 `table-fixed` + 7 列 `colgroup`，项目名称、当前任务、当前处理者最多两行换行截断，UUID/需求 ID/能力 key/时间使用省略或稳定不换行，避免长项目名和长数字员工名称撑宽首页队列。验证：`corepack pnpm --filter ./apps/web run test -- src/features/projects/index.test.tsx`、`git diff --check` 通过；真实 Chrome 打开 `/projects` 复测 table 从 1893px 收敛到 1454px，与容器同宽，页面无横向溢出。
- 2026-07-05 16:02：项目管理首页队列的“当前处理者”改为从项目成员快照解析显示名：当前任务分配数字员工时优先展示对应数字员工名称，人类待处理任务优先展示负责人名称，缺少成员快照时才回退内部 ID；风险信号读取补充项目成员接口，首页仍不拉取详情 overview/events。验证：`corepack pnpm --filter ./apps/web run test -- src/features/projects/index.test.tsx` 通过。
- 2026-07-05 04:42：将项目 Runtime Placement 做成一等配置与诊断事实，拆分规划资格和派发就绪：项目可绑定/释放 Runtime 节点并聚合执行就绪状态，协调规划不再因 Runtime 未就绪丢失可规划员工，派发前置 gate 会持久化缺 placement、Runtime/Provider/工作区等阻塞事实；项目 task graph 与流程编排页在无任务节点时显示结构化阻塞原因和下一步动作，项目详情页新增 Runtime placement/readiness 面板。验证：`go test ./apps/control-plane/internal/project ./apps/control-plane/internal/api ./apps/control-plane/internal/workflow/projectcoordination -count=1`、`corepack pnpm --filter ./apps/web run test -- src/lib/api/projects.test.ts src/features/projects/index.test.tsx src/features/workflows/workflow-graph-adapter.test.ts src/features/workflows/index.test.tsx`、`corepack pnpm verify:contracts` 通过；`make -C apps/control-plane migrate-validate` 因当前环境缺少 `docker` 可执行文件，Atlas 无法启动 `postgres:16` dev 容器，未完成迁移可重放校验。
- 2026-07-05 15:10：项目管理首页重构为全宽运行队列：移除首页右侧选中项目上下文栏和“最近事件/最后事件”口径，队列改为展示项目、任务发起描述、当前节点/能力、当前处理者、执行状态与“最后运行时间”，行操作直接进入对应流程编排；首页不再因为默认选中项目而拉取详情 overview/events，避免把不确定事件或假阶段放到首页。验证：`corepack pnpm --filter ./apps/web run test -- src/features/projects/index.test.tsx`、`corepack pnpm --filter ./apps/web run typecheck`、`git diff --check` 通过；真实 Chrome 连接 Web(:3000)+Control Plane(:8081)，登录 `admin/admin` 打开 `/projects`，确认项目队列表头为“项目 / 当前任务 / 当前节点/能力 / 当前处理者 / 执行状态 / 最后运行时间 / 操作”，无 `project-selected-context-panel`、无“最近事件/最后事件”，行操作链接到 `/workflows/{demandId}`，浏览器 error 日志为空。

- 2026-07-03 23:09：将数字员工 Provider 提升为创建时固定身份事实，创建数字员工不再要求 Runtime 绑定，旧员工级 execution-instance 绑定写入口改为拒绝并提示 Runtime 应绑定到项目；ProjectTask dispatch 改为按项目 Runtime placement 与员工 Provider 解析执行节点，并在任务 attempt 上记录项目、数字员工、Provider 与 Runtime 执行事实。验证：`go test ./apps/control-plane/internal/employee -run 'TestBindExecutionInstance|TestCreateDigitalEmployeeDoesNotRequireRuntimeBinding|TestDigitalEmployeeRun' -count=1`、`go test ./apps/control-plane/internal/project -run 'TestQueueProjectTaskWithAttempt|TestProjectTaskAttempt|TestExecutionTrace' -count=1`、`go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectStoreDispatchProjectTask' -count=1`、`cargo test --manifest-path apps/runtime-agent/Cargo.toml project_task --test runtime_command_executor_test -- --nocapture` 通过；真实链路以隔离 Control Plane(:18081)+Web(:13000)+Runtime Agent+Codex Provider smoke 跑通项目 `b2e286be-bd9c-425d-9dee-6f1c96f19764`、任务 `3cb572eb-3898-41fa-a845-cf3404c2a121`、attempt `793bc9ed-5a58-5c2d-b4ad-7b23c25cc3d9`，Chrome 回读项目页和员工页确认员工未绑定 Runtime、Provider 为 `codex`、任务按项目 Runtime placement 执行并回写 `provider runtime smoke completed`。

- 2026-07-03 20:52：修正 Shell 页头迁移后业务主操作位置：数字员工首页“模板管理 / 创建数字员工”、技能市场“上传技能”以及同类“新建项目 / 新建团队 / 新建用户 / 注册 MCP”等入口不再渲染到全局顶栏，改回页面内容区首个无背景操作行；项目管理与数字员工模板 wrapper 的 `actions` 也统一落在 `Main` 内，避免后续调用方再次把创建类按钮顶到 shell。验证：新增员工/技能回归测试断言按钮不在 `header` 且仍在 `main`；`corepack pnpm --filter ./apps/web run test -- src/features/employees/index.test.tsx src/features/skills/index.test.tsx --maxWorkers=1`、`corepack pnpm --filter ./apps/web run test -- src/features/projects/index.test.tsx src/features/teams/index.test.tsx src/features/users/index.test.tsx src/features/mcp/index.test.tsx src/features/employees/templates.test.tsx --maxWorkers=1`、`corepack pnpm --filter ./apps/web run typecheck`、`git diff --check` 通过；真实浏览器连接运行中的 Web(:3000)+Control Plane(:8081)，在 1280px 与 390px 下打开 `/employees`、`/skills`，确认 shell 顶栏不含这些按钮、内容区按钮链接存在且无横向溢出。

- 2026-07-03 20:34：Web 认证后控制台页面统一 Shell 页头：新增 `ShellPageHeader` / `ShellPageHeaderBack` 作为业务页头入口，标题统一进入全局顶栏，标题模式搜索保持居中窄化，迁移流程编排、项目、技能、数字员工、团队、收件箱、Runtime、MCP、权限、成本、审计、日志、账户等页面，清理旧 `<Header><Search/><ThemeSwitch/></Header>` 死代码和内容区重复主标题；页面内容区默认继承整页背景，不新增 `bg-v3-bg`。验证：`corepack pnpm --filter ./apps/web run typecheck`、`corepack pnpm --filter ./apps/web run test -- --maxWorkers=1`、受影响页面定向测试、`git diff --check` 通过；真实浏览器连接运行中的 Web(:3000)+Control Plane(:8081)，在 1280px 与 390px 下打开 `/workflows`、`/projects`、`/skills`、`/employees`、`/inbox`，确认 shell 页头、main 内无重复页头、main 无新增背景层且无横向溢出。

- 2026-07-03 09:23：Project task graph 可视化补齐桌面编排视图：项目管理详情新增计划任务图画布，复用流程编排任务节点展示数字员工头像、名称、角色、任务描述和中文状态；流程编排与项目详情共享 `PLAN_TASK_GRAPH_LAYOUT` 标准布局，最多 10 个动态任务按稳定网格自动排布并保留连线，避免纵向叠放、折叠和遮挡。验证：`corepack pnpm --filter ./apps/web run test -- workflow-task-node plan-graph-canvas plan-task-graph project-execution-trace-panel src/features/workflows/index.test.tsx src/features/projects/index.test.tsx src/features/projects/config.test.tsx workflow-graph-adapter`、`corepack pnpm --filter ./apps/web run typecheck`、`git diff --check` 通过；分支内真实浏览器验证打开项目 `bf75f89e-c5fe-4c2f-b77d-71c7b8afe1b0`，确认 10 个任务节点、10 个数字员工头像、连线路径存在、桌面布局无重叠且行分布为 2/2/2/2/2。合并后仍需在 `main` 上重跑真实链路 smoke 后才能删除分支/worktree。

- 2026-07-02 20:12：Project task graph 员工身份补齐：`GET /api/v1/projects/{projectId}/task-graph` 的 assigned digital employee 现在返回专业角色 `employee_role` 与头像 `avatar_asset`，服务层通过注入的 `DigitalEmployeeIdentityLookup` 从数字员工身份读取并复用内置头像资产解析，Repository 不引入跨包依赖；OpenAPI、生成 Go 类型和 Web API 类型同步补齐字段；`verify:contracts` 的 Rust Runtime client guard 对齐当前 RuntimeCommand/ProjectTaskAttempt 写回路径，不再要求已 deprecated 的 legacy `/runtime/tasks/*` polling client。验证：`corepack pnpm test:go`、`corepack pnpm verify:contracts`、`corepack pnpm --filter ./apps/web run typecheck`、`git diff --check` 通过；真实链路以当前分支 Control Plane(:8081) 登录 `admin/admin`，对项目 `f2e0e0a0-0483-4d8f-9656-5695efe241c7`、需求 `58de5a8f-deb4-4335-8e8e-a2b9a56707f3` 调用 task-graph，确认员工 `Smoke Executor` 返回 `employee_role=general_engineer` 与 `avatar_asset.id=engineer-m-01`、`thumbnail_url=/images/digital-employee-avatars/engineer-m-01-256.webp`。

- 2026-07-02 17:57：ProjectTask dispatch/runtime recovery 首版：dispatch failure 已记录后会转成 retry scheduled 或 waiting-human recovery decision，dispatch 重试按 `dispatch_failed` 事件计数对 `max_attempts` 封顶，coordinator 在 retry 到期后用 workflow timer 自动重派（不再依赖后续 signal）；恢复对 Temporal activity 重试幂等（pending decision 不重复、未到期 retry 不重排）；queued 未 started 和 running lease lost 可通过 Control Plane recovery 终态化旧 attempt 并按 retry policy 推进（带退避，waiting-human 时创建 `project_task_recovery` 决策并投影 inbox）；`ScheduleProjectTaskRetry` 允许从 `queued` 重排以替换 stale attempt；`ListDispatchableTasks` 尊重 `retry_not_before`，避免未到期重试被立即重新分派。验证：`go test ./apps/control-plane/internal/project ./apps/control-plane/internal/workflow/projectcoordination ./apps/control-plane/internal/storage -count=1`（project 包 4 个预存在失败与本变更无关：`TestRecordPreDispatchGateResult*`、`TestListWorkflowInstancesOrdersAttentionBeforePagination`、`TestProjectTaskGraphReadReturnsGraphScopedSidecarsAfterUnrelatedRows` 在未改动的 main 上同样失败，属环境时区/共享 dev 库问题）、`git diff --check` 通过；`node scripts/verify-foundation-contracts.mjs` 在 main 与本分支同样失败（Rust Control Plane client 缺 runtime task 路径，预存在，与本变更无关）。真实链路 smoke：分支代码以临时 Control Plane(:18081，隔离 Temporal task queue `superteam-project-coordination-smoke-dr`)接共享 dev DB/Temporal，创建项目 `1175f901-a93b-4630-a92e-839c90b254d4` 并提交需求 `84899199-b303-4519-b7ff-e7e4c673abba`，批准 plan review 后 coordinator 真实执行 `DispatchProjectTask`（terminal 失败）→ `RecoverTaskDispatchFailure`；落库验证 `project_task.dispatch_failed` + `project_task.recovery_requested`(failure_family=invalid_contract) 事件，任务 `cc7fcdee-355e-486d-a2a3-cd50f46ce0be` 为 `waiting_human`/`plan_invalid` 且 `waiting_request_id` 链接 pending 的 `project_task_recovery` 决策 `426e0e5b-6bc8-4444-9436-4c511a4b2037`。retryable→retry_scheduled+timer 分支由 Temporal 测试环境 `TestProjectCoordinatorRedispatchesAfterRetryBackoff` 与 pg 集成测试覆盖。合并后仍需在 `main` 上重跑真实链路 smoke 后才能删除分支/worktree。

- 2026-07-01 23:01：修复项目管理首页首开风险数据逐项刷新导致的半成品计数和排序抖动：当前页任务、决策、证据信号未全部返回前，风险摘要与队列统一显示“风险识别中”，待当前页信号稳定后再一次性展示真实风险计数、行级标签和排序；后台 refetch 已有稳定结果时不再清空页面。验证：`corepack pnpm --filter ./apps/web run test -- src/features/projects/index.test.tsx`、`corepack pnpm --filter ./apps/web run typecheck`、`git diff --check` 通过；重启真实 Web 后用 Chrome 打开 `/projects?finalSmoke=1`，页面加载真实 Control Plane 数据，项目管理标题、风险摘要、项目队列均可见且浏览器 error/warning 日志为空。

- 2026-07-01 17:52：修复 Runtime Agent 项目任务完成写回对 Provider `result_contract.verification[].evidence` 别名不兼容的问题：写回前将非后端契约字段 `evidence` 归一到 `summary` 并移除原字段，避免 Control Plane 严格解码返回 `json: unknown field "evidence"`，导致 `task_runs` 已 completed 但 `project_tasks/project_task_attempts` 仍停在 running。验证：`cargo test --manifest-path apps/runtime-agent/Cargo.toml`、`git diff --check` 通过；真实链路在合并后的 main 临时 worktree 上用 Control Plane(:8081)+Temporal+Runtime Agent+Codex Provider 新建项目 `0a90c33e-a1b6-4094-837b-45aca03b62bc` 和需求 `7c60646a-55fa-4d35-bb10-f9de0f5a5820`，Provider 输出刻意包含 `verification[].evidence`，最终 `/api/v1/runtime/project-task-attempts/97c65b42-fd90-5716-a130-718e99beb20f/complete` 返回 202，ProjectTask `c1a7bb58-8f00-41c3-a430-7b7ea1a34a74` completed、attempt succeeded、`project_task_results` 为 `completed/accepted/complete_accepted`。

- 2026-07-01 15:43：Web 侧栏菜单补强字体渲染与深色模式对比度：全局开启字体平滑，侧栏菜单字号提升至 15px，并为深色模式下未激活菜单文字和图标增加可读性覆盖；同步补充侧栏菜单浏览器测试。验证：`corepack pnpm --filter ./apps/web run test -- src/components/layout/sidebar-menu-sizing.test.tsx src/components/layout/sidebar-collapsed-alignment.test.tsx src/styles/v3-shell-background.test.tsx`、`git diff --check` 通过。

- 2026-07-01 15:15：ProjectCoordinator 增加历史安全续跑：工作流在 Temporal 建议时 `ContinueAsNew`，并通过持久化 `project_decision_requests.plan_revision_id` 与 `project_plan_revisions.created_event_id` 在 Continue-As-New 后从数据库恢复人工计划评审路由；新路由用 `workflow.GetVersion("route-human-decision-from-store", ...)` 保护，保留 legacy pending map 回放分支，避免已运行协调线程 replay 非确定性。验证：`go test ./apps/control-plane/internal/project ./apps/control-plane/internal/workflow/projectcoordination ./apps/control-plane/internal/storage -count=1`、`atlas migrate validate --dir file://apps/control-plane/internal/storage/migrations`、`git diff --check` 通过；开发库 Atlas 状态 Current Version 044 且无 pending，确认 `project_decision_requests.plan_revision_id` 与 `project_plan_revisions.created_event_id` 已存在；真实 Temporal/Control Plane smoke 中项目协调线程记录 `route-human-decision-from-store` version marker，并调度 `LoadHumanDecisionRoute`、`ResolvePlanRevisionReview`、`DecomposeAcceptedPlanRevision`、`DispatchProjectTask`、`FinishCoordinationJob`。

- 2026-07-01 14:58：Project Workspace autonomous outer loop 首个可运行切片补齐：项目支持源码仓库绑定与 Runtime 放置事实，项目任务分派向 Runtime 传递 workspace/git/handoff/预算上下文，Runtime 将员工 home 与项目 worktree 分离并用真实 Provider CWD 执行，执行结果写回 attestation、budget heartbeat 与结构化 result contract，成功验收要求 runtime attestation 引用；同时补齐 bounded revision loop、路径脱敏、OpenAPI/sqlc/Atlas 生成物和 Runtime/Control Plane 回归测试。验证：`make -C apps/control-plane generate-sqlc`、`corepack pnpm generate:control-plane`、`corepack pnpm verify:contracts`、`go test ./apps/control-plane/internal/storage -run 'ProjectRepoBindingAndAttestation|ProjectTaskAttestationRuntimeContext' -count=1`、`go test ./apps/control-plane/internal/project ./apps/control-plane/internal/workflow/projectcoordination ./apps/control-plane/internal/employee ./apps/control-plane/internal/api ./apps/control-plane/internal/storage -count=1`、`cargo test --manifest-path apps/runtime-agent/Cargo.toml`、`cargo fmt --manifest-path apps/runtime-agent/Cargo.toml --check`、`git diff --check` 通过；分支内真实链路以临时 Control Plane(:18081)+Runtime Agent+Codex Provider 跑通 repo-bound 项目任务，task `553e3812-6215-41c6-9e77-bfc9456ea0f8` completed、attempt `bd2534c3-d882-5471-aeaa-3a17b0113f14` succeeded、result `accepted/complete_accepted`，receipt/attestation 未持久化 `agent_home_dir`、`workspace_path`、`mcp_config_path` 或 `/Users/tinker`。合并后仍需在 `main` 上重跑真实链路 smoke 后才能删除分支/worktree。

- 2026-07-01 02:01：用户管理页改为表格治理台：从用户主页移除内嵌账号操作、登录日志、授权决策、可选团队和审计日志入口，仅保留人类用户账号状态、控制台访问、成员身份、禁用/启用、重置密码与创建用户；创建用户支持不预选团队，后续可到团队管理分配。新增三版用户管理原型和 GPT Image 参考图。验证：`corepack pnpm --filter ./apps/web run test -- src/features/users/index.test.tsx`、`corepack pnpm --filter ./apps/web run typecheck`、`go test ./apps/control-plane/internal/auth -run 'CreateUser|ManagedUser|ValidateActiveTenantTeamIDs' -count=1`、`git diff --check` 通过；真实链路重启 Web 后用 Chrome 插件登录 `admin/admin` 打开 `/users`，确认用户治理表加载真实用户，`main` 与用户管理 layout 内不再出现可选团队、审计日志或日志类型入口。

- 2026-06-30 11:12：登录验证码新增开发期开关：Control Plane 配置增加 `auth.captchaEnabled`，并支持 `AUTH_CAPTCHA_ENABLED=false` 环境变量关闭；关闭时 `/api/auth/captcha` 返回 `{"enabled":false}`，登录接口不再要求 `captcha_id`/`captcha_code`，Web 登录表单隐藏验证码并直接提交账号密码。默认仍为开启状态。验证：`go test ./apps/control-plane/internal/auth ./apps/control-plane/internal/config ./apps/control-plane/internal/app`、`corepack pnpm --filter ./apps/web run test -- src/lib/api/auth.test.ts src/features/auth/auth-provider.test.tsx src/features/auth/sign-in/components/user-auth-form.test.tsx`、`corepack pnpm --filter ./apps/web typecheck`、`corepack pnpm verify:contracts`、`git diff --check` 通过；真实链路以 `AUTH_CAPTCHA_ENABLED=false` 重启 Control Plane，确认 `/api/auth/captcha` 返回 disabled、`POST /api/auth/login` 不带验证码返回 200 且设置 session cookie，并在真实 Web 登录页确认验证码控件不存在、`admin/admin` 登录进入首页；随后已按默认命令重启 Control Plane 并确认验证码恢复 enabled。

- 2026-06-30 03:55：登录页新增强制图形验证码：Control Plane 生成 4 位数字+字母图片验证码并以 PostgreSQL 一次性 challenge 校验，登录请求必须提交 `captcha_id`/`captcha_code`；Web 登录表单加载验证码、支持刷新，并在登录失败后清空验证码但保留账号密码。验证：`go test ./apps/control-plane/internal/auth ./apps/control-plane/internal/storage -run 'Test.*Captcha|TestLogin|TestAuthCaptchaChallenge' -count=1`、`go test ./apps/control-plane/...`、`corepack pnpm --filter ./apps/web run test -- src/lib/api/auth.test.ts src/features/auth/auth-provider.test.tsx src/features/auth/sign-in/components/user-auth-form.test.tsx`、`corepack pnpm --filter ./apps/web run test -- src/lib/api/auth.test.ts src/features/auth/auth-provider.test.tsx src/features/auth/sign-in/components/user-auth-form.test.tsx src/components/layout/header.test.tsx`、`corepack pnpm --filter ./apps/web run typecheck`、`corepack pnpm verify:contracts` 通过；迁移 `039_auth_captcha_challenges.sql` 已应用到开发库且状态 OK；当前分支 Control Plane(:18081)+Web(:3001) 真实链路验证 `/api/auth/captcha` 返回 PNG data URL、缺失验证码登录 400、错误验证码登录 401 并写入 `captcha_invalid` 登录日志、`admin/admin` 携带新验证码登录成功并进入认证首页。已知：`corepack pnpm --filter ./apps/web run test` 全量 Web 测试在 Vitest/Playwright harness 层抛 `route.fulfill: The object has been collected to prevent unbounded heap growth`，未落到本功能断言。

- 2026-06-30 01:06 项目管理风险首页布局改为队列主导：桌面首页右侧上下文收窄为辅助栏，风险队列表格从 6 列收敛为“项目 / 风险与落点 / 状态 / 操作”4 列，并将负责人和处置落点折入行内次级信息，减少宽屏首屏挤压。

- 2026-06-30 00:08 项目管理首页改为风险优先队列，基于当前页项目补强任务、决策、证据和协调状态风险信号，并保留统一详情跳转。

- 2026-06-30 00:21：Web 默认首页改为“任务中枢”，复用任务发起表单作为首页任务提交入口；侧栏工作区首项同步改为“任务中枢”，并移除重复的“任务发起”菜单项；默认侧栏样式切换为浮动玻璃面板，增加 26px 圆角、渐变背景和激活项左侧锚点。验证：`corepack pnpm --filter ./apps/web run test -- src/components/layout/sidebar-data.test.ts src/context/search-provider.test.tsx src/features/dashboard/index.test.tsx src/features/task-launches/index.test.tsx src/styles/v3-shell-background.test.tsx src/components/config-drawer.test.tsx src/components/layout/sidebar-menu-sizing.test.tsx`、`git diff --check` 通过；真实运行 Web 登录 `admin/admin` 打开 `/`，页面标题为“任务中枢”，侧栏有“任务中枢/收件箱”且无“任务发起”，浮动侧栏计算样式生效，并确认 `/api/auth/me`、`/api/v1/projects` 真实 Control Plane 返回 200。

- 2026-06-29 14:23 项目创建流程收敛为负责人池模型：新建项目使用主负责人 + 额外 owner 成员，多来源团队只用于筛选数字员工；任务发起不再要求选择审核人，后端默认派给主负责人。

- 2026-06-29 00:23: 项目管理新建项目改为分屏配置台，支持授权团队、人类角色、数字员工池、策略预设和创建前审阅。

- 2026-06-28 23:14 完成了「任务编排模板库」的全栈开发，包括 OpenAPI 契约、后台 SQLC 和服务、控制平面 API 路由、前端模板选择器组件和插入表单的对接集成。

### Changed

- 2026-06-28 03:26：项目管理详情默认视图收敛为项目负责人主循环：前置当前需求、当前执行、最新结果、待负责人处理、当前阻塞、项目负责人组和项目服务池；路由决策、协调任务、执行摘要、转派请求、协调线程、Dispatch gate 技术详情、治理和归档入口统一折叠到“高级项目事实”。默认事件流改为负责人可读动态，避免泄漏 `route_decision` 等内部协调对象。验证：`corepack pnpm --filter ./apps/web run test -- src/features/projects/index.test.tsx`、`corepack pnpm --filter ./apps/web run typecheck`、`git diff --check` 通过；真实 Web 连接真实 Control Plane 打开带 dispatch gate 的项目，默认视图业务标签可见、内部标签折叠隐藏，展开高级事实后内部面板可见且无横向溢出。
- 2026-06-28 02:52：数字员工新增只读模板管理入口与目录页：数字员工首页增加“模板管理”次操作，`/employees/templates` 基于真实 `create-options` 展示内置模板表格，`/employees/templates/$templateType` 展示模板能力、默认注入、治理影响和“用此模板创建数字员工”入口；创建页模板表格统一改为“选择内置模板 / 模板能力 / 默认注入 / 风险等级”，模板列不再重复英文类型，`/employees/new?template=...` 会预选对应模板并进入预检。验证：`corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx src/features/employees/index.test.tsx src/features/employees/templates.test.tsx src/navigation-rules.test.ts`、`corepack pnpm --filter ./apps/web run typecheck`、`corepack pnpm --filter ./apps/web run build`、`git diff --check` 通过；真实浏览器从 `/employees` 点击“模板管理”进入 `/employees/templates`，查看 `frontend_engineer` 模板详情，再进入 `/employees/new?template=frontend_engineer`，创建页显示“已选择前端开发模板”，预检页由真实后端返回 `Runtime 可用: 1/10 个运行绑定可用` 且无浏览器 error log。
- 2026-06-27 22:49：数字员工创建页改为四步创建流：先用可搜索/风险筛选的模板表格选择专业模板，再进入独立配置预检页，预检通过后完成身份、能力、治理和运行配置，最后在确认页核对团队、模板、名称、角色、能力数量、Runtime、预算和环境变量数量后提交创建；模板表格仅展示模板真实提供的默认角色、推荐技能/MCP、风险触发和默认注入摘要，不把 Runtime/Provider 可用性混入模板列。验证：`corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx`、`corepack pnpm --filter ./apps/web run typecheck`、`corepack pnpm --filter ./apps/web run build` 通过；真实浏览器登录 `admin/admin` 打开 `/employees/new`，模板页加载 9 个模板、表格不含 Runtime/Provider/运行可用性字段，进入预检页后真实后端返回 `Runtime 可用: 0/10 个运行绑定可用`，继续配置按钮按预期禁用，页面无横向溢出且无浏览器 error log。
- 2026-06-24 14:37：控制台与技能市场背景切换为 v3 Acrylic Shell：全局 Shell 使用柔和底色、低对比渐变与网格，左侧导航、顶部栏、顶部搜索/主题/侧栏开关和左侧激活菜单项改为 token 化毛玻璃与半透明 control 表面，并移除 inset 内容容器的顶部空白和顶部圆角，让顶栏贴边融合；同时保留卡片、表格和表单实底高对比；同步补充 shell/acrylic token 与表面设计规范。验证：`corepack pnpm --filter ./apps/web run test src/styles/v3-shell-background.test.tsx src/components/layout/authenticated-layout.test.tsx src/components/layout/header.test.tsx`、`git diff --check` 通过；真实运行 Web/Control Plane 下打开 `/skills`，页面加载技能市场且 body/shell 为 Acrylic 背景，Header/Sidebar/顶部控件/左侧激活菜单项为 blur 或半透明 acrylic 表面，Header 与内容 inset 的 `y=0`、`marginTop=0`、顶部圆角为 0，无 Vite error overlay。
- 2026-06-23 16:36：任务发起页重构为单张 v3 Soft-Flat 命令面板：保留中枢指令区、项目/审核人/优先级/风险级别和保存草稿/提交需求动作，移除底部“添加附件 / 关联链接 / 导入资料”资料入口及右侧说明栏，提交 payload 不变；同步将任务发起页、命令面板、主题/侧栏/外观设置等可见英文改为简体中文，并在 `DESIGN.md` 增加“简体中文优先”约束。验证：新增任务发起浏览器测试断言资料动作不渲染，`corepack pnpm --filter ./apps/web run test` 449 项通过、命令面板/外观设置/任务发起定向测试 30 项通过、`corepack pnpm --filter ./apps/web run typecheck`、`git diff --check` 通过；重启真实 Web 后打开 `/task-launches`，真实 Control Plane 数据加载成功且浏览器日志无错误。
- 2026-06-23 15:16：统一 Web 菜单页标题区：`V3PageHeader` 增加受控 `icon`/`iconTone`/`actions` 入口，工作台、技能市场与收件箱页头使用统一图标样式；收件箱移除页头右侧与下方指标卡重复的开放/高风险/阻断摘要。验证：Dashboard/Skills/Inbox 定向浏览器测试 20 项通过，`corepack pnpm --filter ./apps/web run typecheck`、`git diff --check` 通过；真实运行 Web 下打开 `/`、`/skills`、`/inbox` 确认页头图标和统计分布正确。
- 2026-06-23 11:33：SuperTeam Web 全站 v3 Soft-Flat 迁移收口：所有业务页面与 Shell 使用 `--v3-*` token 和 `components/superteam` v3 组件，删除旧液态/玻璃组件导出、组件测试、样式 token、全局旧 class 与过时原型目录；设计系统文档改为 v3 唯一基线。验证：旧组件符号与旧样式前缀 grep 归零，`corepack pnpm --filter ./apps/web run typecheck`、`run build`、`run test`、`git diff --check` 通过；重启真实 Control Plane/Web 后用 Playwright 登录 admin/admin 打开 22 个主菜单路由，均 HTTP 200、无旧 class/slot、无页面 JS 错误。

### Fixed

- 2026-06-28 23:56：修复任务发起「浏览模板库」弹窗布局异常：模板卡片网格原用视口断点 `sm:grid-cols-2 xl:grid-cols-3`，被 ≥1280px 视口 `xl` 断点在窄弹窗内压成过窄多列、正文过早截断，改为 `grid-cols-[repeat(auto-fill,minmax(15rem,1fr))]` 跟随容器宽度；并修复 `DialogContent` 默认 `sm:max-w-lg`（512px）覆盖了组件 `max-w-[900px]`、导致弹窗实际仅 512px 内容区挤压的问题，补加 `sm:max-w-[900px]` 覆盖默认值；同时清理未用的 `useMutation`、`replaceAll`→`split/join`（es2021 lib）与未用的 `Button`。验证：真实 Web（:3000）连接真实 Control Plane（:8081），登录 `admin/admin` 打开 `/task-launches` 点击「✨ 浏览模板库」，弹窗宽度 512→900px、模板卡片 ~321px、正文完整；`corepack pnpm --filter ./apps/web run typecheck` 通过；`git diff --check` 通过。

- 2026-06-28 22:46：Runtime Agent 运行时 session auth 增加自愈：所有 Control Plane 业务请求改为请求时读取共享 `RuntimeAuthState`，临近过期会暂停业务流量等待安全 token，daemon supervisor 会在过期前主动续期，并在 401 fallback 时使用 `bootstrap_key` 重新 enrollment；WebSocket 重连每次重新取 Authorization，并用 generation guard 忽略旧 session 的迟到 401，避免 12 小时 TTL 后长期重复 401；续期 401 后的 re-enroll 会消费对应 reauth event，避免重复 enrollment，active task 的 lease 与 event writeback 遇到 typed auth 401 时暂停等待 auth 恢复，不再因为 token 切换取消或失败任务。验证：`corepack pnpm verify:runtime-agent`、Control Plane runtime/api 定向与完整 Go 测试、`git diff --check` 通过；真实链路重启 runtime-agent 后 `local-dev-node` 建立 active session，强制 DB session 过期后运行中 agent 触发一次 typed 401 并自动 `Runtime session established`，DB 恢复 active session 且 `latest_seen_at` 更新；真实 Codex Provider smoke 输出 `session_started`、`runtime auth self-healing smoke`、`turn_completed`。
- 2026-06-23 16:04：修复技能上传页两处问题：发布链路中“校验依赖”在未声明 CLI/env 依赖时默认置灰并标记 `aria-disabled`，但沿用等待步骤文字 token、不额外降低透明度，声明依赖后才进入完成态；Control Plane 上传技能 zip 时忽略 `__MACOSX`、`.DS_Store`、`._*` 等压缩元数据再识别包根目录，避免包含 `skill/SKILL.md` 的 macOS zip 被误报“不含 SKILL.md”。验证：新增上传页依赖灰态浏览器测试与后端元数据 zip 解析测试，定向 Web/Go 测试通过；真实链路验证见本次任务记录。
- 2026-06-23 03:46：Project coordinator Phase 4 result contract/revision-loop 收口修复：`CompleteProjectTaskAttempt` 对无效 `result_contract` 不再直接 400 丢失执行结果，而是持久化 rejected `project_task_results`、链接 `latest_task_result_id`，并把 task/attempt 路由到 `waiting_human` 澄清；Runtime Agent 对 Codex `item.completed`/`text_delta` 与空 `turn.completed` summary 的真实输出形态做终态摘要兜底，并将 `acceptance_results[].evidence_refs` 规整为字符串数组，避免结构化结果被降级为 legacy fallback。验证：`corepack pnpm verify:contracts`、`go test ./apps/control-plane/...`、`cargo test --manifest-path apps/runtime-agent/Cargo.toml`、`cargo fmt --manifest-path apps/runtime-agent/Cargo.toml --check`、`git diff --check` 通过；真实链路以临时 Control Plane(:18081)+Runtime Agent(:17077)+Codex Provider 跑通，任务 `completed`、attempt `succeeded`、`project_task_results` 为 `accepted/complete_accepted`，receipt summary 保留 Codex 输出的结构化 `result_contract`。
- 2026-06-22 16:19：修复上传技能工作台提交中文技能名称时后端 slug 生成失败的问题：`UploadSkill` 现在保留中文展示名，但当展示名无法 slugify 时，会依次从 `SKILL.md` 标题和 zip 文件名去掉后缀生成包 slug，支持“技能包描述名称/包标识”和“技能中文名称”分离。验证：新增 `TestServiceUploadSkillUsesArchiveFilenameSlugWhenDisplayNameIsChinese`，`go test ./apps/control-plane/internal/skill/...` 通过；重启 Control Plane 后用示例 zip 真实 multipart 上传返回 201，并在技能管理页看到“示例浏览器上传技能”及 CLI/env 依赖声明。
- 2026-06-22 04:54：Project coordinator Phase 3 pre-dispatch gate 收尾修复：高风险项目任务在 gate `waiting_human` 后，人工 `project_task_approval` 批准会通过版本化 Temporal 分支调用 `ApplyPreDispatchGateDecision` 重新评估 gate，并以 `human_resolved` 原因继续 dispatch；非 gate decision 仍只记录 observed 事件，不触发分派。修复 decision-linked `waiting_human` gate 的幂等更新条件，使审批后同一 idempotency key 可从 `waiting_human` 更新为 `passed` 并绑定 attempt，但已绑定 attempt 的 gate 不会被覆盖；Runtime start-session payload 对缺省 `session_policy` 统一补 `mode=new`、`recoverable=true`；Runtime `/started` 写回对“Runtime command 早于 queued attempt 可见”的短暂竞态做有条件重试，真实 lease/node/task 冲突仍立即失败。验证：`make generate-sqlc`、`go test ./apps/control-plane/internal/project ./apps/control-plane/internal/workflow/projectcoordination ./apps/control-plane/internal/employee ./apps/control-plane/internal/app ./apps/control-plane/internal/api ./apps/control-plane/internal/storage -count=1`、`git diff --check` 均通过；真实链路以临时 Control Plane(:18081)+Temporal+Runtime Agent+Codex Provider 跑通，gate `waiting_human -> passed`，attempt `running -> succeeded`，task_run/receipt `completed`，receipt payload 中 `session_policy={"mode":"new","recoverable":true}`。
- 2026-06-21 02:46：执行证据补齐进入证据读模型（金路径证据缺口）：任务完成回写（`CompleteProjectTaskAttemptWriteback` / `CompleteProjectTaskAttemptAcceptanceWriteback`）原本只把 `evidence_refs` 存进 `project_execution_summaries` 行的 JSONB 列，并未抽取到独立的 `project_evidence_refs` 读模型——而 `/projects/{id}/evidence` 读取后者，导致「执行摘要带 evidence_refs 但证据列表空」。新增 `extractExecutionEvidenceRefsWithQueries`：在两条完成回写事务内、`createExecutionSummaryWithQueries` 之后，把每个 `{ref,type,provider_session_id?}` 条目映射为一条证据行（evidence_type/source_type=type、source_ref=ref、execution_summary_id、project_task_id、submitted_by=digital_employee、metadata=原条目）。best-effort：单条畸形跳过、不回滚任务完成（权威 refs 仍在摘要上）。验证：`go build ./...` 通过、`go test ./internal/project/` 通过（无回归）；真实链路待验证——当前开发库被重置后无处于 queued/running 的 task attempt，无法在本次跑通一次「带 evidence_refs 的任务完成 → 证据行落库」的真实回写，故不声明端到端已验证。
- 2026-06-21 02:21：修复批准路由决策后任务分派被永久阻塞（卡死 `dispatching` run 占用单活跃槽位）：数字员工 run 进入 `dispatching`（已向 runtime 节点下发 start-session 命令但 runtime 从未回报启动，如节点失联/回调丢失/进程中途崩溃）后会无限期停留，`GetActiveRun` 一直返回它，单活跃 run 守卫（`ErrConflict: active digital employee run exists`）随之永久挡住该员工后续所有 dispatch——表现即「人类决策队列批准失败、拒绝正常」（批准触发 dispatch 命中冲突重试 3 次全败、任务停在 planned；拒绝不走 dispatch 故无碍）。在 `DigitalEmployeeRunService.CreateRun` 冲突分支新增陈旧预确认 run 回收：当被阻塞的活跃 run 处于 `queued`/`dispatching`（命令已发未确认启动）且 `time.Since(UpdatedAt) > staleDispatchTTL(5min)` 时判定废弃，就地标记 `failed`（error_code `dispatch_stale`/error_family `dispatch_timeout`）并写 `run_reaped_stale` 生命周期事件 + `digital_employee_run_reaped_stale` 审计，释放活跃槽位后继续创建新 run；同幂等重试仍走原有重发路径，`running`/`cancelling`（runtime 已确认在跑）即使久也不回收。验证：`go test ./internal/employee/` 通过（新增陈旧回收/新鲜不回收/running 不回收 3 例 + 既有冲突用例）；Control Plane(:8081) 重启加载新代码并健康后，对真实开发库中卡死超过一天的 run `c612ec8c`（dispatching）触发 `POST /digital-employees/{id}/runs`，确证其被回收为 `failed/dispatch_stale`、新 run 创建并派发到真实节点、该员工活跃 run 集合由「卡死旧 run」变为「单一新 run」，且 `task_events` 写入 `run_reaped_stale`、`audit_events` 写入 `digital_employee_run_reaped_stale`。

### Removed

- 2026-06-21 02:31：删除「任务发起」侧孤儿详情组件：`task-launches/components/task-launch-detail.tsx`（`TaskLaunchDetail`）及 `features/task-launches` 中未被任何路由消费的 `TaskLaunchDetailPage`/`TaskLaunchDetailView`（路由 `/task-launches/$demandId` 实为重定向到 `/workflows/$demandId`，而后者 `WorkflowDetail` 已基于真实 `task-graph` 渲染完整任务节点/负责人/阻塞/输入输出/执行结果，孤儿组件既不可达又与之重复）。统一以 `/workflows/$demandId` 为唯一的计划/详情入口，保留 `/task-launches/$demandId` 重定向作为书签兜底；同步移除对应测试与未用的 `rerenderWithQueryClient` 辅助。验证：`corepack pnpm --filter ./apps/web run typecheck` 通过、`run test -- task-launches` 7/7 通过、Vite HMR 加载 `task-launches/index.tsx` 无报错；发起器（选项目→提交→导航 `/workflows/$demandId`）行为不变。
- 2026-06-21 00:52：删除已废弃的 `runtime_leases` 表：新增 migration 028 `DROP TABLE IF EXISTS runtime_leases` 并已应用到当前开发库；同步移除迁移/查询测试中对该表作为当前表的要求，规范文档明确项目任务执行租约由 `project_task_attempts` 承载，避免后续重建独立 runtime lease 表。

### Added

- 2026-06-28 09:07：新增 MCP HTTP 能力管理（注册表 + 绑定 + 环境变量预检 + Provider 配置投射）：migration 037 建立 `mcp_servers` 注册表与 `team_mcp_bindings`/`digital_employee_mcp_bindings_v2`，仅允许 streamable_http/http 传输与 none/bearer_env/headers_env 鉴权，并按 (tenant,name,url) 从旧 `team_mcp_servers`/`digital_employee_mcp_bindings` 回填（bearer_env 行同步写入 `required_env_vars` 占位保持预检一致），atlas.sum 已重算；Control Plane 新增 `/api/v1/mcp-servers`、`/teams/{teamId}/mcp-bindings`、`/digital-employees/{employeeId}/mcp-bindings-v2`、`/effective-mcp-config` 全套 CRUD 与 `mcp_registry.read/manage` 租户管理员鉴权，绑定按 `mcp_server_id` 而非原始 URL，员工绑定返回 `blocked_missing_env` 预检；start-session payload 投射环境变量满足的有效 MCP（阻塞项排除）；Runtime Agent `mcp_config.rs` 以原子写为 Codex(`config.toml` 读合并保留)、Claude Code(`.mcp.json`)、OpenCode(`opencode.json` 读合并保留) 渲染配置，只写环境变量名不写凭据值；Web 新增一级 `/mcp` 管理页与侧栏入口，团队/员工能力面板改为注册表选择绑定，员工绑定在缺必需环境变量时禁用并提示精确变量名。验证：`go test ./internal/capability ./internal/api ./internal/authz ./internal/employee ./internal/storage`、`cargo test --manifest-path apps/runtime-agent/Cargo.toml --lib`、`corepack pnpm --filter ./apps/web run test`（mcp/teams/employees/capabilities 共 36 项）、`corepack pnpm --filter ./apps/web run typecheck`、`corepack pnpm verify:contracts`、`git diff --check` 通过；真实链路在合并后的 main 重启 Control Plane，admin 会话真实创建 MCP 定义、stdio 传输被 400 拒绝、团队按 `mcp_server_id` 绑定返回 active、员工 v2 绑定返回 `blocked_missing_env missing=['GITHUB_TOKEN']`，upsert `GITHUB_TOKEN` 后 effective-mcp-config 变为 `active missing=None`，Web `/mcp` 与 web→api 代理均 200。
- 2026-06-24 05:08：技能市场新增同步技能安装闭环：安装弹窗支持选择团队或单个数字员工，Control Plane 对目标员工、Provider 与 Runtime 节点做预检并通过 `install_skills` Runtime command 同步等待短超时结果；团队安装当前要求所有目标位于同一个已连接 Runtime node，否则在预检阶段失败且不派发安装命令，单 Runtime node 内由 Runtime `rollback_on_failure` 保障；失败原因写入 `audit_events`，并在当次安装弹窗展示返回原因与 blocked targets。Runtime Agent 从技能 archive 拉取 zip、校验 checksum、按 Provider 写入 `opencode -> .opencode/skills/<skill_key>`、`codex -> .agents/skills/<skill_key>`、`claude-code -> .claude/skills/<skill_key>`，并回写安装 receipt；技能详情展示已安装团队/员工、Provider、路径与 checksum。验证：migration 035 已应用且 pending=0，`go test ./apps/control-plane/internal/storage ./apps/control-plane/internal/skill ./apps/control-plane/internal/api ./apps/control-plane/internal/app -count=1`、`cargo test --manifest-path apps/runtime-agent/Cargo.toml`、`corepack pnpm --filter ./apps/web run test`、`corepack pnpm --filter ./apps/web run typecheck`、`corepack pnpm verify:contracts`、`git diff --check` 通过；临时真实栈 Control Plane(:18081)+Web(:3001)+Runtime Agent 跑通员工安装，`POST /api/v1/skills/{id}/install` 返回 201，DB `skill_installations` 为 installed，Runtime receipt completed，员工 workspace 下生成 `.agents/skills/example-browser-upload-skill/SKILL.md` 与 `.skill-checksum`，真实浏览器 `/skills` 可看到安装记录、Provider、路径和 checksum。
- 2026-06-23 22:24：新增技能详情页 `/skills/{skillId}`：技能市场“查看详情”改为进入只读技能档案，展示已绑定团队、已绑定数字员工、创建与上传数据、归档包元数据、运行依赖与标签；技能详情 API 客户端新增 `getSkill`，绑定名称缺失时回退展示团队/员工 ID，避免真实数据空名导致审计信息不可见。验证：技能 API/列表/详情浏览器测试 14 项通过，`corepack pnpm --filter ./apps/web run typecheck`、`run build`、`git diff --check` 通过；重启真实 Web 后用 admin 会话打开 `/skills/e8f30e51-2202-4db5-8321-76f262e739d4`，Control Plane 详情接口返回 team_bindings/archive 字段，页面渲染技能档案、团队绑定、数字员工空态、创建上传数据且无控制台错误。
- 2026-06-22 16:00：新增技能上传单页工作台 `/skills/upload`，技能管理页上传入口改为跳转独立页面；zip 文件名去掉 `.zip` 后缀生成“技能包描述名称”，并单独填写“技能中文名称”用于市场展示和后续管理；运行依赖仅声明 CLI/环境变量名，移除步骤条、解析结果表、权限勾选和环境变量缺失检查。上传 FormData 不再提交空 `name`/`description`/`tags`/运行依赖字段，保留服务端从 `SKILL.md` 兜底解析。验证：`corepack pnpm --filter ./apps/web run test`、`corepack pnpm --filter ./apps/web typecheck`、`corepack pnpm --filter ./apps/web build`、`go test ./apps/control-plane/internal/skill/...`、`git diff --check` 通过；临时 Vite 服务 `/skills/upload` HTTP 200；真实浏览器点击验证因当前工具未暴露 Chrome/Browser 控制能力未完成。
- 2026-06-22 02:54：Added CLI runtime dependency checks for skills, Runtime tool capability reporting, and encrypted digital employee environment variable injection.
- 2026-06-21 15:32：Project coordinator Phase 1 planning profile 补齐两项 P0 缺口。（1）任务类型默认能力注册表（`task_type_defaults.go`）：为 `database_analysis`/`incident_triage`/`feature_development` 三类任务注册平台级默认 required capabilities（含别名归一化），planner 解码后 `ApplyTaskTypeDefaults` 将默认能力 union 合并到每个 PlannedTask.RequiredCapabilities，使能力匹配即使模型遗漏声明也能生效；默认流入 scoring → hard-failure → human-review 升级完整链路，task_type_defaults_applied 记入 planner_metadata 供审计。planner 系统提示新增 canonical task_kind 引导（database_analysis / incident_triage / feature_development），避免模型自创 generic kind 导致默认不触发。（2）Load / Reliability 真实数据接入：新增 sqlc 批量查询 `CountDigitalEmployeeOperationalSignals`（JOIN project_task_attempts + project_tasks，30 天窗口，按 assigned_digital_employee_id 聚合 in_flight/success/failure/human_reject 计数），employee.PgRepository 新增 `GetDigitalEmployeeOperationalSignals` + `OperationalSignals` 类型并加入 Repository 接口，planning_profile_adapter 在 `PlanningProfileRecords` 中调用并填充 `LoadState`（in_flight_tasks/available_slots/lendable）与 `ReliabilitySignals`（status/success_rate/recent counts），使 scoreLoad(10) + scoreReliability(5) 不再对全体员工恒定。验证：`go build ./...` 通过；`go test ./internal/workflow/projectcoordination/... ./internal/app/... ./internal/employee/...` 全绿（含 §9 默认补齐/unknown kind 不补/union 保留 5 例 + 反例集成测试「无 DB 能力员工被升级 human review」+ adapter LoadState/ReliabilitySignals 填充断言）。真实链路验证：对运行中 Control Plane(:8081) 用 admin 会话提交 demand 到含 2 DE 成员的项目，deepseek planner 端到端跑通——route decision 生成、task_kind=`database_analysis`、`task_type_defaults_applied=["db_slow_query_analysis"]` 记入 planner_metadata、`required_capabilities` 含 4 个默认能力（database.read/sql.analysis/data.quality.check/business.metric.interpretation）与模型自定义 union、selection evidence 完整持久化、缺能力任务正确升级 requires_human_approval；新 SQL 查询对真实开发库验证跑通（全窗口返回 1 员工 2 成功 1 失败）。
- 2026-06-21 11:37: Project coordinator Phase 1 real-chain validation hardened OpenAI-compatible planner output handling: normalized `selection_score` floats are accepted, platform-side planning profile scoring now overwrites model-provided selection evidence, and missing capability/profile gaps automatically route tasks and route decisions to human review instead of failing the plan.
- 2026-06-21 11:10：Runtime Agent 在执行项目任务 Provider 前先回写 `project_task_attempts/{attemptId}/started`，使 execution ledger 与项目执行 Trace 在真实 Runtime/Provider 链路中包含 `attempt.started`、Provider session 事件、终态与证据引用的完整顺序；补充 Runtime Agent 回写单测并修正 provision payload 测试中的技能 archive contract fixture。
- 2026-06-21 05:21：Added project execution ledger and execution trace read model for task attempts, Provider events, evidence refs, and Web project detail review.
- 2026-06-21 04:58: Control Plane project coordinator Phase 1 planning profile now injects digital-employee capability, runtime, permission, and selection evidence into planner snapshots and task metadata.
- 2026-06-21 01:23：新增 OpenFGA shadow 试点基础：Control Plane 配置支持 `AUTHZ_ENGINE=db|openfga_shadow|openfga` 与 `OPENFGA_*`，默认仍使用 DB 授权；新增 OpenFGA HTTP client、tenant/team/project 映射模型、shadow authorizer、direct OpenFGA fail-closed authorizer、tuple sync/backfill 层和 `cmd/openfga-backfill`；项目 team scope 写路径在 DB 成功后最佳努力同步 OpenFGA，shadow 模式记录 DB/OpenFGA diff 与 OpenFGA 错误审计 snapshot；权限中心 overview 展示当前引擎、OpenFGA store/model 与近 24h diff 计数。开发环境新增 OpenFGA + SQLite volume compose 服务、`scripts/dev-services.sh start openfga`、模型 bootstrap 脚本和配置示例。
- 2026-06-20 23:18：团队借调接入项目协调线程（闭环借调最后一块）：协调线程在构建可执行数字员工池（`LoadProjectCoordinationSnapshot`，挑人前）新增借调闸门——归属团队不等于项目自有团队（`Project.TeamID`）的数字员工，只有当项目持有 (project, team) 维度的有效借调授权（status ∈ approved/auto_approved）时才进入可执行池；无授权的外团队员工被静默剔除并写 `project.lending.employee_skipped` 协调事件（最佳努力、不阻断规划）。无自有团队/无归属团队的员工不受闸门约束；借调查找出错时与就绪检查一致 fail-open（权威授权仍由审批流把守）。新增 sqlc 查询 `ListEffectiveLendingTeamsForProject` + `teamlending` repo `ListEffectiveLendingTeams` + service `EffectiveLendingTeams`；协调侧新增 nil-safe `LendingGatekeeper` 接口与 `ProjectStore.WithLendingGatekeeper`，app.go 以 `lendingGatekeeperAdapter`（employee 仓储解析员工归属团队 + teamlending 仓储解析项目有效授权）装配。验证：`go test ./internal/teamlending ./internal/workflow/projectcoordination` 通过（含闸门三态 + fail-open + 跳过事件断言、`EffectiveLendingTeams` 单测）；新 SQL 在真实开发库验证只返回 approved/auto_approved 授权团队、排除 rejected/revoked；`go build ./...` 通过、Control Plane(:8081) 重启加载新装配并健康、协调 worker 正常注册。
- [2026-06-20 18:29] ProjectTask durable closure now exposes execution packets, liveness projection, and real-chain smoke coverage.
- 2026-06-20 20:43：团队借调（lending）后端落地（D1 团队级 (project,team) 粒度 / D2 超纲强制转人工 / D3 请求自带 status + inbox/audit / D4 创建后配置）：新增 `internal/teamlending` 模块（types/repository/pg_repository/service[含 auto/manual + 超纲例外]/handler）+ sqlc 查询 `internal/storage/queries/team_lending.sql`（policy upsert/get、request create/list-by-team/list-by-project/approve/reject/revoke）；路由 `/api/v1/teams/{teamId}/lending-policy`(GET/PUT)、`/lending-requests`(GET)、`/lending-requests/{requestId}/approve|reject|revoke`(POST)、`/api/v1/projects/{projectId}/lending-requests`(POST/GET)；authz 新增 `team.lending.policy.read/edit`、`team.lending.request.read/decide`；`audit.Service.RecordEvent` 记录 `team.lending.*` 审计，`inbox` 新增 `team_lending` 事项类型投影到团队负责人（pending 创建 / 裁决时 resolve）；OpenAPI 补 lending paths+schemas 并 `pnpm generate:control-plane` + `pnpm verify:contracts` 通过；前端团队详情新增「借调」tab（编辑策略 + 待审批/历史，复用 LiquidTabsList/Badge）。真实链路验证：对运行中 Control Plane(:8081) 用 admin 会话 curl 走 PUT/GET policy、POST request(auto/超纲例外/manual)、approve/reject/revoke、状态机二次裁决 400、项目侧 list 全路径，并查库确认 team_lending_request 状态、audit_events(team.lending.*)、inbox_items(open→resolved) 一致；go test `internal/teamlending` 通过；headless chromium 真实浏览器点穿「借调」tab（登录→团队详情→借调 tab 渲染策略表单+待审批+历史，lending API 实返 200，5/5 断言通过）。
- 2026-06-20 17:47：技能包存储重构为 zip 整包对象存储模式：删除逐文件 `skill_files` 表与在线编辑链路（`UpdateSkillFile` handler/service/repo/web/contract），删除 `skills.status` 列（移除 installed/available 概念），改为上传 zip 整包存 S3（`storage.S3ObjectStore.PutObject`）、DB 仅存 archive 元数据（`archive_object_ref/archive_filename/archive_size_bytes/archive_checksum_sha256/archive_file_count`）；Web 技能页删除 Monaco 编辑器/文件树/保存按钮/已安装-市场双 tab，改为统一技能列表 + archive 详情 + 团队/Agent 绑定面板；Runtime Agent 新增 `skills.rs` 模块实现 `materialize_skills`（从 S3 拉 zip → 校验 SHA256 → 解压到 `agent_home/skills/{skill_key}/`，支持幂等跳过、路径穿越防护、200MB/10000 文件上限），`RuntimeSkillPayload` 改为 archive 字段格式，`RuntimeCommandExecutor` 在 `ProvisionInstance` 时自动 materialize skills；Control Plane `buildProvisionInstancePayload` 改用 `ListSkillsForRuntime` 查询员工生效技能并填入 archive 元数据。migration 025 清理 diagnose/tdd 种子数据。真实链路验证：migration 025 已 apply 到开发库、上传真实 zip 到运行中 Control Plane（:8081）验证 S3 落盘 + DB archive 元数据填充 + 列表/详情 API + 团队绑定 + 旧编辑路由 404。
- [2026-06-20 17:43] ProjectTask recovery now supports retry scheduling, typed waiting-human pauses, and acceptance-gated completion.
- 2026-06-20 17:15：新建团队页重构为方案 B「全页生命周期创建台」：抽屉式两步表单 `CreateTeamDrawer` 替换为独立路由 `/teams/new`（`CreateTeamView` + `CreateTeamPage`），左栏团队身份/团队负责人/初始成员三卡、右栏实时预览卡 + 创建后生命周期清单（治理、对外借调、数字员工、外部能力、审计）+ 提交区；团队负责人文案修正为团队管理者语义（成员增删与团队配置，审批/驳回/补证/验收归项目负责人）。仅复用既有 teams 接口，借调等生命周期项为信息占位。新增 `create-team-draft.ts`（草稿类型与 `CreateTeamInput` 映射）、`create-team-page.test.tsx`（5 项浏览器测试），删除 `create-team-drawer.tsx`/`create-team-basic-step.tsx` 并将创建用例迁出 `index.test.tsx`。新增原型 `docs/prototypes/create-team-concept-a-focused-sheet.html`、`create-team-concept-b-lifecycle-console.html`(+png) 与设计文档 `docs/design/team_lending/team-lending-and-create-team-design.md`（团队借调模型设计，后端待落地）。真实链路验证：web typecheck 通过、teams 浏览器测试 17 项通过、对运行中 Control Plane（:8081）以真实会话 GET/POST `/api/v1/teams` 验证页面负载契约（HTTP 201 持久化、metadata.display 正确）并清理测试数据。
- 2026-06-20 16:18：运行时 ProjectTask 回写切换到 attempt-scoped contract：Control Plane 新增 `/api/v1/runtime/project-task-attempts/{attemptId}` started/lease/complete/fail 路由与持久化写回，终态写回同步记录 `project_tasks.terminal_event_id` 与 attempt terminal facts，Runtime Agent 改用 attempt_id、lease_token、runtime_node_id 和 idempotency_key 回写完成/失败；项目协调分派预生成 deterministic attempt id/lease 并在 Runtime command metadata 中补齐 attempt 与 runtime node facts，旧 `/api/v1/runtime/project-tasks/{id}` 回写路由不再暴露。
- 2026-06-20 04:13：ProjectTask durable closure Phase 1 完成控制平面基础：新增 `project_task_attempts`、queued attempt 排队写入、dispatch 创建 attempt 的 durable 状态桥接，并以 accepted plan revision exact-once decomposition 防止规划重试重复生成项目任务。
- 2026-06-20 03:34：ProjectTask durable closure Phase 1 Task 5 将项目协调分派改为 queue-attempt 写入：`DispatchProjectTask` 保留真实 Runtime run 创建桥接，但以 `QueueProjectTaskWithAttempt` 绑定 `digital_employee_run_id`/`runtime_task_id`/`runtime_node_id` 并保持任务 durable 状态为 `queued`；`project_task.dispatched` 事件携带 attempt/run/runtime facts，queued attempt 的 execution context packet 补齐项目、需求、任务、员工、run、runtime 与 handoff contract 信息，兼容旧 Runtime project-task writeback 依赖的 run/task 绑定。
- 2026-06-20 03:06：ProjectTask durable closure Phase 1 补齐任务排队写入入口：新增 ProjectTask/Attempt 状态常量、attempt domain/repository contract、`QueueProjectTask` service 方法、`project_task_attempts` sqlc 查询与 PgRepository 事务实现；排队时写入 `project_task.dispatched` 事件、创建 queued attempt、更新任务为 queued 并绑定 current_attempt/attempt_count，补充 service memory repo 与 PgRepository 回归测试。
- 2026-06-19 02:44：新增数字员工 operational status 读模型，Control Plane 基于 Runtime、Provider、run、project task 与员工级人工决策事实解析 `working`/`idle`/`queued`/`waiting_human`/`error`/`unavailable`/`needs_configuration`，并保持 `project_acceptance` 为项目级状态、不投影成每个员工的 `waiting_human`；overview API/OpenAPI/TypeScript 类型新增 `operational_status_counts` 与 `operational_state`，Web 数字员工工作台改以 operational status 作为主徽标并保留 `workbench_status` 兼容信息。新增 employee operational resolver、SQL facts、sqlc 映射、索引迁移、API route 测试、Web 类型与浏览器测试，并以当前 worktree 的独立 Control Plane/Web 真实链路验证 overview 和员工页渲染。
- 2026-06-18 23:04：项目验收闭合(Gap 3):项目全部 demand 到达终态后,协调器自动把项目转入 `acceptance` 并向负责人发起一条 `project_acceptance` 决策(复用既有 approval + decision_request + inbox 投影,点亮「待确认」待办);人工 `accepted` → 项目归档(archived)并写验收结论,`rejected`/`needs_more_evidence` → 项目回 `running` 返工,均经 `SignalHumanDecisionSubmitted` 回流。新增 sqlc `CountProjectDemandsByTerminality`/`TransitionProjectStatus`、repo `AreAllProjectDemandsTerminal`/`TransitionProjectStatus`,协调器 `IsProjectAcceptanceReady`/`RequestProjectAcceptanceReview`/`ApplyProjectAcceptanceDecision` 活动与 workflow 触发+回流分支;状态迁移用前向守卫保证幂等(重复完成/重试不重复建验收请求)。无 schema 变更(`decision_type` 为 VARCHAR、`projects.status` 复用既有列)。补充 store 单测(幂等、accept→archived、reject→running)与 Temporal workflow 触发+决策回流测试。
- 2026-06-18 15:41：任务完成时自动把数字员工回写的结构化 `evidence_refs`/`artifact_refs` 物化进 `/projects/{id}/evidence`、`/artifacts` 读模型，复用既有 `CreateEvidenceRefWithEvent`/`CreateArtifactRef` 写入路径并产出 `project.evidence.linked`/`project.artifact.linked` 审计事件,使运行总览的「证据/工件」卡片有真实数据;解析器容忍字符串或 `{ref/id/title/type}` 结构、缺引用自动跳过,物化为完成主链的最佳努力步骤(失败仅审计、不回退已完成任务),完成状态守卫保证一次任务只物化一次。补充解析器纯函数单测与完成路径物化事件断言。

### Changed

- 2026-06-23 10:02：成本中心 `/costs` 继续迁移到 v3 Soft-Flat：保留 `project_id` search 参数、`CostsProjectView` 与项目预算摘要/流水接口调用，路由标题和无项目上下文状态统一改用 `V3PageHeader`、`IconTile`、`WorkSurface` 与 `V3EmptyState`，移除该路由旧 glass 卡片与旧语义图标引用。验证：新增 `/costs` 路由浏览器测试覆盖无项目态、项目预算接口透传、v3 surface 和旧 DOM 归零。
- 2026-06-23 02:26：审计中心 `/audit` 继续迁移到 v3 Soft-Flat：保留 `project_id` search 参数与 `/api/v1/audit/events` 真实接口查询，页面标题、无项目上下文、加载、错误、空状态和项目审计表格统一改用 `IconTile`、`WorkSurface`、`V3Table`、`StatusPill` 与 v3 状态组件，移除该路由旧 glass 卡片与旧状态徽标引用。验证：新增 `/audit` 路由浏览器测试覆盖 v3 surface 和旧 DOM 归零；`corepack pnpm --filter ./apps/web run typecheck`、`run build`、`run test` 通过；重启真实 Web 后用 admin 会话打开 `/audit?project_id=55da605e-8083-42a0-978c-e4d031c33451`，`/api/v1/audit/events` 返回 200，桌面与 390px 移动端旧 DOM 为 0 且无横向溢出。
- 2026-06-23 01:53：Runtime 节点、权限中心与共享占位菜单继续迁移到 v3 Soft-Flat：`/runtime` 改用 v3 指标卡、`WorkSurface` 节点/事件表格、`StatusPill`、软卡片和 v3 筛选区域，保留 Runtime 总览、接入审批、Provider 能力、事件筛选与批准/拒绝交互；`/permissions` 入口、授权概览、审计表、Runtime 范围、成员角色和权限诊断统一改用 v3 图标、Tab 容器、软卡片、`WorkSurface` 表格、`StatusPill` 与 v3 表单按钮，保留五个授权子面板原 API、表单校验和确认弹窗行为；`UnimplementedPage` 改用 v3 软卡片与图标，保持占位路由“不使用 mock 数据”语义。验证：本批目标测试 19 个通过，迁移范围旧 glass 符号 grep 为 0。
- 2026-06-23 01:36：项目管理、流程编排入口与收件箱继续迁移到 v3 Soft-Flat：`/projects` 列表与运行详情改用 v3 指标、`WorkSurface` 表格、`StatusPill`、治理 Tabs 和软卡片，删除已无调用点的旧项目列表/Workflow 实例卡片；`/workflows` 入口改为 v3 signature + 指标 + 流程实例表格；`/inbox` 改为 v3 指标、筛选面板、待办表格和动作 Dialog。保留原接口、筛选、分页、项目详情回调、Inbox 操作和 TanStack Router 跳转。验证：本批目标测试 55 个通过，`corepack pnpm --filter ./apps/web run typecheck`、`run build`、`run test` 通过；重启真实 `control-plane`/`web` 后用 admin 会话打开 `/projects`、`/workflows`、`/inbox`，真实数据加载成功，无页面崩溃、无横向溢出，本批实际渲染范围旧 glass slot/class 计数为 0。
- 2026-06-23 00:51：登录后首页与 Shell 迁移到 v3 Soft-Flat：`/` 工作台改用 `V3PageHeader`、`SignatureCard`、`V3MetricCard` 与 `WorkSurface` 登录日志表格，保留原 `getHealth`、`/api/auth/me`、`/api/auth/login-logs` 数据路径；现有左侧栏/Header/Auth 布局只重新着色为中性 v3 背景、白色侧栏、蓝色激活态和弥散阴影，不改成顶栏。验证：Dashboard/Shell 定向测试通过，`corepack pnpm --filter ./apps/web run typecheck`、`run build`、`run test` 通过；重启真实 `control-plane`/`web` 后用 admin 会话打开 `/`，`/health`、`/api/auth/me`、`/api/auth/login-logs` 均返回 200，桌面与 390px 移动端无页面横向溢出，Dashboard/Shell 旧 glass class 计数为 0。
- 2026-06-23 00:30：登录页 `/login` 迁移到 v3 Soft-Flat：认证壳层改用中性 v3 背景与软卡片，登录表单输入、错误态和提交按钮改用 v3 token 与 `V3Button`，保留原 redirect、登录接口调用、表单校验和 session 跳转行为。验证：登录页/表单定向测试通过，`corepack pnpm --filter ./apps/web run typecheck`、`run build`、`run test` 通过；重启真实 `control-plane`/`web` 后打开 `/login?redirect=/skills`，admin 登录返回 200 并跳转 `/skills`，移动端 390px 无横向溢出且页面无旧 auth/liquid class。
- 2026-06-23 00:17：技能管理首页迁移到 v3 Soft-Flat 样板页：新增复用型 `v3-components`（SoftCard、V3MetricCard、StatusPill、IconTile、WorkSurface、V3Table、V3Button、V3Segmented、状态覆盖组件等），技能市场首页改用 v3 token 和软壳/脆数据面容器，保留原搜索、筛选、分页、上传入口与列表/网格切换行为。验证：`corepack pnpm --filter ./apps/web run typecheck`、`run build`、`run test` 通过；重启 `control-plane`/`web` 后用真实 admin 会话打开 `/skills`，`/api/v1/skills` 返回 200。
- 2026-06-22 19:15：重构技能管理首页为技能市场视图：保留技能上传入口，新增市场指标、搜索筛选、列表/网格切换、风险/状态/绑定目标展示，以及“查看详情”“安装”占位按钮；详情和安装流程不在本次实现范围内。验证：`corepack pnpm --filter ./apps/web run typecheck`、`corepack pnpm --filter ./apps/web run test`、`git diff --check` 通过；真实链路将在合并到 `main` 后用当前 `main` 页面连接真实 Control Plane 复验。
- 2026-06-21 02:15：修复真实 OpenFGA 1.18 链路验证暴露的 tuple 写入兼容问题：`OpenFGAHTTPClient.WriteTuples` 现在接受 Write API 成功返回的 `200` 或 `204`，并在 writes/deletes payload 中分别带 `on_duplicate: "ignore"`、`on_missing: "ignore"`，确保 backfill、单记录 upsert/revoke 和重试同步具备幂等性。
- 2026-06-21 01:44：`scripts/dev-services.sh start openfga` 改为默认优先使用本机已安装的 `openfga` CLI 启动，自动执行 `openfga migrate` 后以 SQLite 文件 `.scratch/openfga/openfga.db` 运行服务，并继续支持 `SUPERTEAM_DEV_OPENFGA_MODE=compose` 走 Docker Compose；新增本机 CLI 启动、状态与停止的脚本回归测试。
- 2026-06-21 00:26：需求路由 planner HTTP 调用去 DeepSeek 专名化，重命名为 OpenAI-compatible planner/client/request/config 边界；生产装配继续从 `planner.baseURL`/`planner.model`（或对应环境变量）取模型与 endpoint，不再在代码默认值里绑定具体 DeepSeek provider/model，`config.example.yaml` 改为通用占位并标注 DeepSeek 与千问兼容端点示例。补充 qwen 模型请求构造与 provider-neutral 错误文案测试。真实链路验证：重启运行中 Control Plane 后，通过真实 admin 会话向现有项目提交需求，协调线程执行 `PlanDemandRoute` 并生成 route decision、task graph 节点与待审核决策，无 chat/model/provider 错误。
- 2026-06-20 23:49：修复项目需求编排到人工验收的真实闭环断点：Inbox API 对空 actions 返回空数组并在 Web 列表兼容旧数据；项目决策待办允许无 approval_request_id，使 `project_task_acceptance` 可投影到 Inbox；高风险/需验收任务完成后会生成验收待办，人工批准后将 `waiting_human` 项目任务推进到 completed；必需人审的新需求在 planner 输出不合规时按策略合成“执行任务 + route review”计划；项目无 team_id 时团队借调闸门 fail-open，避免把执行员工池过滤为空。真实链路验证：Web 提交需求 → route review 人工批准 → Runtime Agent 执行 → task acceptance 人工批准 → demand/task 均 completed。
- 2026-06-19 11:12：收窄数字员工 operational status 的 `queued` 口径：项目任务只有 `planned`/`assigned` 会计入员工级“排队”，`pending` 仍视为 intake/pre-dispatch 事实，`blocked` 交由项目/流程阻塞、员工级 `waiting_human` 或 `error` 等更高优先级状态表达；同步更新 sqlc 查询、状态文案、后端断言测试和状态口径 spec。
- 2026-06-18 21:43：修复项目任务派发后永不回写导致的闭环断裂:Runtime Agent 仅在 `handoff_contract.completion_path == "project_task_writeback"` 时才回写项目任务完成,而 control-plane 此前原样透传 planner 的 handoff_contract(该字段时有时无),导致任务跑完却静默跳过回写、项目任务恒停 `assigned`、需求不收敛、证据不上浮。现 control-plane 在 `DispatchProjectTask` 派发 metadata 里强制 `completion_path = "project_task_writeback"`(新增 `projectTaskDispatchHandoffContract`);并加 Runtime Agent 兜底——当 `source == "project_task_dispatch"` 且有 `project_task_id` 时,缺省 `completion_path` 也按回写处理(显式非匹配值仍尊重),使运行链路对 CP 漏设该字段更健壮。补充 Go 派发断言与 Rust 单测。
- 2026-06-18 20:37：协调器执行人选择改为只考虑 runtime-ready 数字员工:新增 `digital_employee_runtime_readiness` 视图(migration 022),把 overview 的 runnable 判定(employee/execution status、effective_config、runtime_node 在线、provider 健康、agent_home_dir、governance approved 等)提升为单一权威读视图,杜绝重复判定漂移;配套 `AreEmployeesRuntimeReady` sqlc 查询与 `employee.Repository.AreRuntimeReady`。协调器 `LoadProjectCoordinationSnapshot` 经注入的 `DigitalEmployeeReadinessChecker` 过滤掉未就绪成员,使推理 planner 只把任务派给真正可执行的员工,修复之前选中未绑定 runtime 的员工导致任务静默 stranded(`assigned` 无 run、`/claim` 恒 204)的断点;checker 缺省/报错时 fail-open 不阻断规划。
- 2026-06-18 16:10：需求路由规划改为「仅推理模型」:删除生产路径里的非推理 heuristic 兜底(`HeuristicRoutePlanner` 仅保留为测试桩),`routePlannerFromConfig` 一律构造 DeepSeek 推理规划器,`PlanDemandRoute` 活动在规划器报错或缺省时直接抛错而不再静默降级为全量派发。为让推理模型规划真实跑通,将 planner HTTP 请求超时从 20s 提到 120s、协调器活动 `StartToCloseTimeout` 从 30s 提到 180s;并放宽任务字段解析,推理模型把 `input_requirements`/`handoff_contract` 输出为数组或标量时归一化为对象(数组→`{items:[]}`、标量→`{value:...}`)而非判失败,修复 `deepseek-v4-pro` 规划真实返回被 schema 形状误拒导致需求恒停 planning_pending 的链路阻塞。
- 2026-06-17 03:15：重构流程编排为“入口实例卡片 + 任务图工作台”两层体验，入口页只展示工作流全局状态、进度、阻塞和最新事件，详情页接入真实 task graph 读模型并以 React Flow 展示任务节点、阶段摘要、Runtime run、人工决策和右侧节点检查器；Workflow Instances API 同步补齐进度、优先级、风险、SLA、阶段统计和节点阻塞/状态原因字段，并支持通过环境变量追加本地调试 Console CORS origin 以完成多端口真实浏览器验证。
- 2026-06-16 21:06：补齐项目需求生命周期收口，`project_demands.status` 新增 planned/executing/completed/failed 终态，并在任务图规划（planned）、任务分派（executing）、全部项目任务完成或失败（completed/failed）的写回点按前向守卫推进，与项目事件序列共用 per-project 咨询锁避免并发回退；新增 `UpdateProjectDemandStatus`/`CountProjectTaskStatusesByDemand` sqlc 查询、OpenAPI 枚举与迁移注释，并以真实 Postgres 集成测试验证需求从 planning_pending 走到 executing 再到 completed，修复需求恒显示“规划中”的状态失真。
- 2026-06-16 03:43：Web 流程编排详情页新增只读 `@xyflow/react` ProjectTask DAG，可从真实 task graph read model 渲染任务节点、依赖、人工决策附件、Runtime run 和执行结果，并提供节点详情检查器。
- 2026-06-16 00:09：Control Plane 新增 Workflow Instances read model API，基于项目需求事实返回当前 Console 用户可见的工作流实例摘要、进度、状态优先级和协调作业引用，并补齐 sqlc、HTTP route 与 OpenAPI 契约。
- 2026-06-14 22:08：项目协调底座新增 DAG 规划与任务图执行能力，包含 DeepSeek/OpenAI-compatible planner 配置、ProjectTask 图与依赖持久化、ready 节点分派、完成契约校验、失败恢复决策、任务图 read model，以及 Runtime Agent 对 ProjectTask completion writeback 的真实执行桥接。
- 2026-06-14 01:59：Runtime Agent 新增 Codex Provider 接入，包含 provider catalog 配置、Codex CLI JSONL adapter、`.codex` 工作区隔离、Runtime command/legacy task provider selection、capability 上报，以及 HTTP/CLI smoke 路径和回归测试覆盖。
- 2026-06-13 22:24：Control Plane 消费 Runtime `sync_workspace_files` 终态回写，按 receipt 目标更新数字员工 workspace file sync 投影，成功记录 synced hash，失败记录错误信息，并避免误触发 provisioning 生命周期状态或清理逻辑。
- 2026-06-13 03:01：Control Plane: 打通 ProjectTask 协调分派到 DigitalEmployeeRun 的执行桥接，成功分派先创建真实 run 再原子绑定 digital_employee_run_id/runtime_task_id（绑定先于事件，避免孤儿 run），重复分派与 Temporal 重试幂等；失败时按可重试性记录分派失败事件，终态失败标记为不可重试，单个任务分派失败不再中断同批其余任务。
- 2026-06-12 21:20：新增“任务发起”一级入口，替换原主导航“任务中心”，支持按项目提交需求、自动或显式选择人类审核人，并提供任务发起详情页追踪协调 Job、路由决策、项目任务和人类决策请求等首版协调事实。
- 2026-06-12 23:21 新增收件箱可操作工作队列：增加 `inbox_items` read model、Inbox API、审批/项目决策投影、左侧菜单入口和 `/inbox` 页面，支持个人待办、团队只读视图和轻量处理动作。
- 2026-06-12 16:25：新增当前登录用户账户自服务页 `/settings/account`，支持查看个人资料、DiceBear 头像、自己的最近登录记录，并通过自服务 API 修改资料和密码。
- 2026-06-12 11:45：补齐项目管理 V2 Web 治理写入入口，项目详情支持跳转审计/成本视图，并可在证据链、验收结论、归档预览中新增证据、验证证据、提交验收和生成归档快照。
- 2026-06-11 23:52：完成项目管理 V2 治理证据归档闭环，新增项目证据链、工件与报告引用、预算流水、验收结论、归档快照、归档工件保留锁、配置修订历史，以及审计中心和成本中心 project_id 联动。
- 2026-06-11 15:45：项目管理 V1 接入 Temporal 虚拟协调线程，新增 RouteDecision、CoordinationJob、ExecutionSummary、TransferRequest、人类决策投影和 Workflow signal 可观测能力。
- 2026-06-11 04:01：项目管理 V0 新增真实项目管理入口、项目事实模型、项目事件流、配置治理页与提交需求到当前项目能力。
- 2026-06-10 13:44：完成双层技能管理主链路收口，贯通团队公共技能/MCP 与数字员工个人技能/MCP 的 Control Plane 契约、服务接口、Web 管理入口和回归测试，形成团队共享能力与员工个性化配置分层治理能力。
- 2026-06-10 13:28：数字员工配置页升级为三页签工作台，新增宪法/人格工作目录文件编辑、个人技能与 MCP 配置，并保留高级 JSON 配置修订表单。
- 2026-06-10 13:11：团队详情“能力与知识”页签新增公共技能与公共 MCP 管理界面，支持查看已安装团队技能、从技能市场安装、移除技能，以及创建和移除团队 MCP 服务并绑定可选用户凭据。
- 2026-06-09 01:03：新增数字员工详情页单次调用测试 Dashboard 静态原型，参考 paperclip Agent 页的信息架构，突出调用记录、端到端健康、单次测试入口以及团队共享能力与员工覆盖关系。
- 2026-06-07 19:50：新增数字员工内置头像库，提供 20 个工程师头像资产、Control Plane 头像资产列表 API、创建时 `avatar_asset_id` 校验与 metadata 快照，并在 Web 创建、列表和详情页展示员工头像。
- 2026-06-07 15:35：数字员工页面新增"配置"功能，支持创建配置修订版本（Config Revision），包括 Role Profile、Constitution Addendum、Capability Selection、Context Policy Override、Approval Policy Override 和 Output Contract Addendum 的 JSON 配置编辑。
- 2026-06-07 02:28：Runtime Agent 完成执行能力底座核心模块开发，新增按执行实例隔离的工作目录管理、S3 工件上传基础设施和 Provider 会话状态扩展支持。
- 2026-06-07 02:52：Runtime Agent 执行器适配新工作目录结构，execute_task 使用 create_run_workspace 创建 workspace/logs/artifacts 隔离目录，完成端到端执行闭环基础改造。

### Changed

- 2026-06-22 18:25：重构上传技能页 `/skills/upload` 的单页工作台视觉布局：参考新原型改为顶部 zip 包状态带、左侧技能信息与运行依赖声明、右侧发布摘要卡片；保留技能包描述名称由 zip 文件名生成、技能中文名称单独填写、CLI/环境变量仅声明不检测的业务语义，并修复 CLI/环境变量逐字输入时 token 被截断为单字符的问题。验证：`corepack pnpm --filter ./apps/web run test`、`corepack pnpm --filter ./apps/web typecheck`、`git diff --check` 通过；本地 Web/Control Plane/Runtime 服务运行中，真实 Chromium 登录 `admin/admin` 后打开 `/skills/upload`，选择示例 zip 并填写依赖声明，桌面与 390px 移动端无横向溢出，逐字输入 `gh,node` 与 `GH_TOKEN,OPENAI_API_KEY` 可正确保留完整 token。
- 2026-06-18 15:45：对齐流程编排工作台与真实期望的偏移，移除流程详情页左侧「流程实例」侧栏（其操作顺序信息已在右侧流程图中渲染，属冗余），详情页改为全宽流程图；流程图节点详情由原右侧固定卡片改为点击「数字人执行任务」节点才弹出的居中弹窗 Dialog，进入详情页不再预选节点、弹窗不自动弹出，点击画布空白或关闭按钮收起弹窗；同步删除已无消费者的 `workflow-instance-list.tsx` 与节点检查器的固定卡片分支，并更新相关组件测试。
- 2026-06-17 04:02：项目创建抽屉改为基于当前登录人授权范围选择团队，加载 `/api/auth/me` 与用户可选团队范围，只允许提交 active 授权团队，并默认使用当前用户作为项目负责人。
- 2026-06-17 02:41：重做用户管理中新建人类平台用户流程，改为抽屉式表单，直接设置用户名、名称、密码、内置头像资产和多选可选团队，并在用户详情中展示可选团队范围。
- 2026-06-16 00:55：调整登录页品牌布局，移除右上角品牌横幅，裁剪主展示图透明留白并放大主视觉，同时压紧品牌图与账号登录卡之间的垂直间距。
- 2026-06-16 00:35：重做“炬枢平台 / 新炬网络”品牌资源，使用 gpt-image2 生成更高对比的控制平面 mark，并由本地精确排版合成透明登录展示图、右上角横幅和侧栏 mark，避免图片与浅色登录背景混色后不可读。
- 2026-06-16 00:11：调整“炬枢平台 / 新炬网络”品牌展示，放大认证后 Shell 左上角品牌 mark 与文字，并取消登录页品牌图的背景混合模式，改用原色透明 PNG、轻量光晕和对比度增强提升可读性。
- 2026-06-15 23:32：登录页与认证后 Shell 品牌替换为“炬枢平台 / 新炬网络”，新增 gpt-image2 生成后重排并透明化处理的登录横幅和主展示图，登录页使用品牌图融入背景，侧栏左上角使用图片 mark 搭配可读品牌文字。
- 2026-06-14 10:28：收紧 `AGENTS.md` 与 `$superteam-completion-check`，将前后端、跨层、Runtime/Provider、数据库/迁移和合并后的真实端到端验证提升为默认完成硬门禁；缺少服务、认证、Provider 或安全环境时必须标记阻塞，不能以“未做真实链路验证”作为完成说明。
- 2026-06-13 11:48：优化任务发起页项目、审核人、优先级和风险级别字段展示，改为响应式参数条布局，减少宽屏两列布局造成的大空档并保持移动端自动换行。
- 2026-06-13 11:45：优化任务发起页低高度与移动端自适应布局，将提交需求和保存草稿提升到首屏标题区可见位置，并固定侧边栏激活项在不同主题测试顺序下保持白色文字。
- 2026-06-13 11:10：重构 Web 任务发起页为提交前需求工作台，收敛为需求、项目、审核人、优先级、风险级别和补充资料，右侧仅用编号说明提交后的动态编排顺序，避免提前展示编排状态或上下文边界配置。
- 2026-06-13 10:58：进一步瘦身 `AGENTS.md`，移除阶段性参考、完整技术清单、常用命令和重复收尾检查，将宪法收敛为项目定位、架构职责边界、主栈边界、数据库入口、目录边界和核心协作不变量。
- 2026-06-13 10:53：精简 `AGENTS.md` 目录边界，删除完整目录树，保留 Web、Control Plane、Runtime、contracts、connectors 和 HTML 原型的关键放置规则，降低宪法上下文噪音。
- 2026-06-13 10:47：将 `$superteam-completion-check` 从全局 Codex skills 迁移为项目内 `.codex/skills/superteam-completion-check`，并调整 `.gitignore` 仅允许项目 skills 进入版本控制，避免 SuperTeam 专用规则污染其他仓库。
- 2026-06-13 10:45：扩展 `$superteam-completion-check`，从 `AGENTS.md` 提炼通用收尾检查项，覆盖变更日志、生成代码、数据库迁移、前端状态保持、真实浏览器验证、分层边界和项目/审批/人类决策不变量。
- 2026-06-13 10:43：新增本地 Codex skill `$superteam-completion-check`，将 SuperTeam 任务完成前的真实链路、运行中服务、数据库迁移和交付声明检查从 `AGENTS.md` 长规则中提炼为可复用收尾流程；`AGENTS.md` 改为短触发规则。
- 2026-06-13 10:36：收紧 `AGENTS.md` 开发宪法，要求跨层功能完成前区分 mock/局部验证与真实链路验证，确认运行中服务已加载当前代码，并验证数据库迁移实际落库后才能声明功能可用。
- 2026-06-13 10:29：修正收件箱 `016_inbox_items.sql` 对应的 Atlas checksum，并完成真实 Control Plane 链路验证，避免迁移未落库时 `/inbox` 页面长期停留在加载态。
- 2026-06-12 19:04：Web Browser-mode 测试脚本改为按最低支持 revision 选择本机已安装的最新 `chromium_headless_shell-*` 可执行文件，避免测试环境被锁死到单个 Playwright 缓存 revision。
- 2026-06-12 18:38：优化数字员工创建页专业模板卡片的宽屏自适应布局，模板区在大屏下切换为三列展示，并移除与模板选择无关的头像堆叠，改为展示默认角色等后端模板字段。
- 2026-06-12 18:13：数字员工创建页改为“选择路径/专业模板/预检摘要”与“员工画像蓝图配置”两段式工作台，并让创建候选接口返回 `creation_checks` 服务端预检摘要，保证前端预检展示与 Control Plane 创建校验口径一致。
- 2026-06-13 00:10：修复收件箱项目 ID 和目标用户 ID 筛选，非法或 nil UUID 不再进入查询参数，并为无效输入保留草稿值和中文校验提示。
- 2026-06-12 23:51：收件箱页面新增可操作筛选栏，支持按状态、事项类型、风险等级、项目 ID 和目标用户 ID 查询，并在筛选变更时重置分页偏移。
- 2026-06-12 23:36：收件箱团队只读视图接入现有授权器，具备租户级 `team.read` 权限时开放团队列表和 badge 团队数量，未授权仍按个人范围返回，并补充团队待办查询索引。
- 2026-06-12 16:17：收紧创建数字员工身份步骤的头像选择区，头像项改为固定小尺寸并自动换行，避免宽屏下圆形头像被网格撑得过大。
- 2026-06-12 16:08：数字员工工作台员工卡片列表改为前端分页浏览，默认每页 12 个，翻页和每页数量会下传 overview `limit/offset`，避免一次性拉取和渲染全部员工。
- 2026-06-11 17:19：补充项目管理 V1 后端端到端仿真测试，覆盖需求 Workflow signal 失败重试、Runtime 任务写回身份绑定、终态 signal 重试和读模型无重复事实。
- 2026-06-11 17:01：收紧项目管理 V1 协调运行态一致性，Runtime 写回需通过任务状态和数字员工运行记录绑定校验，高风险路由需等待人类审批后再分派，并为 Workflow signal 失败审计事件提供可重试入口。
- 2026-06-11 10:04：修复项目配置子路由被项目详情父路由吞掉的问题，确保 `/projects/{projectId}/config` 正确渲染配置治理页。
- 2026-06-10 13:59：修复双层技能管理后端终审问题，允许数字员工归属人管理个人配置/能力与用户自管理凭据，并对有效技能/MCP 结果去重且保留配置修订遗漏字段。
- 2026-06-10 13:54：修复数字员工高级配置表单的修订提交行为，改为只提交用户实际编辑过的 JSON 配置或预算策略，并在脏 JSON 字段无效时阻止保存。
- 2026-06-09 09:37：恢复 Runtime WebSocket 默认 Origin 校验，移除对所有跨源握手的全局放行，并补齐跨源连接拒绝的回归测试。
- 2026-06-08 22:37：调整数字员工卡片选中视觉，左侧选中标记仅在选中卡片显示且不承载团队或运行状态颜色，未选中卡片保持干净边界。
- 2026-06-08 22:31：优化数字员工卡片选中态，移除“选中/已选中”文字按钮，改为单层选中边框与轻量高亮，避免边框叠加。
- 2026-06-08 21:28：修复数字员工工作台筛选时清空旧数据造成的整页加载闪烁，并支持点击员工卡片本体切换选中状态。
- 2026-06-08 20:51：优化数字员工工作台布局，将筛选区收敛到员工卡片列表同宽左列，右侧待处理队列和选中员工面板提升到首屏顶部，并把员工卡片团队名合并到身份元信息行以减少空白。
- 2026-06-08 03:05：数字员工首页改为执行工作台卡片视图，新增每日 Token 预算治理配置、overview 预算摘要和运行前预算拦截。
- 2026-06-07 23:18：深度统一二级页面与表单页（如团队详情页、数字员工创建与配置页）的头部图标样式，将其从黑白描边容器全面替换为彩色的 `SemanticIconTile` 语义容器，实现全站内外视觉高度一致。
- 2026-06-07 23:05：统一全站所有业务模块与建设中占位页（任务中心、项目管理等）的页头（Page Header）图标视觉规范，全面替换为 `size="lg"` 的彩色语义容器（`SemanticIconTile`），消除跨模块切换时的图标尺寸不一致与单调感。
- 2026-06-07 22:45：团队列表卡片设计引入高级玻璃拟物风格（Premium Glassmorphism），增加底层光晕微渐变和悬浮交互呼吸光效。同时完成数据库、OpenAPI及Go服务层的联席负责人（多负责人）底层改造，并在前端支持头像堆叠组件（AvatarStack），全面打通真实数字员工头像资产数据渲染链路。
- 2026-06-07 18:07：优化团队管理列表界面视觉：卡片在大屏设备上由 3 列变更为 4 列布局，提升宽屏空间利用率；顶部搜索及筛选工具栏重构，采用 Flex 悬浮条样式和项目的原生 `Select` 下拉组件替代原生 `<select>` 标签，使整体界面风格更现代、统一。
- 2026-06-07 16:15：重构团队管理列表页面为响应式卡片网格布局，替代原有的表格视图。新版卡片顶部高亮人类负责人，底部引入数字员工头像堆叠展示设计，并提供全局团队/Agent 规模统计。
- 2026-06-07 14:44：修复团队列表、团队详情数字员工入口和用户管理页内部跳转使用原生 `<a href>` 导致 Web Shell 与左侧菜单整页刷新的问题，统一改用 TanStack Router `Link`，并让侧栏菜单在详情/创建子路由保持父级选中态。
- 2026-06-07 02:28：Runtime Agent 工作目录从按 task_id 隔离重构为按 execution_instance_id + run_id 隔离，支持 logs/workspace/artifacts 子目录结构；旧版 create_task_workspace 标记为 deprecated。
- 2026-06-07 02:28：Runtime Agent 添加 aws-sdk-s3 依赖和 S3Section 配置支持，为工件上传到 S3 兼容存储做准备。
- 2026-06-07 02:28：ProviderEvent::SessionStarted 扩展支持可选的 session_state 字段，为 Provider 会话状态回写 Control Plane 奠定基础。
- 2026-06-07 02:28：重新设计数字员工列表工作台，新增 Control Plane overview read model，支持指标、筛选、执行端、最近运行、治理摘要和预算摘要展示。
- 2026-06-06 20:24：修复创建数字员工时专业类型默认能力和上下文未按团队治理策略收敛导致 effective config blocking validation 的问题，并避免创建页默认选择 disabled 团队。
- 2026-06-06 14:44：精简 Web 技能市场操作区，移除顶部“同步市场”和市场右侧“技能详情”上传卡片，技能市场卡片在桌面宽度改为三列展示。
- 2026-06-06 01:02：修复 Web 全局搜索按钮在移动端 Header 中按长占位文案撑破视口的问题，保证数字员工创建页移动端视觉验证不出现业务 Header 溢出。
- 2026-06-06 00:38：完成数字员工创建闭环，实现 Owner 归属、专业类型注册表、创建候选接口、ready 创建编排、前端四步创建向导、OpenAPI 与数据库迁移。
- 2026-06-05 18:05：优化 `AGENTS.md` 协作宪法，将项目、项目协调员、数字员工和人类归属人的规则收敛为宪法级边界；明确 Project 是场景中立的业务闭环容器，不定义封闭项目类型枚举，详细对象和流程规则保留在项目协调员控制平面设计文档中。
- 2026-06-05 11:12：补齐 Runtime command writeback 与数字员工 run API 的 OpenAPI/Go route/Rust client/TypeScript client 契约守卫，合并后新增接口会纳入 `verify:contracts` 门禁。
- 2026-06-05 10:52：合并后真实验证数字员工执行闭环，补齐 Runtime `provision_instance`、Provider 事件与终态 command writeback、`start_session` 输入 payload、Provider 能力真实探测和 `provider-run/v1` 任务隔离，避免 legacy polling 抢占数字员工 run。
- 2026-06-05 02:15：数字员工详情页接入执行闭环，支持开始任务、活跃运行轮询、事件流、结果、失败原因、历史运行切换和停止操作；Control Plane run events 改为读取真实 `task_events` 时间线，并补齐 run OpenAPI 契约。
- 2026-06-05 01:44：数字员工 Runtime provisioning preflight 改为严格执行团队 Provider 与 Runtime allowlist，空或缺失 allowlist 不再默认放行，并补齐真实 SQL 查询测试覆盖策略拒绝与 enrollment 撤销。
- 2026-06-05 01:37：数字员工 Runtime provisioning 创建链路补齐独立清理上下文、团队 Provider/Runtime 策略准入、Runtime session/enrollment 一致校验和 provisioning command payload 脱敏。
- 2026-06-05 01:18：数字员工创建改为 Runtime provisioning 主链路，创建时要求 Runtime 节点和 Provider 类型，完成 DB preflight、WebSocket 在线校验、`provision_instance` command 下发、receipt 等待与失败清理，避免 Web 看到半成品执行实例。
- 2026-06-05 01:04：Runtime command terminal 重放修复 task event 的 `raw_event_ref` 与 `log_ref` 投影，确保最小重放请求也使用已持久化 run 的结果与日志引用。
- 2026-06-05 01:00：Runtime command writeback 继续收紧终态重放：Runtime 事件 metadata 写入前脱敏，同状态 terminal 重放改用已持久化 run 事实修补 receipt、task event 和 Provider projection，并拒绝冲突的结果投影。
- 2026-06-05 00:52：Runtime command writeback 改为校验已认证 Runtime 节点身份，终态与 provisioning 回写在事务内锁定 command receipt 并修补重放投影，同时合并 Provider session patch 并脱敏 Runtime 来源的结构化敏感字段。
- 2026-06-05 00:30：Runtime command writeback 补齐数字员工执行实例 provisioning 成功/失败的 ready 与清理状态回写，并允许终态 run 重放已持久化事件序号保持幂等。
- 2026-06-05 00:08：数字员工 run Console API 分页参数改为按 int32 范围解析，避免超大 `offset` 在下传查询前发生整数截断。
- 2026-06-05 00:03：数字员工 run Console API 移除响应中的内部 `idempotency_fingerprint`，补齐列表与事件分页参数校验和默认值，并将 Runtime dispatch 失败统一映射为运行时不可用。
- 2026-06-04 14:49：Runtime Agent 删除旧版 `auth_token` 配置兼容入口，YAML、`RUNTIME_AGENT_AUTH_TOKEN` 环境变量和 CLI `--auth-token` 不再作为 `bootstrap_key` 兜底。
- 2026-06-04 14:33：数字员工 API 授权从复用 `runtime_scope.manage` 改为独立 `employee.*` 业务 action，并按 tenant 集合资源和单个 employee 资源记录授权决策，为后续 OpenFGA 渐进接入保留稳定边界。
- 2026-06-04 11:00：Web `Main` 布局组件改为默认铺满内容区，并新增 `contained` 窄版选项，后续控制台页面无需逐页传 `fluid` 即可复用权限中心式全宽布局。
- 2026-06-03 23:15：`AGENTS.md` 明确要求每条新增 `CHANGELOG.md` 变更记录带具体时间，默认使用本地 `Asia/Shanghai` 时间。
- Web 设计系统收敛为浅色液态玻璃控制台方向，`DESIGN.md` 明确蓝绿色主强调、液态玻璃卡片、胶囊按钮、语义图标和“只迁移 UI/UX 样式、不带入示例业务内容”的规则。
- Web 登录页、认证后 Shell 和工作台首页按浅色液态玻璃设计语言更新，统一按钮、卡片、背景、导航激活态、搜索框和输入控件质感。
- `DESIGN.md` 补充页面级和模块级 Tab 设计规则，明确玻璃胶囊激活态、底边流光锚点、中文长标签适配和 SuperTeam 组合组件复用边界。
- Web 主按钮和侧栏激活菜单降低左侧白色反光强度，避免高光压住图标和中文文字。
- Web Shell 背景色彩调整为参考图方向的淡青绿、暖米色和冷绿低饱和 wash，并将认证后控制台背景收敛为单一 `sidebar-wrapper` 背景源；顶部栏和内容容器保持透明，不再在刷新完成后覆盖外层背景，侧栏面板补充柔和 tint 并弱化与主页面之间的深色分割线。
- Web 侧栏 icon 折叠态收紧内容区内边距并居中导航按钮，修复图标贴近右侧边界的问题。
- Web 侧栏菜单默认字号从 14px 提升到 16px，并将菜单行高调高到 44px，提升导航可读性。
- Web 侧栏选中菜单改为清晰白色文字和图标，移除文字阴影并避免默认 active utility 覆盖导致文字发暗发糊。
- Web 暗色主题补充独立玻璃覆盖，修复左侧菜单、卡片和图标容器复用浅色白色高光导致的刺眼与可读性问题。
- Web 登录页暗色主题补充独立 Auth Shell、品牌标识和登录卡片覆盖，避免浅色背景与暗色文字 token 混用。
- Control Plane 初始数据库 schema 重写为 UUID-first 形态，合并早期 auth session、Web 登录日志、操作日志和中文注释迁移，并新增默认租户/团队骨架以支撑后续分布式与多团队数字员工管理。
- 任务、执行、审计、工件、Web 登录日志和操作日志改为应用层校验的 UUID 引用，避免跨模块重 FK 和级联删除；任务、工件、用户、Runtime 节点等核心实体补充软删除、禁用、归档、取消或终止时间戳。
- Runtime 任务状态机允许已领取任务直接进入 completed/failed，修复当前 claim -> events -> complete/fail HTTP 合约没有单独 running 接口时的完成链路阻断。
- 将数据库表设计规则从 `AGENTS.md` 沉淀到根目录 `DATABASE_DESIGN.md`，统一后续 UUID-first、租户/团队、索引、迁移、sqlc 与 OpenAPI 设计规范。
- 数据库迁移规范明确普通功能开发必须新增 forward migration，禁止回写已存在于 `atlas.sum` 的迁移文件，并将 rebuild-only 限定为需当次确认和写明备份、重建、验证命令的例外流程。
- Control Plane 请求日志新增 `remote`、`ua` 和 `referer` 字段，便于定位未知请求来源。
- Control Plane 与 Runtime Agent 本地开发配置统一收敛为 YAML 文件：Control Plane 使用被 Git 忽略的 `apps/control-plane/config/config.yaml`，Runtime Agent 使用被 Git 忽略的 `apps/runtime-agent/config.yaml`；Runtime Agent 示例配置从 TOML 切换为 `config.example.yaml`，并移除 Control Plane / Runtime Agent 的 `.env.example` 示例入口。
- Web 与 Control Plane 示例配置同步本地默认端口到 `8081`，避免前端示例 API 地址和后端示例监听端口不一致导致开发登录误报为用户名或密码错误。
- Control Plane 本地开发脚本默认加载 `apps/control-plane/config/config.yaml`，并兼容 `pnpm dev:control-plane -- --config ...` 的参数传递形式；配置入口统一以 YAML 文件为准。
- Web 控制台从旧 Next.js + 前端 workspace packages 结构激进重铺为 Vite + TanStack Router + shadcn-admin 单应用结构；前端 API client、认证状态、页面和 UI 组件集中到 `apps/web/src`，后端 Control Plane API 契约保持不变。
- Web 控制台移除 shadcn-admin demo 路由和 mock 数据页面，改为 SuperTeam 工作台、用户管理和任务/审批/审计等领域入口。
- Web shadcn-admin 路由接入 `AuthProvider`，认证守卫统一未登录跳转 `/login`，并将登出流程切换为 Control Plane session logout。
- Web shadcn-admin 登录表单从 mock cookie/token store 切换为 Control Plane cookie session auth，新增 `AuthProvider` / `useAuth` 负责加载当前用户、登录、登出和窗口聚焦后的会话刷新。
- 将一键验收脚本扩展为开发门禁入口：`pnpm verify:foundation` 现在聚合契约、TypeScript、Go 和 Rust 基础验证，并新增 `verify:web`、`verify:control-plane`、`verify:runtime-agent`、`verify:db` 领域门禁。
- 在 `docs/development.md` 中新增“开发验证门禁”，定义基础门禁、领域门禁、场景 smoke 和后续功能开发时的动态更新规则。
- Web 控制台在 Vite 环境下使用 `VITE_CONTROL_PLANE_URL` 配置 Control Plane 地址；未显式配置时继续跟随当前浏览器 host 推导，避免本地开发时 `127.0.0.1` 与 `localhost` 混用导致登录 Cookie 不被后续请求携带。
- 调整 Control Plane storage sqlc 查询集成测试：
  - 移除 testcontainers 本机容器 fallback，避免完整 Go 测试依赖 Docker/Podman。
  - 测试仅在显式配置 `TEST_DATABASE_URL` 和 `TEST_REDIS_URL` 时连接远端或专用测试环境运行；也支持通过 `ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1` 复用 `DATABASE_URL` 和 `REDIS_URL`。
  - 未配置测试环境时跳过 `apps/control-plane/internal/storage/queries` 集成测试。
- Provider 会话事件新增关联 ID 约束，要求 `request_id` 或 `command_id` 至少填写一个，避免无法回溯到平台请求或命令的事件进入审计链路。
- 数字员工执行链路补强 Runtime 与 Provider 可用性校验：执行实例绑定前要求 Runtime 已批准且 Provider 能力可用，Provider 会话和事件写入会拒绝禁用、错误或失效的执行上下文。
- Provider 会话事件追加时重新校验会话、数字员工与执行实例仍处于可接收事件状态，防止关闭、禁用或错误的执行上下文继续写入事件流。
- Provider 会话创建与事件追加只允许绑定 `ready` / `active` 执行实例，防止 `provisioning` 等未就绪状态进入 Provider 执行链路。
- Runtime Enrollment 和 Runtime Session 查询补强租户、Bootstrap Key、批准状态与撤销边界校验，移除可绕过执行前置条件的数字员工执行实例直接插入查询。
- Runtime hello 接入写入固定为 pending 状态，并要求有效 Bootstrap Key 与 Runtime 节点外部 ID 匹配，防止 hello 绕过人工审批或错绑节点。
- Runtime Agent 启动链路从旧版长生命周期 runtime token 注册切换为 bootstrap key hello 接入，批准后使用短期 runtime session token 访问 heartbeat、任务领取和事件回传接口，并按真实 Control Plane contract 上报扁平 capabilities。
- Runtime Agent 建立短期 session 后会主动连接 Control Plane Runtime WebSocket 命令通道，并支持接收 `ensure_instance` 命令创建数字员工执行实例目录。

### Fixed

- 2026-06-15 22:47：修复项目需求到多数字员工执行闭环：heuristic planner 可按策略 fan-out 到所有 active executor 且不触发人工审核，ProjectTask 提示不再暴露内部写回机制，Runtime Agent 支持 Control Plane 轻量 `stop_session` payload 取消活跃 run 并回写 cancelled 终态，Claude Code Provider 默认使用 Runtime 治理的非交互权限模式，避免真实执行链路卡在人类审批或 Provider 权限提示。
- 2026-06-15 18:24：修复项目任务发起到 task-graph 的真实链路：提交需求和 workflow signal 重试前会幂等确保 Temporal 项目协调 workflow 存在，DeepSeek planner 请求增加短超时并在外部 planner 超时后回退到 heuristic，避免 `PlanDemandRoute` activity 被 30 秒 StartToClose timeout 打断而无法持久化任务图。
- 2026-06-14 10:32：修复 Runtime command terminal event 与 active-run registry 清理之间的竞态，Provider turn 已完成后立即移除活跃 run，避免 `stop_session` 在短窗口内误取消已完成执行。
- 2026-06-13 23:28：修复 Runtime Agent 数字员工工作目录初始化时 `CLAUDE.md` 被写成普通副本的问题，改为在 Unix 环境下生成指向 `AGENTS.md` 的兼容软链，并覆盖已有旧副本，保持 Claude Code 与 OpenCode 的工作目录只创建对应 Provider 配置目录。
- 2026-06-13 13:21：修复数字员工详情页发起测试任务时被 stale 活跃运行卡住的问题；Control Plane 会根据终态 Runtime command receipt 回收仍显示为 queued/dispatching/running/cancelling 的旧 run，列表返回终态状态，新建 run 也可在同次请求内继续分派。
- 2026-06-13 13:38：修复数字员工显示 Ready 但 Runtime 命令通道未连接时仍可发起测试任务的问题；Runtime overview 现在返回实时 command channel 连接状态，数字员工详情页会在通道断开时禁用开始任务并给出原因。

### Added

- 2026-06-06 09:44：完成技能管理主链路，新增 Control Plane 技能包、文件、团队归属和 Agent 安装绑定 API，Web 技能页支持已安装技能树、Monaco 文件编辑、技能市场上传标签展示和彩色图标，以及 zip 弹窗上传并绑定多个团队。
- 2026-06-05 21:55：新增数字员工创建闭环设计规格，明确专业执行型数字员工创建为 `ready` 准备态，引入 `owner_user_id`、服务端专业类型注册表、创建候选接口和分步创建向导；项目协调员不进入数字员工创建模型。
- 2026-06-05 18:35：Web 控制台在“平台管理”下新增“成本管理”入口和 `/costs` 占位页，预留按数字员工查看 token 消耗成本以及每日、每月 token 成本统计视图，后续实现参考 `paperclip` 的 costs 模块。
- 2026-06-05 16:22：为项目协调员控制平面设计补充 gpt-image2 生成的架构流程图和项目内数据流转图，便于对齐 Project、Coordinator、执行员工、Runtime、Provider、证据和人类决策关系。
- 2026-06-05 16:06：新增项目协调员控制平面全局设计总结，明确一个项目一个协调员数字员工、人类归属人、RouteDecision/RouteOutcome、预算、置信度扩散、证据链和人类驳回机制。
- 2026-06-05 16:12：Runtime 节点页升级为总览工作台，新增 Runtime overview/events Console API、`runtime_events` 统一事件流、Provider 能力聚合、接入批准/拒绝操作和事件审计 Tab；本期不包含 Runtime 详情页、接入密钥和诊断包功能。
- 2026-06-05 01:00：Web 控制台新增“技能管理”“项目管理”和“协作集成”一级菜单及占位页，其中“协作集成”用于后续接入钉钉、飞书等企业通讯软件的消息交互、审批触达和结果通知。
- 2026-06-05 00:24：Control Plane 新增 Runtime command HTTP writeback API，Runtime session auth 可回写 provider 事件、完成、失败、取消和超时终态，并将事实持久化到 command receipt、task run/event 与 Provider session/event 双投影。
- 2026-06-04 23:48：Control Plane 暴露数字员工 run Console API 路由，支持创建、列表、详情、事件查询和停止，并接入独立 `employee.run.*` 授权 action 与真实 app wiring。
- 2026-06-04 14:28：新增 `scripts/dev-services.sh` 本地开发服务管理脚本，支持 Control Plane、Web 和 Runtime Agent 的状态检查、启动、停止和重启，并以 `.scratch/dev-services` 记录 PID 与日志；新增脚本级测试覆盖启停与重启流程。
- 2026-06-04 10:38：用户管理页按“用户 360 详情台”方向升级为主从详情工作台，使用权限中心一致的铺满页面布局，并调整为更宽的用户列表和三等宽概览卡片；页面接入用户列表、权限中心成员角色、登录日志和授权拒绝记录，并补齐新建用户、启用/禁用账号和重置密码入口。
- 2026-06-04 05:20：团队列表搜索补齐负责人用户名、显示名和邮箱匹配，并修复新建团队失败后关闭重开抽屉仍显示旧错误的问题。
- 2026-06-04 05:02：团队管理体验补齐团队图标、用户头像身份展示、弱分页、目标化两步创建抽屉和详情成员页用户搜索，继续沿用浅色液态玻璃企业控制台设计风格。
- 2026-06-04 04:02：Runtime Agent 新增 runtime command execution layer，支持 `start_session`、`resume_session`、`send_input`、`stop_session` 在本地解析 payload、创建执行实例目录、驱动 Provider run、维护 command/session/run 映射并取消 active run；Runtime 仅执行本地命令，不判断租户、团队或审批。
- 2026-06-04 02:29：团队接口头像回显兼容历史用户空头像种子，按用户名生成稳定 fallback，并修复团队 metadata display 规范化时修改调用方输入的问题。
- 2026-06-04 01:49：用户管理补齐 DiceBear adventurer 头像配置字段、OpenAPI 响应和 Web 列表展示；Control Plane 存储头像来源、样式、种子和扩展选项，前端使用本地 DiceBear JS 包生成稳定 SVG 头像。

#### 团队管理权限底座 (2026-06-03)

- Control Plane `authz` 新增 OpenFGA-ready 团队管理 action 语义，覆盖团队 CRUD、禁用/归档/恢复、成员增删改、特权角色申请/批准、治理配置读写/批准、能力绑定和团队审计读取。
- `DBAuthorizer` 新增团队管理授权矩阵：租户 owner/admin 可创建并管理所有团队；团队 owner/admin 可维护本团队基础信息和普通成员；团队 owner 或 approver 可批准治理配置；直接添加/提升 owner/admin/approver 会要求走特权角色申请/批准语义，并保留最后团队负责人保护。
- 团队 API 路由停止复用 `runtime_scope.manage`，改为按路由发送 `team.create`、`team.read`、`team.governance.edit`、`team.governance.approve` 和 `team.governance.read` 等业务 action。
- 权限中心诊断契约和 Web 表单补齐团队管理 action 枚举，能按 action 自动派生 tenant/team resource，为后续 OpenFGA Authorizer 或 tuple 同步保留稳定 actor/action/resource 边界。

#### Web 数字员工创建流程 (2026-06-03)

- 数字员工页面新增创建草稿员工并预览生效配置流程，可基于团队当前治理配置生成个人配置修订，成功预览后提示可提交负责人确认，阻断错误或预览失败时给出页面反馈。

#### Web 团队管理页面 (2026-06-03)

- 新增团队管理控制台基础计划的列表摘要、详情概览、生命周期操作和前端详情框架。
- 新增团队成员管理、普通角色直接变更和高权限角色申请审批流程。
- Web 控制台新增“团队管理”侧栏入口和 `/teams` 页面，可查看团队负责人、当前治理配置修订号、宪法硬性规则、能力边界和内部协作自动轮次；当前治理配置缺失或加载失败时按单行降级提示，不阻断团队列表展示。
- 新增团队治理草稿、能力与知识绑定、治理策略编辑和批准生效流程。
- 新增团队详情中的数字员工入口和团队管理审计记录。
- 2026-06-03 23:29：团队列表 API 新增 `governance_status` 筛选和负责人摘要响应，前端后续可按设计稿展示负责人姓名/邮箱并区分草案待批准、已生效、未配置等治理状态。
- 2026-06-04 00:52：团队管理页补齐真实筛选工具条和右侧两步新建团队抽屉，创建团队时通过 Control Plane 事务一次性写入团队、负责人和初始成员，并以真实接口刷新列表。
- 2026-06-04 01:08：新建团队抽屉将负责人选择改为独立搜索，避免团队名称和负责人候选查询耦合，补齐加载、空态和选中态反馈。

#### 团队治理后端 (2026-06-03)

- Control Plane 新增租户团队领域服务、PostgreSQL repository 和 API 路由，支持团队负责人、共享治理配置版本创建与当前版本查询；相关路由由 Web 控制台会话认证保护，并统一经过 `team.*` 业务授权校验。
- 数字员工服务新增个人配置版本、生效配置预览与校验、审批落库能力；预览会阻断团队能力白名单外的个人能力、团队上下文范围外的上下文覆盖以及降低团队审批要求的个人审批覆盖。
- 数字员工创建新增同租户团队存在性校验；生效配置预览与审批只接受 active 团队治理配置版本，避免 draft 配置绕过团队负责人确认。
- 团队治理配置版本创建不再接受客户端指定 `approved_by`，审批归属由当前 Web 控制台登录用户在服务端注入。
- 新增 `003_add_team_governance_config.sql` forward migration，补齐已执行旧版 001/002 的远端库中的团队治理配置、数字员工个人配置和生效配置快照表。
- 数字员工生效配置审批要求已有 ready 或 active 的唯一执行实例，并新增 `/api/v1/digital-employees/{employeeId}/config-revisions`、`/effective-configs/preview`、`/effective-configs/approve` API 路由。
- OpenAPI 与基础契约守卫补齐团队治理、数字员工配置版本和生效配置预览/审批 API，确保新增后端路由纳入契约核验。
- Web API client 新增团队列表、团队创建、团队治理配置版本创建与当前版本查询能力，并补齐数字员工个人配置版本、生效配置预览和审批调用入口。

#### Web 液态玻璃组件化 (2026-06-03)

- 新增 `apps/web/src/components/superteam` 项目级设计组件层，沉淀 `LiquidCard`、`LiquidPill`、`PrimaryLiquidButton`、`SemanticIconTile`、`StatusBadge` 和 `MetricCard`。
- 新增 `LiquidTabsList` 和 `LiquidTabsTrigger` 复用组件，将页面 Tab 统一为玻璃胶囊选中态与底边流光指示，并迁移权限中心和团队详情页。
- 工作台首页指标卡改为复用 `MetricCard`，减少页面内手写玻璃卡片、语义图标和状态胶囊样式。
- 为液态玻璃设计组件补充 Vitest 浏览器组件测试，锁定组件 slot、核心 class 和基础渲染行为。

#### 数字员工后端与执行实例服务 (2026-06-02)

- 新增 Control Plane 数字员工领域服务、PostgreSQL repository 和 HTTP handler，支持创建草稿、列表、详情、状态更新以及唯一执行实例查询和绑定。
- 数字员工执行实例绑定复用现有 sqlc upsert 查询，默认绑定为 ready 状态，并通过 Web 控制台 cookie session 注入租户上下文保护 `/api/v1/digital-employees` 路由。
- Web 控制台新增数字员工页面和 API client，支持查看数字员工业务身份、状态、风险等级和唯一执行实例绑定信息。

#### Runtime 接入与短期会话服务 (2026-06-02)

- 新增 Control Plane Runtime Enrollment / Runtime Session 领域服务，支持 Runtime hello 接入、人工批准/拒绝/撤销、短期 session token 签发、校验与续期。
- Runtime session token 采用确定性 lookup hash 加 bcrypt secret hash 的双哈希模型，避免原始 token 或可直接校验的单一 hash 明文落库。
- Runtime hello 会扫描有效 Bootstrap Key 并用 bcrypt 校验原始 secret；pending 接入不会返回 session，approved 且已挂接 Runtime 节点的接入才签发短期 session。
- Runtime enrollment 撤销会使关联 active session 失效，session 校验与续期会重新检查接入仍处于 approved 且未撤销状态。
- 接通 Runtime Enrollment、短期 Session 续期与 Capability 上报 HTTP 路由；`/api/v1/runtime/enrollments/hello` 支持公开 bootstrap hello，`/api/v1/runtime/session/renew` 和能力上报要求短期 session token。
- 新增 Runtime session middleware，并让 heartbeat、claim、task event、complete、fail 和 lease 路由在迁移期同时接受短期 session token 或旧版 `Authorization + X-Node-ID` runtime token。
- Runtime enrollment 管理路由改为 Web 用户 cookie session 保护，并将 canonical session renew 与 capability upsert 成功响应对齐 OpenAPI 契约。
- Runtime enrollment 管理路由补充 `runtime_scope.manage` 授权校验，并将当前 Web 用户与租户上下文传入批准、拒绝和撤销操作。
- 修正 Runtime 接入服务的多租户路径：hello 阶段不再创建默认租户 Runtime 节点，改为仅写入 pending enrollment；批准阶段按租户创建或复用 Runtime 节点并 attach，session 校验改为按全局 lookup hash 查找，支持非默认租户续期。
- 将 Runtime enrollment 批准改为单条 SQL 原子完成 pending 校验、tenant-safe Runtime 节点 upsert 和 enrollment attach，避免并发拒绝/撤销后留下未挂接节点，并修复 tenant-aware node upsert 的全局 `node_id` 冲突竞态。
- 将 Runtime session token 默认有效期修正为 12 小时，并补充签发和续期过期时间断言。
- 新增 Runtime WebSocket Command Channel：Control Plane 维护 Runtime 连接注册表，短期 session 认证的 Runtime 可通过 `/api/v1/runtime/ws` 建立命令通道并接收 JSON command；旧版 Runtime token 不能访问该通道。
- 加固 Runtime WebSocket Command Channel 连接注册表：Dispatch 不再持有全局锁等待满载 channel，Runtime 重连和断开清理不会被阻塞，并补充 registry 未配置与 client close 后注销测试。
- Web 控制台新增 Runtime 节点页面，支持查看待接入 Runtime enrollment、批准接入请求以及查看已接入 Runtime 节点状态和 Provider 能力。

#### Control Plane 渐进式授权边界 (2026-06-01)

- 新增 Control Plane 渐进式授权边界：`internal/authz` 统一 `Authorizer` 接口，第一版使用 PostgreSQL 权限事实判断 Web 控制台访问和 Runtime claim 范围。
- `/api/auth/me` 登录后增加 `console.access` 授权检查，认证和授权保持分层。
- Runtime claim 任务前增加 `task.claim` 范围检查，Runtime 节点不能领取超出 `runtime_node_scopes` 的任务。
- 授权决策接入 `web_operation_logs`，记录允许/拒绝结果、授权引擎、命中规则、Actor、资源和租户/团队上下文，为后续权限审计视图和 OpenFGA backend 留出稳定审计底座。

#### 权限中心 MVP (2026-06-01)

- 新增 Control Plane 权限中心 API 契约和 `internal/authzcenter` 应用服务层，提供授权概览、授权审计、Runtime 范围、成员角色只读视图和权限诊断接口。
- 新增 Web 一级菜单“权限中心”和 `/permissions` 页面，包含“授权概览”“授权审计”“Runtime 范围”“成员角色”“权限诊断”五个 Tab。
- Runtime 范围管理支持新增租户/团队 scope 以及启用、禁用已有 scope；Web 表单按租户或团队自动派生只读范围值，避免提交不符合后端约束的 payload。
- Runtime scope 写操作统一经过 `runtime_scope.manage` 授权检查并写入 `web_operation_logs`，权限中心读接口通过 `authz_center.read` 做租户边界控制。
- 权限诊断通过统一 `Authorizer.Check` dry-run 返回授权结果，并按 action 自动匹配资源类型与必要字段校验。
- Web 权限中心 API client 与页面补充 Vitest 覆盖，锁定请求方法、请求体、Runtime scope 写入确认和诊断行为。

#### Web Vite 控制台重铺 (2026-05-31)

- 新 Web 壳接入 `shadcn-admin` 的侧边栏、顶部栏、主题、命令面板和响应式布局。
- 保留真实 Control Plane 登录、当前用户、退出登录和路由保护主链路，继续使用 cookie session 与 `credentials: "include"`。
- 新增 Vite 环境变量 `VITE_CONTROL_PLANE_URL`，保留本地 `localhost` / `127.0.0.1` host 对齐策略。
- 删除不再服务 Web 的旧 UI、views、core 和 api-client 前端 workspace 拆分。

#### Web 外部能力占位入口 (2026-05-31)

- 新增 Web 控制台一级菜单“外部能力”及 `/capabilities` 占位页。
- 页面先说明后续外部能力扩展范围，包括 Dify Workflow、Deephub Agent、企业内部 HTTP 接口、数据分析服务、ITSM、CMDB/监控/日志平台、自研脚本服务和 MCP Server/Connector。
- `/capabilities` 当前作为公开占位说明页，不要求登录态，避免点击一级菜单后被全局登录守卫重定向到登录页。
- 为工作台、任务中心、数字员工、流程编排、审批中心和审计日志补齐公开占位页；除用户管理这类真实管理功能外，未开发一级菜单不再跳转登录页。

#### Web 平台 Shell 完善 (2026-05-31)

- 新增路由驱动的 `ConsoleAppShell`，统一 Web 控制台菜单 active、面包屑、登录用户展示和登出入口。
- `ConsoleShell` 支持面包屑渲染，并保留业务页面通过插槽注入页面操作。
- Web 控制台新增平台通用 empty、loading、error、forbidden 状态组件，用户管理页接入列表加载、空数据和加载失败状态。
- 新增 Web 控制台 `not-found`、`error`、`loading` 和 `/forbidden` 页面，先形成公共异常与权限不足入口。

#### Web 用户管理 MVP (2026-05-31)

- 新增 Web 控制台用户管理页 `/users`，支持用户列表、创建用户、启用/禁用用户和重置密码。
- Control Plane Auth API 新增用户管理接口：`GET/POST /api/auth/users`、`PATCH /api/auth/users/{id}/status`、`POST /api/auth/users/{id}/reset-password`。
- 用户管理写操作接入 `web_operation_logs`，记录创建用户、启用/禁用用户和重置密码操作。
- `apps/web/src/lib/api` 新增用户管理 client 方法，并将认证用户 ID 对齐为后端用户主键 `int64`。
- 新增迁移为 `auth_users` 与 `web_operation_logs` 补充中文表注释和字段注释。

#### Web 会话闭环体验 (2026-05-31)

- Web 控制台 Shell 新增右上角用户菜单插槽，支持展示当前登录用户和账户操作。
- 首页接入 `useAuth()` 当前用户状态，移除顶部账户区域的硬编码用户展示，并提供退出登录操作。
- Web 认证状态在窗口重新聚焦时复查 `/api/auth/me`，遇到 401 会清空当前用户并交由认证守卫回到登录页。
- 前端 API client 增加带 HTTP status 的 `ApiRequestError`，供前端认证层稳定识别会话失效。

#### Web 登录主链路 (2026-05-31)

- 接入 Web 控制台登录、当前用户和登出 API，使用 `auth_sessions` 持久化浏览器会话。
- 在 `apps/web/src` 内新增 auth client、AuthProvider/useAuth 和登录页。
- Web 根布局接入认证守卫，未登录用户进入 `/login`，登录成功后返回控制台首页。
- 将浏览器 session token 以 SHA-256 hash 写入 `auth_sessions.token_hash`，避免原始 cookie token 明文入库。
- 收紧 Control Plane CORS，使携带 cookie 的本地 Web 调用只允许 `localhost:3000` 和 `127.0.0.1:3000`。
- 新增幂等开发账号迁移，方便本地使用 `admin / admin` 完成 Web 登录烟测。

#### Web 登录日志与操作日志底座 (2026-05-31)

- 新增 `web_login_logs` 表，独立记录 Web 控制台登录成功、登录失败和登出事件，不复用人工审核相关的 `audit_events`。
- 新增 `web_operation_logs` 表，预留 Web 控制台后续功能操作日志、资源操作结果和请求上下文记录。
- 登录、登录失败和登出链路接入 `web_login_logs` 写入；日志写入异常不阻断主登录链路。
- 新增 `GET /api/auth/login-logs` 查询接口，要求有效登录 Cookie，并支持 `limit` / `offset` 分页参数。
- 前端 API client 新增 `listLoginLogs`，供 Web 端调用登录日志接口。

#### Core Summary 状态映射 (2026-05-30)

- 为任务和 Runtime 节点 summary helper 增加稳定状态 tone，供后续 Web 页面复用。
- 为 Runtime 节点 summary 增加负载百分比，避免每个页面重复计算槽位占用。

#### API Client 最小任务与 Runtime 覆盖 (2026-05-30)

- 为前端 API client 补齐任务详情、任务状态更新、任务取消和 Runtime 节点详情的最小 client 方法。
- 通过 Vitest 锁定这些方法使用的 Control Plane canonical path。

#### Foundation 契约漂移检查 (2026-05-30)

- 新增 `pnpm verify:contracts`，检查 Control Plane OpenAPI、Go route、Rust Control Plane client 和 TypeScript api-client 的关键路径一致性。
- 新增 `pnpm verify:foundation`，聚合契约检查、TypeScript 测试、TypeScript 类型检查和 Runtime Agent Rust 测试。

#### Foundation Readiness 底座收口设计 (2026-05-30)

- 新增 Foundation Readiness 设计文档，明确在进入具体功能开发前采用 Web、Control Plane、Runtime Agent、contracts 与共享 packages 的均衡收口方案。
- 定义本阶段的维护性、可扩展性、可复用性标准，并明确不提前实现登录认证、Temporal、OpenFGA、完整业务页面和生产级 Provider 治理。

#### Web 真实数据接入底座 (2026-05-29)

- 为任务和 Runtime 节点补充最小 API client 与 core summary helper，后续页面可从 mock 数据平滑切换到真实接口。

#### Foundation fake provider 端到端验收 (2026-05-29)

- 新增 fake provider 风格的最小端到端验收，覆盖任务创建、Runtime 注册、claim、事件回传和完成状态。
- 对齐 Runtime Agent 客户端的节点注册/心跳响应模型，移除未在 Control Plane contract 中承诺的内部数据库 `id` 依赖。
- 将 Control Plane Runtime 写入端点接入 Runtime token + `X-Node-ID` 认证，并让 Runtime Agent 对心跳、claim、事件、完成、失败和 lease 请求携带节点身份。
- 修正 Runtime token 生成脚本，使其写入当前 `auth_runtime_tokens(node_id, token_hash, expires_at)` schema。

#### Foundation Hardening 设计 Spec (2026-05-29)

- 新增 Foundation Hardening 设计文档，明确 Control Plane 启动边界、sqlc 生成闭环、契约事实源、Runtime Agent daemon 边界、执行事件流和 Web 真实数据接入底座。

#### Web 根布局 hydration 兼容 (2026-05-29)

- 在 Web 根布局 `<html>` 上启用 `suppressHydrationWarning`，降低浏览器扩展向根节点注入属性时触发的 hydration mismatch 噪音，并补充对应布局测试。

#### Web 控制台通用骨架 (2026-05-29)

- 沉淀 Web 控制台外部系统骨架复用组件：新增 `ConsoleShell`、状态胶囊、图标徽章、指标块、分区面板和时间线项，并将首页改为基于共享组件挂载。

#### Control Plane S3 对象存储接入 (2026-05-29)

- 使用 AWS SDK for Go v2 的 `config`、`credentials`、`service/s3` 初始化控制平面 S3 客户端。
- 新增 `S3ObjectStore` 边界，封装对象上传、下载、存在性检查和删除，并返回稳定的 `s3://bucket/key` 工件引用。
- 补齐 S3 配置校验，启动前检查 endpoint、region、bucket、access key 和 secret key。
- 更新配置模板和配置指南，保留 MinIO/localstack path-style 默认值，并补充 Volcengine TOS virtual-hosted 配置示例。

#### Runtime 任务执行结果 API (2026-05-29)

- 补齐 Runtime task events、complete、fail 和 lease endpoint 的基础处理，支持 Runtime Agent 回传结构化执行事件和终态。

#### Phase 4 - Runtime Agent Control Plane 集成 (2026-05-29)

- 添加 Runtime Agent Control Plane 客户端 (`apps/runtime-agent/src/controlplane/`)
  - client.rs: HTTP 客户端实现
    - ControlPlaneClient 结构：封装 reqwest HTTP 客户端
    - register(): 注册节点到 Control Plane
    - heartbeat(): 发送心跳更新节点状态和负载
    - claim_task(): 长轮询获取任务（支持超时）
    - 完整的错误处理和上下文信息
  - models.rs: API 模型定义
    - TaskStatus 枚举 (pending/claimed/running/completed/failed/cancelled)
    - NodeStatus 枚举 (online/offline)
    - RegisterNodeRequest/Response
    - HeartbeatRequest/Response
    - Task 模型（包含完整任务信息）
    - 所有模型支持 serde 序列化/反序列化
  - mod.rs: 模块导出
- 更新 Cargo.toml
  - 将 reqwest 从 dev-dependencies 移至 dependencies
  - 启用 json 和 rustls-tls 特性
- 添加集成测试 (`apps/runtime-agent/tests/controlplane_client_test.rs`)
  - 客户端创建测试
  - 请求序列化测试
  - 集成测试（需要运行的 Control Plane，默认 ignored）
    - 节点注册测试
    - 心跳更新测试
    - 任务 claim 超时测试
  - 所有单元测试通过

#### Phase 2.3 - 任务调度器 (2026-05-29)

- 添加任务调度器 (`apps/control-plane/internal/runtime/scheduler.go`)
  - Scheduler 结构：负责任务到节点的调度
  - SelectNode 方法：智能节点选择
    - 查询支持指定 Provider 且在线的节点
    - 过滤负载已满的节点 (current_load >= max_slots)
    - 选择负载最低的节点实现负载均衡
    - 自动更新节点 current_load
  - 错误处理：ErrNoAvailableNode
- 添加调度器测试 (`apps/control-plane/internal/runtime/scheduler_test.go`)
  - 单节点调度测试
  - 负载均衡测试（多节点选择最低负载）
  - Provider 过滤测试
  - 容量过滤测试（排除满载节点）
  - 无可用节点错误测试
  - 复杂场景测试（混合 Provider、负载、容量）
  - 11 个测试用例全部通过

#### Phase 2.2 - Runtime 服务 (2026-05-29)

- 添加 Runtime 节点管理服务 (`apps/control-plane/internal/runtime/`)
  - models.go: 领域模型定义
    - NodeStatus 枚举 (online/offline)
    - Node 模型及辅助方法 (IsOnline, HasCapacity, SupportsProvider)
    - RegisterNodeRequest, UpdateHeartbeatRequest 请求模型
    - ListNodesFilter 过滤器模型
    - pgtype 类型转换辅助函数
  - repository.go: 数据访问接口
    - CRUD 操作 (CreateNode, GetNode, ListNodes, UpdateHeartbeat, UpdateLoad, UpdateStatus, DeleteNode)
    - ListOnlineNodes: 查询心跳在阈值内的在线节点
  - service.go: 业务逻辑实现
    - RegisterNode: 注册新节点或更新已存在节点
    - UpdateHeartbeat: 更新心跳和负载，自动检测节点状态
    - GetNode: 根据 ID 查询节点
    - ListNodes: 列出节点，支持状态过滤和分页
    - ListOnlineNodes: 列出在线节点（60秒心跳阈值）
    - JSON 序列化支持 (providers, metadata)
  - service_test.go: 完整的单元测试
    - 使用 testify/mock 实现 MockRepository
    - 覆盖所有服务方法的正向和负向测试用例
    - 输入验证测试
    - 分页和限制测试
    - 15 个测试用例全部通过

#### Phase 2.1 - 任务服务 (2026-05-29)

- 添加任务管理服务 (`apps/control-plane/internal/task/`)
  - models.go: 任务领域模型
  - repository.go: 任务数据访问接口
  - state_machine.go: 任务状态机
  - service.go: 任务服务实现
  - service_test.go: 单元测试

#### Phase 1.3 - 数据层测试 (2026-05-29)

- 添加完整的数据层测试套件 (`apps/control-plane/internal/storage/queries/queries_test.go`)
  - Runtime 节点测试：创建、查询、心跳更新、在线节点列表
  - 任务测试：创建、查询、列表过滤、状态更新、状态转换、事件流
  - 认证测试：用户创建、查询、Runtime token 创建和验证
  - 审计测试：事件创建、列表查询、统计、时间过滤
- 添加 Runtime 节点查询 (`apps/control-plane/internal/storage/queries/runtime.sql`)
  - CreateRuntimeNode, GetRuntimeNode, UpdateRuntimeNodeHeartbeat
  - UpdateRuntimeNodeLoad, UpdateRuntimeNodeStatus
  - ListOnlineNodes, ListRuntimeNodes, DeleteRuntimeNode
- 添加认证查询 (`apps/control-plane/internal/storage/queries/auth.sql`)
  - CreateUser, GetUser, GetUserByUsername, GetUserByEmail
  - UpdateUser, ListUsers, DeleteUser
  - CreateRuntimeToken, GetRuntimeToken, ValidateRuntimeToken, DeleteRuntimeToken
- 添加测试辅助脚本 (`apps/control-plane/test.sh`)
  - 基于显式远端或专用测试环境变量运行 storage 查询集成测试
- 添加测试文档 (`apps/control-plane/internal/storage/queries/README.md`)
  - 测试覆盖说明
  - 运行指南
  - 故障排查
- 添加测试依赖
  - stretchr/testify (latest)

#### Phase 1.2 - 配置 sqlc (2026-05-29)

- 配置 sqlc 代码生成 (`apps/control-plane/sqlc.yaml`)
- 生成任务查询代码 (`apps/control-plane/internal/storage/queries/tasks.sql.go`)
- 生成审计查询代码 (`apps/control-plane/internal/storage/queries/audit.sql.go`)

#### Phase 1.1 - 数据库迁移 (2026-05-29)

- 初始数据库 schema (`apps/control-plane/internal/storage/migrations/001_initial.sql`)
  - Runtime 节点表 (runtime_nodes)
  - 认证表 (auth_users, auth_runtime_tokens)
  - 任务表 (tasks, task_executions, task_state_history, task_events, task_artifacts)
  - 审计表 (audit_events)
  - 索引和触发器

### Changed

#### Foundation Readiness 文档收口 (2026-05-30)

- 同步 README、开发指南、API 文档和下一步指引，明确底座阶段的启动、验证和契约守护边界。

#### Foundation 文档同步 (2026-05-29)

- 同步 README、开发指南、API 文档和下一步指引，使文档状态与已验证的 Foundation baseline 保持一致。

#### Runtime Agent daemon 默认语义 (2026-05-29)

- 将 Runtime Agent 正式运行边界收敛为受 Control Plane 管理的 daemon，并补充认证 token 配置、环境变量和 CLI 覆盖。

#### Runtime API 契约路径收敛 (2026-05-29)

- 将 Runtime 任务 claim、事件、完成、失败和 lease 续约路径统一收敛到 Control Plane 的 `/api/v1/runtime/tasks/...` canonical contract，并将 Runtime Agent 本地契约保留为诊断和本地 run API。

#### Control Plane 启动边界收敛 (2026-05-29)

- 收敛 Control Plane 主启动入口，明确 health-only router 与产品 API server 的边界，并通过统一装配路径连接存储、服务和 handlers。

- 将 Control Plane PostgreSQL 和 Redis 配置示例切换到 `doc/database/conn_info.md` 记录的远端地址，并修正连接验证命令。
- 在远端 PostgreSQL 创建 `superteam` 应用用户、数据库和 schema，并从本地 `127.0.0.1` 的 `superteam` 数据库迁移当前 schema 与迁移记录。

### Deprecated

### Removed

### Fixed

#### 项目需求启动详情分页前过滤 (2026-06-12 17:48)

- 修复需求启动详情读模型先按项目分页再内存过滤导致相关事实被较新无关记录截断的问题，改为在仓储查询层按需求、任务、决策和事件关联条件过滤后再限制返回数量。

#### Control Plane 迁移命令目录对齐 (2026-05-30)

- 修正 `apps/control-plane/Makefile` 的 Atlas 迁移目录，统一指向实际 schema 源 `internal/storage/migrations`。

#### Control Plane API 响应契约对齐 (2026-05-30)

- 为任务与 Runtime 节点 API 响应补充显式 DTO，统一输出 snake_case 字段，避免直接编码领域模型时泄漏 Go 字段名。
- 将任务响应中的 `params` 规范化为 JSON object；空值、无效 JSON 或非对象输入统一回退为 `{}`，避免返回 base64 字符串。
- 更新 API/e2e 测试，锁定 `create/get/list/update/cancel/claim/complete/fail` 等任务路径及 Runtime 节点路径的真实 JSON shape。
- 收敛 Runtime claim 到 canonical `/api/v1/runtime/tasks/claim`，移除旧别名路由，并同步 API/OpenAPI 文档对 complete 与 lease 当前能力边界的描述。

#### Runtime Agent 配置入口统一 (2026-05-29)

- 统一 Runtime Agent 配置模型，支持 `--config` 加载 TOML 配置文件。
- 将配置优先级收敛为：CLI 参数 > `RUNTIME_AGENT_*` 环境变量 > `config.toml` > 默认值。
- 同步 `.env.example`、`config.example.toml`、README、配置指南和 `dev:runtime-agent` 脚本，避免 `RUNTIME_NODE_ID` / `RUNTIME_AGENT_NODE_ID` 等命名漂移。
- 忽略本地真实配置 `apps/runtime-agent/config.toml` 和 `.superteam/` 运行状态目录，保留可提交示例配置。

### Security

#### 配置文件忽略规则收敛 (2026-05-29)

- 扩展 `.gitignore` 环境配置规则，忽略项目内真实 `.env*` 和 `config.yaml` 文件，仅保留 `.env.example`、`config.example.yaml` 等示例配置可同步。

## [0.1.0] - 2026-05-29

### Added
- 项目初始化
- 基础目录结构
- CLAUDE.md 项目文档

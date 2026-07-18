# 违反检测门 P1 follow-up 交接（review_gate 竞态 + 飞书/Web 实时刷新）

> 日期：2026-07-18
> 状态：**已完成**（2026-07-18 11:40 合并 main，三场景真实 E2E 全 PASS；实施记录见文末 §5）
> 背景：违反检测门 P1 已合并 main（66ea3ee9），真实 E2E 两确认 PASS（干净→默认放行；违反→held→人类签放行），但揪出一个真实竞态缺陷 + 另有一个飞书/Web 实时刷新缺口。本文精确捕获这两项 + P1 已知局限，供新会话直接开工。
> 相关 spec：`2026-07-17-review-gate-violation-detection-design.md`（含 §1 能守护/守护不了边界表、§7.1 P1 实际范围）。

---

## 0. review_gate P1 是什么（新会话背景）

审核/门控从"证明正确性"重构为"检测违反明确条件"：LLM 判官对开放任务"做对了没有"证不了伪（判官与执行者同类模型无独立真值）。门**不证明正确，只检测是否违反明确定义的条件**（规则型/LLM-prompt 型检测器，指向被审任务真工件）；**检出违反才 hold，检不出默认放行**，人类只在最终验收。已落地：`review_gate` verification_method + secret_leak 规则检测器 + code_review LLM 检测器 + coordination_policy 配置层 + 收敛闸对 review_gate 默认反转（仅 verdict=unsatisfied 才 held）+ 触发接线（`review-gate-trigger` GetVersion 围栏 → `RunReviewGateForTask` Activity）+ 人类签署白名单含 review_gate + executor 投影跳过 review_gate。关键文件：`apps/control-plane/internal/workflow/projectcoordination/{detector.go, detector_registry.go, review_gate.go, review_gate_trigger.go}`、`internal/project/demand_acceptance_gate.go`、迁移 073。

---

## 1.【承重·真实竞态缺陷】review_gate-only 需求在检测器出结果前 auto-complete → 门被绕过

### 现象（E2E 实测）
只注一条 `review_gate` 判据、无其他 backstop 判据的需求：被审任务产出含密钥的真工件 → secret_leak **真检出**了 → 但需求仍走到 **completed**，门被绕过。（有 human_judgment/automated_test 等 backstop 判据时不暴露，因 backstop 先 hold 住。）

### 根因
- 任务完成写回里 **同步** 跑 `gatedCompletionStatus`（`internal/project/pg_repository.go` 的 `recomputeProjectDemandStatusWithQueries` → `gatedCompletionStatus` → `CountUnsatisfiedBlockingCriteria` → `ResolveUnsatisfiedBlockingCriteria`）。
- 那一刻 review_gate 判据**还没 verdict** → 收敛闸默认反转判它**放行**（P1 的核心反转：`!hasVerdict` → 不 pending）。
- 其他 executor 判据满足 → 需求判定 completed。
- 而 `RunReviewGateForTask`（写 review_gate verdict 的 Activity）是**异步**的（经 `review-gate-trigger` 围栏在协调线程里派，含 deepseek ~13s 调用），**晚 ~13 秒**才写 unsatisfied——需求早已 completed。

即：**默认放行 + 异步检测 = 需求可能在检测器 hold 之前完成。** 这是默认放行反转的必然另一面。

### 建议修法（同步保守占位 + 异步解析）
被审任务完成时，在写回路径里**同步**为其命中的 review_gate 判据写一条**保守占位 verdict（hold 态）**（例如 verdict=`pending` 或直接先写 `unsatisfied` 占位），让收敛闸先 **hold 住**需求；随后异步的 `RunReviewGateForTask` 把它**翻成** `satisfied`（放行）或 `unsatisfied`（留人类）。
- **bounded 不违反反转精神**：检测器一定会跑（任务完成触发）且很快写结果（~13s），所以是"短暂 hold 到检测器出结论"，不是"无限等 correctness 证明"。
- 收敛闸需能识别占位 hold 态：给 `ResolveUnsatisfiedBlockingCriteria` 的 review_gate 分支加"占位/pending verdict 也 held"（现只对 `unsatisfied` held）。
- 同步占位的写入点：`recordProjectTaskAttemptResult` / writeback 事务内，判断被完成任务是否某 review_gate 判据的 satisfied_by（复用 `criteriaSatisfiedByTask` / `listCriteriaForTaskByMethod` 的匹配规则）→ 是则同步写占位。
- **替代方案**（更重，不推荐 P1.1）：让 `gatedCompletionStatus` 感知"review_gate 判据的 satisfied_by 任务已完成但无终态 verdict → pending"（需 join 任务状态），或把检测器同步进写回（LLM 13s 阻塞写回，不可取）。

### 验收
真实 E2E：只注 review_gate 判据（无 backstop）+ 含密钥产出 → 需求**不再 auto-complete**，held 至 acceptance_pending（占位 hold），检测器 ~13s 后写 unsatisfied 维持 held → 人类签放行；干净产出 → 占位 hold ~13s → 检测器写 satisfied → 放行（默认放行仍成立，只是延后到检测器出结论）。replay 绿。

---

## 2.【飞书/Web 实时刷新缺口】外部渠道 resolve 不推给已打开的 Web 页

### 现象（用户实测）
用户在飞书上批准了一条决策（plan_review"确认项目计划版本"），但已打开的 Web"待我处理"仍显示该项 + 可执行动作按钮（同意/驳回/要求补证）。

### 诊断（后端 resolve 传播无 bug；实时刷新是真缺口 + 流程续进）
**定论**：查"已 resolved 决策但 inbox 还 open 的" = **0 行**——后端 resolve 完全传播（决策 status_snapshot、inbox_items status、飞书 card 三处同刻一致 resolved）。用户实测"刷新后仍显示待批准"的原因有二：① 该 demand 流程**往下续进**，飞书批完 plan_review 后又新建了 project_acceptance/clarification 等**新的待处理决策**（这些是真 pending，不是没传播）；② **Web 对外部渠道（飞书）的 resolve 无实时推送**——已打开的 Web 视图不刷新就看不到状态变化。**实时刷新确认为真需求**（用户明确要）。

### （历史诊断保留）后端已 resolved，纯 Web 陈旧的部分
飞书审批**完全生效**，后端三处全 resolved（同一时刻 01:39:04）：
- `project_decision_requests` 该决策 `status_snapshot='approved'`、`resolved_at` 已写、`resolved_event_id` 已写。
- `inbox_items` 该 item `status='resolved'`、`updated_at` 已更新（收件箱物化表已闭）。
- 飞书 `feishu_outbox` card_update 已 sent（卡置灰）。
- CP 日志：`POST /api/v1/connector/decisions/{id}/resolve 200`。
inbox 查询按 `status` 过滤（`status='resolved'` 的不再列为待处理）。**所以刷新 Web 页该项即消失**——是已打开页面没自动刷新，非数据错。

### 根因 + 建议修法
经外部渠道（飞书）或他人做的 resolve，**不会实时推给已打开的 Web 视图**；Web 只在刷新/切页/下次轮询时反映。run-overview 已有 SSE（`GET /digital-employees/activity/stream`），但 **inbox / 决策视图未接实时**。
- 修法：给 inbox / 决策详情视图加**轮询失效**（React Query 定时 invalidate，仿 run-overview 的节流失效）或 **SSE**（决策 resolve 事件推送 → 前端失效 inbox/decision 查询）。轮询更简单、够用。
- 归属：飞书集成的联调 UX 缺口（见 memory `feishu-integration-design`：联调中断、遗留缺陷）。前端主：`apps/web/src/features/inbox/`（inbox-shell.tsx / inbox-item-list.tsx）+ `lib/api/inbox.ts`。

### 验收
飞书审批后，**不刷新** Web 页，该待处理项在数秒内自动消失/转已通过（轮询或 SSE 生效）。

---

## 3. P1 其他已知局限（whole-branch 评审记录，非本次必修，酌情）
- **多 distinct-task review_gate 判据 last-writer-wins**：一判据被多个不同 task 满足时，后完成的干净 task 覆盖前一个 violation（逃逸）。rework 同判据修订 correct-by-design。修=worst-wins/union 持久化。P1 用单 satisfied_by 规避。
- **内联缺失回退**：diff 只在对象存储时，code_review LLM 退扫 executor 自述（削弱接地）；fails-toward-release 无害。修=P2 对象存储 diff 取回。
- **block vs need_human 未差异强制**：目前都只在验收门 held，无中途硬阻断（spec §6 安全硬门=P2 需接命令日志/transcript）。
- **minor-tolerance 路径当前无 minor-emitting 条件**（死管道待未来条件）。
- Minor：redactSecret 对 ≤4 字符匹配不脱敏（现规则不可达）；enabledConditions 静默回退无日志。

## 3.5 前置清理已完成（2026-07-18，dev 库）
用户指示清理收件箱（dev 环境）。**已做**：删除全部 33 个 E2E 测试项目（12 个 ReviewGate/ADV-E2E + 21 个 Intent-P1/P2a/P2b/PostureA/任务图测试 fixture）及其全部关联数据，事务级联（~35 张表，含 inbox_items/decisions/demands/tasks/attempts/results/verdicts/criteria/feishu_outbox/plan_revisions/coordination_jobs/…），并**扫清历史孤儿**（81 verdicts + 82 criteria + 零散 evidence/budget 等，指向早前已删项目）。复验：projects=0、open_inbox=0、demands/tasks/decisions/verdicts/criteria=0。**保留不动**：append-only 审计账本 execution_ledger_events（2474 事件，无 FK、按设计不可删）、数字员工（38）、执行实例/runtime 节点/团队/租户等共享资源。清理脚本教训（供未来 dev 清理复用）：project_tasks 有 4 个自引用 FK 列（current_attempt_id/latest_task_result_id/latest_dispatch_gate_result_id/revision_of_task_id）需先 NULL 断环；task 簇删除序 results→attempts→dispatch_gate；FK 多为 NO ACTION 非 CASCADE 须显式删；事务 + ON_ERROR_STOP 保证出错回滚不留残缺。

## 4. 环境/状态
- P1 已合并 main 66ea3ee9；分支 feat/review-gate-p1 + worktree 已删（P1 核心 E2E 过）。竞态修 + 飞书刷新是**新工作**（新分支）。
- E2E 环境事实：主 checkout 跑 dev-services；执行实例直插 DB 造；deepseek 判官；完成需 object-store 可核验证据工件（手工完成用真实 prior-run artifact sha256 过 gate）；共享 dev 库 + 并发会话（隔离 worktree + ref 手术合并铁律见 memory `shared-checkout-concurrent-session-git-safety`）。
- 建议顺序：先修 §1 竞态（承重、门被绕过）→ 再修 §2 飞书刷新（UX）。

---

## 5. 实施记录（2026-07-18，已合并 main，本 spec 完结）

CHANGELOG 2026-07-18 11:40 条目为完整口径；此处只记 spec 层面结论与残余。

### §1 竞态：落地形态与 spec 建议的差异
- 采纳"同步保守占位 + 异步解析"，但**占位写进写回事务**（经 `ReviewGatePlaceholders` 字段传入两条 writeback 请求，事务内、recompute 前写）而非 spec 提示的 writeback 前独立写——评审证明事务外写会在写回失败时留孤儿 pending 永久 hold。
- 占位匹配规则在 spec 的 `criteriaSatisfiedByTask` 之上补了 **revision-root key**（镜像触发器规则）：修订任务 planned key 是派生值（`<base>#revision-<n>`），只匹配自身 key 会让修订腿完全绕门。
- spec 未写但必须补的另一半：**检测器写完 verdict 后每 distinct demand 一次 `RecomputeProjectDemandStatus`**——否则干净产出永滞 acceptance_pending。
- **E2E 揪出的第三块**（spec 与评审都没预见）：`gatedCompletionStatus` 判据/verdict 读取原走连接池 r.q，看不到写回事务内未提交的占位 → 默认放行照旧绕门。收敛闸全部读取改走调用方事务 q（`gatedCompletionStatusWithQueries`）。memory fake 无事务隔离，单测全绿盖不住这类缺陷——真实 PG E2E 是必要门禁的实证。
- 收敛闸语义：review_gate 判据"有 verdict 且非 satisfied 一律 HOLD"（含 pending 与未知值，fail-toward-human）；完全无 verdict 仍默认放行。
- 失败姿态修订：门被**触发**但未出结论（Activity 错误耗尽重试/协调线程死亡）= 占位 held 留人类签，不再默认放行；未触发（任务未完成）仍默认放行。trigger/workflow 注释与日志已同步改述。

### §2 实时刷新：落地 + E2E 揪出的更深一层
- inbox 列表/审批中心 5s 轮询、侧栏 badge 30s 轮询（仿 run-overview 既有惯例，未做 SSE）。
- **E2E 揪出**：收件箱列表默认不带 status，服务端返回全部状态——几天前 resolved 的旧项一直被标成"待处理"，外部渠道 resolve 后轮询拿到的还是同一批（§2 历史诊断"刷新后该项即消失"对默认视图并不成立）。修=默认 `status=open`（审批中心本就如此），用户可切"所有"。
- 验收实测：打开的收件箱页不刷新，外部渠道（curl 走与飞书 connector 同一 resolve 后端路径）签署后 **≤6 秒**该项自动消失，徽标/统计同步归零。

### 已知局限（全分支多智能体评审 15 agents，修 4 记 4）
- **F1（§3 既有家族的加重面）**：多任务共享同一 review_gate 判据时聚合行 last-writer-wins + recompute forward-only——A 任务干净放行可发生在 B 任务违规检测出结果前，B 的 unsatisfied 落在已 completed 的需求上。P1 以单 satisfied_by 规避；根治=worst-wins/按轮持久化（P2）。
- **F3**：pre-P1 启动且已处理过完成信号的协调线程被 GetVersion 钉在 DefaultVersion——占位会写但检测器永不跑，需求 held 至人签（此前是默认放行）。dev 库项目已清零无此类工作流；出现时人签可解，属 fail-toward-human。
- **F6**：检测器驱动的自动放行不 resolve 已开出的 demand_acceptance 决策（悬挂收件箱项）。触达需要"无人参与的第二轮完成"序列，当前流程（单 satisfied_by + review_gate 无自动返工）不产生；修复需扩宽协调层 approval seam，暂记。
- **F7**：verdict upsert 与 recompute 两次独立调用非原子；错误传播给 Activity 重试（幂等）自愈，永久失败=held 留人类，不会静默放行。
- **requires-acceptance 腿的占位时长**：占位在结果提交时写入，检测器在人批任务后才触发——人批等待期判据显示"检测中"（期间任务非终态、hold 冗余无害；注释已如实记载）。

### E2E 复用资产（散场清理后仍有效的打法）
- 自铸 runtime session：向 `runtime_sessions` 插 sha256 lookup hash + bcrypt secret hash（绑定既有 approved enrollment），用毕删除。
- 证据工件走真实 presign：`POST /api/v1/runtime/artifacts/presign` → PUT 上传 → 完成体 `artifact_refs` 带 `{sha256,size_bytes,is_evidence:true}` 过证据接地门。
- 直插 fixture 注意：`project_tasks.input_requirements` 必须是 jsonb **对象**（`[]` 会让 taskFromRecord 解码 500）；`project_plan_revisions` 需 planner_input_hash/plan_fingerprint；attempts 需 idempotency_key。
- fixture 项目经官方 DELETE API 归档（软删）。

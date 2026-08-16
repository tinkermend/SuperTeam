# 场景模板闭环健壮性优化方案

| 字段 | 值 |
|---|---|
| 日期 | 2026-08-16 |
| 性质 | 优化方案（本文件即交付物；不含实现） |
| 触发 | PulseAI 上 `ecc_code_delivery` 真链复跑（复跑 3 / 复跑 4 / CHANGELOG 小单） |
| 人类拍板 | **流程结构以场景模板为唯一权威**；需求正文不得增删模板步骤 |

---

## 0. 本次真链事实（结论以此为准，不据推测立项）

| 单 | 结果 | 关键事实 |
|---|---|---|
| 复跑 3 `2f76aae7` | **failed** | 开发 #1 因工件上传 403 失败 → 15:11–15:14 看门狗把 review/security/commit/push 从 `blocked` 收成 `cancelled`；18:00 人类批重试，18:04 `develop#2` **completed** 并交出 `head_commit`/`branch_ref`，需求仍 `failed`，项目一度进 `acceptance` |
| 复跑 4 `6f6dd5e2` | 停在验收 | 开发会话真跑约 39 分钟（`cmd-89b36b6d`），18:58 进 `waiting_human`（自报 5 项缺陷定案）；为控范围人工停掉下一轮，开发变 `cancelled` |
| CHANGELOG 小单 `68a8008e` | **completed** | 五步全绿：develop 19:03 → review 19:05 → security 19:06 → commit 19:08 → push 19:13（人闸后派发）；`handoff_summary` 5/5 fulfilled；需求验收签署后 `completed` |

**已证明可用**：模板骨架实例化、五步依赖放行、人闸、需求验收收敛、本机 RustFS 预签名回写。
**未证明可用**：失败路径。开发步一失败，图会被拆掉且重试接不回来。

模板 `push` 步的判据本身写了兜底：「人类确认后推送远程（无 origin 则停在本地提交）」。小单实际产出 `push_receipt.md`，如实记录 `git remote -v` 为空、停在本地 `d1355f3`。**这是对的**，但它的理由里同时引用了需求正文「不要推远程」——说明当前靠 Provider 自觉裁决，平台没有表达过优先级。

---

## 1. 权威顺序（本方案的地基）

按人类拍板固化为三层，任何一层不得改写上一层：

| 层 | 权威内容 | 谁定 | 硬约束 |
|---|---|---|---|
| **L1 流程结构** | 有哪些步、依赖边、每步由哪个角色/数字员工处理、`produces` / `required_inputs`（**交接包**）、判据、`requires_human_approval` | **场景模板骨架** | 需求描述、执行者说明、员工技能**都不得**另开流程、增删步骤或改交接契约 |
| **L2 业务内容** | 这一单填进模板槽位的材料（做什么、范围） | 需求正文 | 只能填 `objective`/`summary`/判据补充；必须能被某步的 `required_inputs`/`produces` 接住 |
| **L3 执行手法** | TDD、提交规范、工具选择 | 员工人格 / 技能 | 不得改变 L1 产出与交接形状，不得突破步骤预算 |

场景模板在底层**串联的是交接，不是散文**。上一步做完，平台按模板边把**命名槽位**交给下一步数字员工（谁处理、依赖哪条边、槽位叫什么、kind 是什么，由骨架规定）。需求描述、开发者说明只填进槽位，填不进就标冲突（模板赢），**不能**变成跳步。

**禁止把「交接必须写全」做成对 AI 的反复打回。** 散文、理由、summary 由模型生成，缺字段再派一轮往往仍缺。闸门只应卡**平台能独立核验的事实**（例如 `git_commit` 是否真有 SHA、工件是否真有 attestation），不能卡「模型有没有把交接包写完整」。槽位怎么填、A 把什么交给 B，单开设计（见 §4.6），不在本期用补做循环凑字。

**推论 1**：需求正文写「不要推远程」不等于跳过 `push` 步。`push` 步必须执行，其合法结果由**步骤判据**枚举（真推送 / 无 origin 停本地 / 无变更可推），并以 `push_receipt` 作为交给下游/验收的交接证据。
**推论 2**：冲突不能静默由 Provider 消化。实例化时判定冲突 → 落审计 + 在**规划确认卡**上显式列出「正文诉求已被模板覆盖」→ 步骤 prompt 内写明优先级。
**推论 3**：技能「先写失败测试」不得改写本步槽位。判据的 `satisfied` / `not_applicable` 若要成立，应尽量由平台事实推导，而不是靠执行者把豁免写成 satisfied。
**推论 4**：人类验收点也由模板规定。不另造「逐条同意 N/A」闸。§4.4.2 = **D1**。

---

## 2. P0 — 失败路径闭环（不修则模板闭环只在顺利路径成立）

### 2.1 滞留收敛与人类重试的竞态（本次最伤）

**现状**：`SweepStrandedBlockedProjectTasks`（`internal/project/stranded_blocked_reconcile.go`，看门狗每分钟一轮，`internal/app/stuck_task_reconciler.go:57`）只要上游全部 `failed/cancelled` 就把 `blocked` 下游取消（判据 SQL `internal/storage/queries/project.sql:1943`，**不看是否还有未收敛的恢复决策**）。
人类随后点重试走 `createRecoveryReplacementTask`，其中 `rewireRecoverableDependents`（`internal/workflow/projectcoordination/project_store.go:2268`）对**终态**下游 `continue` 跳过——`cancelled` 是终态，于是 `develop#2` 建出来时没有任何下游挂在它后面。

**方案（两侧都要改）**：

1. **看门狗加闸**：上游失败若仍存在「可行动的恢复路径」——存在 pending 的 `task_failure_recovery` 决策，或人类再派发预算未用尽——则**不取消**下游，最多标注一次滞留观测事件。取消只在恢复路径关闭后发生（人类选 `cancel_downstream`、预算耗尽、需求被关闭）。判据进 SQL，不靠调用方自觉。
2. **取消原因分型**：`project_tasks` 上区分 `cancel_reason = system_stranded | human_reject | demand_closed`（现在两条路径都只写 `cancelled` + summary 文案）。
3. **重试复活**（已拍板：自动全部复活）：`rewireRecoverableDependents` 对 `cancel_reason = system_stranded` 的下游**全部先复活为 `blocked` 再重挂边**，不让人在恢复卡上逐个勾选；`human_reject` 与 `demand_closed` 保持终态不动。复活要发事件，人能在时间线看到「系统取消已因重试撤销」。

### 2.2 重试成功后需求状态不跟随

**现状**：`develop#2` `completed` 之后需求仍 `failed`（读模型不是滞后，18:04 完成到 19:00 都没变）。原因是需求状态在开发 #1 失败时已重算成 `failed`，替换任务完成时没有把需求拉回 `executing`。

**方案**：恢复替换任务创建成功即把需求从 `failed` 拉回执行态（复用已有 `ReopenProjectDemandForReplanningInput` 的语义但不重规划），替换任务终态再走 `RecomputeProjectDemandStatus`。**判据**：任何时刻「需求 failed」必须等价于「图上没有可推进的任务且没有开放恢复卡」。

### 2.3 结项闸门过宽

**现状**：复跑 3 的图被看门狗拆掉后，项目进 `acceptance` 并开「结项确认」卡，卡文案把 5 个需求全列为失败。

**方案**：`IsProjectAcceptanceReady` 增加排除条件——存在 `system_stranded` 取消且恢复路径未关闭的需求时不算 ready。结项卡只在「需求都已验收，或人类明确放弃」时出现。系统取消 ≠ 业务失败，这条语义要在读模型与卡文案里同时成立。

---

## 3. P1 — 对象存储与写回健壮性

### 3.1 缺对象回 403 被当成 500

`S3ObjectStore.StatObject`（`internal/storage/storage.go:342`）只把 `NotFound/NoSuchKey/404` 当「不存在」；云桶（本次为 TOS）在无 `ListBucket` 权限时对缺对象返回 **403**，于是 `PresignRuntimeArtifact`（`internal/project/artifact_storage.go:135`）包装成错误 → 接口 500 → Runtime 侧 `upload artifact raw.jsonl` 整单失败。切回本机 RustFS 后本机不再触发，**云部署仍会**。

**方案**：Head 的 403 与「无权限」分开——403 且 `HeadBucket` 可达时按「不存在」处理并降级为直接签发 PUT（对象重复写是幂等的，代价可接受）；真正的凭据/策略错误要带 `failure_family` 上抛，不要让 Runtime 以为控制面挂了。同时 presign 接口对「缺对象探测失败」不再回 500。

### 3.2 Attestation 写入 500

`CreateProjectTaskAttestation`（`internal/storage/queries/project_runtime_affinity.sql:15`）是 `INSERT ... ON CONFLICT DO NOTHING` + `UNION ALL` 回读；同一 attempt 先成功后失败时出现无行/字段不齐，写回报 500（本次失败单日志可见，小单成功路径未触发）。

**方案**：冲突后按终态**更新**或显式返回既有行，字段列表与回读侧对齐并加回归用例（成功后再失败、并发双写）。

### 3.3 配置来源不可见

`S3_*` 环境变量能静默覆盖 yaml（`internal/config/config.go:246`），本次就是被根目录 `.env` 指到云桶。Runtime 进程里至今还留着一组过期远程 `S3_*`（上传走预签名所以未爆）。

**方案**：① CP 启动日志打印 `endpoint/bucket/region/forcePathStyle` 与「来源=yaml|env」，**不打印密钥**；② env 与 yaml 不一致时 WARN；③ `dev-services.sh` 不把过期 `.env` 灌进 Runtime；④ `./scripts/ops/init-object-store.sh --check` 纳入起服前置说明。

### 3.4 `/health` 不探对象存储

现状 `/health` 只证明进程活着，桶不存在要等到任务上传时才炸。**方案**：加 `objectStore` 探针（HeadBucket，可 `?deep=1` 触发），让「新环境桶没建」在启动即暴露。这也是私有化 P0-5 的一部分。

---

## 4. P2 — 执行语义与范围控制

### 4.1 步骤级预算：只观测不打断（已拍板）

现有看门狗只管**假死**，不管**范围膨胀**：复跑 4 一个开发步真跑 39 分钟并自报 5 项缺陷；小单开发约 2 分钟、审查/安全各约 1.5 分钟。

**方案**：步骤级预算（墙钟 / 工具调用次数 / 改动文件数）作为**观测阈值**，可在模板骨架按步覆盖。超限只做三件事：发一条超预算观测事件、在需求时间线与任务卡上标注（含已跑时长与当前改动文件数）、在项目运行视图点亮提示。**不转人类卡、不判失败、不动会话**。参考阈值：开发 15 分钟，审查/安全/提交各 10 分钟，推送 5 分钟。

**残留风险（明确接受）**：39 分钟那类单不会被平台自动拦下，控范围仍靠需求正文与步骤契约；人看到标注后自行决定是否停。因此 §4.2 的「停止只作用于下一轮」必须先落地，否则人工干预会误伤已完成产出。

### 4.2 「成功待确认」与「失败要重跑」必须分开

复跑 4 的开发其实**执行成功**并进了 `waiting_human`，随后又自动重排一轮；人工停下一轮时，已完成的产出被整体标 `cancelled`。

**方案**：停止/取消动作只作用于**下一轮尝试**，不回收已 `complete` 的 attempt 产出；`waiting_human`（成功待确认）与 `failed`（要重跑）在恢复卡上给不同动作集。

### 4.3 派发 `project conflict` 假终态

attempt 已 `started`、活动却被判终态拒绝（与「命令 409 / attempt 假活」同源：终态协议与并发写不一致）。**方案**：把并发冲突统一当**可恢复**，走退避重试或转人类卡，禁止把活任务判死。

### 4.4 自动化判据不接受执行者自评

**两张真链对照（必须同时修，否则验收语义自相矛盾）**：

| 单 | `template_criterion_1`「测试先行」 | 后果 |
|---|---|---|
| 顺利小单 `68a8008e` | `satisfied` + `judge_type=executor`，summary 自称「无生产代码，豁免测试先行」 | 闸门当绿灯；人类只签了兜底 `human_final_confirmation`。五步全绿被自评稀释。 |
| A 期验收 2 `f055f210` | **无 verdict**（人类不能签 `automated_test`；`POST .../criterion-verdicts` 只收 `human_judgment`） | 五步 completed、人类终检已签，需求永久 `acceptance_pending`。 |

平台里其实已经有第三条路：`not_applicable`。`docs/superpowers/specs/2026-07-16-intent-acceptance-criteria-full-design.md` 与 `TestNotApplicableAutomatedCriterionDoesNotDeadlock` 规定：执行者对 automated 判据交 **N/A + 理由 + evidence** 时投影 `verdict=not_applicable`，**该条不再阻断**，只留人类兜底判据。`f055f210` 的死锁是因为开发步**既没交 satisfied（无测试证据），也没交 N/A**，闸门把「无 verdict」当永久不满足。

**D 期要落地的不是新发明 N/A，而是把「什么时候允许哪一种 verdict」写成平台规则，并逼执行者走合法出口。**

#### 4.4.1 闸门矩阵（automated_test + blocking）

| 执行者申报 | 平台核验 | 投影 | 是否阻断需求验收 |
|---|---|---|---|
| `satisfied` | 本步 attempt 有服务端核实的成功 attestation，且证据能对应判据（测试命令/退出码，或步骤 `produces` 已声明的工件） | `satisfied` / `executor` | 否 |
| `satisfied` | 无 attestation，或证据只是自然语言「已豁免」 | **拒绝投影为 satisfied**；任务仍可 `completed`（产出已在），该判据保持无 verdict 或改写为待人类看的 N/A 候选 | **是**（今日 `f055f210`）；必须再走 N/A 出口，否则死锁 |
| `not_applicable` | 有非空理由；建议带 evidence_ref（diff 范围、未改 `src/` 等） | `not_applicable` / `executor` | **见 §4.4.2 拍板** |
| `unsatisfied` | — | `unsatisfied` | 是 |
| 未申报 | — | 无 verdict | 是（死锁，禁止作为文档小单的合法终态） |

人类对 `human_judgment` 的签署语义不变。人类**不能**把 automated 判据改签成 satisfied 来「补票」——那会把自动化闸变成橡皮图章。若要推翻 N/A，走人类 `unsatisfied` 覆盖（已有 override 矩阵）再返工。

#### 4.4.2 N/A 之后要不要再让人逐条点

已收束为 **D1**：N/A 若由平台规则投影，不另开签署闸。人类只签模板放置的兜底 `human_judgment`。验收卡展示理由是知情。废弃 D2。

#### 4.4.3 不靠执行者作文，也不打回 AI 补写

假 `satisfied` 仍然禁止（无 attestation 不得投影通过）。但 **缺 verdict / 缺 N/A 理由不得触发「再跑一轮把交接写全」**。文档小单卡死（`f055f210`）要靠平台从 diff/工件推断 N/A 或「无测试证据」，而不是指望模型自觉申报。启发式可后置，不进 B 期。

禁止：技能文案覆盖步骤槽位。

### 4.5 模板契约冲突检测（落地 §1）

**问题**：需求正文「不要推远程」被 Provider 写进 `push_receipt` 理由，看起来像执行者决定跳过推送。权威顺序要求：`push` 步必须跑；合法结果由步骤判据枚举（真推送 / 无 origin 停本地 / 无变更可推），证据是 `push_receipt`，任务状态仍是 `completed`（已拍板，不新增 `skipped`）。

**方案分三截，实例化时做完，不把 NLP 做成开放域裁判：**

1. **冲突检测（保守规则，漏报优于乱报）**  
   对需求 `title`/`summary`/`objective`/人类判据原文做有限模式匹配，例如：跳过/不要/禁止 +（推送|push|远程|origin）、不要测试、跳过审查。命中则生成 `contract_conflicts[]`：`{step_key, demand_span, template_rule, resolution: "template_wins"}`。不试图理解所有自然语言。
2. **规划确认卡**  
   冲突列表作为卡上独立区块：「以下正文诉求已被场景模板覆盖，步骤仍会执行」。人点确认 = 接受模板优先。无冲突则区块不出现。
3. **步骤 prompt 头部固定优先级**  
   每步注入：L1 步骤必须执行；本步合法结果集（从骨架 `produces` + 判据抄来）；冲突条目（若有）。`push` 步明确写：无 origin / 无 upstream 时产出 `push_receipt` 并 `completed`，**不要**把任务标失败或 skipped。

检测漏报时（正文换一种说法），仍靠 prompt 里的合法结果集兜底——这是 4.5 的核心，规则列表只是让人类在规划卡上看得见。

### 4.6 A→B 交接要单独设计（不在 B 期实现，也不走「写全再放行」）

已有底稿：`docs/superpowers/specs/2026-07-13-handoff-contract-execution-loop-design.md`（直接前驱注入、`produces`/`required_inputs` 图校验、缺项补做）。**那份里的「缺项即补做循环」对 AI 散文不成立**，本方案明确降级：补做只适用于平台可核验槽位（无 `head_commit` 却声明了 `git_commit`），不适用于 summary/理由没写全。

后续交接设计要回答的是机制，不是作文提纲：

| 问题 | 现状（真链已见到） | 设计方向（待写专稿，未拍板细节） |
|---|---|---|
| **怎么交** | 调度边已通（A completed → B 从 blocked 放行）；派工单主要仍是原始需求 + 本步契约，上游 result 注入不完整 | 一条依赖边 = 一份交接；只注入**直接前驱**（07-13 §3.1，防 token 膨胀与越级） |
| **交什么** | 模板骨架已有命名槽位（如 develop→review 的 `head_commit`）；执行者另写长 summary | **槽位由模板命名 + kind**；能平台填的（SHA、branch、工件 ref、attestation）由控制面填进下一员工上下文；模型结论是附加物，缺了不打回 |
| **谁保证** | 现在既靠模型写 `handoff_summary`，又靠判据自评 | 履约核对只盯 kind 可核验项；散文不进闸 |

专稿应修订 07-13，而不是在闭环健壮性方案里用一段话代替。B 期不碰交接。

---

## 5. 分期与验收（done looks like）

| 期 | 内容 | 验收（必须真链） |
|---|---|---|
| **A** | §2.1 + §2.2 + §2.3 | **真链验收 2** `f055f210`（2026-08-16）：假桶让 develop 失败后观察约 150s，下游 review/security/commit/push **全部仍 blocked**（实现为失败后 5 分钟宽限，不必干等到看门狗取消）。恢复真桶并批重试后：`develop#2` completed，边挂到替换任务，review→security→commit→push 依次 completed。需求进 `acceptance_pending`，项目保持 `running`。人类判据已签；`template_criterion_1`（自动化「测试先行」）无 verdict，需求未收敛到 `completed`——属 D 期「自动化判据不接受执行者自评 / 文档单应 N/A」，不挡 A 期失败路径闭环。 |
| **B** | §3.1 + §3.2 + §3.3 + §3.4 | HeadObject 403 且 HeadBucket 可达 → 当缺对象并签发 PUT；凭据错误带 `failure_family=object_store_access`。attestation 同键冲突改为 UPDATE 终态。启动日志打印 endpoint/bucket/region/forcePathStyle 与 yaml\|env 来源、不打密钥。`GET /health` HeadBucket 失败 503。Runtime 起服 unset `S3_*`。 |
| **C** | §4.1 + §4.2 + §4.3 | 开发步超预算只出观测事件与卡片标注、会话继续跑；人工停止只作用于下一轮，已 `complete` 的产出不被清掉；并发冲突不再产生假终态 |
| **D** | §4.4 + §4.5 | 真链：只改文档的单「测试先行」不得是 executor `satisfied`；必须作为模板枚举的交接出口写入 `not_applicable`+理由（或有测试 attestation 的真 satisfied）；无 verdict 不得作为终态。正文含「不要推远程」时规划卡列出冲突，push 仍执行并交出 `push_receipt`。人类只在模板放置的人闸上签；验收卡展示 N/A 理由。 |

期间的流程验证单一律沿用**小单口径**：真改代码或文档，但一件小事、几分钟内可 commit；不再用「全仓排查致命缺陷」做流程烟测。

---

## 6. 人类拍板记录（2026-08-16）

1. **场景模板是流程宪法，串联的是命名槽位**：步骤、角色落点、`produces`/`required_inputs` 由模板规定。需求描述不能另写一条流程。A→B 怎么注入、交哪些可核验项，见拍板 8，不靠模型把交接包写全。
2. **超预算只观测不打断**：告警 + 时间线/卡片标注，不转人类卡、不判失败（§4.1）。接受「长任务不会被自动拦」的残留风险。
3. **复活粒度=自动**：`system_stranded` 取消的下游在重试时全部自动复活并重挂边，不做逐条勾选（§2.1-3）。
4. **未执行步口径保持现状**：`push` 无 origin 时仍是 `completed` + `push_receipt` 说明，不新增 `skipped` 终态，读模型与统计口径不动（§1 推论 1）。
5. **流程验证单一律走小单口径**：真改代码或文档，一件小事、几分钟内可 commit。
6. **D 期 N/A 签署粒度 = D1（由拍板 1 收束）**：`not_applicable` 是模板枚举的合法交接出口之一，投影后不另开「逐条同意不适用」卡。人类只签模板放置的兜底 `human_judgment`；验收卡必须展示 N/A 理由（知情，不是第二宪法）。废弃 D2。
7. **下一期实现 = B**：生产上 Head 403 不得变成接口 500；health/配置来源一并做。D/交接专稿不插队。
8. **交接不靠 AI 写全**：禁止「缺 summary/N/A 理由就再派一轮」。A→B 交什么、怎么注入，修订 `2026-07-13-handoff-contract-execution-loop-design.md` 另开；可核验槽位与散文分开。

---

## 7. 相关文档

- 失败归因与重试：`docs/superpowers/specs/2026-08-10-retry-redispatch-and-failure-attribution.md`
- 预派发/僵尸收敛：`docs/superpowers/specs/2026-08-11-predispatch-approval-orphan-zombie-design.md`
- 自治档位与剧本：`docs/superpowers/specs/2026-08-13-autonomy-envelope-policy-playbook-design.md`
- 对象存储与私有化：`docs/ops/local-rustfs.md`、`docs/ops/private-deploy-ops-maintainability.md`

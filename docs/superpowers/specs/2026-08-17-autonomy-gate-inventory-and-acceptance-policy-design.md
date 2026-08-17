# 自治治理增补：闸点全清单 × 验收判据口径 × 高风险声明式

- 日期：2026-08-17
- 状态：**开放问题已全部收口**（§7.1 四条拍板 + 复审增补拍板）；可直接排实施计划（§8 的 F0–F7，另有 §7.2 明示的本批不做项）。§2 盘点与 §3 缺陷为代码读证事实，**已于 2026-08-17 复审经五路代码核验属实**（个别行号随工作树在途改动漂移，已回写；实施时以符号定位为准）
- 性质：**增补设计稿**，承接并修订 `2026-08-13-autonomy-envelope-policy-playbook-design.md`（下称「母稿」）。母稿 P0–P5 已实施并有真实链路记录（CHANGELOG 2026-08-13 各条），本文不推翻已落地部分，只修订 §4.1 闸点清单、§4.2 承重句的唯一违反处、§4.3 验收特别条款与三档模型、§5.7 策略键、§9 最后一期的内容
- 已拍板（用户 2026-08-17 对话，逐条不得静默偏离）：
  1. 先补闸点清单盘点并逐个表态，再排实施；不先写代码
  2. 高风险走**声明式**——由剧本/规则显式声明，planner 自报字段不再作闸输入（与母稿 §4.5 和 2026-08-07 撤销出口深度缩放的理由一致）
  3. **验收 `auto` 档保留**（驳回本文初稿的「建议撤销」），须给出非橡皮图章的做法 → §6 重写
  4. 新增 `auto_recheck` 档（§2.1）
  5. 声明式改造**一步到位，不设审计观察期**——planner 自报高风险退出后不再有过渡态兜底
  6. 验收停不停**按 exit 档细分**（母稿 §5.6 留白兑现）：浅档收口自动完成、深档收口停人，判定权归剧本作者 → §6.5。**明确不含**计划确认——`1e0494b3` 的无条件停人不动
  7. **预签即审核**（复审拍板）：创建自动化规则 / 外部集成绑定时的选档动作本身就是人类审核，执行期不逐单问人；planner 自撰的 `human_judgment` 判据**不是签字**，不得拦预签规则 → §6.4
  8. **预签不变量**（复审拍板）：预签规则在运行时遇到的每道停人闸都必须是创建时已确定的，创建时不可预见的停人是缺陷 → §6.4 / §6.5.6
  9. **exit 档漂移收口**（复审拍板）：`full_auto` 绑多出口剧本须创建时 pin exit 或显式确认分档语义，配置面加兼容性预检 → §6.5.6
- 证据等级声明：本文 §2/§3 全部来自**代码静态读证**（含 file:line），**未做实跑验证**。实施前须按 §8 补真实链路复验，不得据本文直接宣称行为已验证。2026-08-17 复审已对 §2/§3 全部承重引用做五路并行代码核验：结论全部成立；复审发现的因果错误与事实错误（§3.3 第三口子、§4.2 第二步、§2.2 budget 子路、行号/日期）已回写本文

---

## 1. 为什么需要这份增补

母稿 §4.1 写道「全平台人类闸就是这张清单」，列了 7 种派发前动作加计划确认与验收共 9 项，并据此断言「策略的消费者即它们」。逐行对代码后，这句话在三个方向上不成立：

1. **漏盘**：实际能阻断自动推进的人类闸有 **19 个** `decision_type`（另有 `team_privileged_role_grant` 属组织治理，不计）。母稿 §4.1 的九项映射到其中 **6 个**（`plan_review`、`demand_acceptance`，以及门禁铸出的 `project_task_approval` / `project_task_budget_approval` / `project_task_runtime_recovery` / `project_task_missing_context`），**漏盘 13 个**。`project_task_acceptance`（下游放行）、`project_task_iteration_exhausted`（迭代耗尽）、`upstream_supplement_review`（上游补做）等都不在母稿清单里，但都会让 `full_auto` 规则停下。
2. **虚列**：母稿清单里的 `permission_approval` / `tool_authorization` / `replan_decision` 三项**从未被门禁铸出**，其中前两项还被写进了 `full_auto` 白名单——也就是说母稿承诺的「自动放行覆盖 7 种派发前动作」，实际只有 4 种存在、其中 2 种是死的。
3. **口径含混**：母稿 §4.3 说 `auto` 档的前提是「该闸有机器可判定判据」。这句话把两个不同的问题揉成了一个，导致无法对具体某道闸做出判断（见 §2.1）。

更要紧的是两条母稿完全没有意识到的事实（均见 §3.3）：

- **验收面上 `full_auto` 与 `pause_at_gate` 的行为完全一致**。选「遇闸暂停」不会让验收停人，选「完全自动化」也不会让验收放行——验收闸根本不读 `autonomy_tier`。这是配置面对用户撒谎，比缺功能更严重。
- **验收是全平台唯一一处仍在「跳过闸」而非「策略放行闸」的地方**，直接违反母稿 §4.2 的承重句。判据全过时整道闸被跳过（零决策记录），而「一条判据都没有」同样算作全过。这既是审计断链，也是一个正在发生、无人选择过的橡皮图章。

---

## 2. 闸点全清单与逐个表态

### 2.1 判据：把「机器可判」拆成两问

母稿的「有机器可判定判据」不可操作。本文改用两问，**只有两问都为「是」才能进 `auto` 档**：

| 问 | 含义 |
|---|---|
| **Q1 触发是否机器判定？** | 闸为什么开——服务端能不能独立复算出「该开这道闸」 |
| **Q2 放行所需事实，服务端手上有没有？** | 闸凭什么关——放行需要的事实是服务端可独立取得/复检的，还是只存在于某个人的脑子里 |

几乎所有闸的 Q1 都是「是」（这正是母稿被误导的地方——看起来到处都能自动）。**决定档位的是 Q2。**

三种 Q2 结果对应三种处置，**其中第三种是母稿没有的**：

| Q2 结果 | 档位 | 语义 |
|---|---|---|
| 事实服务端已有 | `auto` | 策略签字放行，`resolved_by=policy:{id}` |
| 事实服务端可复检（探针/重算/重试） | **`auto_recheck`** | 不是「人点头」也不是「策略点头」，是**服务端自己复检后放行**；复检不过则退回 `human` |
| 事实只在人手里 | `human` | 永远停人，`full_auto` 也不放行 |

`auto_recheck` 是本次盘点新识别出的档：`runtime_recovery`、`project_task_recovery` 这类闸，放行需要的是「工作区/节点现在真的好了」，这既不是人类判断也不该由预签策略盲签，而是服务端应当自己去探。今天它们被一律归入 `human`，导致自动化撞上一次节点抖动就永久停车等人。

### 2.2 派发前门禁（`EvaluatePreDispatchGate`）

常量 `apps/control-plane/internal/project/predispatch_gate.go:32-38`；求值 `:182`；白名单 `apps/control-plane/internal/workflow/projectcoordination/policy_autonomy.go:55-65`。

| 动作 | 是否真被铸出 | 触发条件 | Q1 | Q2 | 现状档 | **表态** |
|---|---|---|---|---|---|---|
| `risk_approval` | 是（`:375`） | `task.RequiresHumanApproval && !granted` | 是 | 事实=「有人批过这个动作」，预签策略即是那个批准 | `auto` | **`auto` 保持**。但触发源须按 §4 改声明式 |
| `budget_approval` | 是（`:349-370`，3 子路） | 无任务预算声明 / token 耗尽 / 需批旗标 | 是 | **复审修正**：子路②（token 耗尽）是服务端数值事实；子路①③（`task_budget_missing`、`needs_budget_approval`）来自 **planner metadata**（`workflow/projectcoordination/predispatch_gate.go:848-867`），非服务端事实 | `auto` | **拆档（修正版）**：②走 `auto_recheck`（下次派发复算 consumed<limit）；①③与 `risk_approval` 同构，按 §4.2 第五步来源授权处理——planner 自报的旗不得被策略自动放行。「剧本与项目皆无预算」默认**停人**（无预算不等于放行，与 E1 同型）；「剧本兜底」指剧本提供预算声明，而非跳闸 |
| `missing_context` | 是（`:391`） | `missing_context_refs` 非空 | 是 | **补 ref 只能人做** | `human` | **`human` 保持**（母稿判断正确） |
| `runtime_recovery` | 是（`:332`） | `!WorkspaceReady` | 是 | 工作区是否恢复**服务端可探** | `human` | **改 `auto_recheck`**。今天一次抖动即永久停车 |
| `permission_approval` | **否——从未铸出** | 无 `addBlocker` 路径 | — | — | 在白名单（`policy_autonomy.go:59`） | **退役常量**，或接真实权限闸（见下） |
| `tool_authorization` | **否——从未铸出** | 无 | — | — | 在白名单（`:60`） | **退役常量** |
| `replan_decision` | **否——从未铸出** | 硬状态走 `PreDispatchGateStatusReplanRequired`，不是人类闸 | — | — | 不在白名单 | **退役常量** |

**承重结论：`full_auto` 白名单 4 项里有 2 项是死的。** 母稿宣称的权限/工具授权自动放行是纸面能力。真实的权限闸是 `project_task_permission`（运行中 blocker 文本或 `FailureFamilyPermissionRequired` 落 `waiting_human`，`service.go:5005`），**策略不认它**。

### 2.3 协调轨与任务轨的其余人类闸

| `decision_type` | 铸闸点 | 触发 | Q1 | Q2 | 现状 | **表态** |
|---|---|---|---|---|---|---|
| `plan_review` | `project_store.go:2735` | plan 模式无条件；或 `ReviewRequired` | 是 | 预签策略即批准 | `auto`（已实现） | **保持** |
| `project_task_approval` | `predispatch_gate.go:376`（派发前铸卡；运行路 `service.go:7285` 定型、`:5634` 落库） | 见上 risk_approval | 是 | 同上 | `auto` | **保持** |
| `project_task_acceptance`（下游放行） | `service.go:4039` | 完成时有人类复核信号 **且** 有未终态下游 | 是 | 「替这份产出背书」是人类判断 | **`human`，母稿漏盘** | **`human` 定档**，并写进清单让用户在配置面看得见 |
| `demand_acceptance`（验收签署） | `project_store.go:4853` | 有 blocking 判据未被释放（机器或人类；**此触发条件本身要改**，见 §6.1） | 是 | 分情形——机器判过 vs 机器未判完 vs 只有人能判 | `human`，且判据全过时**整道闸被跳过** | **设 `auto` 档 = 证据充分性门禁**，见 §6 |
| `project_acceptance`（结项） | `project_store.go:2855` / `service.go:8589` | 全部需求终态 | 是 | 结项是业务裁决 | `human` | **`human` 定档** |
| `planning_gap`（缺员） | `project_store.go:4497` | planner 找不到合适员工 | 是 | 补人/豁免都是人的动作 | `human` | **`human` 定档**（母稿判断方向正确但未列） |
| `casting_expansion`（扩编） | `casting_expansion.go:40` | 剧本角色未编制 | 是 | 选谁演是人的决定 | `human` | **`human` 定档** |
| `planning_failed` | `project_store.go:4710` | 规划重试耗尽 | 是 | 重试是机器可判的；改派/关单是人的 | `human` | **拆动作**：`retry_planning` 可 `auto_recheck`（有限次），`reassign`/`close_demand` 留人。retry 的 `auto_recheck` 本批不做（§7.2） |
| `task_failure_recovery` | `project_store.go:1557` | 任务终态失败 | 是 | 重试机器可判；`cancel_downstream` 是业务裁决 | `human` | **拆动作**：同上（retry 的 `auto_recheck` 本批不做，§7.2） |
| `project_task_recovery` | `pg_repository.go:5751` / `service.go:6835` | 派发失败无自动重试 / attempt 丢失超预算 | 是 | 服务端可探 | `human` | **改 `auto_recheck`** |
| `project_task_runtime_recovery` | 见 2.2 | 同 | 是 | 服务端可探 | `human` | **改 `auto_recheck`** |
| `project_task_clarification` | `service.go:5766` / `handoff_package.go:541` | 执行者提问 / handoff 槽位缺口 | 是 | 回答问题只能人 | `human` | **`human` 定档** |
| `project_task_permission` | `service.go:5005` | 运行中报权限不足 | 是 | 授权只能授权者给 | `human` | **`human` 定档**；同时说明 2.2 的死常量不是它 |
| `project_task_plan_invalid` | `service.go:4988` | 执行者说计划/契约不对 | 是 | 计划是否仍成立是规划判断 | `human` | **`human` 定档** |
| `project_task_missing_context` | 见 2.2 | 同 | 是 | 同 | `human` | **`human` 定档** |
| `project_task_budget_approval` | 见 2.2 | 同 | 是 | 同 | `auto` | 见 2.2 拆档 |
| `upstream_supplement_review` | `project_store.go:1746` | plan 模式缺上游输入；loop 模式补做预算耗尽 | 是 | 「要不要多做一批活」是打法选择 | **`human`，母稿漏盘** | 方向**可 `auto`**（若剧本声明允许自动补链，loop 语义本就如此），但声明机制尚不存在——**本批维持 `human`**，表达能力另立一期（§7.2） |
| `project_task_iteration_exhausted` | `project_store.go:1644` | 同一失败重复 / 补做预算耗尽 | 是 | 「还要不要继续砸资源」是业务裁决 | **`human`，母稿漏盘，且是坏的**，见 §3.1 | **先修缺陷再定档**；定 `human` |
| `project_task_human_wait` | `service.go:7297` | 兜底分支 | — | — | `human` | 不应出现；出现即 bug |
| `team_privileged_role_grant` | `permission/subject_team_role.go:89` | 人申请特权团队角色 | — | — | 组织治理，非项目闸 | **不纳入自治模型** |

### 2.4 不铸卡但同样阻断的人类持有

这些**没有 `decision_type`**，因此不会出现在任何「闸点清单」里，但照样让自动化停下：

| 持有 | 位置 | 处置表态 |
|---|---|---|
| `review_gate` verdict 非 `satisfied` | `review_gate.go:191-193`；聚合口径 `demand_acceptance_gate.go:238-257` | 检测门语义正确（无 verdict 默认放行），**保持**；`pending` 占位卡住今无告警——F4 的 E4 使 pending 判据 HOLD 后不再静默卡死，独立告警另行补（§7.2） |
| `adversarial_review` `escalate_human` / 引擎错误 | `adversarial_trigger.go:135-143` | **保持 `human`**——判不出来就不放行是对的 |
| Transfer request 落 `waiting_human` | `pg_repository.go:6413`；协调线程仅记事件 `workflow.go:87` | **待复核**：疑似无 resolve API 的运维死角；**不挂 F0–F7**，作为独立复核项排期（§7.2） |
| 工作区供给待确认 | `ItemTypeProjectWorkspaceProvisionPending` | 已有独立设计（`2026-08-12-project-workspace-provisioning-model.md`），**不纳入本文** |

---

## 3. 三个必须先修的缺陷（不是缺口）

以下三条是代码读证发现的**现存缺陷**，与自治档位无关也应当修；`full_auto` 只是把它们放大。

### 3.1 `project_task_iteration_exhausted` 是死胡同卡

`RequestProjectTaskIterationExhaustedReview`（`project_store.go:1644-1739`）做两件事：把全部下游任务改成 `blocked`（`:1674`），然后铸卡。但它**没有**把任务本身置为 `waiting_human`。

而卡的 resolve 路径落在 `workflow.go:432` 的 `default` 分支 → `applyPreDispatchGateDecision`，该活动是按任务的 `waiting_human` + `waiting_request_id` 驱动的。任务不在那个状态，于是放行是 **no-op**：卡关掉了，下游永久 `blocked`。

复审核验发现 no-op 比上述还早一层：`ApplyPreDispatchGateDecision` 的类型判别 switch（`predispatch_gate.go:321-326`）根本不含 `project_task_iteration_exhausted`，无论任务处于何种状态都空返回——即便任务真在 `waiting_human` 也放不了行，结论双重成立。

对照 `task_failure_recovery` 有真实的 `ApplyFailureRecoveryDecision`（`project_store.go:2039`，处理 retry / reassign / cancel_downstream），可见这是遗漏而非设计。

**影响**：任何模式下人类都无法从这道闸恢复；`full_auto` 撞上必然烂尾。**优先级最高，与自治无关也要修。**

### 3.2 `full_auto` 白名单半数指向死常量

见 §2.2。`policy_autonomy.go:59-60` 白名单里的 `permission_approval` / `tool_authorization` 在 `EvaluatePreDispatchGate` 中无任何 `addBlocker` 路径。

**影响**：配置面与文档都在宣称「权限与工具授权可由策略自动放行」，实际未发生过一次；而真会挡住自动化的 `project_task_permission` 不在白名单。属于**能力虚标**。

### 3.3 验收面上两个档位行为完全一致（承重）

需求收敛走 `gatedCompletionStatusWithQueries`（`pg_repository.go:6256-6273`）：

```6262:6272:apps/control-plane/internal/project/pg_repository.go
	unsatisfied, err := countUnsatisfiedBlockingCriteriaWithQueries(...)
	if unsatisfied > 0 {
		return ProjectDemandStatusAcceptancePending, nil
	}
	return ProjectDemandStatusCompleted, nil
```

这段**不读 `autonomy_tier`，也不读任何自治策略**。所以：

- 一条 `pause_at_gate` 规则跑出的低风险需求，若判据全被机器释放，**零人类触点直达 `completed`**——用户选的「遇闸暂停」在验收面上没有兑现。
- 一条 `full_auto` 规则跑出的需求，若有 blocking `human_judgment` 判据，**照样停在验收等人**——用户选的「完全自动化」在验收面上没有兑现。

换句话说，验收停不停，今天由「planner 有没有插人类判据」决定（`ensureHumanJudgmentCriterion`，`acceptance_criteria.go:108-130`），而那个注入的主触发器 `planTouchesHighRisk` 读的正是 planner 自报的三个字段（`:155-170`）。

**这就是用户 2026-08-17 拍板要消除的循环论证**，也是 2026-08-07 撤销出口深度缩放时已经否决过一次的形状。

同一段代码还藏着两个独立的坏行为，二者都与档位无关，属于既有缺陷：

- **判据全过时整道闸被跳过**：需求静默 `completed`，不铸卡、不产生决策对象、无 `resolved_by`。这是全平台**唯一**一处仍在「跳过闸」而非母稿 §4.2 要求的「策略放行闸」的地方，等于验收面审计断链——事后无法回答「这单为什么没人验收就完成了」。
- **零判据同样静默完成**：`unsatisfied == 0` 对「一条验收判据都没有」也成立。planner 未产出任何判据的需求**零证据自动完成**，没有任何人选择过这个行为。

这两条是 §6 重新定义 `auto` 档的直接依据。

另有三个把「机器可判」放得过宽的口子，`auto` 档设计必须先堵：

- **`not_applicable` 是执行者自述且免 attestation**。`validateAcceptanceCriterionAttestation`（`service.go:4298`）只对 `passed` / `human_overridden` 收紧；`not_applicable` 直接跳过，而它在收敛闸里**释放判据**（`demand_acceptance_gate.go:259`）。在无人类判据的计划上，执行者把每条判据标 N/A 即可自动完成。
- **attestation 门是粗粒度的**。`service.go:4310-4312`：只要该 attempt 存在**任意一条**成功 attestation，本任务上**全部**自称绿灯的 `automated_test` 判据一次性放行。它证明的是「这个 attempt 跑过命令」，不是「这条判据成立」。
- **planner 可以直接自报 `human_judgment` 判据，并压住兜底注入**（复审新增第三口子）。planner 提示词要求其为 `plan_acceptance_criteria` 声明 `verification_method`/`severity`（`openai_compatible_planner.go:360`，注册表含 `human_judgment`）；`ensureHumanJudgmentCriterion` 见到已存在的 `human_judgment` 即幂等返回（`acceptance_criteria.go:108-116`），`collapseBlockingHumanJudgment` 只收敛不剔除。也就是说「停不停人」至今仍受 planner 自报影响——§4 只治理了三个风险字段，没治理判据本身。这直接违反 §6.4 的预签不变量，修法见 §6.4。

### 3.4 剧本声明的 `human_gate` 被 `full_auto` 静默覆盖（违反单向阀）

剧本可以声明 `constraints[].kind = human_gate`（可带 `when.exit_at_or_beyond` 限定出口档），治理阶段将其落为目标任务的 `RequiresHumanApproval = true`：

```648:661:apps/control-plane/internal/workflow/projectcoordination/template_governance.go
		case "human_gate":
			step, ok := prunedSet[constraint.Target]
			...
			task.RequiresHumanApproval = true
			plan.ConstraintNotes = append(plan.ConstraintNotes, PlanConstraintNote{
				Kind:    "human_gate",
				Message: fmt.Sprintf("发布任务已强制人类审批：由 human_gate@%s v%d 触发", ...),
			})
```

而 `RequiresHumanApproval` 是派发前 `risk_approval` 闸的唯一触发源（`workflow/predispatch_gate.go:220-228`），该闸又在 `full_auto` 自动放行白名单里（`policy_autonomy.go:57`）。

**后果：剧本作者写下「这一步必须人批」，一条 `full_auto` 规则就把它自动签掉了。** 这直接违反母稿 §5.3 单向阀「剧本上限压过发起面所选档」——剧本的收紧被规则的放宽覆盖，方向反了。

根因与 §3.2 同类：**白名单按「闸的类型」授权，而不是按「这道闸是谁要求开的」**。同一个 `risk_approval` 闸，触发源可能是 planner 随口自报（可自动放行），也可能是剧本作者签字声明（绝不可自动放行），今天二者不可分辨。

修法即 §4.2 第一步的来源标记——分离出来源后，白名单改为按来源授权（§4.2 第五步）。**这是来源标记除验收之外的第二个消费者，也是它必须先做的第二个理由。**

---

## 4. 高风险声明式改造（拍板 2 的落法）

### 4.1 现状：三个字段，两种来源，混在一起

`planTouchesHighRisk`（`acceptance_criteria.go:159-175`）只读三个字段：`plan.RequiresHumanReview`、任一 `task.RequiresHumanApproval`、任一 `task.RiskLevel ∈ {high, critical, 高, 高风险, 严重}`。

但这三个字段有**两种截然不同的来源**：

| 来源 | 写入点 | 可信度 |
|---|---|---|
| **planner 自报** | `decodePlannerJSON`（`openai_compatible_planner.go:414-457`），由系统提示词约定，无 JSON Schema | **不可作闸输入**（拍板 2） |
| **平台判定** | `applyRequiredHumanReviewPolicy`（`:790-797`，项目策略要求）、`ApplyPlanningProfileScores`（`graph_validation.go:158-161`，能力硬失配）、`EnforceScenarioTemplateGovernance`（`template_governance.go:657/691/725`，剧本 `human_gate` 与降级） | **服务端声明式事实，应保留为闸输入** |

**关键实施难点：这两种来源今天写进同一批字段，事后无法区分。** 因此声明式改造的第一步不是删字段，而是**分离来源**。

### 4.2 改造方案

**第一步（承重，无行为变化）：给高风险信号加来源标记。**
在 `RouteDecisionPlan` 上新增与三个既有字段并列的 `RiskAttribution`（或等价结构），记录每个高风险信号是 `planner_self_reported` 还是 `platform_derived:{policy|profile_score|template_governance}`。三个平台写入点同时写标记。planner 解码路径只写 `planner_self_reported`。此步不改任何判定逻辑，只让事实可分辨。

**第二步：`planTouchesHighRisk` 只认 `platform_derived`，并同步收口 `plan_revision_payload.go` 的三条就地判据。**
自报信号降级为**展示与排序**用途（收件箱风险徽标、任务列表排序），不再驱动 `ensureHumanJudgmentCriterion`。
**复审修正**：初稿称此步「也不再驱动 `plan_revision_payload.go` 的 `ReviewRequired`」，系因果错误——`ReviewRequired` 由 `ValidatePlanRevisionPayload` 的三条就地判据独立驱动（`plan_revision_payload.go:195-196` / `:232-234` / `:236-238`，直读 payload 里的自报字段），不经 `planTouchesHighRisk`（`project_store.go:688-694` 注释明示两路无关）。因此第二步必须**同时**把这三条就地判据改为只认 `platform_derived`，否则 planner 自报 `RiskLevel=high` 仍会触发 `plan_review` 闸、再被 `full_auto` 自动签掉——正是本节要消灭的自我拉扯。此改动进 F6。

**第三步：补齐声明式来源，填上自报信号退出后的空缺。**

| 声明位置 | 字段 | 语义 | 约束 |
|---|---|---|---|
| 剧本 spec | 既有 `risk_policy` 扶正 + 新增 step 级 `human_gate`（部分已存在于 template_governance） | 「本打法哪些步骤触达高风险」 | 剧本作者是人，签字即声明 |
| 项目 `coordination_policy` | 既有 `require_human_acceptance` | 「本项目一律要人验收」 | 保留，见 §5 |
| 自动化规则 / 外部集成绑定 | 无需新字段 | 由 `autonomy_tier` 承担 | 受单向阀约束 |

**第四步：把 `autonomy_tier` 接进验收面——落在闸上，不落在判据上。**

> 初稿此处写的是「`pause_at_gate` 时强制注入一条人类兜底判据」。**§6 重写后改为在闸上表态**：验收闸无条件触发，`pause_at_gate` 不放行（§6.5）。这样不必为了停人而伪造一条判据，判据集合保持诚实——它只描述「这单该验什么」，不承担「谁来签」。

方向仍然是单向阀：`autonomy_tier` 只能**加严**，**不能放松**——`full_auto` 不得抑制剧本/项目/平台判定要求的 `human_judgment` 判据（这正是 §6.4 的 E6）。

**第五步：`full_auto` 白名单改为按「谁要求开这道闸」授权，而不是按闸的类型。**

这是 §3.4 的修法，也是来源标记的第二个消费者：

| `risk_approval` 的触发源 | 可否策略自动放行 | 理由 |
|---|---|---|
| planner 自报 `RequiresHumanApproval` | 声明式改造后**此源不再触发闸** | 自报不作闸输入（拍板 2） |
| 剧本 `human_gate` 声明 | **否** | 剧本上限压过规则档（母稿 §5.3） |
| 项目策略 `require_human_review_for_new_demands` | **否** | 项目上限压过规则档 |
| 能力硬失配（`ApplyPlanningProfileScores`） | **否** | 平台判定的结构性缺口，预签策略无从代答 |

推论：**声明式改造做完后，`risk_approval` 闸实际上不再有可自动放行的触发源**。这不是把 `full_auto` 架空——它意味着这道闸从「AI 说要人批、策略说不用」的自我拉扯，变成「只有人声明要批时才开，开了就真要人」。`full_auto` 的价值转移到验收面（§6）与其余闸点上，语义反而更诚实。

**复审已核对坐实**：`budget_approval` 同构——子路①③的触发旗标来自 planner metadata（`workflow/projectcoordination/predispatch_gate.go:848-867`），按本步来源授权同法处理（见 §2.2 拆档表态修正版）。

### 4.3 风险（拍板 5：一步到位，不设观察期）

自报信号退出后，**「planner 识别出、但剧本没声明」的高风险将不再触发人类兜底**。代价是放弃模型的临场判断这一层，收益是消除循环论证并让停不停变得可预测、可配置、可审计。

用户 2026-08-17 明确拍板**不设审计观察期**，声明式一步到位。因此本文初稿建议的「对被降级的自报信号打审计事件、跑一段再定」**作废**。

实施推论（不因取消观察期而放松）：

- 第二步上线**必须**与第三步（补齐剧本声明）**同批**，不得先降级后补声明——否则中间态是「自报退出了、声明还没有」，等于净裸奔一段。
- 上线前须逐个剧本核对：既有剧本里凡是会触达仓库写、发布、外部系统写的打法，都要有对应的 `human_gate` 约束或出口档验收判据声明（§6.5.2）。这份核对清单是 **F6 的前置交付物**，且与 F5 补的剧本字段配套——F5 先给剧本作者表达能力，F6 才敢把自报信号撤掉。

---

## 5. 项目级自治策略键的收敛

`projects.coordination_policy` 今天有四个自治相关键，彼此互不相识：

| 键 | 读者 | 配置面可见 | 语义 |
|---|---|---|---|
| `autonomy_ceiling` | `autonomypolicy.CoordinationPolicyCeiling` | **有** | 跨调用面自治上限（母稿 §5.7） |
| `require_human_review_for_new_demands` | `applyRequiredHumanReviewPolicy` | **有** | 强制 plan 人批，且给全部 task 打 `RequiresHumanApproval` |
| `require_human_acceptance` | `acceptance_criteria.go:146` | **无** | 强制注入人类验收判据 |
| `acceptance_human_judgment_exempt` | `acceptance_criteria.go:135` | **无** | 豁免上一条（不豁免高风险） |

母稿 §5.7 盘点时只看见前两个。**表态：**

- `require_human_acceptance` **保留，语义收窄为「本项目要求人类验收」的声明式来源**（§4.2 第三步表格的一行），即注入 `human_judgment` 判据从而触发 §6.3 的 E6。它**不再**兼任「档位的替身」——档位对验收的作用改由闸上表态承担（§6.5），二者分工清晰不重叠。同时补进配置面：今天它能改变行为却在 UI 上不存在。
- `acceptance_human_judgment_exempt` **退役**：它豁免的是「策略触发的注入」，在声明式改造后注入来源变成剧本声明与档位合成，一个只作用于旧触发器的豁免键没有位置，且它天然与「只能收紧」的单向阀反向。退役走迁移清理，参照母稿 §6.4 的开发环境清理口径。
- `require_human_review_for_new_demands` **保留**，但须修一处：它今天顺手给**每个** task 打 `RequiresHumanApproval`（`openai_compatible_planner.go:790-797`），而该字段又是 `planTouchesHighRisk` 的输入之一——一个「计划要人批」的策略被放大成了「全部任务高风险」。§4.2 分离来源后此放大自然消解（标记为 `platform_derived:policy`，语义清晰），但需在实施时确认没有别的读者依赖这个副作用。

---

## 6. 验收闸 `auto` 档：证据充分性门禁

> **本节于 2026-08-17 复评后重写。** 初稿曾论证「验收 `auto` 档必为橡皮图章、建议撤销」，该论证**有误**，用户驳回。错因记录在 §6.1，因为它是一个容易重犯的推理陷阱。

### 6.1 初稿错在哪（留档防重犯）

初稿的推理是：验收卡的存在条件是「至少一条 blocking 判据未被机器释放」（`demand_acceptance_gate.go:214-264`），所以给卡加 `auto` resolver 只能去签机器判不了的判据 → 橡皮图章。

**漏洞：这个「存在条件」是当前实现的产物，不是规范。** 判据全过时之所以不铸卡，是因为 `gatedCompletionStatusWithQueries`（`pg_repository.go:6256-6273`）在那种情形下**直接跳过了整道闸**，让需求静默 `completed`。

而母稿 §4.2 的承重句是「闸照触发、决策对象照产生，只是 `resolved_by` 从某个人变成 `policy:{...}`」。**验收因此是全平台唯一一处仍在「跳过闸」而非「策略放行闸」的地方——它本身就违反母稿 §4.2，是一处审计断链。** 初稿把这个违规状态当成正确基线，于是论证出了「合规方案不存在」。

**顺带暴露的更严重事实**：`unsatisfied == 0` 的判定对「一条判据都没有」同样成立。planner 未产出任何验收判据的需求，今天**零证据自动完成**，没有任何人选择过这个行为。这才是真正的橡皮图章，而且正在发生。

### 6.2 `auto` 档的正确定义

> **验收闸对所有受策略治理的需求无条件触发**（不再在判据全过时跳过）。是否放行由**证据充分性门禁**判定：全过则策略放行并写 `resolved_by=policy:{rule_id}`，任一不过则升级 `human`（park 进收件箱）。

这逐字兑现了母稿 §4.3 特别条款「机器判据全过才自动通过，任一不过升级 human」，且**同时收紧了两个方向**：

| 情形 | 今天 | `auto` 档 |
|---|---|---|
| 零验收判据 | **静默完成** | **升 human**（无证据不等于通过） |
| 判据全过 | 静默完成，**无决策记录** | 完成 + 决策对象 + `resolved_by=policy:{rule_id}` |
| 执行者全标 `not_applicable` | 静默完成 | **升 human** |
| 有声明式来源的 `human_judgment` 判据 | 停人 | 停人（不变）；planner 自撰的不停（§6.4） |

### 6.3 证据充分性门禁（E1–E6，全部机器可判、服务端可独立复算）

| # | 条件 | 不过时 | 依据 |
|---|---|---|---|
| **E1** | 至少一条**阻断性机器判据**存在且已释放（`automated_test` 带真实 attestation / `adversarial_review` 判 `satisfied` / 平台生成的交付物判据） | 升 human | 消除「零证据自动完成」 |
| **E2** | 需求全部任务声明的 `produces` 均已交付 | 升 human | `task_result_contract.go:533-547` 的 `handoff_deliverable_missing` 上提到需求层 |
| **E3** | 无任何阻断判据处于 `unsatisfied` | 升 human | 机器判过且判为否，不得覆盖 |
| **E4** | 无任何阻断判据处于「未判完」（无 verdict / `review_gate` 占位 `pending` / 对抗复核 `escalate_human` 或引擎错误） | 升 human | **未判完 ≠ 判过了**，初稿把这两者混为一谈 |
| **E5** | `not_applicable` 不计入 E1，且不得覆盖全部阻断判据 | 升 human | N/A 是执行者自述且免 attestation（§3.3） |
| **E6** | 无**声明式来源**的阻断性 `human_judgment` 判据（`planner_authored` 不计，见 §6.4） | 升 human | 见 §6.4 |

E1/E2/E3 三项正好绑定母稿 §4.3 特别条款点名的三样：任务结果契约、交付物门禁、对抗性复核。

### 6.4 E6 的判据句：预签不变量（2026-08-17 复审重写）

**预签即审核**（用户复审拍板）：创建自动化规则 / 外部集成绑定时的选档动作本身就是人类审核——若执行期还要逐单问人，自动化就没有意义。`full_auto` 的语义是「创建者已代未来签掉**自己有权处置的事**」，闸照铸、`resolved_by=policy:{rule_id}`。

由此得出 E6 的判据句，也是本文的承重不变量：

> **一条预签规则在运行时遇到的每一道停人闸，都必须是创建时已确定的；凡是创建时不可预见的停人，都是缺陷。**

用不变量检验「谁有权让预签规则停人」——只有**创建时已确定的签字**才算：

| 来源 | 创建时可见？ | 处置 |
|---|---|---|
| 剧本声明本打法高风险（含出口档 `human_judgment` 判据） | 是——绑定哪个剧本是创建者自己选的 | 剧本上限压过规则档（母稿 §5.3 单向阀），**必停人** |
| 项目 `require_human_acceptance` | 是——项目策略在配置面 | 项目上限压过规则档，**必停人** |
| 平台判定（能力硬失配 / 剧本治理 `human_gate`） | 是——服务端声明式事实 | **必停人** |
| **planner 自撰 `human_judgment` 判据（§3.3 第三口子）** | **否——运行时才出现** | **不是签字，E6 不计**；降级为展示告警或转机器判据（如 `adversarial_review`），**不得转停人** |

初稿「注入来源只剩三种」的说法有漏：planner 提示词本就要求其为判据声明 `verification_method`（§3.3 第三口子），自撰的 `human_judgment` 至今直通判据集。修法与 §4.2 第一步同构——**给 `PlanAcceptanceCriterion` 加来源标记**（`template_declared` / `platform_injected` / `planner_authored`），E6 只认前两种；并在 F5 把「spec 判据折叠」从 planner 手里收归**服务端执行**（实例化与折叠两个入口统一打 `template_declared`，同时杜绝 planner 在折叠时篡改模板判据的 statement/severity）。

所以：声明式来源的 `human_judgment` 让 `full_auto` 需求停人，不是 auto 档没生效，是单向阀按设计生效；而 planner 自撰的 `human_judgment` 若也停人，则是不变量被违反——两类停人从此可分辨、分别对待。

### 6.5 出口档细分（拍板 D，母稿 §5.6 留白兑现）

用户 2026-08-17 拍板：**不做全局一刀切，直接按 exit 档细分**——浅档收口自动完成，深档收口停人。

#### 6.5.1 好消息：声明式原语已经存在，且已在消费

盘点发现所需机器**基本齐备**，D 因此不比 A/B 更费工，而且更正确：

| 原语 | 位置 | 现状 |
|---|---|---|
| 出口深度序 | `spec.Exits[]` 数组位置；`SpecV2.ExitIndex`（`spec.go:119-128`） | 已有 |
| 出口档条件式 | `constraints[].when.exit_at_or_beyond`（`spec.go:80-84`），求值 `ExitCondMet` | 已有，注册校验（`spec.go:435-447`） |
| 出口档人类卡点 | `constraint.kind = human_gate` + `when`（`template_governance.go:648-661`） | 已有，已实跑 |
| **出口档验收判据** | `default_acceptance_criteria[].applies_from_exit`（`spec.go:98-101`），实例化 `template_instantiate.go:148-170` | **已有，但判据方法被写死** |
| 出口选择固化 | planner 选 `exit_deliverable`，`pinned_exit_deliverable` 可强制（提示词 `openai_compatible_planner.go:364`） | 已有 |

#### 6.5.2 唯一缺的一块：判据方法写死为 `automated_test`

```163:169:apps/control-plane/internal/workflow/projectcoordination/template_instantiate.go
		criteria = append(criteria, PlanAcceptanceCriterion{
			ID:                 id,
			Statement:          criterion.Statement,
			SatisfiedBy:        satisfied,
			VerificationMethod: VerificationMethodAutomatedTest,
			Severity:           "blocking",
		})
```

因此剧本今天**无法声明「从某个出口档起，验收要人签」**——它只能声明机器判据。补上 `SpecAcceptanceCriterion.VerificationMethod` / `Severity` 两个字段（默认值保持今天行为，零破坏），剧本作者即可写：

```json
{
  "statement": "发布结果由项目负责人确认",
  "applies_from_exit": "release_record",
  "verification_method": "human_judgment"
}
```

复审核验：`normalizeCriterionDefaults`（`acceptance_criteria.go:74-89`）把空 `VerificationMethod`/`Severity` 归一为 `automated_test`/`blocking`，恰等于今天写死的两个值——补 `omitempty` 字段后存量模板逐字节等价，零破坏成立。另须在 `ParseSpec`/注册侧补判据枚举校验（今天不校验，非法值要到运行期才暴露），随 F5。

#### 6.5.3 关键性质：出口档细分**不需要**在闸里加任何逻辑

补完上面一个字段后，出口档语义**完全由既有 E1–E6 自动兑现**，验收闸本身保持简单：

| 收口深度 | 剧本声明 | 判据集合 | E6 | 结果 |
|---|---|---|---|---|
| 浅档（如 `branch_ref`） | 该条 `applies_from_exit` 未命中 | 无 `human_judgment` | 通过 | **证据充分即自动完成** |
| 深档（如 `release_record`） | 命中，注入 `human_judgment` | 有 `human_judgment` | 不通过 | **停人签** |

`ExitCondMet` 已经在做深度比较，`ensureHumanJudgmentCriterion` 的「已有 human_judgment 就不重复注入」也天然兼容。**出口档细分不是给闸加矩阵，是让剧本把话说清楚**——这正是母稿 §5.6 留白时预设的答案（「答案是挂 exit 档，不是给规则开闸点矩阵」）。

#### 6.5.4 `pause_at_gate` 在 D 下的定义

档位不再单独决定验收停不停，**剧本声明是主，档位是「剧本没声明时怎么解释」**：

| | 剧本声明了该档要人签 | 剧本未声明 |
|---|---|---|
| `full_auto` | **停人**（E6，单向阀：剧本压过规则档） | 证据充分（E1–E5）即自动完成 |
| `pause_at_gate` | **停人** | **停人**（保守解） |

这个结构有一个好的激励：**想减少验收卡，要么显式选 `full_auto`，要么回去把剧本的出口档写清楚**——两条路都指向「有人签字声明」，与拍板 2 的方向一致。而 §7.2 原先担心的收件箱负荷，由剧本作者按打法自行控制，不再是平台一刀切。

#### 6.5.5 与 2026-08-07 出口深度撤销（`1e0494b3`，BREAKING）的关系

出口深度缩放曾被撤销过一次，本节**不是翻案**，三点区分必须写明，实施会话不得混淆：

| 维度 | 被撤销的那套 | 本节 |
|---|---|---|
| 作用的闸 | **计划确认**（`plan_review`） | **需求验收**（`demand_acceptance`） |
| 谁判定深浅 | 判据混入 planner 自报的 `RequiresHumanReview` / `RiskLevel` | **剧本作者声明 `applies_from_exit`**（人签字） |
| 与自治开关的关系 | plan 模式隐式自动派发 = 第二套自治机制，与 loop 模式重复 | 挂在 `autonomy_tier` 单向阀**内部**，不是平行机制 |

撤销时的人类决策原话是「很难判定哪些是高风险、哪些是低风险。**这应该由人类来决策，不应该由 AI 来决策**」——本节正是按这句话落的：判定权交给剧本作者这个人，不交给 planner。

**明确不动**：`coordination_mode=plan` 的计划确认**仍然无条件停人**。那是一条 BREAKING 标记的人类决策，本节不触碰，实施时也不得顺手放开。

另注：`template_governance.go:362-364` 的源码注释已指明「Phase A Task 4 的 `human_checkpoint` 字段未落地，落地后此处应改为 key 在它上面而非 `human_gate`」——本节即该 TODO 的兑现路径，实施时一并收口。

#### 6.5.6 预签完整性与 exit 档漂移（复审新增）

§6.4 不变量照出的第二个漂移源：**exit 档是 planner 运行时选的**。提示词只说「prefer the SHALLOWEST exit」（`openai_compatible_planner.go:364`），不是强制；F5 之后深出口会带 `human_judgment` 判据——「要不要人签」实际由 planner 的临场选择决定，创建者预签的是一个移动靶。

修法两条，均落在创建面：

1. **`full_auto` 规则绑定多出口剧本时，创建时必须 `pinned_exit_deliverable`，或显式确认分档语义**（「浅于 X 档自动、深于 X 档停人」）——`pinned_exit_deliverable` 原语已存在，复用 §6.5 的出口档深度序即可。
2. **创建/编辑规则时做兼容性预检**：按剧本 spec 展开各出口档会不会停人（哪些出口带 `human_gate` / `human_judgment` 判据），让「预审」发生在配置面，而不是事后收件箱。

这与 §6.4 完全咬合：E6 认的每种停人来源在创建时都可见，创建面把它们亮出来让人确认；运行时才冒出来的（自报风险、自撰判据、临场选深档）一律不算数。落点进 F6（依赖 F5 的出口档判据字段）。

### 6.6 保留的对抗复核路径

`adversarial_review` 判据由平台治理注入（`template_governance.go:346-352`），由 LLM 判官聚合裁决，执行者无法自判（`demand_acceptance_gate.go:221-236`），`escalate_human` 与引擎错误一律 HOLD。**这条路径的设计是对的，保持不动**，它是母稿 §4.3 特别条款里「对抗性复核」的既有兑现，并作为 E1 的合格证据来源之一。

### 6.7 实施注意

- **铸卡即放行的开销**：`full_auto` 需求会出现「铸卡→随即策略放行」的短暂状态，与 `plan_review` 在 `full_auto` 下的既有形状同构（母稿 P2 已实跑）。收件箱投影须确认已解决卡不进待办列表，避免闪烁。
- **`gatedCompletionStatusWithQueries` 是承重改动点**：它今天是「算完直接给状态」，改造后需要区分「进 `acceptance_pending` 并铸卡」与「策略放行后进 `completed`」两条路径，且必须幂等（协调线程会重复调用）。
- **非策略治理的需求（人工发起）不受本节影响**：无 `autonomy_tier` 可读时保持今天行为，避免把人工发起的需求一并拖进强制验收。此边界须在实施时写成显式判据并加测试。
- **tier 读取机制（复审补）**：`autonomy_tier` 今天只存在 automation 规则 / 外部集成绑定 / 项目与模板 ceiling 上，**demand 自身没有快照**，验收闸无从读起。F4 须在需求创建时把**有效档**（复用 `effectiveAutomationAutonomy` 的合成逻辑：规则档 × 模板 `AutonomyCeiling` × 项目 `autonomy_ceiling`）快照到 demand 上，验收闸只读快照——规则事后变更不影响在途需求；人工发起（无快照）走今天行为。

---

## 7. 拍板记录

### 7.1 已拍板（用户 2026-08-17，逐条不得静默偏离）

| # | 问题 | 结论 |
|---|---|---|
| 1 | 是否撤销「验收 `auto` 档」这一期 | **不撤销**。用户驳回初稿论证，要求给出非橡皮图章的做法 → §6 重写为证据充分性门禁；错因留档在 §6.1 |
| 2 | 是否新增 `auto_recheck` 档 | **采纳**。三档扩为四档（§2.1），恢复类闸由服务端探针复检放行 |
| 3 | 是否接受「planner 识别出但剧本没声明的高风险不再触发人类兜底」 | **直接接受，不设审计观察期**。声明式一步到位；§4.3 的观察期建议作废 |
| 4 | 验收闸无条件触发后，`pause_at_gate` 与负荷怎么平衡 | **按 exit 档细分**（母稿 §5.6 留白兑现）——浅档收口自动完成、深档收口停人，判定权归剧本作者。落法见 §6.5；原语基本已存在，只缺 `SpecAcceptanceCriterion` 字段与折叠归属（复审补，见 §6.5.2/§6.4） |

复审增补拍板（用户 2026-08-17 复审对话，同日）：

| # | 问题 | 结论 |
|---|---|---|
| 5 | planner 自撰 `human_judgment` 判据能否拦预签规则 | **不能**。预签即审核——创建时的选档就是人类审核；自撰判据不是签字，E6 剔除 `planner_authored`，判据加来源标记（§6.4） |
| 6 | 运行时选 exit 档导致预签漂移 | 创建时 pin exit 或显式确认分档语义 + 配置面兼容性预检（§6.5.6） |
| 7 | 复审发现的事实错误（§4.2 第二步因果错误、§2.2 budget 子路归类、行号/日期漂移） | 按复审结论回写本文；`plan_revision_payload.go` 三条就地判据与 budget 子路①③纳入 F6 |

### 7.2 剩余开放问题

实施层面无。**本批明确不做**（不静默丢弃，均已记 TODO.md）：① `upstream_supplement_review` 的 `auto` 放行——缺「剧本声明允许自动补链」的表达能力，另立一期；② `planning_failed` / `task_failure_recovery` 的 retry `auto_recheck`（有限次重试）；③ transfer request 运维死角实跑复核（§2.4）；④ `review_gate` pending 的独立告警。

### 7.3 实施会话须先读的五条硬约束

1. **`coordination_mode=plan` 的计划确认仍无条件停人**（`1e0494b3` BREAKING 人类决策）。§6.5 只作用于验收闸，不得顺手放开计划确认（§6.5.5）。
2. **声明式第二步与第三步必须同批上线**（§4.3），不得先降级自报信号再补剧本声明。
3. **F3 必须先于 F4**（§8），先让「机器判过了」可信，再让闸无条件触发。
4. **F2 必须先于 F6**（§8），来源不可分辨时白名单按来源授权无从下手。
5. **F2 亦须先于 F4**——E6 只认声明式来源（§6.4），前提是判据来源已可分辨（F2 打标）。

---

## 8. 实施顺序与验收判据

分期原则：**先修缺陷（与自治无关也该修），再补可分辨性，最后动语义。**

| 期 | 内容 | 验收判据 |
|---|---|---|
| **F0** | 修 §3.1 迭代耗尽死胡同卡 | 真实链路：诱发迭代耗尽 → 卡出现 → 批准后下游从 `blocked` 恢复推进（今天恢复不了） |
| **F1** | 清理 §3.2 死常量：退役 `permission_approval` / `tool_authorization` / `replan_decision`，白名单同步 | 代码内无未铸出的动作常量；`policy_autonomy` 白名单每一项都能在门禁里找到 `addBlocker` 路径 |
| **F2** | §4.2 第一步：高风险信号来源标记（无行为变化）+ **判据来源标记**（`PlanAcceptanceCriterion` 增 `template_declared` / `platform_injected` / `planner_authored`，§6.4，同样无行为变化）。**三个下游消费者的共同前置**：验收 E6（F4）、白名单按来源授权（§3.4）、服务端折叠（F5） | 单测：三个平台写入点各自打出正确来源；planner 解码只打自报；判据三来源各打对标记；`planTouchesHighRisk` 行为逐字节不变 |
| **F3** | §6.3 证据充分性门禁的**判据侧**：E5（N/A 不算机器释放）、attestation 细化到判据粒度、E2 交付物判据上提到需求层 | 真实链路：执行者全标 N/A 的需求不再自动完成；声明 `produces` 未交付的需求不再自动完成；伪造 attestation 仍拒（已有护栏，回归即可） |
| **F4** | §6.2 证据充分性门禁的**闸侧**：验收闸无条件触发 + 策略放行 + `resolved_by=policy:{rule_id}`；E6 只认声明式来源（消费 F2 判据标记，§6.4）；`gatedCompletionStatusWithQueries` 改造 + **demand 创建时快照有效档**（§6.7 复审补） | 真实链路：零判据需求**不再自动完成**（今天会）；`full_auto` 证据充分需求完成且**有决策记录可回放**（今天无记录）；`pause_at_gate` 判据全释放的需求**停人签**（今天静默完成——预期修复，须显式测试）；planner 自撰 `human_judgment` 的 `full_auto` 需求**不停**（§6.4）；人工发起需求行为不变 |
| **F5** | §6.5 出口档细分：`SpecAcceptanceCriterion` 增 `verification_method` / `severity`（默认经 `normalizeCriterionDefaults` 保持今天行为）；**spec 判据折叠收归服务端执行**并打 `template_declared`（§6.4，planner 提示词移除折叠指令）；`ParseSpec` 补判据枚举校验；剧本编辑器同步；`template_governance.go:362-364` 的 `human_checkpoint` TODO 收口 | 真实链路：`software_delivery` 模板声明「`release_record` 档起要人签」→ 选浅档 `branch_ref` 的 `full_auto` 需求**自动完成**、选深档 `release_record` 的同规则需求**停人签**。这是 D 的判别性证据，浅深两档必须都跑（参照 `1e0494b3` 记下的方法学教训：单出口模板验不出差异）；服务端折叠产物与 spec 判据逐条一致（含 severity，防 planner 折叠时篡改） |
| **F6** | §4.2 第二三四五步 + §5 策略键收敛。**第二三步同批**（§4.3）。复审扩围：`plan_revision_payload.go` 三条就地判据同改来源授权（§4.2 第二步修正）；budget 子路①③来源授权（§2.2）；**exit pin 与创建时兼容性预检**（§6.5.6，依赖 F5） | 真实链路：剧本 `human_gate` 声明的任务在 `full_auto` 下**仍停人**（今天被自动签掉，§3.4）；planner 自报高风险 → **不停**（拍板 3）；`full_auto` 规则绑多出口剧本未 pin exit → **创建被拦 / 要求确认**（§6.5.6）；`acceptance_human_judgment_exempt` 退役后无残留读者 |
| **F7** | §2.1 `auto_recheck` 档落地（runtime/dispatch 恢复类；planning retry 的 `auto_recheck` 另立一期，§7.2） | 真实链路：停 runtime 制造 `runtime_recovery` → 恢复 runtime → 无人干预自动继续；复检不过退回 `human` 的反向路径也要跑 |

四条排序约束：

- **F3 必须先于 F4**——闸侧无条件触发之前，得先让「机器判过了」这句话可信，否则只是把一个宽口径的自动通过从静默变成有记录。
- **F2 必须先于 F6**——来源不可分辨时，§3.4 的白名单按来源授权无从下手。
- **F5 必须先于 F6**——F5 给剧本作者「按出口档声明要不要人签」的表达能力，F6 才敢把 planner 自报信号撤掉。反序上线会出现一段「自报退了、剧本还没法声明」的裸奔期，这正是 §4.3 取消观察期后必须靠排序守住的东西。
- **F2 亦须先于 F4**——E6 只认声明式来源（§6.4），前提是判据来源已可分辨（F2 打标）。

每期须按宪法「默认完成条件是真实端到端」执行，并记录 `control-plane` / `web` / `runtime` 的 pid 与 `owner=` 以证明验证期内服务未被接管。

---

## 9. 与母稿的关系

| 母稿章节 | 本文处置 |
|---|---|
| §4.1 闸点清单 | **替换**为本文 §2（19 个 decision_type + 不铸卡持有） |
| §4.3 三档 resolver | **扩为四档**（新增 `auto_recheck`，§2.1）；「机器可判」口径**替换**为 Q1/Q2 两问 |
| §4.3 自动验收特别条款 | **保留并给出可执行落法**——本文 §6.3 的 E1–E6 即该条款的判据化；三样点名证据（结果契约 / 交付物门禁 / 对抗复核）分别对应 E1/E2/E3 |
| §4.5 禁止 planner 自报作闸输入 | **强化并给出落法**——见 §4；观察期建议按拍板 3 作废 |
| §5.7 项目策略上限落点 | **补齐**遗漏的两个键，见 §5 |
| §9 分期表最后一期 | **保留该期，重定义内容**——从「给验收卡加 resolver」改为「验收闸无条件触发 + 证据充分性门禁」，拆进本文 §8 的 F3/F4 |
| §4.2「闸照触发、决策对象照产生」 | **发现唯一违反处并修复**——验收今天是跳过闸而非策略放行闸（§6.1），F4 修 |
| §5.3 单向阀 | **发现一处反向违反并修复**——剧本 `human_gate` 声明被 `full_auto` 白名单静默覆盖（§3.4），F6 修 |
| §5.6「按 exit 档细分策略上限」留白 | **兑现**——本文 §6.5，且落在验收闸而非规则闸点矩阵，与留白当时预设的答案一致 |
| （无母稿对应）预签不变量 / 判据来源标记 / exit pin 预检 | **复审新增**（2026-08-17）——§6.4、§6.5.6；补上母稿 §4.5「自报不作闸输入」在判据面与规则创建面的两处漏网 |
| P0–P5 已落地部分 | **不动** |

另与 `1e0494b3`（2026-08-07 撤销出口深度缩放，BREAKING）的关系见 §6.5.5：本文**不翻案**，作用闸不同、判定权归人不归 AI、且挂在单向阀内部。计划确认的无条件停人保持不变。

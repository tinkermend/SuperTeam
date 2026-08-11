# 项目状态分层与待办投影（领域不变量）

日期：2026-07-29  
范围：Control Plane 项目域 + Console 项目管理/验收队列  
状态：已落地（写路径与 UI 投影）；与实现同步，勿用历史脏数据反推规范。

## 1. 分层（不得要求字符串一致）

| 层 | 权威对象 | 字段 | 回答的问题 |
|---|---|---|---|
| 项目生命周期 | `projects` | `status` | 容器是否可调度 / 是否验收 / 是否归档 |
| 需求流程 | `project_demands` | `status` | 业务需求闭环到哪一步 |
| 项目任务执行 | `project_tasks` | `status` | 执行单元能否跑、是否等人、是否终态 |
| 人类决策 | `project_decision_requests` / inbox | `status_snapshot` | 人现在要不要点一张卡 |
| 注意力投影 | 前端 `deriveProjectRiskSummary` / `buildProjectRiskSummaryFromCounts` | reasons / **关注摘要**（列表态计数桶） | 驾驶舱怎么扫读 |

**不变量**：合法情况下，项目 / 需求 / 任务的状态字符串**不必**相同；一致性指投影链收敛，不是三字段同值。

## 2. 投影链

```text
project_task 状态变更
  →（同事务）RecomputeDemandStatus（只前进）
  → 全需求终态后项目可 running→acceptance
  → 人类项目验收通过 → archived

waiting_human 任务
  → 必须可解析到 open decision（waiting_request_id 或 task 上 open 决策）
  → 决策决议后 release/fail 任务（不得留下 orphan）

UI「待办」
  → 仅可行动项：open decision / waiting_human 任务 / 失败待清理 / 协调异常
  → 证据核验、SLA 为「其它信号」，不计入「N 项待处理」杂烩
  → 已归档项目：注意力清空（不刷历史证据）
```

## 3. 执行尝试预算 `max_attempts`

- 任务字段 `project_tasks.max_attempts`：单任务覆盖。
- 平台默认：系统配置 `project_task.default_max_attempts`（默认 3，范围 1–5）。
- 创建时写入具体值；失败恢复用 `EffectiveProjectTaskMaxAttempts`（任务值 > 系统配置 > registry 3）。
- 仅对瞬时可重试失败与派发恢复计数；用尽后 `waiting_human` + 决策卡。

## 4. orphan `waiting_human`（禁止类）

**禁止**：`status=waiting_human` 且没有 open decision 可行动。

- 写路径：失败停泊与 decision 同事务，并绑定 `waiting_request_id`。
- 兜底：看门狗 `SweepOrphanWaitingHumanProjectTasks` 补绑已有 open 决策，否则补建决策卡。

## 5. 中文词表纪律

- `pending`/`open` 全局可译「待处理」，但**不得**用单一「N 项待处理」混合多源 reasons。
- 证据 `verification_status` 展示为「核验·…」；工件保留为「保留未决」等，不与业务待办混名。
- 词表事实源：`apps/web/src/lib/status-labels.ts`。

## 6. 相关实现指针

- 风险投影：`apps/web/src/features/projects/project-risk.ts`
- 任务详情 attempts / orphan：`project-task-detail-dialog.tsx`
- 失败停泊：`project.Service.FailProjectTaskAttempt` + `RecoverProjectTaskAttemptFailureWriteback`
- 默认 attempts：`systemconfig.KeyProjectTaskDefaultMaxAttempts`、`project/max_attempts.go`
- 看门狗：`app/stuck_task_reconciler.go` → `SweepOrphanWaitingHumanProjectTasks`

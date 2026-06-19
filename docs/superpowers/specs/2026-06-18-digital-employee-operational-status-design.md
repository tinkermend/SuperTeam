# Digital Employee Operational Status Design

## Context

SuperTeam already separates durable facts across several layers:

- `digital_employees.status` describes the employee lifecycle and configuration state.
- `task_runs.status` describes a concrete execution run.
- `project_tasks.status` describes business task progress in a project or workflow graph.
- `runtime_nodes` and `runtime_capabilities` describe Runtime and Provider availability.
- approval and decision records describe pending human action.

The Console needs a single operator-facing status for a digital employee, but that status must be derived from these facts instead of replacing them. This design defines the first shared Control Plane read-model contract for that derived status.

## Goals

- Provide one consistent operational status across employee overview, employee detail, and project workflow surfaces.
- Keep run, task, Runtime, Provider, and approval facts independent and auditable.
- Distinguish current execution failures from configuration or availability problems.
- Make "idle" mean the employee can accept work now, not merely "has no active run".
- Preserve root-cause reasons when a higher-priority state, such as human confirmation, becomes the next action.
- Reuse the same reason vocabulary for project-level waiting states such as project acceptance without incorrectly assigning that blocker to every employee.

## Non-Goals

- Do not replace existing `employee.status`, `task_runs.status`, `project_tasks.status`, Runtime node status, or Provider capability status.
- Do not introduce database enum types for the new operational status.
- Do not build an event-sourced status projection in the first version.
- Do not make Console pages compute their own divergent status mappings.

## Chosen Approach

Use a Control Plane read-model resolver named `DigitalEmployeeOperationalStatusResolver`.

The resolver derives an operational state from current database facts and returns it through existing employee-facing APIs. The Web Console only renders the returned state and reasons. Future optimization may persist this projection, but the first version should compute it server-side so the source of truth stays centralized and easy to test.

## Status Contract

The resolver returns:

```ts
type DigitalEmployeeOperationalStatus =
  | "waiting_human"
  | "error"
  | "working"
  | "queued"
  | "unavailable"
  | "needs_configuration"
  | "idle";

type DigitalEmployeeOperationalState = {
  status: DigitalEmployeeOperationalStatus;
  label: string;
  reasons: Array<{
    code: string;
    label: string;
    severity: "info" | "warning" | "error";
    source: "human" | "task" | "run" | "runtime" | "provider" | "configuration";
    resource_id?: string;
  }>;
  primary_blocker?: {
    source: "human" | "task" | "run" | "runtime" | "provider" | "configuration";
    resource_id?: string;
    title: string;
  };
  can_dispatch: boolean;
  active_work_count: number;
  queued_work_count: number;
};
```

Labels:

- `waiting_human`: 待人工确认
- `error`: 异常
- `working`: 工作中
- `queued`: 排队
- `unavailable`: 不可用
- `needs_configuration`: 待配置
- `idle`: 空闲

## Priority

When multiple conditions are true, the resolver uses this fixed priority:

```text
waiting_human > error > working > queued > unavailable > needs_configuration > idle
```

This priority is based on the next required operator action. A task failure that has already created a human recovery decision is shown as `waiting_human`, while the failure remains visible in `reasons`.

## Scope Rules

Operational status is employee-scoped by default. A human decision affects an employee's main status only when it is attached to that employee's current or queued work, or when it blocks a task assigned to that employee.

Project-level decisions, such as final project acceptance, should use the same reason vocabulary on project and workflow surfaces, but they must not automatically mark every project employee as `waiting_human`. In employee overview, a project-level acceptance request is only supporting context unless the UI is explicitly showing that employee inside the blocked project scope.

## Status Definitions

### waiting_human

The next required action is human approval, confirmation, supplemental evidence, or recovery selection.

Sources include:

- pending approval requests,
- pending project decision requests,
- route review gates,
- task failure recovery decisions,
- project acceptance reviews when shown in project or workflow context,
- evidence or contract-missing cases that require human input.

This status does not by itself mark a run or task terminal.

### error

The employee has an execution failure or an active execution that is no longer trustworthy.

Sources include:

- run `failed` or `timed_out`,
- project task `failed` when no higher-priority human recovery action is pending,
- Provider spawn failure, missing binary, permission error, non-zero exit, or unrecoverable Provider error,
- Runtime offline while the employee has active or queued work already assigned,
- retry policy exhausted after transient Provider errors.

Runtime offline during active work is first an operational error in the read model. The underlying run or task should be terminalized only after the recovery or lease window expires and the system can no longer confirm a valid result.

### working

The employee is actively occupying execution capacity.

Sources include:

- run `running`,
- run `cancelling`,
- Provider transient retry inside the retry policy window.

`cancelling` remains `working` until Runtime writes back `cancelled`, `failed`, or `timed_out`.

### queued

The employee has already been selected for work, but Provider execution has not truly started.

Sources include:

- project task `planned` or `assigned` with this employee selected,
- run `queued`,
- run `dispatching`.

Project task `pending` is still intake or pre-dispatch bookkeeping and must not mark the employee as `queued`. Project task `blocked` is dependency or governance blockage and should surface through project/workflow blocked context, `waiting_human`, or `error` when a higher-priority employee-scoped fact exists, not through the employee-level `queued` badge.

Details should distinguish "待分派" from "已下发", but the employee-level main status is `queued`.

### unavailable

The employee has no current non-terminal work, but cannot be dispatched because the execution environment is unavailable.

Sources include:

- Runtime node offline, disabled, or archived,
- Provider capability unavailable or unhealthy,
- execution instance disabled or error.

This is not the same as `error` because no current task has failed.

### needs_configuration

The employee has no current non-terminal work, but is not dispatchable because required setup is missing or not approved.

Sources include:

- missing Runtime binding,
- missing or stale effective config,
- pending config approval,
- missing workspace or agent home prerequisites,
- governance configuration not approved.

### idle

The employee has no non-terminal work and can be dispatched now.

Required conditions:

- employee lifecycle status is `ready` or `active`,
- effective config is approved,
- execution instance is `ready` or `active`,
- Runtime is online and not disabled or archived,
- Provider is available and healthy,
- workspace prerequisites are satisfied,
- no active, queued, waiting-human, or failed-current work is attached.

## Mapping Matrix

| Scenario | Main Status | Reason Codes | Fact Handling |
| --- | --- | --- | --- |
| Pending approval, decision, recovery request, or evidence request | `waiting_human` | `approval_pending`, `decision_pending`, `recovery_required`, `evidence_required` | Keep underlying run or task state unless the owning workflow changes it. |
| Task failed and created human recovery decision | `waiting_human` | `task_failed`, `recovery_required` | Main status follows next action; failure stays in reasons. |
| Project enters final human acceptance review | `waiting_human` on project/workflow surfaces | `project_acceptance_pending` | Project moves to `acceptance`, creates `decision_type=project_acceptance`, and waits for human decision. Do not apply this as every employee's primary status. |
| Project acceptance approved | not an employee status | `project_accepted` | Acceptance record status becomes `accepted`; project archives. |
| Project acceptance rejected or needs more evidence | not an employee status | `project_rejected`, `project_needs_more_evidence` | Acceptance record captures the outcome; project reopens to `running` for rework or evidence. |
| Run is `running` | `working` | `run_running` | Keep run and task non-terminal. |
| Run is `cancelling` | `working` | `run_cancelling` | Wait for Runtime terminal writeback. |
| Run is `queued` or `dispatching` | `queued` | `run_queued`, `run_dispatching` | Execution chain has started but Provider has not truly run. |
| Project task is `planned` or `assigned` for this employee | `queued` | `task_planned`, `task_assigned` | Employee has selected work waiting for dispatch or execution. |
| Runtime offline and employee has no non-terminal work | `unavailable` | `runtime_offline` | Do not mutate run or task facts. |
| Runtime offline while employee has active or queued work | `error` | `runtime_offline`, `execution_untrusted` | Show operational error immediately; terminalize after recovery or lease policy expires. |
| Provider unavailable or unhealthy and employee has no non-terminal work | `unavailable` | `provider_unavailable`, `provider_unhealthy` | Do not create a run. |
| Provider spawn failure, missing binary, or permission error | `error` | `provider_spawn_failed` | Current run and task fail. |
| Provider non-zero exit or unrecoverable execution error | `error` | `provider_failed` | Current run and task fail. |
| Provider transient limit or overload inside retry policy | `working` | `provider_retrying` | Keep run and task non-terminal. |
| Provider retry policy exhausted | `error` | `provider_failed` | Current run and task fail. |
| Run is `failed` or `timed_out` and no higher-priority human decision exists | `error` | `run_failed`, `run_timed_out` | Keep terminal facts and expose reason. |
| Config pending, effective config missing or stale, Runtime binding missing, or workspace missing | `needs_configuration` | `config_pending`, `effective_config_missing`, `runtime_binding_missing`, `workspace_missing` | Do not create a run until setup is complete. |
| No non-terminal work and all dispatch prerequisites are satisfied | `idle` | `none` | Employee can accept work immediately. |

## API Shape

### Employee Overview

`GET /api/v1/digital-employees/overview` should add:

- `items[].operational_state`
- `summary.operational_status_counts`

The existing `workbench_status`, `execution_summary`, and `latest_run_summary` may remain during migration, but the UI should prefer `operational_state` for the main employee status.

### Employee Detail

Employee detail APIs should return the same `operational_state` object. The detail page can show richer reasons and linked blockers, but must not recompute the main status differently from overview.

### Project Workflow Surfaces

Project task graphs should continue displaying raw task and run status for graph nodes. Employee chips, inspectors, and assignment panels should reuse operational reason codes when they describe an employee's dispatchability or blocker.

Project-level blockers should also use the reason vocabulary. For example, a pending `project_acceptance` decision is shown as `waiting_human` with `project_acceptance_pending` on the project or workflow instance, while employee overview remains based on employee-scoped work and dispatchability.

## Data Sources

The resolver reads:

- `digital_employees`
- `digital_employee_effective_configs`
- `digital_employee_execution_instances`
- `projects`
- `runtime_nodes`
- `runtime_capabilities`
- `task_runs`
- `project_tasks`
- approval request records
- project decision projections
- latest project acceptance records when evaluating project or workflow context

It should prefer current non-terminal work over latest historical run. A latest failed run does not permanently keep an otherwise healthy and newly idle employee in `error` if there is no current failed work or unresolved recovery action.

## Implementation Boundaries

- Keep Control Plane as the only place that computes operational status.
- Keep Runtime Agent focused on execution and writeback; it should not compute Console employee status.
- Keep Provider adapters focused on normalized events, retry, and terminal result classification.
- Keep Web focused on rendering labels, tones, filters, and blocker links from the API payload.

## Testing

Unit tests must cover the priority order:

```text
waiting_human > error > working > queued > unavailable > needs_configuration > idle
```

Required cases:

- pending approval, pending decision, recovery request, and evidence request each produce `waiting_human`;
- task failure plus recovery decision produces `waiting_human` with `task_failed` reason;
- pending `project_acceptance` produces `waiting_human` on project and workflow surfaces with `project_acceptance_pending`, but does not mark unrelated or idle employee overview rows as `waiting_human`;
- project acceptance decisions map human `approved` to acceptance record status `accepted`, and `rejected` / `needs_more_evidence` reopen the project to `running`;
- run `running` and `cancelling` produce `working`;
- project task `planned` or `assigned` and run `queued` or `dispatching` produce `queued`;
- Runtime offline with no work produces `unavailable`;
- Runtime offline with active or queued work produces `error`;
- Provider unavailable with no work produces `unavailable`;
- Provider spawn failure or unrecoverable execution failure produces `error`;
- transient Provider retry inside policy produces `working`;
- retry exhaustion produces `error`;
- missing binding or unapproved config produces `needs_configuration`;
- a fully dispatchable employee with no non-terminal work produces `idle`.

API tests must verify that employee overview and detail return the same operational state for the same facts.

Workflow or integration tests should verify that raw task and run statuses remain intact while employee-facing status is derived separately.

## Real-Chain Acceptance

Before claiming the implementation complete, verify these states through the running stack:

1. A dispatchable employee with no work shows `idle`.
2. A selected `planned` or `assigned` project task shows `queued`.
3. A running Runtime/Provider execution shows `working`.
4. A pending approval or decision shows `waiting_human`.
5. A pending final project acceptance review shows `waiting_human` on the project or workflow surface with `project_acceptance_pending`, without changing every project employee's overview status.
6. An accepted project acceptance decision archives the project; rejected or needs-more-evidence decisions reopen it to `running`.
7. An unrecoverable Provider failure shows `error`.
8. Runtime offline with no active work shows `unavailable`.
9. Runtime offline during active work shows `error` with `runtime_offline` and `execution_untrusted`.
10. Fixing Runtime, Provider, or config allows the status to move back to `idle`, `queued`, or `working` without stale reasons.

Mock tests, component tests, and build checks are not enough to call this feature complete. The final verification must include the real Control Plane API and, for execution-path states, the Runtime/Provider chain or a clearly stated blocker.

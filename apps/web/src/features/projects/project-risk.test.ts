import { describe, expect, it } from "vitest";
import type {
  Project,
  ProjectDecisionRequest,
  ProjectEvidenceRef,
  ProjectTask,
} from "@/lib/api/projects";
import {
  buildAttentionBreakdown,
  buildRiskCounts,
  deriveProjectRiskSummary,
  formatAttentionHeadline,
  matchesProjectRiskFilter,
  resolveProjectOwnerLabel,
  sortProjectsByRisk,
  type ProjectRiskSummaryMap,
} from "./project-risk";

const tenantId = "tenant-1";
const ownerId = "user-owner-1";

function project(id: string, overrides: Partial<Project> = {}): Project {
  return {
    coordination_policy: {},
    coordination_status: "registered",
    coordination_workflow_id: `project-coordinator:${id}`,
    directory_name: id,
    goal: "完成项目闭环",
    human_owner_user_id: ownerId,
    id,
    name: `项目 ${id}`,
    status: "running",
    tenant_id: tenantId,
    updated_at: "2099-06-29T08:00:00.000Z",
    workspace_ready_status: "ready",
    ...overrides,
  };
}

function task(projectId: string, overrides: Partial<ProjectTask> = {}): ProjectTask {
  return {
    id: `${projectId}-task-1`,
    project_id: projectId,
    requires_human_approval: false,
    status: "running",
    tenant_id: tenantId,
    title: "执行任务",
    ...overrides,
  };
}

function decision(
  projectId: string,
  overrides: Partial<ProjectDecisionRequest> = {},
): ProjectDecisionRequest {
  return {
    approval_request_id: `${projectId}-approval-1`,
    decision_type: "approval",
    id: `${projectId}-decision-1`,
    project_id: projectId,
    status_snapshot: "pending",
    target_user_id: ownerId,
    tenant_id: tenantId,
    title_snapshot: "确认执行风险",
    ...overrides,
  };
}

function evidence(
  projectId: string,
  overrides: Partial<ProjectEvidenceRef> = {},
): ProjectEvidenceRef {
  return {
    evidence_type: "execution_summary",
    id: `${projectId}-evidence-1`,
    metadata: {},
    project_id: projectId,
    source_ref: "artifact://summary",
    source_type: "artifact",
    submitted_by_id: "employee-1",
    submitted_by_type: "digital_employee",
    summary: "等待验收证据确认",
    tenant_id: tenantId,
    title: "执行证据",
    verification_status: "submitted",
    ...overrides,
  };
}

describe("project risk model", () => {
  it("resolves project owner display names from the principal directory when member snapshots are missing", () => {
    const ownerId = "33333333-3333-4333-8333-333333333333";
    const summary = deriveProjectRiskSummary({
      decisions: [],
      evidence: [],
      members: [],
      principalNamesById: new Map([[ownerId, "开发管理员"]]),
      project: project("project-owner-names", { human_owner_user_id: ownerId }),
      tasks: [],
    });

    expect(summary.owner?.id).toBe(ownerId);
    expect(summary.owner?.label).toBe("开发管理员");
    expect(
      resolveProjectOwnerLabel(
        project("project-owner-names", { human_owner_user_id: ownerId }),
        undefined,
        new Map([[ownerId, "开发管理员"]]),
      ),
    ).toBe("开发管理员");
  });

  it("resolves owner label from directory when risk summary still pending without owner", () => {
    const ownerId = "33333333-3333-4333-8333-333333333333";
    expect(
      resolveProjectOwnerLabel(
        project("project-owner-pending", { human_owner_user_id: ownerId }),
        undefined,
        new Map([[ownerId, "开发管理员"]]),
      ),
    ).toBe("开发管理员");
  });

  it("resolves handler display names from the principal directory when member snapshots are missing", () => {
    const summary = deriveProjectRiskSummary({
      decisions: [],
      evidence: [],
      members: [],
      principalNamesById: new Map([["de-reporter-1", "报告员小王"]]),
      project: project("project-names"),
      tasks: [
        task("project-names", {
          assigned_digital_employee_id: "de-reporter-1",
          status: "running",
        }),
      ],
    });

    expect(summary.currentHandler?.label).toBe("报告员小王");
    expect(summary.currentHandler?.id).toBe("de-reporter-1");
  });

  it("keeps the raw id as handler label only when no name source knows the principal", () => {
    const summary = deriveProjectRiskSummary({
      decisions: [],
      evidence: [],
      members: [],
      principalNamesById: new Map([["someone-else", "其他员工"]]),
      project: project("project-unknown-handler"),
      tasks: [
        task("project-unknown-handler", {
          assigned_digital_employee_id: "de-unknown-9",
          status: "running",
        }),
      ],
    });

    expect(summary.currentHandler?.label).toBe("de-unknown-9");
  });

  it("marks pending human decisions as danger and requiring human action", () => {
    const summary = deriveProjectRiskSummary({
      decisions: [decision("project-1", { status_snapshot: "waiting" })],
      evidence: [],
      project: project("project-1"),
      tasks: [],
    });

    expect(summary.level).toBe("danger");
    expect(summary.requiresHuman).toBe(true);
    expect(summary.primaryReason?.type).toBe("human_decision");
    expect(summary.reasons.map((reason) => reason.type)).toContain("human_decision");
  });

  it("marks pending review tasks as waiting-human without open decision cards", () => {
    const summary = deriveProjectRiskSummary({
      decisions: [],
      evidence: [],
      project: project("project-pending-review"),
      tasks: [
        task("project-pending-review", {
          requires_human_approval: false,
          status: "pending_review",
        }),
      ],
    });

    expect(summary.level).toBe("danger");
    expect(summary.requiresHuman).toBe(true);
    expect(summary.primaryReason?.type).toBe("waiting_human");
    expect(summary.reasons).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          source: "tasks",
          type: "waiting_human",
        }),
      ]),
    );
  });

  it("keeps the approval flag as waiting-human while the task is still open", () => {
    const summary = deriveProjectRiskSummary({
      decisions: [],
      evidence: [],
      project: project("project-open-approval"),
      tasks: [
        task("project-open-approval", {
          requires_human_approval: true,
          status: "running",
        }),
      ],
    });

    expect(summary.requiresHuman).toBe(true);
    expect(summary.reasons.map((reason) => reason.type)).toContain("waiting_human");
  });

  it("stops reporting a human decision once the approval task is completed", () => {
    const summary = deriveProjectRiskSummary({
      decisions: [],
      evidence: [],
      project: project("project-done-approval"),
      tasks: [
        task("project-done-approval", {
          requires_human_approval: true,
          status: "completed",
        }),
      ],
    });

    expect(summary.requiresHuman).toBe(false);
    expect(summary.reasons.map((reason) => reason.type)).not.toContain("human_decision");
  });

  it("marks failed tasks as execution failure danger", () => {
    const summary = deriveProjectRiskSummary({
      decisions: [],
      evidence: [],
      project: project("project-2"),
      tasks: [task("project-2", { status: "failed" })],
    });

    expect(summary.level).toBe("danger");
    expect(summary.primaryReason?.type).toBe("execution_failed");
    expect(summary.reasons).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ source: "tasks", type: "execution_failed" }),
      ]),
    );
  });

  it("ignores dismissed failed tasks in risk reasons", () => {
    const summary = deriveProjectRiskSummary({
      decisions: [],
      evidence: [],
      project: project("project-dismissed"),
      tasks: [
        task("project-dismissed", {
          dismissed_at: "2026-06-29T10:00:00.000Z",
          status: "failed",
        }),
      ],
    });

    expect(summary.level).toBe("none");
    expect(summary.reasons.map((reason) => reason.type)).not.toContain("execution_failed");
  });

  it("marks rejected and submitted evidence as evidence-required warnings", () => {
    const summary = deriveProjectRiskSummary({
      decisions: [],
      evidence: [
        evidence("project-3", { id: "evidence-submitted", verification_status: "submitted" }),
        evidence("project-3", { id: "evidence-rejected", verification_status: "rejected" }),
      ],
      project: project("project-3"),
      tasks: [],
    });

    expect(summary.level).toBe("warn");
    expect(summary.primaryReason?.type).toBe("evidence_required");
    expect(summary.reasons.filter((reason) => reason.type === "evidence_required")).toHaveLength(2);
  });

  it("marks abnormal coordination status as runtime or coordination danger", () => {
    const summary = deriveProjectRiskSummary({
      decisions: [],
      evidence: [],
      project: project("project-4", { coordination_status: "lease_lost" }),
      tasks: [],
    });

    expect(summary.level).toBe("danger");
    expect(summary.primaryReason?.type).toBe("runtime_or_coordination");
    expect(summary.reasons).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          source: "project",
          type: "runtime_or_coordination",
        }),
      ]),
    );
  });

  it("does not mark empty stale running projects as SLA waiting", () => {
    const summary = deriveProjectRiskSummary(
      {
        decisions: [],
        evidence: [],
        project: project("project-5", {
          status: "running",
          updated_at: "2026-06-29T07:00:00.000Z",
        }),
        tasks: [],
      },
      { now: new Date("2026-06-29T10:30:00.000Z") },
    );

    expect(summary.level).toBe("none");
    expect(summary.primaryReason).toBeUndefined();
    expect(summary.reasons.map((reason) => reason.type)).not.toContain("sla_waiting");
  });

  it("marks stale waiting-human tasks as SLA waiting using injected now", () => {
    const waitingSince = "2026-06-29T07:00:00.000Z";
    const summary = deriveProjectRiskSummary(
      {
        decisions: [],
        evidence: [],
        project: project("project-5b", {
          status: "running",
          updated_at: waitingSince,
        }),
        tasks: [
          task("project-5b", {
            status: "waiting_human",
            updated_at: waitingSince,
          }),
        ],
      },
      { now: new Date("2026-06-29T10:30:00.000Z") },
    );

    expect(summary.reasons).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          type: "sla_waiting",
          waitingSince,
        }),
      ]),
    );
    expect(summary.waitingSince).toBe(waitingSince);
  });

  it("marks stale pending decisions as SLA waiting using injected now", () => {
    const waitingSince = "2026-06-29T07:00:00.000Z";
    const summary = deriveProjectRiskSummary(
      {
        decisions: [
          decision("project-5c", {
            created_at: waitingSince,
            status_snapshot: "pending",
          }),
        ],
        evidence: [],
        project: project("project-5c"),
        tasks: [],
      },
      { now: new Date("2026-06-29T10:30:00.000Z") },
    );

    expect(summary.reasons).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          type: "sla_waiting",
          waitingSince,
        }),
      ]),
    );
  });

  it("sorts danger before warn before healthy", () => {
    const danger = project("danger", { updated_at: "2026-06-29T07:00:00.000Z" });
    const warn = project("warn", { updated_at: "2026-06-29T09:00:00.000Z" });
    const healthy = project("healthy", { updated_at: "2026-06-29T10:00:00.000Z" });
    const summaries: ProjectRiskSummaryMap = {
      danger: deriveProjectRiskSummary({
        decisions: [decision("danger")],
        evidence: [],
        project: danger,
        tasks: [],
      }),
      healthy: deriveProjectRiskSummary({
        decisions: [],
        evidence: [],
        project: healthy,
        tasks: [],
      }),
      warn: deriveProjectRiskSummary({
        decisions: [],
        evidence: [evidence("warn")],
        project: warn,
        tasks: [],
      }),
    };

    expect(sortProjectsByRisk([healthy, warn, danger], summaries).map((item) => item.id)).toEqual([
      "danger",
      "warn",
      "healthy",
    ]);
  });

  it("uses the same summaries for counts and risk filters as the queue", () => {
    const humanProject = project("human");
    const failedProject = project("failed");
    const evidenceProject = project("evidence");
    const healthyProject = project("healthy");
    const summaries: ProjectRiskSummaryMap = {
      evidence: deriveProjectRiskSummary({
        decisions: [],
        evidence: [evidence("evidence", { verification_status: "rejected" })],
        project: evidenceProject,
        tasks: [],
      }),
      failed: deriveProjectRiskSummary({
        decisions: [],
        evidence: [],
        project: failedProject,
        tasks: [task("failed", { status: "blocked" })],
      }),
      healthy: deriveProjectRiskSummary({
        decisions: [],
        evidence: [],
        project: healthyProject,
        tasks: [],
      }),
      human: deriveProjectRiskSummary({
        decisions: [decision("human", { status_snapshot: "open" })],
        evidence: [],
        project: humanProject,
        tasks: [],
      }),
    };
    const queue = sortProjectsByRisk(
      [healthyProject, evidenceProject, failedProject, humanProject],
      summaries,
    );
    const counts = buildRiskCounts(queue.map((item) => summaries[item.id]));

    expect(counts).toMatchObject({
      all: 4,
      blocked: 3,
      evidence_required: 1,
      execution_failed: 1,
      human_decision: 1,
      waiting_human: 0,
    });
    expect(queue.filter((item) => matchesProjectRiskFilter(summaries[item.id], "blocked")).map((item) => item.id)).toEqual([
      "human",
      "failed",
      "evidence",
    ]);
    expect(queue.filter((item) => matchesProjectRiskFilter(summaries[item.id], "evidence_required")).map((item) => item.id)).toEqual([
      "evidence",
    ]);
  });

  it("does not treat cancelled tasks as execution-failed pending work", () => {
    const summary = deriveProjectRiskSummary({
      decisions: [],
      evidence: [],
      project: project("project-cancelled"),
      tasks: [task("project-cancelled", { status: "cancelled" })],
    });

    expect(summary.reasons.map((reason) => reason.type)).not.toContain("execution_failed");
    expect(summary.level).toBe("none");
  });

  it("splits actionable attention from evidence signals instead of one pending total", () => {
    const summary = deriveProjectRiskSummary({
      decisions: [decision("project-mix")],
      evidence: [
        evidence("project-mix", { id: "e1", verification_status: "submitted" }),
        evidence("project-mix", { id: "e2", verification_status: "rejected" }),
      ],
      project: project("project-mix"),
      tasks: [
        task("project-mix", { id: "t-wait", status: "waiting_human", title: "等人任务" }),
        task("project-mix", { id: "t-fail", status: "failed", title: "失败任务" }),
      ],
    });

    const breakdown = buildAttentionBreakdown(summary);
    expect(breakdown.decisions).toBe(1);
    expect(breakdown.waitingHuman).toBe(1);
    expect(breakdown.executionFailed).toBe(1);
    expect(breakdown.evidence).toBe(2);
    expect(breakdown.actionableTotal).toBe(3);
    expect(breakdown.signalTotal).toBe(2);
    expect(breakdown.allTotal).toBe(5);

    const headline = formatAttentionHeadline(summary);
    expect(headline.hasActionable).toBe(true);
    expect(headline.primary).toContain("1 待决");
    expect(headline.primary).toContain("1 等人");
    expect(headline.primary).toContain("1 失败");
    expect(headline.detail).toContain("2 证据待核");
    expect(headline.primary).not.toContain("项待处理");
  });

  it("labels evidence-only projects as verification signals, not pending work items", () => {
    const summary = deriveProjectRiskSummary({
      decisions: [],
      evidence: [evidence("project-ev-only", { verification_status: "submitted" })],
      project: project("project-ev-only"),
      tasks: [],
    });
    const headline = formatAttentionHeadline(summary);
    expect(headline.hasActionable).toBe(false);
    expect(headline.primary).toBe("1 条证据待核");
    expect(buildAttentionBreakdown(summary).actionableTotal).toBe(0);
  });


  it("suppresses evidence and task noise for archived projects", () => {
    const summary = deriveProjectRiskSummary({
      decisions: [decision("archived-1", { status_snapshot: "pending" })],
      evidence: [
        evidence("archived-1", { verification_status: "submitted" }),
        evidence("archived-1", { id: "e2", verification_status: "rejected" }),
      ],
      project: project("archived-1", { status: "archived" }),
      tasks: [task("archived-1", { status: "waiting_human" })],
    });

    expect(summary.level).toBe("none");
    expect(summary.reasons).toEqual([]);
    expect(summary.requiresHuman).toBe(false);
    expect(formatAttentionHeadline(summary)).toEqual({
      primary: "无待办",
      hasActionable: false,
    });
  });

});

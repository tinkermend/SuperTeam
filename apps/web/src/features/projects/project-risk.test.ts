import { describe, expect, it } from "vitest";
import type {
  Project,
  ProjectDecisionRequest,
  ProjectEvidenceRef,
  ProjectTask,
} from "@/lib/api/projects";
import {
  buildRiskCounts,
  deriveProjectRiskSummary,
  matchesProjectRiskFilter,
  sortProjectsByRisk,
  type ProjectRiskSummaryMap,
} from "./project-risk";

const tenantId = "tenant-1";
const ownerId = "user-owner-1";

function project(id: string, overrides: Partial<Project> = {}): Project {
  return {
    approval_policy: {},
    coordination_policy: {},
    coordination_status: "registered",
    coordination_workflow_id: `project-coordinator:${id}`,
    evidence_policy: {},
    goal: "完成项目闭环",
    human_owner_user_id: ownerId,
    id,
    name: `项目 ${id}`,
    status: "running",
    tenant_id: tenantId,
    updated_at: "2099-06-29T08:00:00.000Z",
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

  it("marks pending review tasks as human decisions without approval flag", () => {
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
    expect(summary.primaryReason?.type).toBe("human_decision");
    expect(summary.reasons).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          source: "tasks",
          type: "human_decision",
        }),
      ]),
    );
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

  it("marks stale running projects as SLA waiting warnings using injected now", () => {
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

    expect(summary.level).toBe("warn");
    expect(summary.primaryReason?.type).toBe("sla_waiting");
    expect(summary.waitingSince).toBe("2026-06-29T07:00:00.000Z");
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
});

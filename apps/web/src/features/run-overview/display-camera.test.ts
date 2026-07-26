import { describe, expect, it } from "vitest";
import { buildDisplayShotQueue, detectNewFailures, shotDwellMs, shotKey } from "./display-camera";
import type { RuntimeOverviewActivityItem, RuntimeOverviewEmployee } from "./runtime-overview-model";
import type { ProjectRunBandOption } from "./runtime-overview-project-lens";

function employee(employeeId: string, status: RuntimeOverviewEmployee["status"], statusSince?: string): RuntimeOverviewEmployee {
  return {
    employeeId,
    teamId: "team-1",
    floorId: "floor-1",
    name: employeeId,
    roleLabel: "工程师 AI",
    status,
    statusReasons: [],
    statusSince,
    recentEvents: [],
    projects: [],
    projectCount: 0,
    artifacts: [],
  };
}

function project(projectId: string, hasActive: boolean): ProjectRunBandOption {
  return {
    projectId,
    name: projectId,
    participantCount: 1,
    runningCount: hasActive ? 1 : 0,
    queuedCount: 0,
    waitingHumanCount: 0,
    failedCount: 0,
    unassignedCount: 0,
    completedTodayCount: 0,
    hasActive,
    source: "summary",
  };
}

function activity(employeeId: string, status: string, occurredAt: string): RuntimeOverviewActivityItem {
  return { employeeId, employeeName: employeeId, teamId: "team-1", label: "运行失败", status, occurredAt };
}

describe("buildDisplayShotQueue", () => {
  it("puts urgent employees first then active projects, skipping idle and inactive", () => {
    const queue = buildDisplayShotQueue(
      [employee("emp-idle", "idle"), employee("emp-working", "working"), employee("emp-error", "error")],
      [project("p-active", true), project("p-quiet", false)],
    );

    expect(queue.map(shotKey)).toEqual(["employee:emp-error", "employee:emp-working", "project:p-active"]);
  });

  it("returns an empty queue when nothing is active", () => {
    expect(buildDisplayShotQueue([employee("emp-idle", "idle")], [project("p-quiet", false)])).toEqual([]);
  });
});

describe("detectNewFailures", () => {
  it("does not alert on the first activity batch", () => {
    expect(detectNewFailures(undefined, [activity("emp-a", "failed", "t1")])).toEqual([]);
  });

  it("reports only newly appeared failed items", () => {
    const first = [activity("emp-a", "failed", "t1"), activity("emp-b", "completed", "t1")];
    const next = [activity("emp-c", "failed", "t2"), ...first];

    const failures = detectNewFailures(first, next);
    expect(failures.map((item) => item.employeeId)).toEqual(["emp-c"]);
    // 同一条失败在后续批次里不再重复插队。
    expect(detectNewFailures(next, next)).toEqual([]);
  });

  it("ignores new non-failed items", () => {
    expect(detectNewFailures([], [activity("emp-a", "completed", "t1")])).toEqual([]);
  });
});

describe("shotDwellMs", () => {
  it("holds project shots longer and alert shots longest", () => {
    const employeeDwell = shotDwellMs({ kind: "employee", employeeId: "e" });
    const projectDwell = shotDwellMs({ kind: "project", projectId: "p" });
    const alertDwell = shotDwellMs({ kind: "employee", employeeId: "e", alert: true });
    expect(projectDwell).toBeGreaterThan(employeeDwell);
    expect(alertDwell).toBeGreaterThan(projectDwell);
  });
});

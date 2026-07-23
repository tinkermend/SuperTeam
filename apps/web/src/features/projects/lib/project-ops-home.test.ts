import { describe, expect, it } from "vitest";
import type { ProjectDemand, ProjectEvent, ProjectTask } from "@/lib/api/projects";
import {
  buildWeekPulse,
  filterOpsEvents,
  resolveTaskMode,
  selectActiveOrBlockedTasks,
} from "./project-ops-home";

const demand = (overrides: Partial<ProjectDemand> = {}): ProjectDemand => ({
  attachments: [],
  id: "demand-1",
  project_id: "project-1",
  reviewer: null,
  source_refs: {},
  source_type: "manual",
  status: "executing",
  submitted_by_user_id: "user-1",
  tenant_id: "tenant-1",
  title: "需求",
  ...overrides,
});

const task = (overrides: Partial<ProjectTask> = {}): ProjectTask => ({
  id: "task-1",
  project_id: "project-1",
  requires_human_approval: false,
  status: "running",
  tenant_id: "tenant-1",
  title: "任务 A",
  ...overrides,
});

describe("project-ops-home helpers", () => {
  it("resolves coordination mode from demand", () => {
    const demands = new Map([
      ["demand-1", demand({ coordination_mode: "loop" })],
    ]);
    expect(resolveTaskMode(task({ demand_id: "demand-1" }), demands)).toBe("loop");
    expect(resolveTaskMode(task({ demand_id: "missing" }), demands)).toBe("plan");
  });

  it("builds a 7-day pulse week with time and mode", () => {
    const now = new Date("2026-07-22T12:00:00"); // Wednesday
    const days = buildWeekPulse({
      demands: [demand({ coordination_mode: "loop" })],
      now,
      tasks: [
        task({
          created_at: "2026-07-22T09:30:00",
          demand_id: "demand-1",
          id: "t1",
          status: "completed",
          title: "联调",
        }),
      ],
    });
    expect(days).toHaveLength(7);
    const today = days.find((day) => day.isToday);
    expect(today?.chips[0]?.mode).toBe("loop");
    expect(today?.chips[0]?.timeLabel).toMatch(/\d{2}:\d{2}/);
  });

  it("selects active or blocked tasks and filters ops events", () => {
    expect(
      selectActiveOrBlockedTasks([
        task({ id: "a", status: "running", updated_at: "2026-07-22T10:00:00" }),
        task({ id: "b", status: "completed", updated_at: "2026-07-22T11:00:00" }),
        task({ id: "c", status: "failed", updated_at: "2026-07-22T12:00:00" }),
      ]),
    ).toEqual([
      expect.objectContaining({ id: "c" }),
      expect.objectContaining({ id: "a" }),
    ]);

    const events: ProjectEvent[] = [
      {
        actor_id: "s",
        actor_type: "system",
        created_at: "2026-07-22T10:00:00Z",
        event_type: "project.created",
        id: "e1",
        payload: {},
        project_id: "p",
        sequence_number: 1,
        tenant_id: "t",
      },
      {
        actor_id: "s",
        actor_type: "system",
        created_at: "2026-07-22T11:00:00Z",
        event_type: "decision.requested",
        id: "e2",
        payload: {},
        project_id: "p",
        sequence_number: 2,
        summary: "待审批",
        tenant_id: "t",
      },
    ];
    expect(filterOpsEvents(events).map((e) => e.id)).toEqual(["e2"]);
  });
});

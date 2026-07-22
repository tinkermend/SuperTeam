import { describe, expect, it } from "vitest";
import {
  buildUpcomingFires,
  countUpcomingFires,
  buildRecentFireActivity,
} from "./automation-dashboard-rail";
import type { AutomationRule } from "@/lib/api/automations";
import { buildNextFireById } from "../schedule-next";

const baseRule: AutomationRule = {
  id: "11111111-1111-1111-1111-111111111111",
  tenant_id: "22222222-2222-2222-2222-222222222222",
  team_id: "33333333-3333-3333-3333-333333333333",
  project_id: "44444444-4444-4444-4444-444444444444",
  project_name: "示例项目",
  name: "工作日巡检",
  enabled: true,
  coordination_mode: "loop",
  schedule_kind: "cron",
  cron_expr: "0 9 * * *",
  timezone: "Asia/Shanghai",
  overlap_policy: "skip",
  actor_user_id: "55555555-5555-5555-5555-555555555555",
  consecutive_failure_count: 0,
  created_at: "2026-07-20T00:00:00.000Z",
  updated_at: "2026-07-20T00:00:00.000Z",
  latest_fire: {
    id: "66666666-6666-6666-6666-666666666666",
    tenant_id: "22222222-2222-2222-2222-222222222222",
    rule_id: "11111111-1111-1111-1111-111111111111",
    scheduled_fire_at: "2026-07-21T01:00:00.000Z",
    idempotency_key: "k1",
    status: "succeeded",
    created_at: "2026-07-21T01:00:01.000Z",
  },
};

describe("automation dashboard rail helpers", () => {
  it("builds upcoming fires within horizon using nextFireById", () => {
    const now = new Date("2026-07-22T01:30:00.000Z");
    const nextFireById = buildNextFireById([baseRule], now);
    const rows = buildUpcomingFires([baseRule], { now, nextFireById, limit: 8 });
    expect(rows).toHaveLength(1);
    expect(rows[0]?.rule.name).toBe("工作日巡检");
    expect(rows[0]?.nextAt).toBe("2026-07-23T01:00:00.000Z");
  });

  it("counts upcoming fires without the rail display cap", () => {
    const now = new Date("2026-07-22T01:30:00.000Z");
    const many = Array.from({ length: 12 }, (_, index) => ({
      ...baseRule,
      id: `11111111-1111-1111-1111-1111111111${index.toString(16).padStart(2, "0")}`,
      name: `规则-${index}`,
    }));
    const nextFireById = buildNextFireById(many, now);
    expect(countUpcomingFires(many, nextFireById, 72, now)).toBe(12);
    expect(buildUpcomingFires(many, { now, nextFireById, limit: 8 })).toHaveLength(8);
  });

  it("builds recent fire activity newest first", () => {
    const rows = buildRecentFireActivity([baseRule]);
    expect(rows).toHaveLength(1);
    expect(rows[0]?.fire.status).toBe("succeeded");
  });
});

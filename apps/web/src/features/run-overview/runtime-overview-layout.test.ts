import { describe, expect, it } from "vitest";
import { buildFloorLayouts } from "./runtime-overview-layout";
import type { RuntimeOverviewFloorId } from "./runtime-overview-model";

const teamIdsByFloor: Record<RuntimeOverviewFloorId, string[]> = {
  "floor-1": ["team-1", "team-2", "team-3", "team-4", "team-5", "team-6"],
  "floor-2": ["team-7", "team-8", "team-9", "team-10"],
  "floor-3": ["team-11", "team-12", "team-13"],
};

describe("runtime overview floor layout", () => {
  it("uses office-zone capacities instead of fixed ten-seat workspaces", () => {
    const layouts = buildFloorLayouts(teamIdsByFloor);
    const allowedCapacities = new Set([3, 4, 6, 8, 10]);

    for (const floor of layouts) {
      for (const workspace of floor.layout.teamWorkspaces) {
        expect(allowedCapacities.has(workspace.capacity)).toBe(true);
        expect(workspace.capacity).toBe(workspace.seats.length);
        expect(workspace.capacity).toBeGreaterThanOrEqual(3);
      }
    }

    const capacities = layouts.flatMap((floor) => floor.layout.teamWorkspaces.map((workspace) => workspace.capacity));
    expect(capacities).toContain(3);
    expect(capacities.filter((capacity) => capacity === 10).length).toBeLessThanOrEqual(3);
  });

  it("keeps team summary callouts outside office zones and points them back to the workspace", () => {
    const layouts = buildFloorLayouts(teamIdsByFloor);

    for (const floor of layouts) {
      for (const workspace of floor.layout.teamWorkspaces) {
        expect(workspace.cardAnchor.y).toBeLessThanOrEqual(Math.min(...workspace.polygon.map((point) => point.y)) - 24);
        expect(workspace.calloutTarget.x).toBeGreaterThanOrEqual(Math.min(...workspace.polygon.map((point) => point.x)));
        expect(workspace.calloutTarget.x).toBeLessThanOrEqual(Math.max(...workspace.polygon.map((point) => point.x)));
        expect(workspace.calloutTarget.y).toBeGreaterThanOrEqual(Math.min(...workspace.polygon.map((point) => point.y)));
        expect(workspace.calloutTarget.y).toBeLessThanOrEqual(Math.max(...workspace.polygon.map((point) => point.y)));
      }
    }
  });
});

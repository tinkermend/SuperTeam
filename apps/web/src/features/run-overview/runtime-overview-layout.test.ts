import { describe, expect, it } from "vitest";
import { buildFloorLayouts } from "./runtime-overview-layout";
import type { RuntimeOverviewFloorId } from "./runtime-overview-model";

const teamIdsByFloor: Record<RuntimeOverviewFloorId, string[]> = {
  "floor-1": ["team-1", "team-2", "team-3", "team-4", "team-5", "team-6"],
  "floor-2": ["team-7", "team-8", "team-9", "team-10"],
  "floor-3": ["team-11", "team-12", "team-13"],
};

describe("runtime overview floor layout", () => {
  it("places team summary cards away from the first employee avatar lane", () => {
    const layouts = buildFloorLayouts(teamIdsByFloor);

    for (const floor of layouts) {
      for (const workspace of floor.layout.teamWorkspaces) {
        const firstSeat = workspace.seats[0];

        expect(workspace.cardAnchor.x).toBeGreaterThanOrEqual(firstSeat.x + 180);
      }
    }
  });
});

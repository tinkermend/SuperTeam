import { describe, expect, it } from "vitest";
import { buildFloorLayouts, runtimeOverviewSlotCapacities } from "./runtime-overview-layout";
import { type RuntimeOverviewFloorId, UNASSIGNED_TEAM_ID } from "./runtime-overview-model";

const teamIdsByFloor: Record<RuntimeOverviewFloorId, string[]> = {
  "floor-1": ["team-1", "team-2", "team-3", "team-4", "team-5", "team-6", "team-7", "team-8"],
  "floor-2": ["team-9", "team-10", "team-11", "team-12", "team-13", "team-14", "team-15", "team-16"],
  "floor-3": ["team-17", "team-18", "team-19", "team-20", "team-21", "team-22", "team-23", "team-24"],
  lobby: [],
};

const teamCardBounds = {
  width: 196,
  height: 108,
};

const seatHotZoneBounds = {
  xRadius: 30,
  topOffset: 58,
  bottomOffset: 6,
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

    // 团队工位容量口径不含大厅候岗工位。
    const capacities = layouts.flatMap((floor) =>
      floor.layout.teamWorkspaces
        .filter((workspace) => workspace.teamId !== UNASSIGNED_TEAM_ID)
        .map((workspace) => workspace.capacity),
    );
    expect(capacities).toContain(3);
    expect(capacities.filter((capacity) => capacity === 10).length).toBeLessThanOrEqual(3);
  });

  it("keeps team summary callouts near the workspace and points them back to the desks", () => {
    const layouts = buildFloorLayouts(teamIdsByFloor);
    const failures: string[] = [];

    for (const floor of layouts) {
      for (const workspace of floor.layout.teamWorkspaces) {
        const workspaceBounds = boundsFromPoints(workspace.polygon);
        const cardBounds = cardBoundsFromAnchor(workspace.cardAnchor);
        const minSeatY = Math.min(...workspace.seats.map((seat) => seat.y));
        if (rectangleGap(cardBounds, workspaceBounds) > 130) {
          failures.push(`${floor.floorId} ${workspace.teamId} card is too far from its office zone`);
        }
        if (workspace.calloutTarget.x < workspaceBounds.left || workspace.calloutTarget.x > workspaceBounds.right) {
          failures.push(`${floor.floorId} ${workspace.teamId} callout x misses office zone`);
        }
        if (workspace.calloutTarget.y < workspaceBounds.top || workspace.calloutTarget.y > workspaceBounds.bottom) {
          failures.push(`${floor.floorId} ${workspace.teamId} callout y misses office zone`);
        }
        if (workspace.calloutTarget.y > minSeatY - 36) {
          failures.push(`${floor.floorId} ${workspace.teamId} callout points too low`);
        }
      }
    }

    expect(failures).toEqual([]);
  });

  it("does not allocate a team workspace inside the floor-one meeting room", () => {
    const [floorOne] = buildFloorLayouts(teamIdsByFloor);

    const meetingRoomWorkspaces = floorOne.layout.teamWorkspaces.filter((workspace) => {
      const minX = Math.min(...workspace.polygon.map((point) => point.x));
      const maxX = Math.max(...workspace.polygon.map((point) => point.x));
      const minY = Math.min(...workspace.polygon.map((point) => point.y));
      return minX < 540 && maxX < 560 && minY < 390;
    });

    expect(meetingRoomWorkspaces).toHaveLength(0);
  });

  it("keeps the floor-one ten-seat team callout aligned with its desk zone", () => {
    const [floorOne] = buildFloorLayouts(teamIdsByFloor);
    const workspace = floorOne.layout.teamWorkspaces.find((item) => item.capacity === 10);

    expect(workspace).toBeDefined();
    if (!workspace) return;

    const workspaceBounds = boundsFromPoints(workspace.polygon);
    const cardBounds = cardBoundsFromAnchor(workspace.cardAnchor);
    const minSeatY = Math.min(...workspace.seats.map((seat) => seat.y));

    expect(workspace.cardAnchor.x).toBe(455);
    expect(cardBounds.right).toBeGreaterThanOrEqual(workspaceBounds.left);
    expect(cardBounds.bottom).toBeLessThanOrEqual(minSeatY - 24);
  });

  it("hosts the lobby workspace on a dedicated 大厅 floor outside team distribution", () => {
    const layouts = buildFloorLayouts(teamIdsByFloor);
    const lobbyFloor = layouts.find((floor) => floor.floorId === "lobby");

    expect(lobbyFloor).toBeDefined();
    expect(lobbyFloor?.label).toBe("大厅");
    expect(lobbyFloor?.teamIds).toEqual([]);
    const lobby = lobbyFloor?.layout.teamWorkspaces.find((workspace) => workspace.teamId === UNASSIGNED_TEAM_ID);
    expect(lobby?.decorationVariant).toBe("lobby");
    expect(lobby?.capacity).toBe(10);
    expect(lobby?.seats.length).toBe(10);
    // 候岗不进团队分配容量表、不计任何容量；其他楼层不含候岗工位。
    expect(runtimeOverviewSlotCapacities().lobby).toEqual([]);
    expect(lobbyFloor?.summary.capacityTotal).toBe(0);
    for (const floor of layouts.filter((item) => item.floorId !== "lobby")) {
      expect(floor.layout.teamWorkspaces.some((workspace) => workspace.teamId === UNASSIGNED_TEAM_ID)).toBe(false);
      expect(floor.label).toMatch(/^\d层$/);
    }
  });

  it("keeps team summary cards from covering seat and avatar positions", () => {
    const layouts = buildFloorLayouts(teamIdsByFloor);
    const failures: string[] = [];

    for (const floor of layouts) {
      for (const workspace of floor.layout.teamWorkspaces) {
        const cardBounds = cardBoundsFromAnchor(workspace.cardAnchor);

        const overlappingSeats = workspace.seats
          .map((seat, seatIndex) => ({ seatHotZone: seatHotZoneFromSeat(seat), seatIndex }))
          .filter(({ seatHotZone }) => rectanglesOverlap(cardBounds, seatHotZone));
        if (overlappingSeats.length > 0) {
          failures.push(
            `${floor.floorId} ${workspace.teamId} card overlaps seats ${overlappingSeats.map(({ seatIndex }) => seatIndex + 1).join(", ")}`,
          );
        }
      }
    }

    expect(failures).toEqual([]);
  });
});

function boundsFromPoints(points: Array<{ x: number; y: number }>) {
  return {
    left: Math.min(...points.map((point) => point.x)),
    right: Math.max(...points.map((point) => point.x)),
    top: Math.min(...points.map((point) => point.y)),
    bottom: Math.max(...points.map((point) => point.y)),
  };
}

function cardBoundsFromAnchor(anchor: { x: number; y: number }) {
  return {
    left: anchor.x,
    right: anchor.x + teamCardBounds.width,
    top: anchor.y,
    bottom: anchor.y + teamCardBounds.height,
  };
}

function seatHotZoneFromSeat(seat: { x: number; y: number }) {
  return {
    left: seat.x - seatHotZoneBounds.xRadius,
    right: seat.x + seatHotZoneBounds.xRadius,
    top: seat.y - seatHotZoneBounds.topOffset,
    bottom: seat.y + seatHotZoneBounds.bottomOffset,
  };
}

function rectangleGap(
  a: { left: number; right: number; top: number; bottom: number },
  b: { left: number; right: number; top: number; bottom: number },
) {
  const xGap = Math.max(b.left - a.right, a.left - b.right, 0);
  const yGap = Math.max(b.top - a.bottom, a.top - b.bottom, 0);
  return Math.hypot(xGap, yGap);
}

function rectanglesOverlap(
  a: { left: number; right: number; top: number; bottom: number },
  b: { left: number; right: number; top: number; bottom: number },
) {
  return a.left < b.right && a.right > b.left && a.top < b.bottom && a.bottom > b.top;
}

import {
  TEAM_SEAT_CAPACITY,
  type RuntimeOverviewFloor,
  type RuntimeOverviewFloorId,
  type RuntimeOverviewTeamWorkspace,
} from "./runtime-overview-model";

export const RUNTIME_OVERVIEW_CANVAS = {
  width: 1672,
  height: 941,
};

const floorBackgrounds: Record<RuntimeOverviewFloorId, string> = {
  "floor-1": "/images/run-overview/floor-1-office.png",
  "floor-2": "/images/run-overview/floor-2-office.png",
  "floor-3": "/images/run-overview/floor-3-office.png",
};

const floorTeamSlots: Record<RuntimeOverviewFloorId, Array<Omit<RuntimeOverviewTeamWorkspace, "teamId" | "seats">>> = {
  "floor-1": [
    workspace([100, 150, 575, 150, 575, 355, 100, 355], 360, 28, "standard"),
    workspace([615, 150, 1080, 150, 1080, 355, 615, 355], 875, 28, "lab"),
    workspace([1095, 150, 1550, 150, 1550, 355, 1095, 355], 1390, 28, "ops"),
    workspace([70, 450, 540, 450, 540, 720, 70, 720], 350, 330, "review"),
    workspace([600, 450, 1080, 450, 1080, 720, 600, 720], 875, 330, "standard"),
    workspace([1120, 450, 1600, 450, 1600, 720, 1120, 720], 1400, 330, "data"),
  ],
  "floor-2": [
    workspace([190, 155, 735, 155, 735, 392, 190, 392], 500, 24, "lab"),
    workspace([940, 155, 1485, 155, 1485, 392, 940, 392], 1240, 24, "standard"),
    workspace([160, 480, 720, 480, 720, 752, 160, 752], 500, 330, "ops"),
    workspace([950, 480, 1515, 480, 1515, 752, 950, 752], 1240, 330, "review"),
  ],
  "floor-3": [
    workspace([185, 150, 700, 105, 720, 470, 120, 500], 480, 28, "data"),
    workspace([900, 105, 1470, 145, 1535, 420, 850, 465], 1250, 28, "standard"),
    workspace([790, 555, 1425, 485, 1545, 790, 745, 850], 1190, 390, "lab"),
  ],
};

const floorConnectorPaths: Record<RuntimeOverviewFloorId, RuntimeOverviewFloor["layout"]["paths"]> = {
  "floor-1": [
    {
      id: "floor-1-center-up-branch",
      tone: "primary",
      points: [
        { x: 835, y: 390 },
        { x: 835, y: 352 },
      ],
    },
    {
      id: "floor-1-center-down-branch",
      tone: "primary",
      points: [
        { x: 835, y: 397 },
        { x: 835, y: 448 },
      ],
    },
  ],
  "floor-2": [],
  "floor-3": [],
};

const seatAnchors: Record<RuntimeOverviewFloorId, Array<{ x: number; y: number; dx: number; dy: number; rotation?: number }>> = {
  "floor-1": [
    { x: 150, y: 208, dx: 85, dy: 102, rotation: 0 },
    { x: 665, y: 208, dx: 86, dy: 102, rotation: 0 },
    { x: 1150, y: 208, dx: 82, dy: 102, rotation: 0 },
    { x: 125, y: 510, dx: 83, dy: 105, rotation: 0 },
    { x: 655, y: 510, dx: 83, dy: 105, rotation: 0 },
    { x: 1170, y: 510, dx: 84, dy: 105, rotation: 0 },
  ],
  "floor-2": [
    { x: 248, y: 202, dx: 92, dy: 110, rotation: 0 },
    { x: 1010, y: 202, dx: 92, dy: 110, rotation: 0 },
    { x: 245, y: 520, dx: 92, dy: 125, rotation: 0 },
    { x: 1015, y: 520, dx: 92, dy: 125, rotation: 0 },
  ],
  "floor-3": [
    { x: 260, y: 205, dx: 95, dy: 102, rotation: -10 },
    { x: 995, y: 205, dx: 95, dy: 102, rotation: 10 },
    { x: 905, y: 585, dx: 93, dy: 104, rotation: -8 },
  ],
};

function workspace(
  polygonValues: number[],
  cardX: number,
  cardY: number,
  decorationVariant: RuntimeOverviewTeamWorkspace["decorationVariant"],
) {
  return {
    polygon: toPoints(polygonValues),
    cardAnchor: { x: cardX, y: cardY },
    decorationVariant,
  };
}

function toPoints(values: number[]) {
  const points: Array<{ x: number; y: number }> = [];
  for (let index = 0; index < values.length; index += 2) {
    points.push({ x: values[index], y: values[index + 1] });
  }
  return points;
}

function buildSeats(floorId: RuntimeOverviewFloorId, teamId: string, slotIndex: number): RuntimeOverviewTeamWorkspace["seats"] {
  const anchor = seatAnchors[floorId][slotIndex] ?? seatAnchors[floorId][0];
  return Array.from({ length: TEAM_SEAT_CAPACITY }, (_, index) => {
    const column = index % 5;
    const row = Math.floor(index / 5);
    return {
      seatId: `${teamId}-seat-${index + 1}`,
      x: anchor.x + column * anchor.dx,
      y: anchor.y + row * anchor.dy,
      rotation: anchor.rotation ?? 0,
    };
  });
}

export function buildFloorLayouts(teamIdsByFloor: Record<RuntimeOverviewFloorId, string[]>): RuntimeOverviewFloor[] {
  return (Object.keys(floorTeamSlots) as RuntimeOverviewFloorId[]).map((floorId, floorIndex) => {
    const teamIds = teamIdsByFloor[floorId] ?? [];
    const slots = floorTeamSlots[floorId];
    const workspaces = teamIds.slice(0, slots.length).map((teamId, index) => ({
      ...slots[index],
      teamId,
      seats: buildSeats(floorId, teamId, index),
    }));
    return {
      floorId,
      label: `${floorIndex + 1}层`,
      teamIds,
      summary: {
        teamCount: teamIds.length,
        errorCount: 0,
        capacityUsed: 0,
        capacityTotal: teamIds.length * TEAM_SEAT_CAPACITY,
      },
      layout: {
        backgroundImageUrl: floorBackgrounds[floorId],
        canvasWidth: RUNTIME_OVERVIEW_CANVAS.width,
        canvasHeight: RUNTIME_OVERVIEW_CANVAS.height,
        paths: floorConnectorPaths[floorId],
        teamWorkspaces: workspaces,
      },
    };
  });
}

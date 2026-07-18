import {
  type RuntimeOverviewFloor,
  type RuntimeOverviewFloorId,
  type RuntimeOverviewTeamWorkspace,
  type RuntimeOverviewWorkspaceCapacity,
  UNASSIGNED_TEAM_ID,
} from "./runtime-overview-model";

export const RUNTIME_OVERVIEW_CANVAS = {
  width: 1672,
  height: 941,
};

const floorBackgrounds: Record<RuntimeOverviewFloorId, string> = {
  "floor-1": "/images/run-overview/floor-1-office-v4.png",
  "floor-2": "/images/run-overview/floor-2-office-v4.png",
  "floor-3": "/images/run-overview/floor-3-office-v4.png",
  // TODO: 换成专属大厅底图 floor-lobby-office-v4.png（等待素材），当前临时复用 floor-1。
  lobby: "/images/run-overview/floor-1-office-v4.png",
};

const floorTeamSlots: Record<RuntimeOverviewFloorId, RuntimeOverviewTeamSlot[]> = {
  "floor-1": [
    workspace([1180, 645, 1470, 645, 1470, 760, 1180, 760], 1227, 525, "standard", 3, { x: 1225, y: 704, dx: 78, dy: 0 }),
    workspace([600, 245, 910, 245, 910, 360, 600, 360], 657, 125, "lab", 4, { x: 640, y: 304, dx: 76, dy: 0 }),
    workspace([1080, 245, 1510, 245, 1510, 360, 1080, 360], 1197, 125, "ops", 6, { x: 1120, y: 304, dx: 74, dy: 0 }),
    workspace([160, 455, 520, 455, 520, 570, 160, 570], 242, 335, "review", 4, { x: 205, y: 514, dx: 80, dy: 0 }),
    workspace([600, 445, 1070, 445, 1070, 575, 600, 575], 737, 325, "standard", 6, { x: 655, y: 510, dx: 76, dy: 0 }),
    workspace([1180, 445, 1470, 445, 1470, 575, 1180, 575], 1227, 325, "data", 3, { x: 1225, y: 510, dx: 76, dy: 0 }),
    workspace([175, 645, 555, 645, 555, 760, 175, 760], 175, 525, "ops", 4, { x: 230, y: 704, dx: 78, dy: 0 }),
    workspace([620, 635, 1085, 635, 1085, 800, 620, 800], 455, 515, "lab", 10, { x: 680, y: 690, dx: 76, dy: 64, columns: 5 }),
  ],
  "floor-2": [
    workspace([210, 205, 530, 205, 530, 330, 210, 330], 272, 85, "lab", 4, { x: 250, y: 268, dx: 78, dy: 0 }),
    workspace([660, 270, 940, 270, 940, 390, 660, 390], 702, 130, "standard", 3, { x: 705, y: 332, dx: 80, dy: 0 }),
    workspace([1100, 275, 1480, 275, 1480, 395, 1100, 395], 1192, 155, "ops", 6, { x: 1145, y: 336, dx: 72, dy: 0 }),
    workspace([180, 470, 520, 470, 520, 585, 180, 585], 252, 350, "review", 4, { x: 220, y: 528, dx: 78, dy: 0 }),
    workspace([610, 460, 1085, 460, 1085, 625, 610, 625], 750, 340, "data", 10, { x: 665, y: 512, dx: 72, dy: 64, columns: 5 }),
    workspace([1210, 470, 1495, 470, 1495, 590, 1210, 590], 1255, 350, "standard", 3, { x: 1252, y: 532, dx: 78, dy: 0 }),
    workspace([235, 675, 510, 675, 510, 785, 235, 785], 275, 555, "ops", 3, { x: 270, y: 732, dx: 78, dy: 0 }),
    workspace([1210, 675, 1495, 675, 1495, 785, 1210, 785], 1255, 555, "lab", 3, { x: 1250, y: 732, dx: 78, dy: 0 }),
  ],
  "floor-3": [
    workspace([210, 245, 535, 245, 535, 360, 210, 360], 275, 125, "data", 4, { x: 250, y: 306, dx: 76, dy: 0 }),
    workspace([710, 245, 955, 245, 955, 360, 710, 360], 735, 125, "standard", 3, { x: 735, y: 306, dx: 78, dy: 0 }),
    workspace([1070, 245, 1505, 245, 1505, 360, 1070, 360], 1190, 125, "ops", 6, { x: 1115, y: 306, dx: 72, dy: 0 }),
    workspace([185, 455, 520, 455, 520, 570, 185, 570], 255, 335, "review", 4, { x: 225, y: 514, dx: 78, dy: 0 }),
    workspace([620, 455, 1090, 455, 1090, 585, 620, 585], 757, 335, "standard", 6, { x: 675, y: 520, dx: 74, dy: 0 }),
    workspace([1210, 455, 1490, 455, 1490, 570, 1210, 570], 1252, 335, "lab", 3, { x: 1250, y: 514, dx: 78, dy: 0 }),
    workspace([225, 675, 500, 675, 500, 790, 225, 790], 265, 555, "data", 3, { x: 260, y: 732, dx: 78, dy: 0 }),
    workspace([630, 640, 1085, 640, 1085, 805, 630, 805], 1099, 669, "lab", 10, { x: 690, y: 694, dx: 76, dy: 64, columns: 5 }),
  ],
  // 大厅层没有团队 slot：不参与团队分配，只有下方的候岗工位。
  lobby: [],
};

// 候岗工位：大厅层内唯一的工位，10 座开放区网格（几何复用 floor-1 中下部开放办公区，
// 该区域的卡片间距/呼出线/座位热区均已被布局测试覆盖）。不是团队 slot、不计任何容量口径。
// TODO: 专属大厅底图到位后按图微调 polygon 与座位坐标。
const lobbySlot = workspace([630, 640, 1085, 640, 1085, 805, 630, 805], 455, 515, "lobby", 10, {
  x: 690,
  y: 694,
  dx: 76,
  dy: 64,
  columns: 5,
});

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
  lobby: [],
};

type SeatGrid = { x: number; y: number; dx: number; dy: number; columns?: number; rotation?: number };
type RuntimeOverviewTeamSlot = Omit<RuntimeOverviewTeamWorkspace, "teamId" | "seats"> & { seatGrid: SeatGrid };

function workspace(
  polygonValues: number[],
  cardX: number,
  cardY: number,
  decorationVariant: RuntimeOverviewTeamWorkspace["decorationVariant"],
  capacity: RuntimeOverviewWorkspaceCapacity,
  seatGrid: SeatGrid,
) {
  const polygon = toPoints(polygonValues);
  return {
    capacity,
    polygon,
    cardAnchor: { x: cardX, y: cardY },
    calloutTarget: desktopTarget(polygon, seatGrid, capacity),
    decorationVariant,
    seatGrid,
  };
}

function toPoints(values: number[]) {
  const points: Array<{ x: number; y: number }> = [];
  for (let index = 0; index < values.length; index += 2) {
    points.push({ x: values[index], y: values[index + 1] });
  }
  return points;
}

function desktopTarget(points: Array<{ x: number; y: number }>, seatGrid: SeatGrid, capacity: RuntimeOverviewWorkspaceCapacity) {
  const columns = seatGrid.columns ?? capacity;
  const firstRowSeats = Math.min(columns, capacity);
  const firstRowWidth = Math.max(0, (firstRowSeats - 1) * seatGrid.dx);
  return {
    x: Math.round(seatGrid.x + firstRowWidth / 2),
    y: Math.round(Math.max(Math.min(...points.map((point) => point.y)) + 16, seatGrid.y - 46)),
  };
}

function buildSeats(
  teamId: string,
  capacity: RuntimeOverviewWorkspaceCapacity,
  seatGrid: SeatGrid,
): RuntimeOverviewTeamWorkspace["seats"] {
  const columns = seatGrid.columns ?? capacity;
  return Array.from({ length: capacity }, (_, index) => {
    const column = index % columns;
    const row = Math.floor(index / columns);
    return {
      seatId: `${teamId}-seat-${index + 1}`,
      x: seatGrid.x + column * seatGrid.dx,
      y: seatGrid.y + row * seatGrid.dy,
      rotation: seatGrid.rotation ?? 0,
    };
  });
}

export function buildFloorLayouts(teamIdsByFloor: Record<RuntimeOverviewFloorId, string[]>): RuntimeOverviewFloor[] {
  return (Object.keys(floorTeamSlots) as RuntimeOverviewFloorId[]).map((floorId, floorIndex) => {
    const slotTeamIds = teamIdsByFloor[floorId] ?? [];
    const teamIds = slotTeamIds.filter((teamId): teamId is string => Boolean(teamId));
    const slots = floorTeamSlots[floorId];
    const workspaces: RuntimeOverviewTeamWorkspace[] = slots.flatMap((slotConfig, index) => {
      const teamId = slotTeamIds[index];
      if (!teamId) return [];
      const { seatGrid, ...workspaceSlot } = slotConfig;
      return [{
        ...workspaceSlot,
        teamId,
        seats: buildSeats(teamId, workspaceSlot.capacity, seatGrid),
      }];
    });
    if (floorId === "lobby") {
      const { seatGrid, ...lobbyWorkspace } = lobbySlot;
      workspaces.push({
        ...lobbyWorkspace,
        teamId: UNASSIGNED_TEAM_ID,
        seats: buildSeats(UNASSIGNED_TEAM_ID, lobbyWorkspace.capacity, seatGrid),
      });
    }
    return {
      floorId,
      label: floorId === "lobby" ? "大厅" : `${floorIndex + 1}层`,
      teamIds,
      summary: {
        teamCount: teamIds.length,
        errorCount: 0,
        capacityUsed: 0,
        capacityTotal: workspaces
          .filter((workspace) => workspace.teamId !== UNASSIGNED_TEAM_ID)
          .reduce((sum, workspace) => sum + workspace.capacity, 0),
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

export function runtimeOverviewSlotCapacities(): Record<RuntimeOverviewFloorId, RuntimeOverviewWorkspaceCapacity[]> {
  return {
    "floor-1": floorTeamSlots["floor-1"].map((slot) => slot.capacity),
    "floor-2": floorTeamSlots["floor-2"].map((slot) => slot.capacity),
    "floor-3": floorTeamSlots["floor-3"].map((slot) => slot.capacity),
    lobby: [],
  };
}

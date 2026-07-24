import { useLayoutEffect, useMemo, useRef, useState } from "react";
import type { RuntimeOverviewDTO, RuntimeOverviewEmployee, RuntimeOverviewFloorId } from "../runtime-overview-model";
import { UNASSIGNED_TEAM_ID } from "../runtime-overview-model";
import { type ProjectLens, projectLensForFloor } from "../runtime-overview-project-lens";
import type { EmployeeLensState } from "./employee-avatar-node";
import { LobbyWorkspaceRenderer } from "./lobby-workspace-renderer";
import { RuntimeMapSvgLayer } from "./runtime-map-svg-layer";
import { TeamWorkspaceRenderer } from "./team-workspace-renderer";

type RuntimeMapStageProps = {
  activeFloorId: RuntimeOverviewDTO["activeFloorId"];
  onSelectEmployee: (employeeId: string) => void;
  onSelectFloor?: (floorId: RuntimeOverviewFloorId) => void;
  overview: RuntimeOverviewDTO;
  selectedEmployeeId?: string;
  lens?: ProjectLens;
};

const officeSceneVerticalFeather =
  "linear-gradient(to bottom, transparent 0%, #000 4.8%, #000 91.5%, transparent 100%)";
const officeSceneHorizontalFeather =
  "linear-gradient(to right, transparent 0%, #000 1.8%, #000 98.2%, transparent 100%)";

const floorShortLabel: Record<RuntimeOverviewFloorId, string> = {
  "floor-1": "1层",
  "floor-2": "2层",
  "floor-3": "3层",
  lobby: "大厅"
};

export function RuntimeMapStage({ activeFloorId, lens, onSelectEmployee, onSelectFloor, overview, selectedEmployeeId }: RuntimeMapStageProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [scale, setScale] = useState(1);
  const [hoveredLensEdgeId, setHoveredLensEdgeId] = useState<string>();
  const floor = overview.floors.find((item) => item.floorId === activeFloorId) ?? overview.floors[0];
  const selectedEmployee = overview.employees.find((employee) => employee.employeeId === selectedEmployeeId);
  const employeesByTeam = useMemo(() => {
    const map = new Map<string, RuntimeOverviewEmployee[]>();
    for (const employee of overview.employees.filter((item) => item.floorId === floor.floorId)) {
      map.set(employee.teamId, [...(map.get(employee.teamId) ?? []), employee]);
    }
    return map;
  }, [floor.floorId, overview.employees]);

  // 项目透镜投影：当前楼层的可绘制交接边 + 跨楼层出口徽标 + 每员工高亮态。
  const lensProjection = useMemo(
    () => (lens ? projectLensForFloor(lens, overview, floor.floorId) : undefined),
    [lens, overview, floor.floorId],
  );
  const lensStateFor = useMemo(() => {
    if (!lens) return undefined;
    const participants = new Set(lens.participantEmployeeIds);
    const stops = new Set(lens.stopEmployeeIds);
    return (employeeId: string): EmployeeLensState =>
      stops.has(employeeId) ? "stop" : participants.has(employeeId) ? "participant" : "dimmed";
  }, [lens]);

  useLayoutEffect(() => {
    const node = containerRef.current;
    if (!node) return;
    const updateScale = () => {
      const nextScale = Math.min(1, Math.max(0.28, node.clientWidth / floor.layout.canvasWidth));
      setScale(Number(nextScale.toFixed(3)));
    };
    updateScale();
    const observer = new ResizeObserver(updateScale);
    observer.observe(node);
    return () => observer.disconnect();
  }, [floor.layout.canvasWidth]);

  return (
    <section
      className="relative overflow-visible"
      aria-label="运行总览地图画布"
    >
      <div
        ref={containerRef}
        className="relative w-full overflow-hidden bg-transparent"
        style={{ height: Math.max(360, Math.round(floor.layout.canvasHeight * scale)) }}
      >
        <div
          className="absolute left-0 top-0 origin-top-left"
          style={{
            width: floor.layout.canvasWidth,
            height: floor.layout.canvasHeight,
            transform: `scale(${scale})`
}}
        >
          <div
            data-runtime-map-scene={floor.floorId}
            className="absolute inset-0 z-0 overflow-hidden"
            style={{
              WebkitMaskImage: officeSceneVerticalFeather,
              maskImage: officeSceneVerticalFeather
}}
          >
            <div
              data-runtime-map-feather={floor.floorId}
              className="absolute inset-0"
              style={{
                WebkitMaskImage: officeSceneHorizontalFeather,
                maskImage: officeSceneHorizontalFeather
}}
            >
              <img
                src={floor.layout.backgroundImageUrl}
                alt=""
                data-runtime-map-background={floor.floorId}
                className="absolute inset-0 z-0 h-full w-full select-none object-cover opacity-[0.965] mix-blend-multiply saturate-[0.95] contrast-[0.98]"
                draggable={false}
              />
              <div
                className="pointer-events-none absolute inset-0 z-[1]"
                style={{
                  background: "linear-gradient(180deg, rgba(246,248,251,0.018), rgba(232,238,246,0.028))"
}}
              />
            </div>
          </div>
          <RuntimeMapSvgLayer
            floor={floor}
            selectedTeamId={selectedEmployee?.teamId}
            lensEdges={lensProjection?.fullEdges}
            hoveredLensEdgeId={hoveredLensEdgeId}
            onHoverLensEdge={setHoveredLensEdgeId}
          />
          {floor.layout.teamWorkspaces.map((workspace) => {
            if (workspace.teamId === UNASSIGNED_TEAM_ID) {
              return (
                <LobbyWorkspaceRenderer
                  key={workspace.teamId}
                  employees={employeesByTeam.get(UNASSIGNED_TEAM_ID) ?? []}
                  lensStateFor={lensStateFor}
                  onSelectEmployee={onSelectEmployee}
                  selectedEmployeeId={selectedEmployeeId}
                  workspace={workspace}
                />
              );
            }
            const team = overview.teams.find((item) => item.teamId === workspace.teamId);
            if (!team) return null;
            return (
              <TeamWorkspaceRenderer
                key={workspace.teamId}
                employees={employeesByTeam.get(workspace.teamId) ?? []}
                lensStateFor={lensStateFor}
                onSelectEmployee={onSelectEmployee}
                selectedEmployeeId={selectedEmployeeId}
                selectedTeamId={selectedEmployee?.teamId}
                team={team}
                workspace={workspace}
              />
            );
          })}
          {/* 跨楼层交接出口：钉在在场端点座位旁，点击切到目标楼层。 */}
          {lensProjection?.portals.map((portal, index) => (
            <button
              key={portal.id}
              type="button"
              data-runtime-lens-portal={portal.targetFloorId}
              aria-label={`交接链路${portal.direction === "outgoing" ? "去往" : "来自"}${floorShortLabel[portal.targetFloorId]}，点击切换楼层`}
              className={`absolute z-40 inline-flex h-7 items-center gap-1 rounded-full border px-2.5 text-xs font-semibold shadow-card transition hover:scale-105 focus:outline-none focus:ring-2 focus:ring-brand ${
                portal.tone === "warning"
                  ? "border-warn/60 bg-warn-soft text-warn-text"
                  : "border-brand/50 bg-white/92 text-brand-deep"
              }`}
              style={{ left: portal.at.x + 26, top: portal.at.y - 90 - index * 4 }}
              onClick={() => onSelectFloor?.(portal.targetFloorId)}
            >
              {portal.direction === "outgoing" ? "转" : "自"} {floorShortLabel[portal.targetFloorId]}
            </button>
          ))}
        </div>
      </div>
    </section>
  );
}

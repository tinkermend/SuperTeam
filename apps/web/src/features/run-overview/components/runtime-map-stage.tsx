import { Hand, Maximize2, Minus, Plus } from "lucide-react";
import { useLayoutEffect, useMemo, useRef, useState } from "react";
import type { RuntimeOverviewDTO, RuntimeOverviewEmployee } from "../runtime-overview-model";
import { RuntimeMapSvgLayer } from "./runtime-map-svg-layer";
import { TeamWorkspaceRenderer } from "./team-workspace-renderer";

type RuntimeMapStageProps = {
  activeFloorId: RuntimeOverviewDTO["activeFloorId"];
  onSelectEmployee: (employeeId: string) => void;
  overview: RuntimeOverviewDTO;
  selectedEmployeeId?: string;
};

const officeSceneVerticalFeather =
  "linear-gradient(to bottom, transparent 0%, #000 4.8%, #000 91.5%, transparent 100%)";
const officeSceneHorizontalFeather =
  "linear-gradient(to right, transparent 0%, #000 1.8%, #000 98.2%, transparent 100%)";

export function RuntimeMapStage({ activeFloorId, onSelectEmployee, overview, selectedEmployeeId }: RuntimeMapStageProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [scale, setScale] = useState(1);
  const floor = overview.floors.find((item) => item.floorId === activeFloorId) ?? overview.floors[0];
  const selectedEmployee = overview.employees.find((employee) => employee.employeeId === selectedEmployeeId);
  const employeesByTeam = useMemo(() => {
    const map = new Map<string, RuntimeOverviewEmployee[]>();
    for (const employee of overview.employees.filter((item) => item.floorId === floor.floorId)) {
      map.set(employee.teamId, [...(map.get(employee.teamId) ?? []), employee]);
    }
    return map;
  }, [floor.floorId, overview.employees]);

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
            transform: `scale(${scale})`,
          }}
        >
          <div
            data-runtime-map-scene={floor.floorId}
            className="absolute inset-0 z-0 overflow-hidden"
            style={{
              WebkitMaskImage: officeSceneVerticalFeather,
              maskImage: officeSceneVerticalFeather,
            }}
          >
            <div
              data-runtime-map-feather={floor.floorId}
              className="absolute inset-0"
              style={{
                WebkitMaskImage: officeSceneHorizontalFeather,
                maskImage: officeSceneHorizontalFeather,
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
                  background: "linear-gradient(180deg, rgba(246,248,251,0.018), rgba(232,238,246,0.028))",
                }}
              />
            </div>
          </div>
          <RuntimeMapSvgLayer floor={floor} selectedTeamId={selectedEmployee?.teamId} />
          {floor.layout.teamWorkspaces.map((workspace) => {
            const team = overview.teams.find((item) => item.teamId === workspace.teamId);
            if (!team) return null;
            return (
              <TeamWorkspaceRenderer
                key={workspace.teamId}
                employees={employeesByTeam.get(workspace.teamId) ?? []}
                onSelectEmployee={onSelectEmployee}
                selectedEmployeeId={selectedEmployeeId}
                selectedTeamId={selectedEmployee?.teamId}
                team={team}
                workspace={workspace}
              />
            );
          })}
        </div>
        <div className="absolute bottom-5 left-1/2 z-50 flex -translate-x-1/2 items-center gap-1 rounded-2xl border border-v3-line bg-white/92 px-3 py-2 text-sm font-semibold text-v3-ink shadow-v3 backdrop-blur">
          <button type="button" className="grid size-8 place-items-center rounded-xl text-v3-ink-2 hover:bg-v3-card-soft" aria-label="拖动画布">
            <Hand className="size-4" />
          </button>
          <span className="mx-1 h-5 w-px bg-v3-line" aria-hidden />
          <button type="button" className="grid size-8 place-items-center rounded-xl text-v3-ink-2 hover:bg-v3-card-soft" aria-label="缩小">
            <Minus className="size-4" />
          </button>
          <span className="px-3 tabular-nums">100%</span>
          <button type="button" className="grid size-8 place-items-center rounded-xl text-v3-ink-2 hover:bg-v3-card-soft" aria-label="放大">
            <Plus className="size-4" />
          </button>
          <span className="mx-1 h-5 w-px bg-v3-line" aria-hidden />
          <button type="button" className="flex h-8 items-center gap-1 rounded-xl px-2.5 text-v3-ink-2 hover:bg-v3-card-soft">
            <Maximize2 className="size-4" />
            适应视图
          </button>
        </div>
      </div>
    </section>
  );
}

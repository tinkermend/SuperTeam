import type { RuntimeOverviewFloor } from "../runtime-overview-model";

type RuntimeMapSvgLayerProps = {
  floor: RuntimeOverviewFloor;
  selectedTeamId?: string;
};

export function RuntimeMapSvgLayer({ floor, selectedTeamId }: RuntimeMapSvgLayerProps) {
  return (
    <svg
      aria-hidden="true"
      className="pointer-events-none absolute inset-0 z-10 h-full w-full overflow-visible"
      viewBox={`0 0 ${floor.layout.canvasWidth} ${floor.layout.canvasHeight}`}
    >
      {floor.layout.paths.map((path) => (
        <polyline
          key={path.id}
          fill="none"
          points={path.points.map((point) => `${point.x},${point.y}`).join(" ")}
          stroke={path.tone === "primary" ? "#4F8BFF" : "#A8C6E6"}
          strokeDasharray="7 7"
          strokeLinecap="round"
          strokeWidth="2"
        />
      ))}
      {floor.layout.teamWorkspaces.map((workspace) =>
        workspace.teamId === selectedTeamId ? (
          <polygon
            key={`${workspace.teamId}-selection`}
            fill="rgba(47,95,255,0.06)"
            points={workspace.polygon.map((point) => `${point.x},${point.y}`).join(" ")}
            stroke="#2F5FFF"
            strokeWidth="3"
          />
        ) : null,
      )}
    </svg>
  );
}

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
      <defs>
        <marker id={`runtime-callout-arrow-${floor.floorId}`} markerHeight="8" markerWidth="8" orient="auto" refX="7" refY="4">
          <path d="M0,0 L8,4 L0,8 Z" fill="#2F5FFF" opacity="0.62" />
        </marker>
      </defs>
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
      {floor.layout.teamWorkspaces.map((workspace) => {
        const selected = workspace.teamId === selectedTeamId;
        return (
          <line
            key={`${workspace.teamId}-callout-link`}
            data-runtime-team-callout-link={workspace.teamId}
            x1={workspace.cardAnchor.x + 98}
            y1={workspace.cardAnchor.y + 82}
            x2={workspace.calloutTarget.x}
            y2={workspace.calloutTarget.y}
            stroke={selected ? "#2F5FFF" : "#7EA7D8"}
            strokeDasharray={selected ? undefined : "6 8"}
            strokeLinecap="round"
            strokeWidth={selected ? 2.5 : 1.5}
            markerEnd={`url(#runtime-callout-arrow-${floor.floorId})`}
            opacity={selected ? 0.78 : 0.34}
          />
        );
      })}
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

import type { RuntimeOverviewFloor } from "../runtime-overview-model";
import type { ProjectLensDrawableEdge, ProjectLensEdgeTone } from "../runtime-overview-project-lens";

type RuntimeMapSvgLayerProps = {
  floor: RuntimeOverviewFloor;
  selectedTeamId?: string;
  lensEdges?: ProjectLensDrawableEdge[];
  hoveredLensEdgeId?: string;
  onHoverLensEdge?: (edgeId?: string) => void;
};

// 透镜边色调：muted=已完成/未开始的依赖、primary=当前活跃交接、warning=下游阻塞。
const lensEdgeStroke: Record<ProjectLensEdgeTone, string> = {
  muted: "#A8C6E6",
  primary: "var(--v3-brand)",
  warning: "var(--v3-warn)",
};

// 连线端点从座位锚点略微抬升到头像中心高度（头像绘制在座位上方 58px 处，见 EmployeeAvatarNode）。
const LENS_EDGE_Y_OFFSET = 34;

export function RuntimeMapSvgLayer({
  floor,
  hoveredLensEdgeId,
  lensEdges,
  onHoverLensEdge,
  selectedTeamId,
}: RuntimeMapSvgLayerProps) {
  // lensEdges 一旦传入（即使当前楼层为空数组）即视为透镜态。
  const lensActive = lensEdges !== undefined;
  return (
    <svg
      aria-hidden="true"
      className="pointer-events-none absolute inset-0 z-10 h-full w-full overflow-visible"
      viewBox={`0 0 ${floor.layout.canvasWidth} ${floor.layout.canvasHeight}`}
    >
      <defs>
        <marker id={`runtime-callout-arrow-${floor.floorId}`} markerHeight="8" markerWidth="8" orient="auto" refX="7" refY="4">
          <path d="M0,0 L8,4 L0,8 Z" fill="#6482A3" opacity="0.68" />
        </marker>
        {(["muted", "primary", "warning"] as const).map((tone) => (
          <marker
            key={tone}
            id={`runtime-lens-arrow-${floor.floorId}-${tone}`}
            markerHeight="9"
            markerWidth="9"
            orient="auto"
            refX="8"
            refY="4.5"
          >
            <path d="M0,0 L9,4.5 L0,9 Z" fill={lensEdgeStroke[tone]} />
          </marker>
        ))}
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
            stroke={selected ? "#3E6FAD" : "#6482A3"}
            strokeDasharray={selected ? undefined : "6 8"}
            strokeLinecap="round"
            strokeWidth={selected ? 2.5 : 1.5}
            markerEnd={`url(#runtime-callout-arrow-${floor.floorId})`}
            // 透镜态下装饰性呼出线整体退后，把视觉焦点让给交接链路。
            opacity={lensActive ? 0.12 : selected ? 0.68 : 0.3}
          />
        );
      })}
      {lensEdges?.map((edge) => {
        const dimmedByHover = Boolean(hoveredLensEdgeId) && hoveredLensEdgeId !== edge.id;
        const midX = (edge.from.x + edge.to.x) / 2;
        const midY = (edge.from.y + edge.to.y) / 2 - LENS_EDGE_Y_OFFSET;
        return (
          <g
            key={edge.id}
            data-runtime-lens-edge={edge.id}
            data-runtime-lens-edge-tone={edge.tone}
            className="pointer-events-auto"
            opacity={dimmedByHover ? 0.18 : 0.92}
            onMouseEnter={() => onHoverLensEdge?.(edge.id)}
            onMouseLeave={() => onHoverLensEdge?.(undefined)}
          >
            {/* 加宽透明命中线，供悬停聚焦 */}
            <line
              x1={edge.from.x}
              y1={edge.from.y - LENS_EDGE_Y_OFFSET}
              x2={edge.to.x}
              y2={edge.to.y - LENS_EDGE_Y_OFFSET}
              stroke="transparent"
              strokeWidth="16"
            />
            <line
              x1={edge.from.x}
              y1={edge.from.y - LENS_EDGE_Y_OFFSET}
              x2={edge.to.x}
              y2={edge.to.y - LENS_EDGE_Y_OFFSET}
              stroke={lensEdgeStroke[edge.tone]}
              strokeLinecap="round"
              strokeWidth="2.5"
              markerEnd={`url(#runtime-lens-arrow-${floor.floorId}-${edge.tone})`}
            />
            {edge.taskCount > 1 ? (
              <text
                x={midX}
                y={midY - 8}
                fill={lensEdgeStroke[edge.tone]}
                fontSize="13"
                fontWeight="600"
                textAnchor="middle"
              >
                ×{edge.taskCount}
              </text>
            ) : null}
          </g>
        );
      })}
    </svg>
  );
}

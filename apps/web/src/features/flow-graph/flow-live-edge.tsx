import type { CSSProperties } from "react";
import {
  BaseEdge,
  getSmoothStepPath,
  type Edge,
  type EdgeProps,
} from "@xyflow/react";
import type { FlowLiveEdgeActivity, FlowLiveEdgeData } from "./flow-graph-adapter";
import { usePrefersReducedMotion } from "./use-prefers-reduced-motion";

export type FlowLiveEdgeType = Edge<FlowLiveEdgeData, "flowLive">;

/** 粒子错峰起飞的相位（秒）：三个数据包沿边依次流动。 */
const PARTICLE_DELAYS_SECONDS = [0, 0.9, 1.8];
const PARTICLE_TRAVEL_SECONDS = 2.7;

/** 活性四态 → 边描边样式；颜色只用状态色 token，不新增 token（spec §2）。 */
const ACTIVITY_EDGE_STYLE: Record<FlowLiveEdgeActivity, CSSProperties> = {
  flowing: { stroke: "var(--brand)", strokeOpacity: 0.45, strokeWidth: 2 },
  done: { stroke: "var(--ok)", strokeOpacity: 0.8, strokeWidth: 2 },
  failed: { stroke: "var(--danger)", strokeOpacity: 0.9, strokeWidth: 2 },
  idle: {
    stroke: "var(--line-strong)",
    strokeDasharray: "4 6",
    strokeWidth: 1.5,
  },
};

/**
 * 活性边（概念 A 粒子流，spec 2026-07-27 §1.1）：跟随 xyflow 自定义 edge 惯例
 * （EdgeProps + getSmoothStepPath + BaseEdge），粒子用 SVG SMIL animateMotion
 * 沿同一 path 几何流动，不另建 overlay 层。`prefers-reduced-motion` 或大图性能
 * 降级（data.scaleDegraded，spec §5 P2-S）下不渲染粒子，flowing 边走同一降级
 * 分支：呼吸边（透明度脉动）。
 */
export function FlowLiveEdge({
  id,
  data,
  label,
  markerEnd,
  sourcePosition,
  sourceX,
  sourceY,
  targetPosition,
  targetX,
  targetY,
}: EdgeProps<FlowLiveEdgeType>) {
  const prefersReducedMotion = usePrefersReducedMotion();
  const [edgePath, labelX, labelY] = getSmoothStepPath({
    sourcePosition,
    sourceX,
    sourceY,
    targetPosition,
    targetX,
    targetY,
  });
  const activity = data?.activity ?? "idle";
  const isFlowing = activity === "flowing";
  // 呼吸边降级两来源取或（spec §5 P2-S）：用户减弱动效偏好 / 大图元素超阈值。
  const degradedToBreathing = prefersReducedMotion || data?.scaleDegraded === true;
  const showParticles = isFlowing && !degradedToBreathing;
  const particlePathId = `flow-live-particle-path-${id}`;

  return (
    <>
      <BaseEdge
        className={isFlowing && degradedToBreathing ? "animate-pulse" : undefined}
        data-activity={activity}
        data-testid={`flow-live-edge-${id}`}
        id={id}
        label={label}
        labelX={labelX}
        labelY={labelY}
        markerEnd={markerEnd}
        path={edgePath}
        style={ACTIVITY_EDGE_STYLE[activity]}
      />
      {showParticles ? (
        <g aria-hidden data-testid={`flow-live-particles-${id}`}>
          <path d={edgePath} fill="none" id={particlePathId} stroke="none" />
          {PARTICLE_DELAYS_SECONDS.map((delaySeconds) => (
            <circle fill="var(--brand)" key={delaySeconds} opacity={0.85} r={3}>
              <animateMotion
                begin={`${delaySeconds}s`}
                dur={`${PARTICLE_TRAVEL_SECONDS}s`}
                repeatCount="indefinite"
              >
                <mpath href={`#${particlePathId}`} />
              </animateMotion>
            </circle>
          ))}
        </g>
      ) : null}
    </>
  );
}

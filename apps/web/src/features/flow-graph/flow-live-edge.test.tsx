import { Position } from "@xyflow/react";
import { afterEach, describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import type { FlowLiveEdgeData } from "./flow-graph-adapter";
import { FlowLiveEdge } from "./flow-live-edge";

/** 覆写 matchMedia 控制 prefers-reduced-motion；返回还原函数。 */
function stubReducedMotion(matches: boolean): () => void {
  const original = window.matchMedia;
  window.matchMedia = ((query: string) =>
    ({
      addEventListener: () => undefined,
      addListener: () => undefined,
      dispatchEvent: () => false,
      matches: query.includes("prefers-reduced-motion") ? matches : false,
      media: query,
      onchange: null,
      removeEventListener: () => undefined,
      removeListener: () => undefined,
    }) as MediaQueryList) as typeof window.matchMedia;
  return () => {
    window.matchMedia = original;
  };
}

let restoreMatchMedia: (() => void) | undefined;

afterEach(() => {
  restoreMatchMedia?.();
  restoreMatchMedia = undefined;
});

function renderEdge(data: FlowLiveEdgeData, label = "已解除阻塞") {
  return render(
    <svg>
      <FlowLiveEdge
        data={data}
        id="edge:task-a:task-b"
        label={label}
        source="task:task-a"
        sourcePosition={Position.Bottom}
        sourceX={0}
        sourceY={0}
        target="task:task-b"
        targetPosition={Position.Top}
        targetX={120}
        targetY={200}
      />
    </svg>,
  );
}

function edgeData(activity: FlowLiveEdgeData["activity"]): FlowLiveEdgeData {
  return { activity, blockerTaskId: "task-a", dependentTaskId: "task-b" };
}

describe("FlowLiveEdge", () => {
  it("renders flowing particles along the edge path when motion is allowed", async () => {
    restoreMatchMedia = stubReducedMotion(false);
    const screen = await renderEdge(edgeData("flowing"));

    const path = screen.container.querySelector('[data-activity="flowing"]');
    expect(path).not.toBeNull();
    const particles = screen.container.querySelector(
      '[data-testid="flow-live-particles-edge:task-a:task-b"]',
    );
    expect(particles).not.toBeNull();
    expect(particles?.querySelectorAll("animateMotion")).toHaveLength(3);
    expect(particles?.querySelectorAll("mpath")).toHaveLength(3);
    // 中文状态标签随边渲染。
    expect(screen.container.textContent).toContain("已解除阻塞");
  });

  it("degrades to a breathing edge without particles under prefers-reduced-motion", async () => {
    restoreMatchMedia = stubReducedMotion(true);
    const screen = await renderEdge(edgeData("flowing"));

    expect(
      screen.container.querySelector(
        '[data-testid="flow-live-particles-edge:task-a:task-b"]',
      ),
    ).toBeNull();
    expect(screen.container.querySelector("animateMotion")).toBeNull();
    const path = screen.container.querySelector('[data-activity="flowing"]');
    expect(path?.classList.contains("animate-pulse")).toBe(true);
  });

  it("stops the flow and shows a danger stroke for failed handoffs", async () => {
    restoreMatchMedia = stubReducedMotion(false);
    const screen = await renderEdge(edgeData("failed"), "已阻塞");

    expect(screen.container.querySelector("animateMotion")).toBeNull();
    const path = screen.container.querySelector<SVGPathElement>(
      '[data-activity="failed"]',
    );
    expect(path).not.toBeNull();
    expect(path?.style.stroke).toContain("--danger");
    expect(path?.classList.contains("animate-pulse")).toBe(false);
  });

  it("renders done and idle edges as static strokes without particles", async () => {
    restoreMatchMedia = stubReducedMotion(false);
    const doneScreen = await renderEdge(edgeData("done"), "已完成");
    expect(doneScreen.container.querySelector("animateMotion")).toBeNull();
    expect(
      doneScreen.container.querySelector<SVGPathElement>('[data-activity="done"]')
        ?.style.stroke,
    ).toContain("--ok");
    doneScreen.unmount();

    const idleScreen = await renderEdge(edgeData("idle"), "已计划");
    expect(idleScreen.container.querySelector("animateMotion")).toBeNull();
    const idlePath = idleScreen.container.querySelector<SVGPathElement>(
      '[data-activity="idle"]',
    );
    // 浏览器把 "4 6" 归一化为 "4, 6"，两种写法都接受。
    expect(idlePath?.style.strokeDasharray.replace(",", "")).toBe("4 6");
  });
});

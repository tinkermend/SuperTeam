import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import {
  useProjectActivityInvalidate,
  type ProjectActivityInvalidateOptions,
} from "./use-project-activity-invalidate";

type FakeStream = {
  close: ReturnType<typeof vi.fn>;
  emitActivity: (data: unknown) => void;
  emitRaw: (data: string) => void;
  source: EventSource;
  urls: string[];
};

function fakeStream(): FakeStream {
  const listeners: Record<string, Array<(event: { data: string }) => void>> = {};
  const close = vi.fn();
  const urls: string[] = [];
  const source = {
    addEventListener: (type: string, listener: (event: { data: string }) => void) => {
      (listeners[type] ??= []).push(listener);
    },
    close,
    removeEventListener: (type: string, listener: (event: { data: string }) => void) => {
      listeners[type] = (listeners[type] ?? []).filter((entry) => entry !== listener);
    },
  } as unknown as EventSource;
  return {
    close,
    emitActivity: (data) => {
      for (const listener of listeners["activity"] ?? []) {
        listener({ data: JSON.stringify(data) });
      }
    },
    emitRaw: (data) => {
      for (const listener of listeners["activity"] ?? []) {
        listener({ data });
      }
    },
    source,
    urls,
  };
}

function Harness(props: ProjectActivityInvalidateOptions) {
  useProjectActivityInvalidate(props);
  return null;
}

async function renderHook(overrides: Partial<ProjectActivityInvalidateOptions> = {}) {
  const stream = fakeStream();
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
  const screen = await render(
    <QueryClientProvider client={queryClient}>
      <Harness
        apiBaseUrl="http://cp.test"
        eventSourceFactory={(url) => {
          stream.urls.push(url);
          return stream.source;
        }}
        projectId="project-1"
        {...overrides}
      />
    </QueryClientProvider>,
  );
  return { invalidateSpy, screen, stream };
}

afterEach(() => {
  vi.useRealTimers();
});

describe("useProjectActivityInvalidate", () => {
  it("invalidates the graph and workflow-detail queries for events of the current project", async () => {
    const { invalidateSpy, stream } = await renderHook();

    expect(stream.urls).toEqual([
      "http://cp.test/api/v1/digital-employees/activity/stream",
    ]);

    stream.emitActivity({ event_id: "evt-1", project_id: "project-1" });

    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ["project-task-graph", "project-1"],
    });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["workflow-detail"] });
    // 一单卷宗是需求处所的主读模型，漏掉它会让时间线/待你处理停在旧值。
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["demand-dossier"] });
  });

  it("ignores events of other projects, missing project_id and malformed payloads", async () => {
    const { invalidateSpy, stream } = await renderHook();

    stream.emitActivity({ event_id: "evt-1", project_id: "project-2" });
    stream.emitActivity({ event_id: "evt-2" });
    stream.emitRaw("not-json");

    expect(invalidateSpy).not.toHaveBeenCalled();
  });

  it("merges events inside the throttle window into one trailing invalidate", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const { invalidateSpy, stream } = await renderHook();

      // 一次 invalidate 会刷新这一处所的全部读模型（卷宗 / 图 / launch-detail），
      // 断言按"刷了几轮"算，避免加一个 key 就要改一串数字。
      const KEYS_PER_INVALIDATE = 3;
      const rounds = () => invalidateSpy.mock.calls.length / KEYS_PER_INVALIDATE;

      // 窗口内三连事件：首个立即刷，其余合并成窗口末一次（不丢最后一拍）。
      stream.emitActivity({ event_id: "evt-1", project_id: "project-1" });
      expect(rounds()).toBe(1);

      stream.emitActivity({ event_id: "evt-2", project_id: "project-1" });
      stream.emitActivity({ event_id: "evt-3", project_id: "project-1" });
      expect(rounds()).toBe(1);

      await vi.advanceTimersByTimeAsync(1_100);
      expect(rounds()).toBe(2);

      // 距上次 invalidate 再过一个完整窗口后，新事件恢复立即刷。
      await vi.advanceTimersByTimeAsync(1_000);
      stream.emitActivity({ event_id: "evt-4", project_id: "project-1" });
      expect(rounds()).toBe(3);
    } finally {
      vi.useRealTimers();
    }
  });

  it("does not connect when disabled and closes the stream on unmount", async () => {
    const disabled = await renderHook({ enabled: false });
    expect(disabled.stream.urls).toEqual([]);

    const { invalidateSpy, screen, stream } = await renderHook();
    screen.unmount();
    expect(stream.close).toHaveBeenCalled();
    stream.emitActivity({ event_id: "evt-after", project_id: "project-1" });
    expect(invalidateSpy).not.toHaveBeenCalled();
  });
});

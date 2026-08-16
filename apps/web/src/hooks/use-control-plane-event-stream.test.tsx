import { afterEach, describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import {
  useControlPlaneEventStream,
  type ControlPlaneEventStreamOptions,
} from "./use-control-plane-event-stream";

type FakeStream = {
  close: ReturnType<typeof vi.fn>;
  emit: (type: string, data?: string) => void;
  source: EventSource;
  urls: string[];
};

function fakeStream(): FakeStream {
  const listeners: Record<string, Array<(event: { data?: string }) => void>> = {};
  const close = vi.fn();
  const urls: string[] = [];
  const source = {
    addEventListener: (type: string, listener: (event: { data?: string }) => void) => {
      (listeners[type] ??= []).push(listener);
    },
    close,
    removeEventListener: (type: string, listener: (event: { data?: string }) => void) => {
      listeners[type] = (listeners[type] ?? []).filter((entry) => entry !== listener);
    },
  } as unknown as EventSource;
  return {
    close,
    emit: (type, data) => {
      for (const listener of listeners[type] ?? []) listener({ data });
    },
    source,
    urls,
  };
}

function Harness(props: ControlPlaneEventStreamOptions) {
  useControlPlaneEventStream(props);
  return null;
}

async function renderStream(overrides: Partial<ControlPlaneEventStreamOptions> = {}) {
  const stream = fakeStream();
  const screen = await render(
    <Harness
      apiBaseUrl="http://cp.test"
      eventSourceFactory={(url) => {
        stream.urls.push(url);
        return stream.source;
      }}
      path="/api/v1/digital-employees/activity/stream"
      {...overrides}
    />,
  );
  return { screen, stream };
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("useControlPlaneEventStream", () => {
  it("connects with the normalized URL and forwards activity events to the latest handler", async () => {
    const onEvent = vi.fn();
    const { stream } = await renderStream({ onEvent });

    expect(stream.urls).toEqual([
      "http://cp.test/api/v1/digital-employees/activity/stream",
    ]);

    stream.emit("activity", `{"event_id":"evt-1"}`);
    expect(onEvent).toHaveBeenCalledTimes(1);
    expect((onEvent.mock.calls[0][0] as MessageEvent).data).toBe(`{"event_id":"evt-1"}`);
  });

  it("strips trailing slashes from the base URL", async () => {
    const { stream } = await renderStream({ apiBaseUrl: "http://cp.test/" });
    expect(stream.urls).toEqual([
      "http://cp.test/api/v1/digital-employees/activity/stream",
    ]);
  });

  it("notifies open and error, and supports custom event names", async () => {
    const onOpen = vi.fn();
    const onError = vi.fn();
    const onEvent = vi.fn();
    const { stream } = await renderStream({
      eventName: "inbox-changed",
      onEvent,
      onError,
      onOpen,
    });

    stream.emit("open");
    expect(onOpen).toHaveBeenCalledTimes(1);

    stream.emit("error");
    expect(onError).toHaveBeenCalledTimes(1);

    stream.emit("inbox-changed");
    stream.emit("activity");
    expect(onEvent).toHaveBeenCalledTimes(1);
  });

  it("does not connect when disabled and closes the stream on unmount", async () => {
    const disabled = await renderStream({ enabled: false });
    expect(disabled.stream.urls).toEqual([]);

    const onEvent = vi.fn();
    const { screen, stream } = await renderStream({ onEvent });
    screen.unmount();
    expect(stream.close).toHaveBeenCalled();
    stream.emit("activity");
    expect(onEvent).not.toHaveBeenCalled();
  });

  it("silently skips connecting when the factory throws", async () => {
    const screen = await render(
      <Harness
        apiBaseUrl="http://cp.test"
        eventSourceFactory={() => {
          throw new Error("no EventSource");
        }}
        path="/api/v1/stream"
      />,
    );
    expect(screen).toBeTruthy();
  });
});

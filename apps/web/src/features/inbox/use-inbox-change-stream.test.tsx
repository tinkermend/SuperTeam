import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { useInboxChangeStream } from "./use-inbox-change-stream";

function Harness({
  apiBaseUrl,
  eventSourceFactory,
  enabled,
}: {
  apiBaseUrl: string;
  eventSourceFactory?: (url: string) => EventSource;
  enabled?: boolean;
}) {
  useInboxChangeStream({ apiBaseUrl, eventSourceFactory, enabled });
  return <div>stream-harness</div>;
}

describe("useInboxChangeStream", () => {
  it("opens the inbox stream and invalidates items + badge on inbox-changed", async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
    const listeners: Record<string, () => void> = {};
    const fakeSource = {
      addEventListener: (type: string, listener: () => void) => {
        listeners[type] = listener;
      },
      removeEventListener: () => {},
      close: vi.fn(),
    } as unknown as EventSource;
    const streamUrls: string[] = [];

    await render(
      <QueryClientProvider client={queryClient}>
        <Harness
          apiBaseUrl="http://control-plane.local"
          eventSourceFactory={(url) => {
            streamUrls.push(url);
            return fakeSource;
          }}
        />
      </QueryClientProvider>,
    );

    expect(streamUrls).toEqual(["http://control-plane.local/api/v1/inbox/stream"]);
    listeners["inbox-changed"]?.();
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["inbox-items"] });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["inbox-badge"] });
  });

  it("does not open a stream when enabled is false", async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    const streamUrls: string[] = [];

    await render(
      <QueryClientProvider client={queryClient}>
        <Harness
          apiBaseUrl="http://control-plane.local"
          enabled={false}
          eventSourceFactory={(url) => {
            streamUrls.push(url);
            return {
              addEventListener: () => {},
              removeEventListener: () => {},
              close: () => {},
            } as unknown as EventSource;
          }}
        />
      </QueryClientProvider>,
    );

    expect(streamUrls).toEqual([]);
  });
});

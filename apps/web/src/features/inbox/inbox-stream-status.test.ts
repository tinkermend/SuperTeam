import { describe, expect, it } from "vitest";
import {
  inboxBadgeRefetchInterval,
  inboxListRefetchInterval,
  INBOX_BADGE_REFETCH_CONNECTED_MS,
  INBOX_BADGE_REFETCH_DISCONNECTED_MS,
  INBOX_LIST_REFETCH_CONNECTED_MS,
  INBOX_LIST_REFETCH_DISCONNECTED_MS,
} from "./inbox-stream-status";

describe("inbox stream refetch intervals", () => {
  it("uses fast polling while disconnected or connecting", () => {
    expect(inboxListRefetchInterval("disconnected")).toBe(INBOX_LIST_REFETCH_DISCONNECTED_MS);
    expect(inboxListRefetchInterval("connecting")).toBe(INBOX_LIST_REFETCH_DISCONNECTED_MS);
    expect(inboxBadgeRefetchInterval("disconnected")).toBe(INBOX_BADGE_REFETCH_DISCONNECTED_MS);
    expect(inboxBadgeRefetchInterval("connecting")).toBe(INBOX_BADGE_REFETCH_DISCONNECTED_MS);
  });

  it("slows polling once the stream is connected", () => {
    expect(inboxListRefetchInterval("connected")).toBe(INBOX_LIST_REFETCH_CONNECTED_MS);
    expect(inboxBadgeRefetchInterval("connected")).toBe(INBOX_BADGE_REFETCH_CONNECTED_MS);
    expect(INBOX_LIST_REFETCH_DISCONNECTED_MS).toBeLessThan(INBOX_LIST_REFETCH_CONNECTED_MS);
    expect(INBOX_BADGE_REFETCH_DISCONNECTED_MS).toBeLessThan(INBOX_BADGE_REFETCH_CONNECTED_MS);
  });
});

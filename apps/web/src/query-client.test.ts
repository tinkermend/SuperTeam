import { describe, expect, it } from "vitest";
import { ApiRequestError } from "@/lib/api";
import { shouldRetryQuery } from "./query-client";

describe("shouldRetryQuery", () => {
  it("does not retry deterministic client-side API errors", () => {
    expect(shouldRetryQuery(0, new ApiRequestError("create options", 422, "team governance required"))).toBe(false);
    expect(shouldRetryQuery(0, new ApiRequestError("authz", 403, "forbidden"))).toBe(false);
    expect(shouldRetryQuery(0, new ApiRequestError("missing", 404, "not found"))).toBe(false);
  });

  it("retries transient non-client errors up to two times", () => {
    expect(shouldRetryQuery(0, new Error("network"))).toBe(true);
    expect(shouldRetryQuery(1, new ApiRequestError("server", 500, "temporary"))).toBe(true);
    expect(shouldRetryQuery(2, new Error("network"))).toBe(false);
  });
});

import { describe, expect, it, vi } from "vitest";
import {
  canonicalLocalDevHref,
  redirectToCanonicalLocalDevHost,
} from "./canonical-local-dev-host";

describe("canonicalLocalDevHref", () => {
  it("rewrites localhost and ::1 to 127.0.0.1 and keeps path/search/hash", () => {
    expect(canonicalLocalDevHref("http://localhost:3000/inbox?view=mine#top")).toBe(
      "http://127.0.0.1:3000/inbox?view=mine#top",
    );
    expect(canonicalLocalDevHref("http://[::1]:3000/projects")).toBe(
      "http://127.0.0.1:3000/projects",
    );
  });

  it("returns null when already on 127.0.0.1 or a non-local host", () => {
    expect(canonicalLocalDevHref("http://127.0.0.1:3000/inbox")).toBeNull();
    expect(canonicalLocalDevHref("https://console.example.com/inbox")).toBeNull();
  });
});

describe("redirectToCanonicalLocalDevHost", () => {
  it("assigns the canonical href and returns true for localhost", () => {
    const assign = vi.fn();
    expect(
      redirectToCanonicalLocalDevHost({ href: "http://localhost:3000/inbox" }, assign),
    ).toBe(true);
    expect(assign).toHaveBeenCalledWith("http://127.0.0.1:3000/inbox");
  });

  it("does nothing when already canonical", () => {
    const assign = vi.fn();
    expect(
      redirectToCanonicalLocalDevHost({ href: "http://127.0.0.1:3000/inbox" }, assign),
    ).toBe(false);
    expect(assign).not.toHaveBeenCalled();
  });
});

import { describe, expect, it } from "vitest";
import { providerDisplayName } from "./provider-label";

describe("providerDisplayName", () => {
  it("maps known provider ids across separator and case variants", () => {
    expect(providerDisplayName("claude-code")).toBe("Claude Code");
    expect(providerDisplayName("claude_code")).toBe("Claude Code");
    expect(providerDisplayName("claude")).toBe("Claude Code");
    expect(providerDisplayName("OpenCode")).toBe("OpenCode");
    expect(providerDisplayName("open-code")).toBe("OpenCode");
    expect(providerDisplayName(" codex ")).toBe("Codex");
  });

  it("falls back to the raw value for unknown providers", () => {
    expect(providerDisplayName("custom-provider")).toBe("custom-provider");
  });
});

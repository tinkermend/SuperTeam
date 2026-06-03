import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import { TeamIconTile, getTeamDisplayConfig } from "./team-icon-tile";

describe("TeamIconTile", () => {
  it("resolves known ops display metadata", async () => {
    expect(getTeamDisplayConfig({ display: { icon_key: "ops", color_tone: "cyan" } })).toMatchObject({
      iconKey: "ops",
      tone: "cyan",
    });
    const screen = await render(<TeamIconTile metadata={{ display: { icon_key: "ops", color_tone: "cyan" } }} />);
    await expect.element(screen.getByLabelText("运维团队图标")).toBeInTheDocument();
  });

  it("falls back to neutral team icon", () => {
    expect(getTeamDisplayConfig({ display: { icon_key: "unknown", color_tone: "unknown" } })).toMatchObject({
      iconKey: "default",
      tone: "neutral",
    });
  });
});

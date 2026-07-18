import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import { TeamIconTile, getTeamDisplayConfig } from "./team-icon-tile";

describe("TeamIconTile", () => {
  it("falls legacy display metadata back to the general team illustration", async () => {
    expect(getTeamDisplayConfig({ display: { icon_key: "ops", color_tone: "cyan" } })).toMatchObject({
      iconKey: "role-general-team",
      imageSrc: "/images/team-role-icons/general-team.webp",
      tone: "cyan",
    });
    const screen = await render(<TeamIconTile metadata={{ display: { icon_key: "ops", color_tone: "cyan" } }} />);
    await expect.element(screen.getByLabelText("通用团队")).toBeInTheDocument();
  });

  it("falls back to neutral team icon", () => {
    expect(getTeamDisplayConfig({ display: { icon_key: "unknown", color_tone: "unknown" } })).toMatchObject({
      iconKey: "role-general-team",
      tone: "neutral",
    });
  });

  it("resolves a generated role illustration", async () => {
    const config = getTeamDisplayConfig({
      display: { icon_key: "role-quality-automation", color_tone: "blue" },
    });

    expect(config).toMatchObject({
      iconKey: "role-quality-automation",
      imageSrc: "/images/team-role-icons/quality-automation.webp",
      label: "自动化测试",
    });

    const screen = await render(
      <TeamIconTile
        metadata={{
          display: { icon_key: "role-quality-automation", color_tone: "blue" },
        }}
      />,
    );
    await expect.element(screen.getByLabelText("自动化测试")).toBeInTheDocument();
  });
});

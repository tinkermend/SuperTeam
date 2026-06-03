import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import { UserIdentity, buildUserAvatarDataUri, getUserIdentityLabel } from "./user-identity";

describe("UserIdentity", () => {
  it("renders display name, email and avatar image", async () => {
    const screen = await render(
      <UserIdentity
        showSecondary
        user={{
          avatar: { provider: "dicebear", seed: "user:zhou", style: "adventurer" },
          display_name: "周敏",
          email: "zhoumin@example.com",
          id: "user-1",
          status: "active",
          username: "zhoumin",
        }}
      />,
    );

    await expect.element(screen.getByText("周敏")).toBeInTheDocument();
    await expect.element(screen.getByText("zhoumin@example.com")).toBeInTheDocument();
    await expect.element(screen.getByAltText("周敏 的头像")).toBeInTheDocument();
  });

  it("renders compact identity with small size API", async () => {
    const screen = await render(
      <UserIdentity
        size="sm"
        user={{
          avatar: { provider: "dicebear", seed: "user:xu", style: "adventurer" },
          display_name: "许越",
          id: "user-3",
          status: "active",
          username: "xuyue",
        }}
      />,
    );

    await expect.element(screen.getByText("许越")).toBeInTheDocument();
    await expect.element(screen.getByAltText("许越 的头像")).toBeInTheDocument();
  });

  it("falls back to username and initials without avatar", () => {
    expect(
      getUserIdentityLabel({
        id: "user-2",
        status: "active",
        username: "operator",
      }),
    ).toEqual({ primary: "operator", secondary: "user-2", initials: "O" });
  });

  it("builds empty src for unsupported avatar descriptor", () => {
    expect(
      buildUserAvatarDataUri(
        { provider: "custom" as "dicebear", seed: "x", style: "unknown" as "adventurer" },
        "operator",
      ),
    ).toBe("");
  });

  it("builds empty src for missing avatar descriptor", () => {
    expect(buildUserAvatarDataUri(undefined, "operator")).toBe("");
    expect(buildUserAvatarDataUri(null, "operator")).toBe("");
  });
});

import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import { UserIdentity, UserIdentityAvatar, buildUserAvatarDataUri, getUserIdentityLabel } from "./user-identity";

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

  it("prefers shared avatar asset thumbnail over DiceBear avatar", async () => {
    const screen = await render(
      <UserIdentityAvatar
        user={{
          avatar: { provider: "dicebear", seed: "user:asset-fallback", style: "adventurer" },
          avatar_asset_id: "engineer-f-03",
          display_name: "头像用户",
          id: "user-asset",
          status: "active",
          username: "asset-user",
        }}
      />,
    );

    await expect.element(screen.getByAltText("头像用户 的头像")).toHaveAttribute(
      "src",
      "/images/digital-employee-avatars/engineer-f-03-256.webp",
    );
  });

  it("falls back to DiceBear when shared avatar asset is unavailable", async () => {
    const screen = await render(
      <UserIdentityAvatar
        user={{
          avatar: { provider: "dicebear", seed: "user:dicebear-fallback", style: "adventurer" },
          avatar_asset_id: "unknown-asset",
          display_name: "默认头像用户",
          id: "user-dicebear",
          status: "active",
          username: "dicebear-user",
        }}
      />,
    );

    await expect.element(screen.getByAltText("默认头像用户 的头像")).toHaveAttribute("src", expect.stringContaining("data:image/svg+xml"));
  });

  it("renders nullable backend identity fields with username and DiceBear fallback", async () => {
    const screen = await render(
      <UserIdentity
        showSecondary
        user={{
          avatar: { provider: "dicebear", seed: "user:nullable", style: "adventurer" },
          avatar_asset_id: null,
          display_name: null,
          email: null,
          id: "user-nullable",
          status: "active",
          username: "nullable-user",
        }}
      />,
    );

    await expect.element(screen.getByText("nullable-user")).toBeInTheDocument();
    await expect.element(screen.getByText("user-nullable")).toBeInTheDocument();
    await expect.element(screen.getByAltText("nullable-user 的头像")).toHaveAttribute("src", expect.stringContaining("data:image/svg+xml"));
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

# Team Management Experience Gap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 补齐团队管理第一阶段体验缺口：头像身份展示、团队图标、弱分页、目标化创建抽屉、成员页用户搜索和设计风格验收。

**Architecture:** 保持团队管理的组织/成员控制台定位，后端只补轻量身份与 `metadata.display` 校验，不新增团队类型表或准确总数分页协议。前端把用户身份、团队图标、角色展示和用户搜索沉淀为复用组件，再在列表、创建抽屉和详情成员页接入。

**Tech Stack:** Go + chi/net/http + pgx/sqlc + OpenAPI；React + TanStack Query + shadcn/ui + Radix UI + Tailwind CSS + lucide-react + DiceBear；Vitest Browser + Go test + Playwright smoke。

---

## File Map

**Backend contract and data**

- Modify: `contracts/control-plane/openapi.yaml`，给 `TeamHumanOwner` 和 `TeamMember` 增加可选 `avatar` 字段，复用现有 auth avatar 语义。
- Modify: `apps/control-plane/internal/storage/queries/tenant_team_config.sql`，团队列表、单团队概览和成员列表查询带出 `auth_users` 的 avatar 字段。
- Generated: `apps/control-plane/internal/storage/queries/*.sql.go`，由 `make -C apps/control-plane generate-sqlc` 更新。
- Modify: `apps/control-plane/internal/tenant/types.go`，增加团队管理消费型 `UserAvatarConfig`，并挂到 `TeamHumanOwner` / `TeamMember`。
- Modify: `apps/control-plane/internal/tenant/pg_repository.go`，把 SQL avatar 字段映射到 tenant domain。
- Modify: `apps/control-plane/internal/tenant/service.go`，复制 avatar 字段并增加 `metadata.display` 轻校验。
- Modify: `apps/control-plane/internal/tenant/handler.go`，响应 JSON 输出 `avatar`。
- Test: `apps/control-plane/internal/tenant/service_test.go`，覆盖 metadata display 校验和 avatar clone。
- Test: `apps/control-plane/internal/api/team_routes_test.go`，覆盖团队 owner/member avatar JSON。

**Frontend shared components**

- Create: `apps/web/src/components/superteam/user-identity.tsx`，统一头像、姓名、辅助信息。
- Create: `apps/web/src/components/superteam/team-icon-tile.tsx`，渲染 `metadata.display.icon_key/color_tone`。
- Create: `apps/web/src/components/superteam/team-role.tsx`，沉淀角色 label、badge 和 select。
- Create: `apps/web/src/components/superteam/user-search-select.tsx`，封装 active 用户搜索。
- Modify: `apps/web/src/components/superteam/index.ts`，导出新增组件。
- Modify: `apps/web/src/features/users/components/user-avatar.tsx`，复用 `UserIdentityAvatar`，避免头像逻辑停留在用户模块私有层。
- Test: `apps/web/src/components/superteam/user-identity.test.tsx`
- Test: `apps/web/src/components/superteam/team-icon-tile.test.tsx`
- Test: `apps/web/src/components/superteam/team-role.test.tsx`
- Test: `apps/web/src/components/superteam/user-search-select.test.tsx`

**Frontend teams feature**

- Modify: `apps/web/src/lib/api/teams.ts`，补 `limit/offset`，补 team owner/member avatar 类型，补 metadata display helper 类型。
- Test: `apps/web/src/lib/api/teams.test.ts`
- Modify: `apps/web/src/features/teams/index.tsx`，维护 `pageIndex/pageSize`，查询包含 `limit/offset`，筛选变化重置第一页，创建 payload 带 `metadata.display`。
- Modify: `apps/web/src/features/teams/components/team-list-table.tsx`，图标、负责人身份、治理状态 tone、行菜单和分页 footer。
- Modify: `apps/web/src/features/teams/components/create-team-drawer.tsx`，stepper、宽度、footer 状态和 draft display 字段。
- Modify: `apps/web/src/features/teams/components/create-team-basic-step.tsx`，团队图标选择和负责人搜索复用。
- Modify: `apps/web/src/features/teams/components/create-team-members-step.tsx`，候选成员表、checkbox、角色筛选、已选成员表和可删除/可改角色。
- Modify: `apps/web/src/features/teams/components/team-members-tab.tsx`，直接添加和高权限申请改成用户搜索。
- Test: `apps/web/src/features/teams/index.test.tsx`

**Docs and verification**

- Modify: `CHANGELOG.md`，新增一条本地 `Asia/Shanghai` 时间的团队管理体验补齐记录。

---

## Task 1: Backend Avatar Echo And Metadata Display Validation

**Files:**

- Modify: `contracts/control-plane/openapi.yaml`
- Modify: `apps/control-plane/internal/storage/queries/tenant_team_config.sql`
- Modify: `apps/control-plane/internal/tenant/types.go`
- Modify: `apps/control-plane/internal/tenant/pg_repository.go`
- Modify: `apps/control-plane/internal/tenant/service.go`
- Modify: `apps/control-plane/internal/tenant/handler.go`
- Test: `apps/control-plane/internal/tenant/service_test.go`
- Test: `apps/control-plane/internal/api/team_routes_test.go`

- [ ] **Step 1: Add failing service tests for metadata display**

Add these tests to `apps/control-plane/internal/tenant/service_test.go` near the existing `CreateTeam` tests:

```go
func TestCreateTeamAcceptsMetadataDisplay(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
	if err != nil {
		t.Fatal(err)
	}
	tenantID := uuid.New()
	actorID := uuid.New()
	ownerID := uuid.New()

	_, err = svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:         tenantID,
		ActorUserID:      actorID,
		Slug:             "security",
		Name:             "安全团队",
		HumanOwnerUserID: &ownerID,
		Metadata: map[string]any{
			"display": map[string]any{
				"icon_key":   "security",
				"color_tone": "teal",
			},
		},
	})
	if err != nil {
		t.Fatalf("expected metadata display to pass validation, got %v", err)
	}
	display := repo.createdTeamWithMembers.Metadata["display"].(map[string]any)
	if display["icon_key"] != "security" || display["color_tone"] != "teal" {
		t.Fatalf("expected metadata display to be preserved, got %#v", display)
	}
}

func TestCreateTeamRejectsInvalidMetadataDisplay(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
	if err != nil {
		t.Fatal(err)
	}
	tenantID := uuid.New()
	actorID := uuid.New()
	ownerID := uuid.New()
	longValue := strings.Repeat("x", 41)

	cases := []struct {
		name     string
		metadata map[string]any
	}{
		{name: "display is not object", metadata: map[string]any{"display": "security"}},
		{name: "icon key is not string", metadata: map[string]any{"display": map[string]any{"icon_key": 123}}},
		{name: "color tone is not string", metadata: map[string]any{"display": map[string]any{"color_tone": 123}}},
		{name: "icon key too long", metadata: map[string]any{"display": map[string]any{"icon_key": longValue}}},
		{name: "color tone too long", metadata: map[string]any{"display": map[string]any{"color_tone": longValue}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateTeam(context.Background(), CreateTeamRequest{
				TenantID:         tenantID,
				ActorUserID:      actorID,
				Slug:             "security",
				Name:             "安全团队",
				HumanOwnerUserID: &ownerID,
				Metadata:         tc.metadata,
			})
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("expected invalid input, got %v", err)
			}
		})
	}
}
```

- [ ] **Step 2: Run service tests and confirm red**

Run:

```bash
go test ./apps/control-plane/internal/tenant -run 'TestCreateTeam(AcceptsMetadataDisplay|RejectsInvalidMetadataDisplay)' -count=1
```

Expected: `TestCreateTeamRejectsInvalidMetadataDisplay` fails because metadata display validation is not implemented.

- [ ] **Step 3: Implement metadata validation**

In `apps/control-plane/internal/tenant/service.go`, call `normalizeTeamMetadata` before repository writes:

```go
metadata, err := normalizeTeamMetadata(req.Metadata)
if err != nil {
	return nil, err
}
```

Use `Metadata: metadata` in `CreateTeamWithInitialMembersParams`. Add these helpers near `cloneMap`:

```go
func normalizeTeamMetadata(metadata map[string]any) (map[string]any, error) {
	cloned := cloneMap(metadata)
	displayValue, ok := cloned["display"]
	if !ok || displayValue == nil {
		return cloned, nil
	}
	display, ok := displayValue.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: metadata.display must be object", ErrInvalidInput)
	}
	for _, key := range []string{"icon_key", "color_tone"} {
		value, ok := display[key]
		if !ok || value == nil {
			continue
		}
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("%w: metadata.display.%s must be string", ErrInvalidInput, key)
		}
		if len(strings.TrimSpace(text)) > 40 {
			return nil, fmt.Errorf("%w: metadata.display.%s is too long", ErrInvalidInput, key)
		}
		display[key] = strings.TrimSpace(text)
	}
	return cloned, nil
}
```

Also use `normalizeTeamMetadata(req.Metadata)` in `UpdateTeam` when metadata is supplied.

- [ ] **Step 4: Add tenant avatar domain types**

In `apps/control-plane/internal/tenant/types.go`, add:

```go
type UserAvatarConfig struct {
	Provider string         `json:"provider"`
	Style    string         `json:"style"`
	Seed     string         `json:"seed"`
	Options  map[string]any `json:"options,omitempty"`
}
```

Extend the existing structs:

```go
type TeamHumanOwner struct {
	UserID      uuid.UUID
	Username    string
	DisplayName string
	Email       string
	Status      string
	Avatar      *UserAvatarConfig
}

type TeamMember struct {
	MembershipID     uuid.UUID
	TenantID         uuid.UUID
	TeamID           uuid.UUID
	UserID           uuid.UUID
	Username         string
	DisplayName      string
	Email            string
	AccountStatus    string
	Avatar           *UserAvatarConfig
	Role             TeamMemberRole
	MembershipStatus string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
```

- [ ] **Step 5: Extend SQL projections**

In `apps/control-plane/internal/storage/queries/tenant_team_config.sql`, add owner avatar fields to both `ListTenantTeamSummaries` and `GetTenantTeamSummary` select lists:

```sql
  owner.avatar_provider AS owner_avatar_provider,
  owner.avatar_style AS owner_avatar_style,
  owner.avatar_seed AS owner_avatar_seed,
  owner.avatar_options AS owner_avatar_options,
```

In `ListTeamMembers` and `GetTeamMember`, add user avatar fields:

```sql
  au.avatar_provider,
  au.avatar_style,
  au.avatar_seed,
  au.avatar_options,
```

Keep the joins against `auth_users` unchanged except for selecting these columns.

- [ ] **Step 6: Regenerate sqlc**

Run:

```bash
make -C apps/control-plane generate-sqlc
```

Expected: sqlc completes and generated row structs include owner/member avatar fields.

- [ ] **Step 7: Map avatar rows in repository**

In `apps/control-plane/internal/tenant/pg_repository.go`, add helper functions:

```go
func avatarFromFields(provider, style, seed pgtype.Text, options []byte) *UserAvatarConfig {
	if !provider.Valid || !style.Valid || !seed.Valid {
		return nil
	}
	avatar := &UserAvatarConfig{
		Provider: provider.String,
		Style:    style.String,
		Seed:     seed.String,
	}
	if len(options) > 0 {
		var parsed map[string]any
		if err := json.Unmarshal(options, &parsed); err == nil && parsed != nil {
			avatar.Options = parsed
		}
	}
	return avatar
}

func cloneUserAvatarConfig(avatar *UserAvatarConfig) *UserAvatarConfig {
	if avatar == nil {
		return nil
	}
	return &UserAvatarConfig{
		Provider: avatar.Provider,
		Style:    avatar.Style,
		Seed:     avatar.Seed,
		Options:  cloneMap(avatar.Options),
	}
}
```

Use this helper when building `TeamHumanOwner` and `TeamMember` records. The exact assignments should follow generated row field names after sqlc generation:

```go
Avatar: avatarFromFields(row.OwnerAvatarProvider, row.OwnerAvatarStyle, row.OwnerAvatarSeed, row.OwnerAvatarOptions),
```

```go
Avatar: avatarFromFields(row.AvatarProvider, row.AvatarStyle, row.AvatarSeed, row.AvatarOptions),
```

- [ ] **Step 8: Clone avatar in service**

In `apps/control-plane/internal/tenant/service.go`, update clone helpers:

```go
func cloneTeamHumanOwner(owner *TeamHumanOwner) *TeamHumanOwner {
	if owner == nil {
		return nil
	}
	return &TeamHumanOwner{
		UserID:      owner.UserID,
		Username:    owner.Username,
		DisplayName: owner.DisplayName,
		Email:       owner.Email,
		Status:      owner.Status,
		Avatar:      cloneUserAvatarConfig(owner.Avatar),
	}
}
```

In `teamMemberFromRecord`, assign:

```go
Avatar: cloneUserAvatarConfig(record.Avatar),
```

- [ ] **Step 9: Expose avatar in handler responses**

In `apps/control-plane/internal/tenant/handler.go`, add response structs:

```go
type userAvatarResponse struct {
	Provider string         `json:"provider"`
	Style    string         `json:"style"`
	Seed     string         `json:"seed"`
	Options  map[string]any `json:"options,omitempty"`
}
```

Add `Avatar *userAvatarResponse `json:"avatar,omitempty"` to `teamHumanOwnerResponse` and `teamMemberResponse`. Add mapper:

```go
func userAvatarResponseFromDomain(avatar *UserAvatarConfig) *userAvatarResponse {
	if avatar == nil {
		return nil
	}
	return &userAvatarResponse{
		Provider: avatar.Provider,
		Style:    avatar.Style,
		Seed:     avatar.Seed,
		Options:  cloneMap(avatar.Options),
	}
}
```

Call it in `teamHumanOwnerResponseFromDomain` and `teamMemberResponseFromDomain`.

- [ ] **Step 10: Update OpenAPI schemas**

In `contracts/control-plane/openapi.yaml`, add a local team avatar schema:

```yaml
    TeamUserAvatar:
      type: object
      required: [provider, style, seed]
      properties:
        provider:
          type: string
          enum: [dicebear]
        style:
          type: string
          enum: [adventurer]
        seed:
          type: string
        options:
          type: object
          additionalProperties: true
```

Add to `TeamHumanOwner.properties` and `TeamMember.properties`:

```yaml
        avatar:
          $ref: "#/components/schemas/TeamUserAvatar"
```

- [ ] **Step 11: Add route tests for avatar JSON**

In `apps/control-plane/internal/api/team_routes_test.go`, update fake `TeamHumanOwner` and `TeamMember` returned by `routeTeamService` to include:

```go
Avatar: &tenant.UserAvatarConfig{
	Provider: "dicebear",
	Style:    "adventurer",
	Seed:     "user:owner",
	Options:  map[string]any{"backgroundColor": []any{"e6fbf5"}},
},
```

Assert the JSON body contains:

```go
if body.Team.HumanOwner.Avatar.Seed != "user:owner" {
	t.Fatalf("expected owner avatar seed, got %#v", body.Team.HumanOwner.Avatar)
}
```

For `ListTeamMembers`, assert:

```go
if members[0].Avatar.Seed != "user:member" {
	t.Fatalf("expected member avatar seed, got %#v", members[0].Avatar)
}
```

- [ ] **Step 12: Run backend verification**

Run:

```bash
go test ./apps/control-plane/internal/tenant ./apps/control-plane/internal/api -count=1
pnpm verify:contracts
git diff --check -- contracts/control-plane/openapi.yaml apps/control-plane/internal/storage/queries/tenant_team_config.sql apps/control-plane/internal/tenant
```

Expected: all commands pass.

- [ ] **Step 13: Commit backend identity slice**

Run:

```bash
git add contracts/control-plane/openapi.yaml apps/control-plane/internal/storage/queries/tenant_team_config.sql apps/control-plane/internal/storage/queries apps/control-plane/internal/tenant apps/control-plane/internal/api/team_routes_test.go
git diff --cached --name-only
git commit -m "feat(control-plane): expose team user avatars"
```

Expected staged files are limited to backend contract/query/tenant/API route test files.

---

## Task 2: Shared Frontend Identity, Icon, Role And User Search Components

**Files:**

- Create: `apps/web/src/components/superteam/user-identity.tsx`
- Create: `apps/web/src/components/superteam/team-icon-tile.tsx`
- Create: `apps/web/src/components/superteam/team-role.tsx`
- Create: `apps/web/src/components/superteam/user-search-select.tsx`
- Modify: `apps/web/src/components/superteam/index.ts`
- Modify: `apps/web/src/features/users/components/user-avatar.tsx`
- Test: `apps/web/src/components/superteam/user-identity.test.tsx`
- Test: `apps/web/src/components/superteam/team-icon-tile.test.tsx`
- Test: `apps/web/src/components/superteam/team-role.test.tsx`
- Test: `apps/web/src/components/superteam/user-search-select.test.tsx`

- [ ] **Step 1: Add failing UserIdentity tests**

Create `apps/web/src/components/superteam/user-identity.test.tsx`:

```tsx
import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import { UserIdentity, buildUserAvatarDataUri, getUserIdentityLabel } from "./user-identity";

describe("UserIdentity", () => {
  it("renders display name, email and avatar image", async () => {
    const screen = render(
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
});
```

- [ ] **Step 2: Add failing icon and role tests**

Create `apps/web/src/components/superteam/team-icon-tile.test.tsx`:

```tsx
import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import { TeamIconTile, getTeamDisplayConfig } from "./team-icon-tile";

describe("TeamIconTile", () => {
  it("resolves known ops display metadata", async () => {
    expect(getTeamDisplayConfig({ display: { icon_key: "ops", color_tone: "cyan" } })).toMatchObject({
      iconKey: "ops",
      tone: "cyan",
    });
    const screen = render(<TeamIconTile metadata={{ display: { icon_key: "ops", color_tone: "cyan" } }} />);
    await expect.element(screen.getByLabelText("运维团队图标")).toBeInTheDocument();
  });

  it("falls back to neutral team icon", () => {
    expect(getTeamDisplayConfig({ display: { icon_key: "unknown", color_tone: "unknown" } })).toMatchObject({
      iconKey: "default",
      tone: "neutral",
    });
  });
});
```

Create `apps/web/src/components/superteam/team-role.test.tsx`:

```tsx
import { describe, expect, it } from "vitest";
import { directTeamRoles, privilegedTeamRoles, teamRoleLabel } from "./team-role";

describe("team roles", () => {
  it("separates direct and privileged roles", () => {
    expect(directTeamRoles.map((role) => role.value)).toEqual(["member", "viewer"]);
    expect(privilegedTeamRoles.map((role) => role.value)).toEqual(["owner", "admin", "approver"]);
  });

  it("returns Chinese labels", () => {
    expect(teamRoleLabel("owner")).toBe("负责人");
    expect(teamRoleLabel("viewer")).toBe("只读观察者");
  });
});
```

- [ ] **Step 3: Add failing UserSearchSelect test**

Create `apps/web/src/components/superteam/user-search-select.test.tsx`:

```tsx
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { UserSearchSelect } from "./user-search-select";

function renderWithQuery(ui: React.ReactNode) {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      {ui}
    </QueryClientProvider>,
  );
}

describe("UserSearchSelect", () => {
  it("searches active users and returns the selected user", async () => {
    const onSelect = vi.fn();
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(String(input));
      expect(url.searchParams.get("status")).toBe("active");
      return new Response(
        JSON.stringify({
          items: [
            {
              avatar: { provider: "dicebear", seed: "user:sun", style: "adventurer" },
              id: "user-sun",
              status: "active",
              username: "sun",
            },
          ],
        }),
        { headers: { "content-type": "application/json" }, status: 200 },
      );
    });

    const screen = renderWithQuery(
      <UserSearchSelect apiBaseUrl="http://control-plane.local" fetcher={fetcher} onSelect={onSelect} />,
    );

    await userEvent.type(screen.getByPlaceholder("搜索用户"), "sun");
    await userEvent.click(screen.getByRole("button", { name: /sun/ }));
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: "user-sun", username: "sun" }));
  });
});
```

- [ ] **Step 4: Run component tests and confirm red**

Run:

```bash
pnpm --filter @superteam/web test -- src/components/superteam/user-identity.test.tsx src/components/superteam/team-icon-tile.test.tsx src/components/superteam/team-role.test.tsx src/components/superteam/user-search-select.test.tsx
```

Expected: tests fail because the new components do not exist.

- [ ] **Step 5: Implement UserIdentity**

Create `apps/web/src/components/superteam/user-identity.tsx` with:

```tsx
import { useMemo } from "react";
import { createAvatar } from "@dicebear/core";
import * as adventurer from "@dicebear/adventurer";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { cn } from "@/lib/utils";

export type UserAvatarDescriptor = {
  options?: Record<string, unknown>;
  provider: "dicebear";
  seed: string;
  style: "adventurer";
};

export type UserIdentityData = {
  avatar?: UserAvatarDescriptor | null;
  display_name?: string | null;
  email?: string | null;
  id: string;
  status?: string;
  username?: string | null;
};

export function getUserIdentityLabel(user: UserIdentityData) {
  const primary = user.display_name?.trim() || user.username?.trim() || user.email?.trim() || user.id;
  const secondary = user.email?.trim() || (primary !== user.username ? user.username?.trim() : "") || user.id;
  const initials = primary.trim().slice(0, 1).toUpperCase() || "?";
  return { initials, primary, secondary };
}

export function buildUserAvatarDataUri(avatar: UserAvatarDescriptor | null | undefined, username: string) {
  if (!avatar || avatar.provider !== "dicebear" || avatar.style !== "adventurer") {
    return "";
  }
  return createAvatar(adventurer, {
    backgroundColor: ["eef8f4", "e6fbf5", "dbeafe"],
    radius: 50,
    seed: avatar.seed || `user:${username}`,
    size: 96,
    ...(avatar.options ?? {}),
  }).toDataUri();
}

export function UserIdentityAvatar({
  className,
  user,
}: {
  className?: string;
  user: UserIdentityData;
}) {
  const label = getUserIdentityLabel(user);
  const src = useMemo(() => buildUserAvatarDataUri(user.avatar, user.username ?? label.primary), [label.primary, user]);

  return (
    <Avatar className={cn("size-9 border border-border bg-background", className)}>
      <AvatarImage alt={`${label.primary} 的头像`} src={src} />
      <AvatarFallback className="text-xs font-medium">{label.initials}</AvatarFallback>
    </Avatar>
  );
}

export function UserIdentity({
  className,
  showSecondary = true,
  size = "md",
  user,
}: {
  className?: string;
  showSecondary?: boolean;
  size?: "sm" | "md";
  user: UserIdentityData;
}) {
  const label = getUserIdentityLabel(user);
  return (
    <div className={cn("flex min-w-0 items-center gap-2", className)}>
      <UserIdentityAvatar className={size === "sm" ? "size-7" : "size-9"} user={user} />
      <div className="min-w-0">
        <div className={cn("truncate font-medium", size === "sm" ? "text-sm" : "text-sm")}>{label.primary}</div>
        {showSecondary ? <div className="truncate text-xs text-muted-foreground">{label.secondary}</div> : null}
      </div>
    </div>
  );
}
```

- [ ] **Step 6: Implement TeamIconTile**

Create `apps/web/src/components/superteam/team-icon-tile.tsx` with icon mapping for `ops/dev/qa/security/default`, using lucide icons `ServerCog`, `Code2`, `FlaskConical`, `Shield`, `UsersRound`. Export `getTeamDisplayConfig`. Use `SemanticIconTile` if its props support custom icon content; otherwise use the same `size-9 rounded-md border` visual pattern with semantic classes. The supported labels must be:

```ts
const iconLabels = {
  default: "默认团队图标",
  dev: "研发团队图标",
  ops: "运维团队图标",
  qa: "测试团队图标",
  security: "安全团队图标",
} as const;
```

The supported tones must be:

```ts
const toneClasses = {
  blue: "border-blue-200 bg-blue-50 text-blue-600",
  cyan: "border-cyan-200 bg-cyan-50 text-cyan-700",
  neutral: "border-slate-200 bg-slate-50 text-slate-600",
  teal: "border-teal-200 bg-teal-50 text-teal-700",
  violet: "border-violet-200 bg-violet-50 text-violet-600",
} as const;
```

- [ ] **Step 7: Implement team role utilities**

Create `apps/web/src/components/superteam/team-role.tsx` with:

```tsx
import { Badge } from "@/components/ui/badge";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import type { TeamMemberRole } from "@/lib/api/teams";

export const teamRoleLabels: Record<TeamMemberRole, string> = {
  admin: "管理员",
  approver: "审批人",
  member: "普通成员",
  owner: "负责人",
  viewer: "只读观察者",
};

export const directTeamRoles = [
  { label: teamRoleLabels.member, value: "member" },
  { label: teamRoleLabels.viewer, value: "viewer" },
] as const;

export const privilegedTeamRoles = [
  { label: teamRoleLabels.owner, value: "owner" },
  { label: teamRoleLabels.admin, value: "admin" },
  { label: teamRoleLabels.approver, value: "approver" },
] as const;

export function teamRoleLabel(role: TeamMemberRole) {
  return teamRoleLabels[role];
}

export function TeamRoleBadge({ role }: { role: TeamMemberRole }) {
  return <Badge variant={role === "owner" ? "default" : "secondary"}>{teamRoleLabel(role)}</Badge>;
}

export function TeamRoleSelect<T extends TeamMemberRole>({
  disabled,
  mode,
  onChange,
  value,
}: {
  disabled?: boolean;
  mode: "direct" | "privileged";
  onChange: (role: T) => void;
  value: T;
}) {
  const roles = mode === "direct" ? directTeamRoles : privilegedTeamRoles;
  return (
    <Select disabled={disabled} onValueChange={(next) => onChange(next as T)} value={value}>
      <SelectTrigger aria-label={mode === "direct" ? "直接生效角色" : "高权限角色"}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {roles.map((role) => (
          <SelectItem key={role.value} value={role.value}>
            {role.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
```

- [ ] **Step 8: Implement UserSearchSelect**

Create `apps/web/src/components/superteam/user-search-select.tsx`. Use `useQuery`, `Input`, `Button`, `UserIdentity` and `listUsers`. It must call:

```ts
listUsers({
  baseUrl: apiBaseUrl,
  fetcher,
  limit: 20,
  offset: 0,
  q,
  status: "active",
})
```

It must hide users whose IDs are in `excludedUserIds`, render loading text `加载用户中`, error text `用户加载失败`, empty text `暂无匹配用户`, and call `onSelect(user)` with the full `UserSummary`.

- [ ] **Step 9: Reuse shared avatar in user management**

Replace `apps/web/src/features/users/components/user-avatar.tsx` implementation with a wrapper:

```tsx
import type { UserAvatar as UserAvatarConfig } from "@/lib/api";
import { UserIdentityAvatar, buildUserAvatarDataUri } from "@/components/superteam/user-identity";

type UserAvatarProps = {
  avatar: UserAvatarConfig;
  username: string;
};

export function UserAvatar({ avatar, username }: UserAvatarProps) {
  return (
    <UserIdentityAvatar
      className="size-10"
      user={{ avatar, id: username, status: "active", username }}
    />
  );
}

export { buildUserAvatarDataUri };
```

- [ ] **Step 10: Export shared components**

Append to `apps/web/src/components/superteam/index.ts`:

```ts
export * from "./team-icon-tile";
export * from "./team-role";
export * from "./user-identity";
export * from "./user-search-select";
```

- [ ] **Step 11: Run component verification**

Run:

```bash
pnpm --filter @superteam/web test -- src/components/superteam/user-identity.test.tsx src/components/superteam/team-icon-tile.test.tsx src/components/superteam/team-role.test.tsx src/components/superteam/user-search-select.test.tsx src/features/users/index.test.tsx
pnpm --filter @superteam/web typecheck
```

Expected: all commands pass.

- [ ] **Step 12: Commit shared component slice**

Run:

```bash
git add apps/web/src/components/superteam apps/web/src/features/users/components/user-avatar.tsx
git diff --cached --name-only
git commit -m "feat(web): add team identity display components"
```

Expected staged files are limited to shared components, their tests, exports, and the user avatar wrapper.

---

## Task 3: Team List Display, Weak Pagination And Row Navigation Menu

**Files:**

- Modify: `apps/web/src/lib/api/teams.ts`
- Test: `apps/web/src/lib/api/teams.test.ts`
- Modify: `apps/web/src/features/teams/index.tsx`
- Modify: `apps/web/src/features/teams/components/team-list-table.tsx`
- Test: `apps/web/src/features/teams/index.test.tsx`

- [ ] **Step 1: Add failing API client pagination test**

In `apps/web/src/lib/api/teams.test.ts`, add:

```ts
it("lists team summaries with weak pagination filters", async () => {
  const fetcher = vi.fn(
    async () =>
      new Response(JSON.stringify([]), {
        headers: { "content-type": "application/json" },
        status: 200,
      }),
  );

  await listTeamSummaries(
    { baseUrl: "http://control-plane.local", fetcher },
    { limit: 20, offset: 40, q: "ops", status: "active" },
  );

  expect(fetcher).toHaveBeenCalledWith(
    "http://control-plane.local/api/v1/teams?status=active&q=ops&limit=20&offset=40",
    {
      credentials: "include",
      headers: { accept: "application/json" },
      method: "GET",
    },
  );
});
```

- [ ] **Step 2: Add failing page interaction tests**

In `apps/web/src/features/teams/index.test.tsx`, extend the teams list case to assert:

```ts
await expect.element(screen.getByText("第 1 页")).toBeInTheDocument();
await expect.element(screen.getByRole("button", { name: "上一页" })).toBeDisabled();
await expect.element(screen.getByRole("button", { name: "下一页" })).toBeEnabled();
await userEvent.click(screen.getByRole("button", { name: "下一页" }));
expect(fetchCalls(fetcher).some(([input]) => String(input).includes("limit=20&offset=20"))).toBe(true);
await userEvent.click(screen.getByRole("button", { name: "团队行操作" }));
await expect.element(screen.getByRole("menuitem", { name: "查看详情" })).toBeInTheDocument();
```

Adjust the fake list endpoint to return exactly 20 items for the first page and fewer than 20 items for the second page.

- [ ] **Step 3: Run focused web tests and confirm red**

Run:

```bash
pnpm --filter @superteam/web test -- src/lib/api/teams.test.ts src/features/teams/index.test.tsx
```

Expected: tests fail because `limit/offset` and pagination footer are missing.

- [ ] **Step 4: Add frontend API types and query params**

Update `apps/web/src/lib/api/teams.ts`:

```ts
export type TeamUserAvatar = {
  options?: Record<string, unknown>;
  provider: "dicebear";
  seed: string;
  style: "adventurer";
};

export type TeamHumanOwner = {
  avatar?: TeamUserAvatar;
  user_id: string;
  username: string;
  display_name?: string;
  email?: string;
  status: string;
};
```

Update `TeamMember` with `avatar?: TeamUserAvatar;`. Extend filters:

```ts
export type ListTeamSummariesFilters = {
  governance_status?: GovernanceSummaryStatus;
  limit?: number;
  offset?: number;
  q?: string;
  status?: TeamStatus;
};
```

In `teamListPath`, append:

```ts
if (filters.limit !== undefined) {
  params.set("limit", String(filters.limit));
}
if (filters.offset !== undefined) {
  params.set("offset", String(filters.offset));
}
```

- [ ] **Step 5: Add pagination state in TeamsView**

In `apps/web/src/features/teams/index.tsx`, add:

```tsx
const [pageIndex, setPageIndex] = useState(0);
const [pageSize, setPageSize] = useState(20);
```

Query with:

```tsx
limit: pageSize,
offset: pageIndex * pageSize,
```

Pass `pageIndex`, `pageSize`, `canGoNext={teams.data?.length === pageSize}`, `onPageChange={setPageIndex}`, and `onPageSizeChange` to `TeamListTable`. In toolbar `onChange`, wrap:

```tsx
onChange={(nextFilters) => {
  setFilters(nextFilters);
  setPageIndex(0);
}}
```

- [ ] **Step 6: Render list icons, identity and menu**

In `apps/web/src/features/teams/components/team-list-table.tsx`, import:

```tsx
import { MoreHorizontal } from "lucide-react";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";
import { TeamIconTile, UserIdentity } from "@/components/superteam";
```

Render team cell with `TeamIconTile` plus name/slug/status. Render owner with:

```tsx
{team.human_owner ? (
  <UserIdentity
    showSecondary
    size="sm"
    user={{
      avatar: team.human_owner.avatar,
      display_name: team.human_owner.display_name,
      email: team.human_owner.email,
      id: team.human_owner.user_id,
      status: team.human_owner.status,
      username: team.human_owner.username,
    }}
  />
) : (
  <span className="text-sm text-muted-foreground">{team.human_owner_user_id ?? "未设置"}</span>
)}
```

Append a final actions column with:

```tsx
<DropdownMenu>
  <DropdownMenuTrigger asChild>
    <Button aria-label="团队行操作" size="icon" type="button" variant="ghost">
      <MoreHorizontal className="size-4" />
    </Button>
  </DropdownMenuTrigger>
  <DropdownMenuContent align="end">
    <DropdownMenuItem asChild>
      <a href={`/teams/${team.id}`}>查看详情</a>
    </DropdownMenuItem>
  </DropdownMenuContent>
</DropdownMenu>
```

- [ ] **Step 7: Render weak pagination footer**

Add footer below the table:

```tsx
<div className="flex items-center justify-between border-t px-3 py-3 text-sm text-muted-foreground">
  <span>第 {pageIndex + 1} 页</span>
  <div className="flex items-center gap-2">
    <Button disabled={pageIndex === 0 || isLoading} onClick={() => onPageChange(pageIndex - 1)} size="sm" type="button" variant="outline">
      上一页
    </Button>
    <select
      className="h-9 rounded-md border bg-background px-2 text-sm"
      onChange={(event) => onPageSizeChange(Number(event.target.value))}
      value={pageSize}
    >
      {[10, 20, 50].map((size) => (
        <option key={size} value={size}>
          {size} 条/页
        </option>
      ))}
    </select>
    <Button disabled={!canGoNext || isLoading} onClick={() => onPageChange(pageIndex + 1)} size="sm" type="button" variant="outline">
      下一页
    </Button>
  </div>
</div>
```

- [ ] **Step 8: Run list verification**

Run:

```bash
pnpm --filter @superteam/web test -- src/lib/api/teams.test.ts src/features/teams/index.test.tsx
pnpm --filter @superteam/web typecheck
```

Expected: all commands pass.

- [ ] **Step 9: Commit team list slice**

Run:

```bash
git add apps/web/src/lib/api/teams.ts apps/web/src/lib/api/teams.test.ts apps/web/src/features/teams/index.tsx apps/web/src/features/teams/components/team-list-table.tsx apps/web/src/features/teams/index.test.tsx
git diff --cached --name-only
git commit -m "feat(web): improve team list management table"
```

Expected staged files are limited to list/API/test files.

---

## Task 4: Target Create Team Drawer Flow

**Files:**

- Modify: `apps/web/src/features/teams/components/create-team-drawer.tsx`
- Modify: `apps/web/src/features/teams/components/create-team-basic-step.tsx`
- Modify: `apps/web/src/features/teams/components/create-team-members-step.tsx`
- Modify: `apps/web/src/features/teams/index.tsx`
- Test: `apps/web/src/features/teams/index.test.tsx`

- [ ] **Step 1: Add failing create drawer interaction test**

In `apps/web/src/features/teams/index.test.tsx`, add a test named `creates team with display metadata and editable initial members`. The test must:

```ts
await userEvent.click(screen.getByRole("button", { name: "新建团队" }));
await userEvent.type(screen.getByLabelText("团队名称"), "安全团队");
await userEvent.type(screen.getByLabelText("团队标识"), "security");
await userEvent.click(screen.getByRole("button", { name: "选择安全团队图标" }));
await userEvent.click(screen.getByRole("button", { name: /owner/ }));
await userEvent.click(screen.getByRole("button", { name: "下一步" }));
await expect.element(screen.getByText("基础信息")).toBeInTheDocument();
await userEvent.click(screen.getByRole("checkbox", { name: /member/ }));
await userEvent.click(screen.getByRole("checkbox", { name: /viewer/ }));
await userEvent.click(screen.getByRole("button", { name: "移除 viewer" }));
await userEvent.click(screen.getByRole("button", { name: "创建团队" }));
```

Assert the POST body:

```ts
expect(JSON.parse(String(postCall[1]?.body))).toMatchObject({
  human_owner_user_id: "owner-user",
  initial_members: [{ role: "member", user_id: "member-user" }],
  metadata: { display: { color_tone: "teal", icon_key: "security" } },
  name: "安全团队",
  slug: "security",
});
```

- [ ] **Step 2: Run create drawer test and confirm red**

Run:

```bash
pnpm --filter @superteam/web test -- src/features/teams/index.test.tsx
```

Expected: test fails because display metadata, checkbox table and selected member removal are missing.

- [ ] **Step 3: Extend create draft**

In `create-team-drawer.tsx`, update draft:

```ts
export type TeamDisplayDraft = {
  color_tone: "blue" | "cyan" | "neutral" | "teal" | "violet";
  icon_key: "default" | "dev" | "ops" | "qa" | "security";
};

export type CreateTeamDraft = {
  display: TeamDisplayDraft;
  initial_members: InitialTeamMemberInput[];
  memberUsers: Record<string, UserSummary>;
  name: string;
  owner?: UserSummary;
  slug: string;
};
```

Set empty draft:

```ts
const emptyDraft: CreateTeamDraft = {
  display: { color_tone: "neutral", icon_key: "default" },
  initial_members: [],
  memberUsers: {},
  name: "",
  slug: "",
};
```

Add a compact stepper above the content:

```tsx
<div className="flex items-center gap-3 border-b px-6 py-4 text-sm">
  <Step active={step === "basic"} done={step === "members"} label="1 基础信息" />
  <div className="h-px flex-1 bg-border" />
  <Step active={step === "members"} done={false} label="2 初始成员" />
</div>
```

- [ ] **Step 4: Implement basic step display and owner search**

In `create-team-basic-step.tsx`, use `UserSearchSelect` for owner and render five icon buttons:

```tsx
const iconOptions = [
  { color_tone: "cyan", icon_key: "ops", label: "选择运维团队图标" },
  { color_tone: "blue", icon_key: "dev", label: "选择研发团队图标" },
  { color_tone: "violet", icon_key: "qa", label: "选择测试团队图标" },
  { color_tone: "teal", icon_key: "security", label: "选择安全团队图标" },
  { color_tone: "neutral", icon_key: "default", label: "选择默认团队图标" },
] as const;
```

When name or slug changes and current display is default, infer:

```ts
function inferDisplay(value: string) {
  const normalized = value.toLowerCase();
  if (normalized.includes("ops") || normalized.includes("运维")) return { color_tone: "cyan", icon_key: "ops" } as const;
  if (normalized.includes("dev") || normalized.includes("研发")) return { color_tone: "blue", icon_key: "dev" } as const;
  if (normalized.includes("qa") || normalized.includes("测试")) return { color_tone: "violet", icon_key: "qa" } as const;
  if (normalized.includes("security") || normalized.includes("安全")) return { color_tone: "teal", icon_key: "security" } as const;
  return { color_tone: "neutral", icon_key: "default" } as const;
}
```

- [ ] **Step 5: Implement members step candidate table**

In `create-team-members-step.tsx`, replace add buttons with:

```tsx
type MemberRoleFilter = "all" | "member" | "viewer";
const [roleFilter, setRoleFilter] = useState<MemberRoleFilter>("all");
const [query, setQuery] = useState("");
```

Query `listUsers` with `q: query`, `status: "active"`, `limit: 20`, `offset: 0`. Render:

```tsx
<Input aria-label="搜索候选成员" onChange={(event) => setQuery(event.target.value)} value={query} />
<select
  aria-label="角色筛选"
  className="h-9 rounded-md border bg-background px-3 text-sm shadow-xs"
  onChange={(event) => setRoleFilter(event.target.value as MemberRoleFilter)}
  value={roleFilter}
>
  <option value="all">全部角色</option>
  <option value="member">普通成员</option>
  <option value="viewer">只读观察者</option>
</select>
```

For each candidate row, checkbox toggles selected membership. Store full user in `memberUsers[user.id]`. The owner row is disabled.

- [ ] **Step 6: Implement selected members table**

Render selected users with `UserIdentity`, `TeamRoleSelect(mode="direct")` and remove button:

```tsx
<Button
  aria-label={`移除 ${user.username}`}
  onClick={() => removeMember(member.user_id)}
  size="icon"
  type="button"
  variant="ghost"
>
  <Trash2 className="size-4" />
</Button>
```

Changing role must update the matching `initial_members` entry without changing the user object.

- [ ] **Step 7: Submit metadata display**

In `TeamsView` create mutation body, add:

```ts
metadata: {
  display: {
    color_tone: draft.display.color_tone,
    icon_key: draft.display.icon_key,
  },
},
```

- [ ] **Step 8: Run create drawer verification**

Run:

```bash
pnpm --filter @superteam/web test -- src/features/teams/index.test.tsx
pnpm --filter @superteam/web typecheck
```

Expected: all commands pass.

- [ ] **Step 9: Commit create drawer slice**

Run:

```bash
git add apps/web/src/features/teams/components/create-team-drawer.tsx apps/web/src/features/teams/components/create-team-basic-step.tsx apps/web/src/features/teams/components/create-team-members-step.tsx apps/web/src/features/teams/index.tsx apps/web/src/features/teams/index.test.tsx
git diff --cached --name-only
git commit -m "feat(web): upgrade create team drawer"
```

Expected staged files are limited to create-drawer files and tests.

---

## Task 5: Team Detail Members User Search Flow

**Files:**

- Modify: `apps/web/src/features/teams/components/team-members-tab.tsx`
- Test: `apps/web/src/features/teams/index.test.tsx`

- [ ] **Step 1: Add failing member detail tests**

In `apps/web/src/features/teams/index.test.tsx`, add assertions in the team detail test:

```ts
await userEvent.type(screen.getByPlaceholder("搜索用户"), "member");
await userEvent.click(screen.getByRole("button", { name: /member/ }));
await userEvent.click(screen.getByRole("button", { name: "添加成员" }));
expect(JSON.parse(String(addMemberCall[1]?.body))).toMatchObject({
  role: "member",
  user_id: "member-user",
});

await userEvent.type(screen.getAllByPlaceholder("搜索用户")[1], "viewer");
await userEvent.click(screen.getByRole("button", { name: /viewer/ }));
await userEvent.type(screen.getByLabelText("申请原因"), "需要维护团队治理");
await userEvent.click(screen.getByRole("button", { name: "提交申请" }));
expect(JSON.parse(String(requestCall[1]?.body))).toMatchObject({
  reason: "需要维护团队治理",
  requested_role: "admin",
  target_user_id: "viewer-user",
});
```

Make the fake `/api/auth/users` endpoint return active users with avatar descriptor.

- [ ] **Step 2: Run detail members test and confirm red**

Run:

```bash
pnpm --filter @superteam/web test -- src/features/teams/index.test.tsx
```

Expected: test fails because raw UUID inputs are still used.

- [ ] **Step 3: Replace raw UUID direct add panel**

In `team-members-tab.tsx`, remove the `Input` for `team-member-user-id`. Add:

```tsx
const [selectedUser, setSelectedUser] = useState<UserSummary | undefined>();
```

Render:

```tsx
<UserSearchSelect
  apiBaseUrl={apiBaseUrl}
  excludedUserIds={existingUserIds}
  fetcher={fetcher}
  onSelect={setSelectedUser}
  value={selectedUser}
/>
```

Submit:

```tsx
if (selectedUser) {
  onSubmit({ role, user_id: selectedUser.id });
}
```

Pass `apiBaseUrl`, `fetcher`, and `existingUserIds={roster.map((member) => member.user_id)}` from `TeamMembersTab`.

- [ ] **Step 4: Replace raw UUID privileged request panel**

In `PrivilegedRequestPanel`, replace `targetUserId` string with `selectedUser?: UserSummary`. Submit:

```tsx
if (selectedUser) {
  onSubmit({
    reason: reason.trim(),
    requested_role: requestedRole,
    target_user_id: selectedUser.id,
  });
}
```

Keep reason text area and `TeamRoleSelect(mode="privileged")`.

- [ ] **Step 5: Render roster identities with avatar**

In `MemberRow`, replace manual display name block with:

```tsx
<UserIdentity
  user={{
    avatar: member.avatar,
    display_name: member.display_name,
    email: member.email,
    id: member.user_id,
    status: member.account_status,
    username: member.username,
  }}
/>
```

Replace inline role badge with `TeamRoleBadge`.

- [ ] **Step 6: Run detail members verification**

Run:

```bash
pnpm --filter @superteam/web test -- src/features/teams/index.test.tsx
pnpm --filter @superteam/web typecheck
```

Expected: all commands pass.

- [ ] **Step 7: Commit detail members slice**

Run:

```bash
git add apps/web/src/features/teams/components/team-members-tab.tsx apps/web/src/features/teams/index.test.tsx
git diff --cached --name-only
git commit -m "feat(web): use user search in team members"
```

Expected staged files are limited to team members UI and tests.

---

## Task 6: Final Regression, Visual Review And Changelog

**Files:**

- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add changelog entry**

Get local time:

```bash
TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'
```

Add this entry under `## [Unreleased]` -> `### Added` in `CHANGELOG.md`, using the command output time:

```md
- YYYY-MM-DD HH:mm：团队管理体验补齐团队图标、用户头像身份展示、弱分页、目标化两步创建抽屉和详情成员页用户搜索，继续沿用浅色液态玻璃企业控制台设计风格。
```

- [ ] **Step 2: Run backend regression**

Run:

```bash
go test ./apps/control-plane/internal/tenant ./apps/control-plane/internal/api -count=1
pnpm verify:contracts
```

Expected: all commands pass.

- [ ] **Step 3: Run frontend regression**

Run:

```bash
pnpm --filter @superteam/web test
pnpm --filter @superteam/web typecheck
pnpm --filter @superteam/web build
```

Expected: all commands pass.

- [ ] **Step 4: Run full diff hygiene**

Run:

```bash
git diff --check
git status --short
```

Expected: `git diff --check` has no output. `git status --short` shows only files intentionally changed by this plan plus pre-existing unrelated files that were not staged.

- [ ] **Step 5: Start local web for visual review**

Run:

```bash
pnpm --filter @superteam/web dev -- --host 127.0.0.1 --port 3000
```

Expected: Vite prints a local URL at `http://127.0.0.1:3000/`.

- [ ] **Step 6: Browser visual review against DESIGN.md**

Open `http://127.0.0.1:3000/teams` in the in-app Browser. Verify desktop `1440x960` and mobile `390x844`:

- 团队列表首屏有标题、主操作、筛选工具条和核心表格。
- 左侧导航、顶部栏和主内容继续是浅色液态玻璃控制台风格。
- 团队图标是小面积语义色，不出现大面积单色页面。
- 表格行高稳定，头像、图标、菜单不造成布局跳动。
- 创建抽屉 footer 固定可见，长候选成员列表只滚动内容区。
- 创建抽屉没有嵌套卡片堆叠，确认卡与列表边框层级清晰。
- 文本不重叠，不压在复杂渐变或透明背景上。

If the page requires login, use the existing local dev account from project setup. If the backend is not running, use the existing Web test fixtures and component tests for behavior, then record that live visual review was blocked by backend availability.

- [ ] **Step 7: Stop dev server**

Stop the Vite process with `Ctrl-C`. Confirm the terminal returns to prompt.

- [ ] **Step 8: Commit final docs and verification marker**

Run:

```bash
git add CHANGELOG.md
git diff --cached --name-only
git commit -m "docs: record team management experience update"
```

Expected staged file is only `CHANGELOG.md`.

---

## Coverage Review

- 用户头像：Task 1 exposes owner/member avatar from backend team APIs; Task 2 renders DiceBear avatar descriptor and fallback initials.
- 团队图标：Task 1 validates `metadata.display`; Task 2 renders icon/tone; Task 4 writes display metadata on create.
- 创建团队抽屉：Task 4 implements stepper, basic confirmation, user search, role filter, checkbox table, selected table, role changes and remove action.
- 团队列表：Task 3 implements icon, owner identity, weak pagination and row menu with only 查看详情.
- 团队详情成员页：Task 5 replaces raw UUID inputs with active user search.
- 角色组件：Task 2 creates labels, badge and constrained select for direct and privileged modes.
- 设计风格：Task 6 checks the implementation against `DESIGN.md` rules for light liquid-glass enterprise console.
- 非目标保护：No task adds avatar upload, object storage, exact total pagination, custom role registry or team lifecycle actions in the list row menu.

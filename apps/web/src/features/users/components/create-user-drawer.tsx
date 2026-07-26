import { useCallback, useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ShieldAlert } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button, Callout } from "@/components/superteam";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle
} from "@/components/ui/sheet";
import { buildUserAvatarDataUri } from "@/lib/avatar-dicebear";
import { listTeamSummaries, type TenantRole, type UserAvatar } from "@/lib/api";
import { cn } from "@/lib/utils";
import { HUMAN_AVATAR_PRESETS, type HumanAvatarPreset } from "../human-avatar-presets";
import { SelectableTeamList } from "./selectable-team-list";

export type CreateUserDraft = {
  avatar: UserAvatar;
  display_name: string;
  /** 飞书通讯录反查(按邮箱)的撞库键,可选。 */
  email: string;
  /** 手机号(建议含国际区号);飞书通讯录反查(按手机号)的撞库键,可选。 */
  mobile: string;
  password: string;
  selectable_team_ids: string[];
  tenant_role: TenantRole;
  username: string;
};

const TENANT_ROLE_OPTIONS: Array<{ value: TenantRole; label: string; hint: string }> = [
  { value: "member", label: "成员", hint: "可进入控制台，常规使用" },
  { value: "viewer", label: "观察者", hint: "可进入控制台，偏只读" },
  { value: "admin", label: "管理员", hint: "可管理用户与租户治理" },
  { value: "owner", label: "所有者", hint: "最高租户权限；仅所有者可授予" }
];

type CreateUserDrawerProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  isSubmitting?: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (draft: CreateUserDraft) => void;
  open: boolean;
  submitError?: string;
};

type CreateUserDraftState = Omit<CreateUserDraft, "avatar"> & {
  avatar: UserAvatar | null;
};

const emptyDraft: CreateUserDraftState = {
  avatar: null,
  display_name: "",
  email: "",
  mobile: "",
  password: "",
  selectable_team_ids: [],
  tenant_role: "member",
  username: ""
};

export function CreateUserDrawer({
  apiBaseUrl,
  fetcher,
  isSubmitting = false,
  onOpenChange,
  onSubmit,
  open,
  submitError
}: CreateUserDrawerProps) {
  const [draft, setDraft] = useState<CreateUserDraftState>(emptyDraft);
  const apiOptions = useMemo(
    () => ({
      baseUrl: apiBaseUrl,
      fetcher
}),
    [apiBaseUrl, fetcher],
  );
  const teamsQuery = useQuery({
    enabled: open,
    queryFn: () => listTeamSummaries(apiOptions, { status: "active" }),
    queryKey: ["users", "create", "teams"]
});
  const teams = teamsQuery.data ?? [];
  const activeTeams = teams.filter((team) => team.status === "active");
  const teamsHasError = Boolean(teamsQuery.error);
  const teamsReady = teamsQuery.isSuccess && !teamsQuery.isFetching && !teamsHasError && activeTeams.length > 0;
  const currentTeams = teamsReady ? activeTeams : [];
  const selectedTeamIdsAreCurrent =
    draft.selectable_team_ids.length === 0 ||
    (teamsReady && draft.selectable_team_ids.every((teamId) => currentTeams.some((team) => team.id === teamId)));
  const canSubmit = Boolean(
    draft.username.trim() &&
      draft.display_name.trim() &&
      draft.password.trim() &&
      draft.tenant_role &&
      draft.avatar &&
      selectedTeamIdsAreCurrent,
  );

  const resetDraft = useCallback(() => {
    setDraft(emptyDraft);
  }, []);

  useEffect(() => {
    if (!open) {
      resetDraft();
    }
  }, [open, resetDraft]);

  function handleOpenChange(nextOpen: boolean) {
    onOpenChange(nextOpen);
    if (!nextOpen) {
      resetDraft();
    }
  }

  return (
    <Sheet open={open} onOpenChange={handleOpenChange}>
      <SheetContent className="flex w-full flex-col gap-0 p-0 sm:max-w-[680px]">
        <SheetHeader className="border-b px-6 py-5">
          <SheetTitle>新建用户</SheetTitle>
          <SheetDescription className="sr-only">创建平台人类用户</SheetDescription>
        </SheetHeader>
        <form
          className="flex min-h-0 flex-1 flex-col"
          onSubmit={(event) => {
            event.preventDefault();
            const selectedAvatar = draft.avatar;
            if (canSubmit && !isSubmitting && selectedAvatar) {
              onSubmit({
                avatar: selectedAvatar,
                display_name: draft.display_name.trim(),
                email: draft.email.trim(),
                mobile: draft.mobile.trim(),
                password: draft.password,
                selectable_team_ids: draft.selectable_team_ids,
                tenant_role: draft.tenant_role,
                username: draft.username.trim()
});
            }
          }}
        >
          <div className="flex-1 overflow-y-auto p-6">
            <div className="grid gap-5">
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="flex flex-col gap-2">
                  <Label htmlFor="create-user-username">用户名</Label>
                  <Input
                    autoComplete="username"
                    id="create-user-username"
                    onChange={(event) => setDraft({ ...draft, username: event.target.value })}
                    value={draft.username}
                  />
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="create-user-display-name">名称</Label>
                  <Input
                    autoComplete="name"
                    id="create-user-display-name"
                    onChange={(event) => setDraft({ ...draft, display_name: event.target.value })}
                    value={draft.display_name}
                  />
                </div>
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="create-user-password">密码</Label>
                <Input
                  autoComplete="new-password"
                  id="create-user-password"
                  onChange={(event) => setDraft({ ...draft, password: event.target.value })}
                  type="password"
                  value={draft.password}
                />
              </div>

              <div className="grid gap-3 sm:grid-cols-2">
                <div className="flex flex-col gap-2">
                  <Label htmlFor="create-user-email">邮箱(可选)</Label>
                  <Input
                    autoComplete="email"
                    id="create-user-email"
                    onChange={(event) => setDraft({ ...draft, email: event.target.value })}
                    placeholder="与飞书档案一致以便同步绑定"
                    type="email"
                    value={draft.email}
                  />
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="create-user-mobile">手机号(可选)</Label>
                  <Input
                    autoComplete="tel"
                    id="create-user-mobile"
                    onChange={(event) => setDraft({ ...draft, mobile: event.target.value })}
                    placeholder="含区号,如 +8613800138000"
                    value={draft.mobile}
                  />
                </div>
              </div>

              <div className="flex flex-col gap-2">
                <Label htmlFor="create-user-tenant-role">租户角色</Label>
                <select
                  className="h-10 rounded-md border border-input bg-background px-3 text-sm"
                  disabled={isSubmitting}
                  id="create-user-tenant-role"
                  onChange={(event) =>
                    setDraft({ ...draft, tenant_role: event.target.value as TenantRole })
                  }
                  value={draft.tenant_role}
                >
                  {TENANT_ROLE_OPTIONS.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label} — {option.hint}
                    </option>
                  ))}
                </select>
                <p className="text-xs text-muted-foreground">
                  租户角色决定能否进入控制台；与下方「创建项目时可选团队」不是同一关系。
                </p>
              </div>

              <AvatarSelection
                disabled={isSubmitting}
                onSelect={(avatar) => setDraft({ ...draft, avatar })}
                presets={HUMAN_AVATAR_PRESETS}
                previewUsername={draft.username.trim() || draft.display_name.trim()}
                selectedAvatar={draft.avatar}
              />

              <section className="rounded-md border p-3">
                <div className="mb-1 flex min-w-0 items-center justify-between gap-3">
                  <h3 className="text-sm font-medium">
                    创建项目时可选择的团队
                    <span className="ml-1.5 text-xs font-normal text-muted-foreground">（可选）</span>
                  </h3>
                  {draft.selectable_team_ids.length > 0 && (
                    <span className="text-xs text-muted-foreground">
                      已选 {draft.selectable_team_ids.length}
                    </span>
                  )}
                </div>
                <p className="mb-3 text-xs text-muted-foreground">
                  仅限制创建项目时的团队选择器，不授予团队成员身份或控制台访问。
                </p>
                {teamsQuery.isLoading || teamsQuery.isFetching ? (
                  <p className="text-sm text-muted-foreground">加载团队中</p>
                ) : null}
                {teamsHasError ? (
                  <Callout
                    tone="danger"
                    title="团队列表加载失败"
                    description="请检查团队服务后重试。"
                    icon={<ShieldAlert aria-hidden className="size-4" />}
                  />
                ) : null}
                {!teamsQuery.isLoading && !teamsQuery.isFetching && !teamsHasError && activeTeams.length === 0 ? (
                  <p className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
                    暂无可用团队，可在创建用户后前往团队管理添加。
                  </p>
                ) : null}
                {currentTeams.length > 0 ? (
                  <SelectableTeamList
                    disabled={isSubmitting}
                    onChange={(selectableTeamIds) =>
                      setDraft({ ...draft, selectable_team_ids: selectableTeamIds })
                    }
                    selectedTeamIds={draft.selectable_team_ids}
                    teams={currentTeams}
                  />
                ) : null}
              </section>

              {submitError ? <p className="text-sm text-destructive">{submitError}</p> : null}
            </div>
          </div>
          <div className="flex justify-between gap-3 border-t p-4">
            <Button onClick={() => handleOpenChange(false)} type="button" variant="outline">
              取消
            </Button>
            <Button disabled={!canSubmit || isSubmitting} type="submit">
              创建用户
            </Button>
          </div>
        </form>
      </SheetContent>
    </Sheet>
  );
}

function AvatarSelection({
  disabled,
  onSelect,
  presets,
  previewUsername,
  selectedAvatar
}: {
  disabled: boolean;
  onSelect: (value: UserAvatar) => void;
  presets: HumanAvatarPreset[];
  previewUsername: string;
  selectedAvatar: UserAvatar | null;
}) {
  return (
    <fieldset className="rounded-md border p-3">
      <legend className="px-1 text-sm font-medium">头像</legend>
      <div className="mt-3 flex flex-wrap gap-2.5">
        {presets.map((preset) => {
          const selected =
            selectedAvatar?.provider === preset.avatar.provider &&
            selectedAvatar.style === preset.avatar.style &&
            selectedAvatar.seed === preset.avatar.seed;
          const src = buildUserAvatarDataUri(preset.avatar, previewUsername || preset.avatar.seed);
          return (
            <button
              aria-label={`选择头像 ${preset.label}`}
              aria-pressed={selected}
              className={cn(
                "flex size-14 shrink-0 items-center justify-center overflow-hidden rounded-full border bg-muted transition",
                selected ? "border-primary ring-2 ring-primary/30" : "hover:border-primary/60",
              )}
              disabled={disabled}
              key={preset.id}
              onClick={() => onSelect(preset.avatar)}
              type="button"
            >
              <img
                alt={preset.label}
                className="size-full scale-125 rounded-full object-cover"
                src={src}
              />
            </button>
          );
        })}
      </div>
    </fieldset>
  );
}

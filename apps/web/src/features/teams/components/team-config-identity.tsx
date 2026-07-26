import { useEffect, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Save, Trash2, UserPlus, X } from "lucide-react";
import {
  Button,
  LoadingState,
  StatusPill,
  WorkSurface
} from "@/components/superteam";
import { TeamIconPicker } from "@/components/superteam/team-icon-picker";
import {
  TeamIconTile,
  type TeamDisplayMetadata
} from "@/components/superteam/team-icon-tile";
import { UserIdentity } from "@/components/superteam/user-identity";
import { UserSearchSelect } from "@/components/superteam/user-search-select";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle
} from "@/components/ui/alert-dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import type { UserSummary } from "@/lib/api";
import { listAuthzMembers } from "@/lib/api";
import type { ApiClientOptions } from "@/lib/api/client";
import { ApiRequestError } from "@/lib/api/client";
import type { AllowedTeamAction, TeamOverview } from "@/lib/api/teams";
import { updateTeam } from "@/lib/api/teams";

function errorText(error: unknown, fallback: string) {
  if (error instanceof ApiRequestError && error.detail) {
    return error.detail;
  }
  if (error instanceof Error && error.message) {
    return error.message;
  }
  return fallback;
}

type TeamConfigIdentityProps = {
  allowedActions: AllowedTeamAction[];
  apiOptions: ApiClientOptions;
  onDeleteTeam: () => void;
  onSaved: () => void;
  overview: TeamOverview;
};

export function TeamConfigIdentity({
  allowedActions,
  apiOptions,
  onDeleteTeam,
  onSaved,
  overview
}: TeamConfigIdentityProps) {
  const team = overview.team;
  const canUpdate = allowedActions.includes("team.update");
  const canDelete = allowedActions.includes("team.delete");

  const [name, setName] = useState(team.name);
  const [slug, setSlug] = useState(team.slug);
  const [description, setDescription] = useState(team.description ?? "");
  const [ownerIds, setOwnerIds] = useState<string[]>(team.human_owner_user_ids ?? []);
  const [iconKey, setIconKey] = useState(
    ((team.metadata ?? {}) as TeamDisplayMetadata).display?.icon_key ?? "",
  );
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [hydratedTeamId, setHydratedTeamId] = useState("");

  useEffect(() => {
    if (hydratedTeamId === team.id) return;
    setName(team.name);
    setSlug(team.slug);
    setDescription(team.description ?? "");
    setOwnerIds(team.human_owner_user_ids ?? []);
    setIconKey(((team.metadata ?? {}) as TeamDisplayMetadata).display?.icon_key ?? "");
    setHydratedTeamId(team.id);
  }, [hydratedTeamId, team]);

  const tenantMembersQuery = useQuery({
    enabled: canUpdate,
    queryFn: () => listAuthzMembers({ ...apiOptions, limit: 200, offset: 0 }),
    queryKey: ["authz-members", "team-owner-candidates", apiOptions.baseUrl]
  });
  const candidateUserIds = (tenantMembersQuery.data?.items ?? [])
    .filter((member) => member.console_access && member.account_status === "active")
    .map((member) => member.user_id);

  const colorTone =
    ((team.metadata ?? {}) as TeamDisplayMetadata).display?.color_tone ?? "neutral";

  const saveMutation = useMutation({
    mutationFn: () =>
      updateTeam(apiOptions, team.id, {
        slug: slug.trim(),
        name: name.trim(),
        description: description.trim(),
        human_owner_user_ids: ownerIds,
        metadata: {
          ...(team.metadata ?? {}),
          display: {
            ...(((team.metadata ?? {}) as TeamDisplayMetadata).display ?? {}),
            color_tone: colorTone,
            icon_key: iconKey
          }
        }
      }),
    onSuccess: onSaved
  });

  // 负责人集合不得为空——这是团队的硬约束，前端先挡住，避免提交后才被服务端拒绝。
  const ownersEmpty = ownerIds.length === 0;
  const canSubmit = canUpdate && !saveMutation.isPending && name.trim() && slug.trim() && !ownersEmpty;

  const ownerProfiles = new Map(
    (team.human_owners ?? []).map((owner) => [owner.user_id, owner]),
  );

  return (
    <div className="flex flex-col gap-4">
      <WorkSurface>
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-5 py-4">
          <div className="min-w-0">
            <h2 className="text-base font-bold text-ink">团队身份</h2>
            <p className="mt-1 text-[13px] text-ink-2">名称、标识、说明、图标与负责人集合。</p>
          </div>
          <Button disabled={!canSubmit} onClick={() => saveMutation.mutate()} size="sm">
            <Save data-icon="inline-start" />
            保存身份
          </Button>
        </div>
        <div className="flex flex-col gap-5 p-5">
          <div className="flex items-start gap-4">
            <TeamIconTile
              className="size-14 rounded-[18px]"
              metadata={{ display: { color_tone: colorTone, icon_key: iconKey } }}
            />
            <div className="flex flex-col gap-2">
              <Label>团队图标</Label>
              <TeamIconPicker colorTone={colorTone} onSelect={setIconKey} value={iconKey} />
            </div>
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <div className="flex flex-col gap-2">
              <Label htmlFor="team-config-name">团队名称</Label>
              <Input
                disabled={!canUpdate}
                id="team-config-name"
                onChange={(event) => setName(event.target.value)}
                value={name}
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="team-config-slug">团队标识</Label>
              <Input
                disabled={!canUpdate}
                id="team-config-slug"
                onChange={(event) => setSlug(event.target.value)}
                value={slug}
              />
            </div>
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="team-config-description">团队说明</Label>
            <Textarea
              disabled={!canUpdate}
              id="team-config-description"
              onChange={(event) => setDescription(event.target.value)}
              rows={3}
              value={description}
            />
          </div>

          <div className="flex flex-col gap-3">
            <div className="flex items-center justify-between gap-2">
              <Label>团队负责人</Label>
              <StatusPill tone={ownersEmpty ? "warn" : "mute"}>{ownerIds.length} 人</StatusPill>
            </div>
            <p className="text-xs text-ink-3">
              负责人可多个且平级，任一负责人即可审批与验收；集合不得为空。
            </p>
            {tenantMembersQuery.isLoading ? <LoadingState label="加载候选用户" /> : null}
            <ul className="flex flex-col gap-2">
              {ownerIds.map((ownerId) => {
                const profile = ownerProfiles.get(ownerId);
                return (
                  <li
                    key={ownerId}
                    className="flex items-center justify-between gap-3 rounded-[14px] border border-line bg-card-soft/60 px-3 py-2.5"
                  >
                    <UserIdentity
                      showSecondary
                      size="sm"
                      user={{
                        id: ownerId,
                        username: profile?.username ?? ownerId,
                        display_name: profile?.display_name,
                        email: profile?.email,
                        avatar: profile?.avatar,
                        status: profile?.status ?? "active"
                      }}
                    />
                    <Button
                      aria-label={`移除负责人 ${profile?.display_name || profile?.username || ownerId}`}
                      disabled={!canUpdate || ownerIds.length <= 1}
                      onClick={() => setOwnerIds((ids) => ids.filter((id) => id !== ownerId))}
                      size="icon"
                      type="button"
                      variant="ghost"
                    >
                      <X className="size-4" />
                    </Button>
                  </li>
                );
              })}
            </ul>
            {canUpdate ? (
              <AddOwnerField
                allowedUserIds={candidateUserIds}
                apiBaseUrl={apiOptions.baseUrl}
                excludedUserIds={ownerIds}
                fetcher={apiOptions.fetcher}
                onAdd={(user) => setOwnerIds((ids) => (ids.includes(user.id) ? ids : [...ids, user.id]))}
              />
            ) : null}
          </div>

          {saveMutation.isError ? (
            <p className="text-[13px] text-danger">
              {errorText(saveMutation.error, "保存失败，请重试")}
            </p>
          ) : null}
          {saveMutation.isSuccess ? (
            <p className="text-[13px] text-ink-2">团队身份已保存。</p>
          ) : null}
        </div>
      </WorkSurface>

      {canDelete ? (
        <WorkSurface>
          <div className="border-b border-line px-5 py-4">
            <h2 className="text-base font-bold text-danger">危险区</h2>
            <p className="mt-1 text-[13px] text-ink-2">
              删除后团队进入「待确认删除」，管理员可在团队管理页恢复，或确认后彻底删除。
            </p>
          </div>
          <div className="p-5">
            <Button
              className="text-danger hover:bg-danger-soft hover:text-danger"
              onClick={() => setDeleteOpen(true)}
              size="sm"
              type="button"
              variant="ghost"
            >
              <Trash2 data-icon="inline-start" />
              删除团队
            </Button>
          </div>
          <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>确认删除团队</AlertDialogTitle>
                <AlertDialogDescription>
                  删除后，所有绑定的数字员工将失去团队归属（进入候岗大厅），技能与能力（MCP）绑定一并解除；团队进入"待确认删除"状态，管理员可在团队管理页恢复，或确认后彻底删除。
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>取消</AlertDialogCancel>
                <AlertDialogAction onClick={onDeleteTeam}>确认删除</AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </WorkSurface>
      ) : null}
    </div>
  );
}

function AddOwnerField({
  allowedUserIds,
  apiBaseUrl,
  excludedUserIds,
  fetcher,
  onAdd
}: {
  allowedUserIds: string[];
  apiBaseUrl: string;
  excludedUserIds: string[];
  fetcher?: typeof fetch;
  onAdd: (user: UserSummary) => void;
}) {
  const [selected, setSelected] = useState<UserSummary | undefined>();

  return (
    <div className="flex flex-col gap-2 border-t border-line pt-4">
      <Label>添加负责人</Label>
      <UserSearchSelect
        allowedUserIds={allowedUserIds}
        apiBaseUrl={apiBaseUrl}
        excludedUserIds={excludedUserIds}
        fetcher={fetcher}
        inputLabel="搜索负责人候选"
        onSelect={setSelected}
        placeholder="搜索已有租户成员"
        value={selected}
      />
      <Button
        className="self-start"
        disabled={!selected}
        onClick={() => {
          if (selected) {
            onAdd(selected);
            setSelected(undefined);
          }
        }}
        size="sm"
        type="button"
        variant="outline"
      >
        <UserPlus data-icon="inline-start" />
        加为负责人
      </Button>
    </div>
  );
}

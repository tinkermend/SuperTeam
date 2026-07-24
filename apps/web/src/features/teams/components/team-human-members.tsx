import { useEffect, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Settings, Trash2, UserPlus } from "lucide-react";
import { StatusPill, Button, LoadingState } from "@/components/superteam";
import {
  TeamRoleBadge,
  TeamRoleSelect,
  type DirectTeamRole
} from "@/components/superteam/team-role";
import {
  UserIdentity,
  UserIdentityAvatar
} from "@/components/superteam/user-identity";
import { UserSearchSelect } from "@/components/superteam/user-search-select";
import { Label } from "@/components/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle
} from "@/components/ui/sheet";
import type { UserSummary } from "@/lib/api";
import type { ApiClientOptions } from "@/lib/api/client";
import type { AllowedTeamAction, TeamMember } from "@/lib/api/teams";
import { addTeamMember, listTeamMembers, removeTeamMember } from "@/lib/api/teams";
import { cn } from "@/lib/utils";

const AVATAR_STACK_LIMIT = 4;

type TeamHumanMembersProps = {
  allowedActions: AllowedTeamAction[];
  apiOptions: ApiClientOptions;
  configOpen: boolean;
  onConfigOpenChange: (open: boolean) => void;
  teamId: string;
};

export function TeamHumanMembersChrome({
  allowedActions,
  apiOptions,
  configOpen,
  onConfigOpenChange,
  teamId
}: TeamHumanMembersProps) {
  const canAddMember = allowedActions.includes("team.member.add");
  const canRemoveMember = allowedActions.includes("team.member.remove");
  const canConfigure = canConfigureTeamMembers(allowedActions);
  const [addResetToken, setAddResetToken] = useState(0);

  const membersQuery = useQuery({
    queryKey: ["team-members", teamId],
    queryFn: () => listTeamMembers(apiOptions, teamId)
});

  const members = membersQuery.data ?? [];
  const existingUserIds = members.map((member) => member.user_id);

  const addMutation = useMutation({
    mutationFn: (input: { role: "member" | "viewer"; user_id: string }) =>
      addTeamMember(apiOptions, teamId, input),
    onSuccess: () => {
      void membersQuery.refetch();
      setAddResetToken((token) => token + 1);
    }
});

  const removeMutation = useMutation({
    mutationFn: (membershipId: string) => removeTeamMember(apiOptions, teamId, membershipId),
    onSuccess: () => {
      void membersQuery.refetch();
    }
});

  return (
    <>
      <TeamMemberAvatarStack
        canConfigure={canConfigure}
        isLoading={membersQuery.isLoading}
        members={members}
        onConfigure={() => onConfigOpenChange(true)}
      />

      <Sheet
        open={configOpen && canConfigure}
        onOpenChange={(open) => {
          if (open && !canConfigure) {
            return;
          }
          onConfigOpenChange(open);
        }}
      >
        <SheetContent className="flex w-full flex-col gap-0 overflow-y-auto sm:max-w-lg">
          <SheetHeader className="border-b border-line px-6 py-5 text-left">
            <SheetTitle>配置团队</SheetTitle>
            <SheetDescription>管理人类成员与角色；普通成员和只读观察者会立即生效。</SheetDescription>
          </SheetHeader>
          <div className="flex flex-col gap-6 px-6 py-5">
            <section className="flex flex-col gap-3">
              <div className="flex items-center justify-between gap-2">
                <h3 className="text-sm font-bold text-ink">人类成员</h3>
                <StatusPill tone="mute">{members.length} 人</StatusPill>
              </div>
              {membersQuery.isLoading ? (
                <LoadingState label="加载人类成员" />
              ) : members.length === 0 ? (
                <p className="text-[13px] text-ink-2">暂无人类成员</p>
              ) : (
                <ul className="flex flex-col gap-2">
                  {members.map((member) => (
                    <li
                      key={member.membership_id}
                      className="flex items-center justify-between gap-3 rounded-[14px] border border-line bg-card-soft/60 px-3 py-2.5"
                    >
                      <UserIdentity
                        showSecondary
                        size="sm"
                        user={{
                          id: member.user_id,
                          username: member.username,
                          display_name: member.display_name,
                          email: member.email,
                          avatar: member.avatar,
                          status: member.account_status || "active"
}}
                      />
                      <div className="flex shrink-0 items-center gap-2">
                        <TeamRoleBadge role={member.role as DirectTeamRole} />
                        {canRemoveMember && member.role !== "owner" ? (
                          <Button
                            aria-label={`移除 ${member.display_name || member.username}`}
                            disabled={removeMutation.isPending}
                            onClick={() => removeMutation.mutate(member.membership_id)}
                            size="icon"
                            type="button"
                            variant="ghost"
                          >
                            <Trash2 className="size-4" />
                          </Button>
                        ) : null}
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </section>

            {canAddMember ? (
              <section className="flex flex-col gap-3 border-t border-line pt-5">
                <h3 className="text-sm font-bold text-ink">添加成员</h3>
                <DirectAddForm
                  apiBaseUrl={apiOptions.baseUrl}
                  canAdd={canAddMember}
                  existingUserIds={existingUserIds}
                  fetcher={apiOptions.fetcher}
                  isPending={addMutation.isPending}
                  onSubmit={(input) => addMutation.mutate(input)}
                  resetToken={addResetToken}
                />
              </section>
            ) : null}
          </div>
        </SheetContent>
      </Sheet>
    </>
  );
}

function TeamMemberAvatarStack({
  canConfigure,
  isLoading,
  members,
  onConfigure
}: {
  canConfigure: boolean;
  isLoading: boolean;
  members: TeamMember[];
  onConfigure: () => void;
}) {
  const visible = members.slice(0, AVATAR_STACK_LIMIT);
  const overflow = Math.max(0, members.length - visible.length);

  if (isLoading) {
    return (
      <div
        aria-hidden
        className="flex h-8 w-24 animate-pulse rounded-full bg-card-soft"
      />
    );
  }

  if (members.length === 0) {
    if (!canConfigure) {
      return <p className="text-[12px] text-ink-3">暂无人类成员</p>;
    }
    return (
      <button
        className="text-[12px] font-semibold text-brand hover:underline"
        onClick={onConfigure}
        type="button"
      >
        添加人类成员
      </button>
    );
  }

  return (
    <Popover>
      <PopoverTrigger asChild>
        <button
          aria-label={`人类成员 ${members.length} 人`}
          className="flex items-center rounded-full p-0.5 transition-colors hover:bg-card-soft focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/60"
          type="button"
        >
          <span className="flex items-center">
            {visible.map((member, index) => (
              <UserIdentityAvatar
                key={member.membership_id}
                className={cn(
                  "size-8 border-2 border-card shadow-none",
                  index > 0 && "-ml-2",
                )}
                user={{
                  id: member.user_id,
                  username: member.username,
                  display_name: member.display_name,
                  email: member.email,
                  avatar: member.avatar,
                  status: member.account_status || "active"
}}
              />
            ))}
            {overflow > 0 ? (
              <span className="-ml-2 grid size-8 place-items-center rounded-full border-2 border-card bg-card-soft text-[11px] font-bold text-ink-2">
                +{overflow}
              </span>
            ) : null}
          </span>
        </button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-80 rounded-[16px] border-line p-3 shadow-card">
        <div className="mb-2 flex items-center justify-between gap-2">
          <p className="text-[13px] font-semibold text-ink">人类成员 · {members.length}</p>
          {canConfigure ? (
            <Button onClick={onConfigure} size="sm" type="button" variant="ghost">
              <Settings data-icon="inline-start" className="size-3.5" />
              配置
            </Button>
          ) : null}
        </div>
        <ul className="flex max-h-64 flex-col gap-2 overflow-y-auto">
          {members.map((member) => (
            <li key={member.membership_id} className="flex items-center justify-between gap-2">
              <UserIdentity
                size="sm"
                user={{
                  id: member.user_id,
                  username: member.username,
                  display_name: member.display_name,
                  email: member.email,
                  avatar: member.avatar,
                  status: member.account_status || "active"
}}
              />
              <TeamRoleBadge role={member.role as DirectTeamRole} />
            </li>
          ))}
        </ul>
      </PopoverContent>
    </Popover>
  );
}

function DirectAddForm({
  apiBaseUrl,
  canAdd,
  existingUserIds,
  fetcher,
  isPending,
  onSubmit,
  resetToken
}: {
  apiBaseUrl: string;
  canAdd: boolean;
  existingUserIds: string[];
  fetcher?: typeof fetch;
  isPending: boolean;
  onSubmit: (input: { role: DirectTeamRole; user_id: string }) => void;
  resetToken: number;
}) {
  const [selectedUser, setSelectedUser] = useState<UserSummary | undefined>();
  const [role, setRole] = useState<DirectTeamRole>("member");

  useEffect(() => {
    setSelectedUser(undefined);
    setRole("member");
  }, [resetToken]);

  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(event) => {
        event.preventDefault();
        if (selectedUser) {
          onSubmit({ role, user_id: selectedUser.id });
        }
      }}
    >
      <div className="flex flex-col gap-2">
        <Label>用户</Label>
        <UserSearchSelect
          apiBaseUrl={apiBaseUrl}
          disabled={!canAdd || isPending}
          excludedUserIds={existingUserIds}
          fetcher={fetcher}
          inputLabel="搜索直接添加用户"
          onSelect={setSelectedUser}
          value={selectedUser}
        />
      </div>
      <div className="flex flex-col gap-2">
        <Label>直接生效角色</Label>
        <TeamRoleSelect
          ariaLabel="直接生效角色"
          disabled={!canAdd || isPending}
          mode="direct"
          onChange={setRole}
          value={role}
        />
      </div>
      <Button disabled={!canAdd || isPending || !selectedUser} type="submit">
        <UserPlus data-icon="inline-start" className="mr-2" />
        添加成员
      </Button>
    </form>
  );
}

export function canConfigureTeamMembers(allowedActions: AllowedTeamAction[]) {
  return (
    allowedActions.includes("team.member.add") ||
    allowedActions.includes("team.member.remove") ||
    allowedActions.includes("team.update")
  );
}

/** 供头卡 meta 使用：人类人数由头像栈表达，此处仅拼数字员工与能力。 */
export function teamHeaderMetaLabel(input: {
  capabilityCount: number;
  digitalEmployeeCount: number;
}) {
  return `${input.digitalEmployeeCount} 数字员工 · ${input.capabilityCount} 能力绑定`;
}

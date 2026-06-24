import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { X } from "lucide-react";
import { directTeamRoles, type DirectTeamRole } from "@/components/superteam/team-role";
import { UserIdentity } from "@/components/superteam/user-identity";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { listUsers, type UserSummary } from "@/lib/api/auth";
import type { InitialTeamMemberInput } from "@/lib/api/teams";
import type { CreateTeamDraft } from "./create-team-draft";

type CreateTeamMembersStepProps = {
  apiBaseUrl: string;
  draft: CreateTeamDraft;
  fetcher?: typeof fetch;
  onChange: (draft: CreateTeamDraft) => void;
};

export function CreateTeamMembersStep({
  apiBaseUrl,
  draft,
  fetcher,
  onChange,
}: CreateTeamMembersStepProps) {
  const [query, setQuery] = useState("");
  const users = useQuery({
    queryKey: ["team-member-candidates", query],
    queryFn: () =>
      listUsers({
        baseUrl: apiBaseUrl,
        fetcher,
        limit: 20,
        offset: 0,
        q: query,
        status: "active",
      }),
  });

  const selectedIds = useMemo(
    () => new Set(draft.initial_members.map((member) => member.user_id)),
    [draft.initial_members],
  );

  const candidates = (users.data?.items ?? []).filter(
    (user) => user.id !== draft.owner?.id && !selectedIds.has(user.id),
  );

  function addMember(user: UserSummary, role: DirectTeamRole = "member") {
    if (user.id === draft.owner?.id || selectedIds.has(user.id)) return;
    onChange({
      ...draft,
      initial_members: [...draft.initial_members, { role, user_id: user.id }],
      memberUsers: { ...draft.memberUsers, [user.id]: user },
    });
  }

  function removeMember(userId: string) {
    const nextMemberUsers = { ...draft.memberUsers };
    delete nextMemberUsers[userId];
    onChange({
      ...draft,
      initial_members: draft.initial_members.filter(
        (member) => member.user_id !== userId,
      ),
      memberUsers: nextMemberUsers,
    });
  }

  function updateRole(userId: string, role: DirectTeamRole) {
    onChange({
      ...draft,
      initial_members: draft.initial_members.map((member) =>
        member.user_id === userId ? { ...member, role } : member,
      ),
    });
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="rounded-md border bg-muted/20 px-3 py-2 text-xs text-muted-foreground">
        创建时仅可加入「普通成员」与「只读观察者」；负责人、管理员、审批人需创建后发起特权角色申请。
      </div>

      <div className="grid gap-2">
        <Label htmlFor="team-member-search">搜索用户</Label>
        <Input
          aria-label="搜索候选成员"
          id="team-member-search"
          onChange={(event) => setQuery(event.target.value)}
          placeholder="按用户名搜索后点击添加"
          type="search"
          value={query}
        />
        <div className="flex min-w-0 flex-col gap-1">
          {users.isLoading ? (
            <p className="px-2 py-1.5 text-sm text-muted-foreground">
              加载候选用户中
            </p>
          ) : users.isError ? (
            <p className="px-2 py-1.5 text-sm text-destructive">
              候选用户加载失败
            </p>
          ) : candidates.length === 0 ? (
            <p className="px-2 py-1.5 text-sm text-muted-foreground">
              暂无可添加的候选用户
            </p>
          ) : (
            candidates.map((user) => (
              <div
                className="flex items-center gap-3 rounded-md px-2 py-1.5 hover:bg-muted/50"
                key={user.id}
              >
                <UserIdentity className="min-w-0" showSecondary size="sm" user={user} />
                <Button
                  aria-label={`添加 ${user.username}`}
                  className="ml-auto"
                  onClick={() => addMember(user)}
                  size="sm"
                  type="button"
                  variant="outline"
                >
                  添加
                </Button>
              </div>
            ))
          )}
        </div>
      </div>

      <div className="flex flex-col gap-2">
        <h3 className="text-sm font-medium">
          已选择的初始成员（{draft.initial_members.length}）
        </h3>
        {draft.initial_members.length === 0 ? (
          <p className="rounded-md border px-3 py-2 text-sm text-muted-foreground">
            暂未选择初始成员。
          </p>
        ) : (
          <ul className="flex flex-col gap-2">
            {draft.initial_members.map((member) => (
              <SelectedMemberChip
                key={member.user_id}
                member={member}
                onRemove={() => removeMember(member.user_id)}
                onRoleChange={(role) => updateRole(member.user_id, role)}
                user={draft.memberUsers[member.user_id]}
              />
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

function SelectedMemberChip({
  member,
  onRemove,
  onRoleChange,
  user,
}: {
  member: InitialTeamMemberInput;
  onRemove: () => void;
  onRoleChange: (role: DirectTeamRole) => void;
  user?: UserSummary;
}) {
  const fallbackUser: UserSummary = {
    avatar: { provider: "dicebear", seed: member.user_id, style: "adventurer" },
    id: member.user_id,
    status: "active",
    username: member.user_id,
  };
  const visibleUser = user ?? fallbackUser;

  return (
    <li className="flex items-center gap-3 rounded-lg border bg-muted/30 px-3 py-2">
      <UserIdentity className="min-w-0" showSecondary size="sm" user={visibleUser} />
      <div className="ml-auto flex items-center gap-2">
        <Select
          onValueChange={(role) => onRoleChange(role as DirectTeamRole)}
          value={member.role}
        >
          <SelectTrigger
            aria-label={`${visibleUser.username} 角色`}
            className="w-32"
            size="sm"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {directTeamRoles.map((role) => (
                <SelectItem key={role.value} value={role.value}>
                  {role.label}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <Button
          aria-label={`移除 ${visibleUser.username}`}
          onClick={onRemove}
          size="icon"
          type="button"
          variant="ghost"
        >
          <X className="size-4" />
        </Button>
      </div>
    </li>
  );
}

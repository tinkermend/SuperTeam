import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Trash2 } from "lucide-react";
import { TeamRoleSelect, type DirectTeamRole } from "@/components/superteam/team-role";
import { UserIdentity } from "@/components/superteam/user-identity";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
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
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { listUsers, type UserSummary } from "@/lib/api/auth";
import type { InitialTeamMemberInput } from "@/lib/api/teams";
import type { CreateTeamDraft } from "./create-team-drawer";

type CreateTeamMembersStepProps = {
  apiBaseUrl: string;
  draft: CreateTeamDraft;
  fetcher?: typeof fetch;
  onChange: (draft: CreateTeamDraft) => void;
};

type MemberRoleFilter = "all" | "member" | "viewer";

export function CreateTeamMembersStep({
  apiBaseUrl,
  draft,
  fetcher,
  onChange,
}: CreateTeamMembersStepProps) {
  const [candidateRoles, setCandidateRoles] = useState<Record<string, DirectTeamRole>>({});
  const [roleFilter, setRoleFilter] = useState<MemberRoleFilter>("all");
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
  const selectedByUserId = useMemo(
    () => new Map(draft.initial_members.map((member) => [member.user_id, member])),
    [draft.initial_members],
  );
  function candidateRoleFor(userId: string) {
    return selectedByUserId.get(userId)?.role ?? candidateRoles[userId] ?? "member";
  }

  const userItems = (users.data?.items ?? [])
    .filter((user) => user.id !== draft.owner?.id)
    .filter((user) => {
      if (roleFilter === "all") return true;

      return candidateRoleFor(user.id) === roleFilter;
    });

  function upsertMember(user: UserSummary, role: DirectTeamRole = "member") {
    if (user.id === draft.owner?.id) return;
    const exists = selectedByUserId.has(user.id);
    const initial_members = exists
      ? draft.initial_members.map((member) =>
          member.user_id === user.id ? { ...member, role } : member,
        )
      : [...draft.initial_members, { role, user_id: user.id }];
    onChange({
      ...draft,
      initial_members,
      memberUsers: { ...draft.memberUsers, [user.id]: user },
    });
  }

  function removeMember(userId: string) {
    const nextMemberUsers = { ...draft.memberUsers };
    delete nextMemberUsers[userId];
    onChange({
      ...draft,
      initial_members: draft.initial_members.filter((member) => member.user_id !== userId),
      memberUsers: nextMemberUsers,
    });
  }

  function updateSelectedRole(userId: string, role: DirectTeamRole) {
    setCandidateRoles((current) => ({ ...current, [userId]: role }));
    onChange({
      ...draft,
      initial_members: draft.initial_members.map((member) =>
        member.user_id === userId ? { ...member, role } : member,
      ),
    });
  }

  function changeCandidateRole(user: UserSummary, role: DirectTeamRole) {
    if (selectedByUserId.has(user.id)) {
      updateSelectedRole(user.id, role);
      return;
    }

    setCandidateRoles((current) => ({ ...current, [user.id]: role }));
  }

  return (
    <div className="flex flex-col gap-5">
      <section className="rounded-md border p-4">
        <h3 className="text-sm font-medium">基础信息</h3>
        <dl className="mt-3 grid gap-3 text-sm sm:grid-cols-3">
          <div>
            <dt className="text-muted-foreground">团队名称</dt>
            <dd className="mt-1 font-medium">{draft.name || "-"}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">slug</dt>
            <dd className="mt-1 font-medium">{draft.slug || "-"}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">负责人</dt>
            <dd className="mt-1 font-medium">{draft.owner?.username ?? "-"}</dd>
          </div>
        </dl>
      </section>

      <div className="rounded-md border px-3 py-2 text-sm text-muted-foreground">
        负责人、管理员、审批人需创建后发起特权角色申请。
      </div>

      <section className="flex flex-col gap-3">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
          <div className="grid flex-1 gap-2">
            <Label htmlFor="team-member-search">候选成员</Label>
            <Input
              aria-label="搜索候选成员"
              id="team-member-search"
              onChange={(event) => setQuery(event.target.value)}
              placeholder="搜索用户名"
              type="search"
              value={query}
            />
          </div>
          <Select onValueChange={(value) => setRoleFilter(value as MemberRoleFilter)} value={roleFilter}>
            <SelectTrigger aria-label="角色筛选" className="w-full sm:w-40">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem value="all">全部</SelectItem>
                <SelectItem value="member">普通成员</SelectItem>
                <SelectItem value="viewer">只读观察者</SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>

        {users.isLoading ? (
          <div className="rounded-md border px-3 py-2 text-sm text-muted-foreground">
            加载候选用户中
          </div>
        ) : null}
        {users.isError ? (
          <div className="rounded-md border px-3 py-2 text-sm text-destructive">
            候选用户加载失败
          </div>
        ) : null}
        {!users.isLoading && !users.isError && userItems.length === 0 ? (
          <div className="rounded-md border px-3 py-2 text-sm text-muted-foreground">
            暂无可添加的候选用户
          </div>
        ) : null}
        {userItems.length > 0 ? (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-10">选择</TableHead>
                <TableHead>用户</TableHead>
                <TableHead>初始角色</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {userItems.map((user) => {
                const selected = selectedByUserId.get(user.id);
                const role = candidateRoleFor(user.id);

                return (
                  <TableRow data-state={selected ? "selected" : undefined} key={user.id}>
                    <TableCell>
                      <Checkbox
                        aria-label={`选择 ${user.username} 为初始成员`}
                        checked={Boolean(selected)}
                        onCheckedChange={(checked) => {
                          if (checked) {
                            upsertMember(user, candidateRoleFor(user.id));
                          } else {
                            removeMember(user.id);
                          }
                        }}
                      />
                    </TableCell>
                    <TableCell>
                      <UserIdentity showSecondary size="sm" user={user} />
                    </TableCell>
                    <TableCell>
                      <TeamRoleSelect
                        mode="direct"
                        onChange={(nextRole) => changeCandidateRole(user, nextRole)}
                        value={role}
                      />
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        ) : null}
      </section>

      <section className="flex flex-col gap-3">
        <h3 className="text-sm font-medium">
          已选择的初始成员（{draft.initial_members.length}）
        </h3>
        {draft.initial_members.length === 0 ? (
          <div className="rounded-md border px-3 py-2 text-sm text-muted-foreground">
            暂未选择初始成员。
          </div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>成员</TableHead>
                <TableHead>角色</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {draft.initial_members.map((member) => (
                <SelectedMemberRow
                  key={member.user_id}
                  member={member}
                  onRemove={() => removeMember(member.user_id)}
                  onRoleChange={(role) => updateSelectedRole(member.user_id, role)}
                  user={draft.memberUsers[member.user_id]}
                />
              ))}
            </TableBody>
          </Table>
        )}
      </section>
    </div>
  );
}

function SelectedMemberRow({
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
    avatar: {
      provider: "dicebear",
      seed: member.user_id,
      style: "adventurer",
    },
    id: member.user_id,
    status: "active",
    username: member.user_id,
  };
  const visibleUser = user ?? fallbackUser;

  return (
    <TableRow>
      <TableCell>
        <UserIdentity showSecondary size="sm" user={visibleUser} />
      </TableCell>
      <TableCell>
        <TeamRoleSelect mode="direct" onChange={onRoleChange} value={member.role} />
      </TableCell>
      <TableCell className="text-right">
        <Button aria-label={`移除 ${visibleUser.username}`} onClick={onRemove} size="icon" type="button" variant="ghost">
          <Trash2 className="size-4" />
        </Button>
      </TableCell>
    </TableRow>
  );
}

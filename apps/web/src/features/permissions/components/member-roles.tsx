import { useQuery } from "@tanstack/react-query";
import { Users } from "lucide-react";
import type { ApiClientOptions, AuthzMemberRecord } from "@/lib/api";
import { listAuthzMembers } from "@/lib/api";
import {
  IconTile,
  StatusPill,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  WorkSurface,
} from "@/components/superteam";

type MemberRolesProps = {
  apiOptions: ApiClientOptions;
};

export function MemberRoles({ apiOptions }: MemberRolesProps) {
  const membersQuery = useQuery({
    queryKey: ["authz-members", apiOptions.baseUrl, 50, 0],
    queryFn: () => listAuthzMembers({ ...apiOptions, limit: 50, offset: 0 }),
  });

  return (
    <WorkSurface>
      <div className="flex items-start gap-3 border-b border-v3-line px-5 py-4">
        <IconTile tone="brand" size="sm">
          <Users />
        </IconTile>
        <div>
          <h2 className="text-base font-bold text-v3-ink">成员角色</h2>
          <p className="mt-1 text-sm text-v3-ink-2">当前只读展示成员、租户/团队角色和控制台访问能力。</p>
        </div>
      </div>
      {membersQuery.isLoading ? (
        <V3LoadingState label="加载成员角色…" />
      ) : membersQuery.isError ? (
        <V3ErrorState
          className="m-5"
          title="成员角色加载失败"
          description="请稍后刷新或检查 Control Plane 连接。"
        />
      ) : (membersQuery.data?.items.length ?? 0) === 0 ? (
        <V3EmptyState title="暂无成员角色记录" description="授权成员同步后会显示在这里。" />
      ) : (
        <MemberRolesTable members={membersQuery.data?.items ?? []} />
      )}
    </WorkSurface>
  );
}

function MemberRolesTable({ members }: { members: AuthzMemberRecord[] }) {
  return (
    <V3Table>
      <thead>
        <V3Tr>
          <V3Th>成员</V3Th>
          <V3Th>账号状态</V3Th>
          <V3Th>控制台</V3Th>
          <V3Th>角色</V3Th>
          <V3Th>最近拒绝原因</V3Th>
        </V3Tr>
      </thead>
      <tbody>
        {members.map((member) => (
          <V3Tr key={member.user_id} tone={member.recent_denied_reason ? "warn" : undefined}>
            <V3Td>
              <div className="flex flex-col gap-1">
                <span className="font-semibold text-v3-ink">{member.display_name || member.username}</span>
                <span className="text-xs text-v3-ink-3">{member.email ?? member.user_id}</span>
              </div>
            </V3Td>
            <V3Td>
              <StatusPill tone={member.account_status === "active" ? "ok" : "mute"}>{member.account_status}</StatusPill>
            </V3Td>
            <V3Td>
              <StatusPill tone={member.console_access ? "ok" : "mute"}>{member.console_access ? "允许" : "无访问"}</StatusPill>
            </V3Td>
            <V3Td>{formatMemberships(member)}</V3Td>
            <V3Td className="max-w-72 truncate">{member.recent_denied_reason ?? "-"}</V3Td>
          </V3Tr>
        ))}
      </tbody>
    </V3Table>
  );
}

function formatMemberships(member: AuthzMemberRecord) {
  if (member.memberships.length === 0) {
    return "无角色";
  }

  return member.memberships
    .map((membership) => {
      const scope = membership.team_id ? `team:${membership.team_id}` : `tenant:${membership.tenant_id}`;
      return `${membership.role} / ${scope} / ${membership.status}`;
    })
    .join("; ");
}

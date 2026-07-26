import { useQuery } from "@tanstack/react-query";
import { StatusPill } from "@/components/superteam";
import { TeamRoleBadge } from "@/components/superteam/team-role";
import {
  UserIdentity,
  UserIdentityAvatar
} from "@/components/superteam/user-identity";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import type { ApiClientOptions } from "@/lib/api/client";
import type { TeamMember } from "@/lib/api/teams";
import { listTeamMembers } from "@/lib/api/teams";
import { cn } from "@/lib/utils";

const AVATAR_STACK_LIMIT = 4;

// 详情头卡的人类成员头像栈——只读。成员的增删改与特权申请在团队配置页的
// 「编制」分区（/teams/$teamId/config），此处不再承载写操作。
export function TeamHumanMembersChrome({
  apiOptions,
  teamId
}: {
  apiOptions: ApiClientOptions;
  teamId: string;
}) {
  const membersQuery = useQuery({
    queryKey: ["team-members", teamId],
    queryFn: () => listTeamMembers(apiOptions, teamId)
  });
  const members = membersQuery.data ?? [];

  if (membersQuery.isLoading) {
    return <div aria-hidden className="flex h-8 w-24 animate-pulse rounded-full bg-card-soft" />;
  }

  if (members.length === 0) {
    return <p className="text-[12px] text-ink-3">暂无人类成员</p>;
  }

  const visible = members.slice(0, AVATAR_STACK_LIMIT);
  const overflow = Math.max(0, members.length - visible.length);

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
                className={cn("size-8 border-2 border-card shadow-none", index > 0 && "-ml-2")}
                user={memberIdentity(member)}
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
          <StatusPill tone="mute">只读</StatusPill>
        </div>
        <ul className="flex max-h-64 flex-col gap-2 overflow-y-auto">
          {members.map((member) => (
            <li key={member.membership_id} className="flex items-center justify-between gap-2">
              <UserIdentity size="sm" user={memberIdentity(member)} />
              <TeamRoleBadge role={member.role} />
            </li>
          ))}
        </ul>
      </PopoverContent>
    </Popover>
  );
}

function memberIdentity(member: TeamMember) {
  return {
    id: member.user_id,
    username: member.username,
    display_name: member.display_name,
    email: member.email,
    avatar: member.avatar,
    status: member.account_status || "active"
  };
}

/** 供头卡 meta 使用：人类人数由头像栈表达，此处仅拼数字员工与能力。 */
export function teamHeaderMetaLabel(input: {
  capabilityCount: number;
  digitalEmployeeCount: number;
}) {
  return `${input.digitalEmployeeCount} 数字员工 · ${input.capabilityCount} 能力绑定`;
}

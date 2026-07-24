import { Link } from "@tanstack/react-router";
import {
  UserIdentity,
  UserIdentityAvatar,
  type UserIdentityData,
} from "@/components/superteam/user-identity";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import type { ProjectMember } from "@/lib/api/projects";
import { cn } from "@/lib/utils";

const STACK_LIMIT = 5;

type ProjectOwnerAvatarStackProps = {
  owners: ProjectMember[];
  principalNamesById?: ReadonlyMap<string, string>;
  usersById?: ReadonlyMap<string, UserIdentityData>;
};

function ownerIdentity(
  owner: ProjectMember,
  principalNamesById?: ReadonlyMap<string, string>,
  usersById?: ReadonlyMap<string, UserIdentityData>,
): UserIdentityData {
  const fromUsers = usersById?.get(owner.principal_id);
  const principalName = principalNamesById?.get(owner.principal_id)?.trim();
  if (fromUsers) {
    return {
      ...fromUsers,
      display_name:
        owner.display_name_snapshot?.trim() ||
        fromUsers.display_name?.trim() ||
        principalName ||
        fromUsers.username?.trim() ||
        "负责人",
    };
  }
  const name =
    owner.display_name_snapshot?.trim() || principalName || "负责人";
  return {
    id: owner.principal_id,
    display_name: name,
    status: owner.status || "active",
    username: name,
  };
}

function ownerDirectoryLink(owner: ProjectMember): {
  label: string;
  to: "/users" | "/employees/$employeeId";
  params?: { employeeId: string };
} | null {
  if (owner.principal_type === "digital_employee") {
    return {
      label: "在数字员工中查看",
      to: "/employees/$employeeId",
      params: { employeeId: owner.principal_id },
    };
  }
  if (owner.principal_type === "human_user") {
    return { label: "在用户管理中查看", to: "/users" };
  }
  return null;
}

/** 项目负责人头像层叠：悬停看姓名，点击展开身份摘要。 */
export function ProjectOwnerAvatarStack({
  owners,
  principalNamesById,
  usersById,
}: ProjectOwnerAvatarStackProps) {
  if (owners.length === 0) {
    return null;
  }

  const visible = owners.slice(0, STACK_LIMIT);
  const overflow = owners.length - visible.length;

  return (
    <div
      className="mt-1.5 flex flex-wrap items-center gap-2"
      data-testid="project-owner-avatars"
    >
      <span className="text-xs text-v3-ink-3">负责人</span>
      <span className="flex items-center">
        {visible.map((owner, index) => {
          const user = ownerIdentity(owner, principalNamesById, usersById);
          const label = user.display_name || user.username || "负责人";
          const directoryLink = ownerDirectoryLink(owner);
          return (
            <Popover key={owner.id}>
              <Tooltip>
                <TooltipTrigger asChild>
                  <PopoverTrigger asChild>
                    <button
                      aria-label={`负责人 ${label}`}
                      className={cn(
                        "rounded-full outline-none transition-transform hover:z-10 hover:scale-105 focus-visible:ring-2 focus-visible:ring-v3-brand/60",
                        index > 0 && "-ml-2",
                      )}
                      type="button"
                    >
                      <UserIdentityAvatar
                        className="size-7 border-2 border-v3-card shadow-none"
                        user={user}
                      />
                    </button>
                  </PopoverTrigger>
                </TooltipTrigger>
                <TooltipContent side="bottom">{label}</TooltipContent>
              </Tooltip>
              <PopoverContent
                align="start"
                className="w-64 rounded-[14px] border-v3-line p-3 shadow-v3"
              >
                <UserIdentity showSecondary size="sm" user={user} />
                {directoryLink ? (
                  directoryLink.params ? (
                    <Link
                      className="mt-2 inline-flex text-[12px] font-semibold text-v3-brand hover:underline"
                      params={directoryLink.params}
                      to={directoryLink.to}
                    >
                      {directoryLink.label}
                    </Link>
                  ) : (
                    <Link
                      className="mt-2 inline-flex text-[12px] font-semibold text-v3-brand hover:underline"
                      to={directoryLink.to}
                    >
                      {directoryLink.label}
                    </Link>
                  )
                ) : null}
              </PopoverContent>
            </Popover>
          );
        })}
        {overflow > 0 ? (
          <span
            aria-label={`还有 ${overflow} 位负责人`}
            className="-ml-2 grid size-7 place-items-center rounded-full border-2 border-v3-card bg-v3-card-soft text-[10px] font-bold text-v3-ink-2"
          >
            +{overflow}
          </span>
        ) : null}
      </span>
    </div>
  );
}

import { useMemo } from "react";
import { useQueries } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Bot, Building2, ChevronRight, Users } from "lucide-react";
import { DynamicIcon, iconNames } from "lucide-react/dynamic";
import {
  getTeamDisplayConfig,
  type TeamDisplayMetadata,
} from "@/components/superteam/team-icon-tile";
import {
  IconTile,
  SoftCard,
  StatusPill,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  WorkSurface,
  type V3Tone,
} from "@/components/superteam";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  UserIdentityAvatar,
  getUserIdentityLabel,
  type UserIdentityData,
} from "@/components/superteam/user-identity";
import { EmployeeAvatar } from "@/features/employees/avatar";
import { employeeAvatarAsset } from "@/features/employees/avatar-library";
import { cn } from "@/lib/utils";
import { listDigitalEmployees } from "@/lib/api/employees";
import type { DigitalEmployee } from "@/lib/api/employees";
import type { TeamListItem } from "@/lib/api/teams";

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const MAX_VISIBLE_EMPLOYEE_AVATARS = 6;

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type TeamCardGridProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  highlightedTeamId?: string;
  isError?: boolean;
  isLoading?: boolean;
  teams: TeamListItem[];
};

// ---------------------------------------------------------------------------
// Summary Stats Bar
// ---------------------------------------------------------------------------

function SummaryStats({
  teams,
  totalDigitalEmployees,
  visibleDigitalEmployees,
}: {
  teams: TeamListItem[];
  totalDigitalEmployees: number;
  visibleDigitalEmployees: number;
}) {
  return (
    <div className="mb-6 flex flex-wrap items-center justify-center gap-3">
      <StatusPill tone="info" showDot={false}>
        <Building2 className="size-3.5" />
        {teams.length} 个团队
      </StatusPill>
      <StatusPill tone="ok" showDot={false}>
        <Users className="size-3.5" />
        {totalDigitalEmployees} 位 agent
      </StatusPill>
      <StatusPill tone="mute" showDot={false}>
        {visibleDigitalEmployees} 位代表成员展示中
      </StatusPill>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Digital Employee Avatar (placeholder – real avatars coming soon)
// ---------------------------------------------------------------------------

function DigitalEmployeeAvatar({
  className,
  employee,
}: {
  className?: string;
  employee: DigitalEmployee;
}) {
  const asset = employeeAvatarAsset(employee);

  return (
    <div
      aria-label={`${employee.name} 的头像`}
      className={cn("relative shrink-0 rounded-full", className)}
      title={`${employee.name} · ${employee.role}`}
    >
      <EmployeeAvatar asset={asset} name={employee.name} size="sm" />
      {/* AI badge */}
      <span className="absolute -bottom-1 left-1/2 z-10 -translate-x-1/2 rounded-sm border border-v3-card bg-v3-brand px-1 py-px text-[9px] font-bold leading-none text-white shadow-v3">
        AI
      </span>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Avatar Stack (overlapping row of avatars)
// ---------------------------------------------------------------------------

function DigitalEmployeeAvatarStack({
  employees,
  total,
}: {
  employees: DigitalEmployee[];
  total: number;
}) {
  const visible = employees.slice(0, MAX_VISIBLE_EMPLOYEE_AVATARS);
  const overflow = total - visible.length;

  if (total === 0) {
    return (
      <span className="text-xs text-muted-foreground">暂无数字员工</span>
    );
  }

  return (
    <div className="flex items-center">
      <div className="flex -space-x-2">
        {visible.map((employee) => (
          <DigitalEmployeeAvatar
            className="ring-2 ring-background"
            employee={employee}
            key={employee.id}
          />
        ))}
      </div>
      {overflow > 0 ? (
        <span className="ml-2 text-sm font-medium text-muted-foreground">
          +{overflow}
        </span>
      ) : null}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Placeholder avatar stack (shown when data hasn't loaded yet)
// ---------------------------------------------------------------------------

function PlaceholderAvatarStack({ count }: { count: number }) {
  if (count === 0) {
    return (
      <span className="text-xs text-muted-foreground">暂无数字员工</span>
    );
  }

  const visible = Math.min(count, MAX_VISIBLE_EMPLOYEE_AVATARS);
  const overflow = count - visible;

  return (
    <div className="flex items-center">
      <div className="flex -space-x-2">
        {Array.from({ length: visible }).map((_, i) => (
          <div
            className="flex size-9 items-center justify-center rounded-full border border-v3-line bg-v3-card-soft ring-2 ring-v3-card"
            // eslint-disable-next-line react/no-array-index-key
            key={i}
          >
            <Bot className="size-4 text-v3-ink-3" />
          </div>
        ))}
      </div>
      {overflow > 0 ? (
        <span className="ml-2 text-sm font-medium text-muted-foreground">
          +{overflow}
        </span>
      ) : null}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Human Owner Section
// ---------------------------------------------------------------------------

function HumanOwnerSection({ team }: { team: TeamListItem }) {
  const owners = getOwnerIdentities(team);

  if (owners.length === 0) {
    return (
      <div className="flex items-center gap-3 py-2">
        <div className="flex size-10 items-center justify-center rounded-full border border-dashed border-v3-line-strong bg-v3-card-soft">
          <Users className="size-4 text-v3-ink-3" />
        </div>
        <span className="text-sm text-muted-foreground">未设置负责人</span>
      </div>
    );
  }

  if (owners.length === 1) {
    const owner = owners[0]!;
    const label = getUserIdentityLabel(owner);
    return (
      <div className="flex items-center gap-3 py-2">
        <UserIdentityAvatar className="size-10" user={owner} />
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-medium text-v3-ink">{label.primary}</div>
          <div className="truncate text-xs text-v3-ink-2">
            {label.secondary}
          </div>
        </div>
      </div>
    );
  }

  const MAX_VISIBLE = 3;
  const visibleOwners = owners.slice(0, MAX_VISIBLE);

  return (
    <div className="flex items-center gap-3 py-2">
      <TooltipProvider>
        <Tooltip delayDuration={300}>
          <TooltipTrigger asChild>
            <div className="flex cursor-help items-center">
              <div className="flex -space-x-2">
                {visibleOwners.map((owner) => (
                  <div
                    className="overflow-hidden rounded-full ring-2 ring-background"
                    key={owner.id}
                  >
                    <UserIdentityAvatar className="size-8" user={owner} />
                  </div>
                ))}
              </div>
            </div>
          </TooltipTrigger>
          <TooltipContent
            className="p-3 bg-popover text-popover-foreground border border-border shadow-md"
            side="bottom"
          >
            <div className="mb-2 text-xs font-semibold text-muted-foreground">
              联席负责人 ({owners.length})
            </div>
            <div className="flex flex-col gap-2.5">
              {owners.map((o) => {
                const label = getUserIdentityLabel(o);
                return (
                  <div className="flex items-center gap-2" key={o.id}>
                    <UserIdentityAvatar className="size-6" user={o} />
                    <div className="flex flex-col">
                      <span className="text-xs font-medium leading-none text-foreground">
                        {label.primary}
                      </span>
                      <span className="mt-1 text-[10px] leading-none text-muted-foreground">
                        {label.secondary}
                      </span>
                    </div>
                  </div>
                );
              })}
            </div>
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>

      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium text-v3-ink">
          {owners
            .slice(0, 2)
            .map((o) => getUserIdentityLabel(o).primary)
            .join("、")}
          {owners.length > 2 ? ` 等 ${owners.length} 人` : ""}
        </div>
        <div className="truncate text-xs text-v3-ink-2">
          联席负责人
        </div>
      </div>
    </div>
  );
}

function getOwnerIdentity(team: TeamListItem): UserIdentityData | undefined {
  const ownerID = team.human_owner_user_ids?.[0];
  if (ownerID) {
    return {
      id: ownerID,
      status: "active",
    };
  }

  return undefined;
}

function getOwnerIdentities(team: TeamListItem): UserIdentityData[] {
  if (team.human_owners && team.human_owners.length > 0) {
    return team.human_owners.map((owner) => ({
      avatar: owner.avatar,
      display_name: owner.display_name,
      email: owner.email,
      id: owner.user_id,
      status: owner.status,
      username: owner.username,
    }));
  }

  const singleOwner = getOwnerIdentity(team);
  if (singleOwner) {
    return [singleOwner];
  }

  return [];
}

// ---------------------------------------------------------------------------
// Single Team Card
// ---------------------------------------------------------------------------

function TeamCard({
  digitalEmployees,
  index,
  isHighlighted,
  isLoadingEmployees,
  team,
}: {
  digitalEmployees: DigitalEmployee[] | undefined;
  index: number;
  isHighlighted: boolean;
  isLoadingEmployees: boolean;
  team: TeamListItem;
}) {
  const metadata = (team.metadata ?? {}) as TeamDisplayMetadata;
  const displayConfig = getTeamDisplayConfig(metadata);
  const TeamIcon = displayConfig.Icon;
  const levelLabel = `L${index + 1}`;
  const visibleCount = digitalEmployees
    ? Math.min(digitalEmployees.length, MAX_VISIBLE_EMPLOYEE_AVATARS)
    : Math.min(team.digital_employee_count, MAX_VISIBLE_EMPLOYEE_AVATARS);
  const totalCount = digitalEmployees
    ? digitalEmployees.length
    : team.digital_employee_count;

  return (
    <SoftCard
      className={cn(
        "group flex min-h-[260px] flex-col overflow-hidden transition-[transform,box-shadow] duration-150 hover:-translate-y-0.5 hover:shadow-v3-pop",
        isHighlighted && "ring-2 ring-v3-brand",
      )}
    >
      {/* ── Header ─────────────────────────────────────────────── */}
      <div className="flex items-start justify-between gap-3 px-5 pt-5">
        <div className="flex items-center gap-3">
          <IconTile aria-label={displayConfig.label} role="img" tone={teamDisplayTone(displayConfig.tone)}>
            {displayConfig.dynamicName ? (
              <DynamicIcon
                aria-hidden="true"
                name={displayConfig.dynamicName as (typeof iconNames)[number]}
              />
            ) : (
              <TeamIcon aria-hidden="true" />
            )}
          </IconTile>
          <div className="min-w-0">
            <h3 className="truncate text-base font-bold leading-tight text-v3-ink">
              {team.name}
            </h3>
            <p className="mt-0.5 text-xs text-v3-ink-2">
              {team.digital_employee_count} 位数字员工
            </p>
          </div>
        </div>
        <StatusPill className="shrink-0 tabular-nums" tone="mute" showDot={false}>
          {levelLabel}
        </StatusPill>
      </div>

      {/* ── Human Owner ────────────────────────────────────────── */}
      <div className="px-5 pt-3">
        <HumanOwnerSection team={team} />
      </div>

      {/* ── Representative Digital Employees ────────────────────── */}
      <div className="mt-auto flex flex-col gap-2 px-5 pb-2 pt-3">
        <div className="flex items-center justify-between text-xs text-v3-ink-2">
          <span>代表成员</span>
          <span className="tabular-nums">
            显示 {visibleCount} / {totalCount}
          </span>
        </div>
        {isLoadingEmployees ? (
          <PlaceholderAvatarStack count={team.digital_employee_count} />
        ) : digitalEmployees ? (
          <DigitalEmployeeAvatarStack
            employees={digitalEmployees}
            total={totalCount}
          />
        ) : (
          <PlaceholderAvatarStack count={team.digital_employee_count} />
        )}
      </div>

      {/* ── Footer ─────────────────────────────────────────────── */}
      <Link
        className="flex items-center justify-between border-t border-v3-line bg-v3-card-soft px-5 py-3.5 text-sm font-bold text-v3-brand-deep transition-colors hover:bg-v3-brand-soft"
        params={{ teamId: team.id }}
        to="/teams/$teamId"
      >
        查看完整部门
        <ChevronRight className="size-4 transition-transform duration-300 group-hover:translate-x-1" />
      </Link>
    </SoftCard>
  );
}

function teamDisplayTone(tone: string): V3Tone {
  const toneMap: Record<string, V3Tone> = {
    blue: "brand",
    cyan: "info",
    neutral: "mute",
    teal: "ok",
    violet: "artifact",
  };

  return toneMap[tone] ?? "mute";
}

// ---------------------------------------------------------------------------
// Grid Component
// ---------------------------------------------------------------------------

export function TeamCardGrid({
  apiBaseUrl,
  fetcher,
  highlightedTeamId,
  isError,
  isLoading,
  teams,
}: TeamCardGridProps) {
  const apiOptions = useMemo(
    () => ({ baseUrl: apiBaseUrl, fetcher }),
    [apiBaseUrl, fetcher],
  );

  // Fetch digital employees per team
  const employeeQueries = useQueries({
    queries: teams.map((team) => ({
      queryKey: ["team-card-digital-employees", team.id],
      queryFn: () => listDigitalEmployees(apiOptions, { team_id: team.id }),
      staleTime: 5 * 60 * 1000,
    })),
  });

  // Build a lookup: teamId → DigitalEmployee[]
  const employeesByTeam = useMemo(() => {
    const map = new Map<string, DigitalEmployee[]>();
    teams.forEach((team, i) => {
      const query = employeeQueries[i];
      if (query?.data) {
        map.set(team.id, query.data);
      }
    });
    return map;
  }, [teams, employeeQueries]);

  // Summary statistics
  const totalDigitalEmployees = teams.reduce(
    (sum, t) => sum + t.digital_employee_count,
    0,
  );
  const visibleDigitalEmployees = teams.reduce((sum, t) => {
    const employees = employeesByTeam.get(t.id);
    return (
      sum +
      Math.min(
        employees?.length ?? t.digital_employee_count,
        MAX_VISIBLE_EMPLOYEE_AVATARS,
      )
    );
  }, 0);

  // ── Loading / Error / Empty states ──────────────────────────
  if (isLoading) {
    return (
      <WorkSurface>
        <V3LoadingState label="团队列表加载中" />
      </WorkSurface>
    );
  }

  if (isError) {
    return (
      <WorkSurface>
        <V3ErrorState title="团队列表加载失败" />
      </WorkSurface>
    );
  }

  if (teams.length === 0) {
    return (
      <WorkSurface>
        <V3EmptyState icon={<Building2 />} title="暂无团队" />
      </WorkSurface>
    );
  }

  return (
    <div>
      <SummaryStats
        teams={teams}
        totalDigitalEmployees={totalDigitalEmployees}
        visibleDigitalEmployees={visibleDigitalEmployees}
      />

      <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        {teams.map((team, index) => (
          <TeamCard
            digitalEmployees={employeesByTeam.get(team.id)}
            index={index}
            isHighlighted={team.id === highlightedTeamId}
            isLoadingEmployees={
              employeeQueries[index]?.isLoading ?? false
            }
            key={team.id}
            team={team}
          />
        ))}
      </div>
    </div>
  );
}

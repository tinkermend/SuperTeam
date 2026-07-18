import { Link } from "@tanstack/react-router";
import { Building2, ChevronRight, ShieldCheck, Users } from "lucide-react";
import {
  getTeamDisplayConfig,
  type TeamDisplayMetadata,
} from "@/components/superteam/team-icon-tile";
import {
  SoftCard,
  StatusPill,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  WorkSurface,
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
import { cn } from "@/lib/utils";
import type { TeamListItem } from "@/lib/api/teams";
import { governanceStatusLabel, teamStatusLabel } from "@/lib/status-labels";

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const governanceToneClass = {
  active: "text-v3-ok",
  draft_pending: "text-v3-warn",
  needs_update: "text-v3-warn",
  not_configured: "text-v3-mute",
} as const;

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type TeamCardGridProps = {
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
  totalCapabilities,
}: {
  teams: TeamListItem[];
  totalDigitalEmployees: number;
  totalCapabilities: number;
}) {
  return (
    <div className="mb-6 flex flex-wrap items-center justify-center gap-3">
      <StatusPill tone="info" showDot={false}>
        <Building2 className="size-3.5" />
        {teams.length} 个团队
      </StatusPill>
      <StatusPill tone="ok" showDot={false}>
        <Users className="size-3.5" />
        {totalDigitalEmployees} 名数字员工
      </StatusPill>
      <StatusPill tone="mute" showDot={false}>
        <ShieldCheck className="size-3.5" />
        {totalCapabilities} 项能力绑定
      </StatusPill>
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
  isHighlighted,
  team,
}: {
  isHighlighted: boolean;
  team: TeamListItem;
}) {
  const metadata = (team.metadata ?? {}) as TeamDisplayMetadata;
  const displayConfig = getTeamDisplayConfig(metadata);

  return (
    <Link
      aria-label={`查看 ${team.name} 团队详情`}
      className="group block h-full rounded-v3-card focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-v3-brand focus-visible:ring-offset-2 focus-visible:ring-offset-v3-bg"
      params={{ teamId: team.id }}
      to="/teams/$teamId"
    >
      <SoftCard
        interactive
        className={cn(
          "flex h-full min-h-[300px] flex-col overflow-hidden border-v3-line bg-v3-card p-0",
          isHighlighted && "border-v3-brand ring-2 ring-v3-brand shadow-v3-pop",
        )}
      >
        <div className="flex min-w-0 items-start justify-between gap-4 px-5 pb-4 pt-5">
          <div className="flex min-w-0 flex-1 items-start gap-4">
            <div
              aria-label={displayConfig.label}
              className="flex size-[4.75rem] shrink-0 items-center justify-center rounded-[18px] border border-v3-line bg-v3-card-soft shadow-sm"
              role="img"
            >
              <img
                alt=""
                className="size-[4.5rem] object-contain"
                decoding="async"
                height={72}
                loading="lazy"
                src={displayConfig.imageSrc}
                width={72}
              />
            </div>
            <div className="min-w-0 flex-1 pt-0.5">
              <div className="flex min-w-0 items-start justify-between gap-3">
                <h3
                  className="line-clamp-2 text-[17px] font-bold leading-snug text-v3-ink"
                  title={team.name}
                >
                  {team.name}
                </h3>
                <StatusPill className="shrink-0" tone="ok">
                  {teamStatusLabel(team.status)}
                </StatusPill>
              </div>
              <p className="mt-1 truncate text-xs font-medium text-v3-ink-2">
                {displayConfig.label}
              </p>
            </div>
          </div>
        </div>

        <div className="px-5">
          <div className="rounded-v3-inner border border-v3-line bg-v3-card-inner px-3.5 py-3">
            <p className="text-[11px] font-semibold tracking-[0.08em] text-v3-ink-3">
              团队说明
            </p>
            <p
              className="mt-1 line-clamp-2 min-h-10 text-sm leading-5 text-v3-ink-2"
              title={team.description || "暂未补充团队说明"}
            >
              {team.description || "暂未补充团队说明"}
            </p>
          </div>
        </div>

        <div className="mx-5 mt-4 grid grid-cols-2 gap-3 border-y border-v3-line py-3.5 text-sm">
          <div
            className="flex min-w-0 items-center gap-2 text-v3-ink-2"
          >
            <Users className="size-4 shrink-0 text-v3-ink-3" />
            <span className="truncate">
              <span className="font-bold tabular-nums text-v3-ink">
                {team.digital_employee_count}
              </span>{" "}
              名数字员工
            </span>
          </div>
          <div className="flex min-w-0 items-center gap-2 text-v3-ink-2">
            <ShieldCheck
              className={cn(
                "size-4 shrink-0",
                governanceToneClass[team.governance_status],
              )}
            />
            <span className="truncate">
              {governanceStatusLabel(team.governance_status)}
            </span>
          </div>
        </div>

        <div className="mt-auto flex min-w-0 items-center gap-3 px-5 py-4">
          <div className="min-w-0 flex-1">
            <p className="mb-1 text-[11px] font-semibold tracking-[0.08em] text-v3-ink-3">
              团队负责人
            </p>
            <HumanOwnerSection team={team} />
          </div>
          <span className="flex shrink-0 items-center gap-1 text-sm font-bold text-v3-brand-deep">
            进入团队
            <ChevronRight className="size-4 transition-transform duration-200 group-hover:translate-x-1" />
          </span>
        </div>
      </SoftCard>
    </Link>
  );
}

// ---------------------------------------------------------------------------
// Grid Component
// ---------------------------------------------------------------------------

export function TeamCardGrid({
  highlightedTeamId,
  isError,
  isLoading,
  teams,
}: TeamCardGridProps) {
  const totalDigitalEmployees = teams.reduce(
    (sum, t) => sum + t.digital_employee_count,
    0,
  );
  const totalCapabilities = teams.reduce(
    (sum, t) => sum + t.capability_count,
    0,
  );

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
        totalCapabilities={totalCapabilities}
      />

      <div className="@container">
        <div
          className="grid grid-cols-1 gap-5 @md:grid-cols-2 @5xl:grid-cols-3"
          data-team-card-grid
        >
          {teams.map((team) => (
            <TeamCard
              isHighlighted={team.id === highlightedTeamId}
              key={team.id}
              team={team}
            />
          ))}
        </div>
      </div>
    </div>
  );
}

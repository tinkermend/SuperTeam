import { Link } from "@tanstack/react-router";
import { Building2, ChevronRight, ShieldCheck, Users } from "lucide-react";
import {
  getTeamDisplayConfig,
  type TeamDisplayMetadata,
} from "@/components/superteam/team-icon-tile";
import {
  AvatarStack,
  CardGridSkeleton,
  EmptyNoData,
  EmptyNoMatch,
  EntityCard,
  ErrorState,
  MetricCard,
  MetricGrid,
  StatusPill,
  WorkSurface,
  type Tone,
} from "@/components/superteam";
import {
  UserIdentityAvatar,
  getUserIdentityLabel,
  type UserIdentityData,
} from "@/components/superteam/user-identity";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import type { TeamListItem } from "@/lib/api/teams";
import { governanceStatusLabel, teamStatusLabel } from "@/lib/status-labels";

type TeamCardGridProps = {
  highlightedTeamId?: string;
  /** 当前是否带筛选/搜索（用于区分无数据 vs 无匹配） */
  isFiltered?: boolean;
  isError?: boolean;
  isLoading?: boolean;
  teams: TeamListItem[];
};

const statusTone: Record<string, Tone> = {
  active: "ok",
  disabled: "mute",
  archived: "mute",
};

const governanceTone: Record<string, Tone> = {
  active: "ok",
  draft_pending: "warn",
  needs_update: "warn",
  not_configured: "mute",
};

function getOwnerIdentity(team: TeamListItem): UserIdentityData | undefined {
  const ownerID = team.human_owner_user_ids?.[0];
  if (ownerID) {
    return { id: ownerID, status: "active" };
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
  const single = getOwnerIdentity(team);
  return single ? [single] : [];
}

function TeamOwnersFooter({ team }: { team: TeamListItem }) {
  const owners = getOwnerIdentities(team);
  if (owners.length === 0) {
    return (
      <div className="flex items-center justify-between gap-2">
        <span className="text-[12px] text-ink-3">未设置负责人</span>
        <span className="inline-flex items-center gap-0.5 text-[13px] font-bold text-brand-deep">
          进入团队
          <ChevronRight className="size-4" aria-hidden />
        </span>
      </div>
    );
  }
  if (owners.length === 1) {
    const label = getUserIdentityLabel(owners[0]!);
    return (
      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <UserIdentityAvatar className="size-8" user={owners[0]!} />
          <div className="min-w-0">
            <p className="truncate text-[13px] font-semibold text-ink">{label.primary}</p>
            <p className="truncate text-[11px] text-ink-3">{label.secondary}</p>
          </div>
        </div>
        <span className="inline-flex shrink-0 items-center gap-0.5 text-[13px] font-bold text-brand-deep">
          进入团队
          <ChevronRight className="size-4" aria-hidden />
        </span>
      </div>
    );
  }
  return (
    <div className="flex items-center justify-between gap-3">
      <TooltipProvider>
        <Tooltip delayDuration={300}>
          <TooltipTrigger asChild>
            <div className="flex min-w-0 cursor-help items-center gap-2">
              <AvatarStack
                max={3}
                items={owners.map((o) => ({
                  id: o.id,
                  name: getUserIdentityLabel(o).primary,
                }))}
              />
              <span className="truncate text-[12px] text-ink-2">
                {owners.length} 位负责人
              </span>
            </div>
          </TooltipTrigger>
          <TooltipContent side="bottom" className="max-w-xs p-3">
            <p className="mb-2 text-xs font-semibold text-ink-3">
              联席负责人 ({owners.length})
            </p>
            <ul className="space-y-1.5">
              {owners.map((o) => {
                const label = getUserIdentityLabel(o);
                return (
                  <li key={o.id} className="flex items-center gap-2">
                    <UserIdentityAvatar className="size-6" user={o} />
                    <span className="text-xs font-medium text-ink">{label.primary}</span>
                  </li>
                );
              })}
            </ul>
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
      <span className="inline-flex shrink-0 items-center gap-0.5 text-[13px] font-bold text-brand-deep">
        进入团队
        <ChevronRight className="size-4" aria-hidden />
      </span>
    </div>
  );
}

function TeamCard({
  team,
  isHighlighted,
}: {
  team: TeamListItem;
  isHighlighted?: boolean;
}) {
  const metadata = (team.metadata ?? {}) as TeamDisplayMetadata;
  const displayConfig = getTeamDisplayConfig(metadata);
  const description = team.description?.trim() || "暂未补充团队说明";

  return (
    <Link
      to="/teams/$teamId"
      params={{ teamId: team.id }}
      aria-label={`查看 ${team.name} 团队详情`}
      className="group block min-w-0 rounded-card outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2"
      data-team-card
      data-highlighted={isHighlighted ? "true" : undefined}
    >
      <EntityCard
        className="h-full transition-shadow group-hover:border-line-strong group-hover:shadow-md"
        interactive
        selected={isHighlighted}
        leading={
          <span
            role="img"
            aria-label={displayConfig.label}
            className="grid size-12 place-items-center overflow-hidden rounded-[15px] border border-line bg-card-soft"
          >
            <img
              alt=""
              className="size-10 object-contain"
              decoding="async"
              height={40}
              loading="lazy"
              src={displayConfig.imageSrc}
              width={40}
            />
          </span>
        }
        title={team.name}
        subtitle={displayConfig.label}
        status={
          <StatusPill tone={statusTone[team.status] ?? "mute"}>
            {teamStatusLabel(team.status)}
          </StatusPill>
        }
        facts={[
          {
            key: "desc",
            label: "说明",
            value: description,
          },
          {
            key: "employees",
            label: "数字员工",
            value: `${team.digital_employee_count} 名`,
          },
          {
            key: "gov",
            label: "治理",
            value: (
              <span className="inline-flex items-center gap-1">
                <ShieldCheck
                  className="size-3.5 shrink-0"
                  data-tone={governanceTone[team.governance_status]}
                />
                {governanceStatusLabel(team.governance_status)}
              </span>
            ),
          },
          {
            key: "cap",
            label: "能力绑定",
            value: `${team.capability_count} 项`,
          },
        ]}
        footer={<TeamOwnersFooter team={team} />}
      />
    </Link>
  );
}

export function TeamCardGrid({
  highlightedTeamId,
  isFiltered = false,
  isError,
  isLoading,
  teams,
}: TeamCardGridProps) {
  const totalDigitalEmployees = teams.reduce(
    (sum, t) => sum + t.digital_employee_count,
    0,
  );
  const totalCapabilities = teams.reduce((sum, t) => sum + t.capability_count, 0);

  if (isLoading) {
    return <CardGridSkeleton count={6} />;
  }

  if (isError) {
    return (
      <WorkSurface>
        <ErrorState title="团队列表加载失败" />
      </WorkSurface>
    );
  }

  if (teams.length === 0) {
    return (
      <WorkSurface>
        {isFiltered ? (
          <EmptyNoMatch
            title="无匹配团队"
            description="当前筛选或搜索条件下没有团队，可调整条件或重置筛选。"
          />
        ) : (
          <EmptyNoData
            icon={<Building2 />}
            title="暂无团队"
            description="创建第一个团队，配置负责人与协作边界。"
          />
        )}
      </WorkSurface>
    );
  }

  return (
    <div className="flex min-w-0 flex-col gap-5">
      <MetricGrid aria-label="团队概览指标">
        <MetricCard
          label="团队总数"
          value={String(teams.length)}
          icon={<Building2 />}
          iconTone="info"
        />
        <MetricCard
          label="数字员工"
          value={String(totalDigitalEmployees)}
          icon={<Users />}
          iconTone="ok"
        />
        <MetricCard
          label="能力绑定"
          value={String(totalCapabilities)}
          icon={<ShieldCheck />}
          iconTone="mute"
        />
      </MetricGrid>

      <div className="@container">
        <div
          className="grid grid-cols-1 gap-4 @md:grid-cols-2 @5xl:grid-cols-3"
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

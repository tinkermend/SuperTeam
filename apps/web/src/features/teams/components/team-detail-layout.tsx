import { Settings } from "lucide-react";
import { Link } from "@tanstack/react-router";
import { TeamIconTile, type TeamDisplayMetadata } from "@/components/superteam/team-icon-tile";
import { StatusPill, Button } from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import type { TeamOverview, TeamStatus } from "@/lib/api/teams";
import { teamStatusLabel } from "@/lib/status-labels";
import { TeamCapabilitiesSummary } from "./team-capabilities-summary";
import { TeamConstitutionSummary } from "./team-constitution-summary";
import { TeamHumanMembersChrome, teamHeaderMetaLabel } from "./team-human-members";
import { TeamRecentChanges } from "./team-recent-changes";
import { TeamRosterPanel } from "./team-roster-panel";

// 团队生命周期收敛：存活团队唯一状态 active，退出只有删除一条路（软删+审计）。
function TeamStatusPill({ status }: { status: TeamStatus }) {
  return <StatusPill tone="ok">{teamStatusLabel(status)}</StatusPill>;
}

type TeamDetailLayoutProps = {
  apiOptions: ApiClientOptions;
  overview: TeamOverview;
};

// 团队详情是观察面：只读。所有写操作集中在 /teams/$teamId/config
// （与员工、项目的详情/配置分离一致），这样权限门禁、离开拦截与影响面预览只需一处实现。
export function TeamDetailLayout({ apiOptions, overview }: TeamDetailLayoutProps) {
  const team = overview.team;
  const canConfigure =
    overview.allowed_actions.includes("team.update") ||
    overview.allowed_actions.includes("team.member.add") ||
    overview.allowed_actions.includes("team.governance.edit") ||
    overview.allowed_actions.includes("team.capability.bind") ||
    overview.allowed_actions.includes("team.capability.manage");

  return (
    <div className="flex flex-col gap-5">
      <div className="rounded-card bg-card p-5 shadow-card">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div className="flex min-w-0 flex-1 items-start gap-3">
            <TeamIconTile
              className="size-14 rounded-[18px]"
              metadata={(team.metadata ?? {}) as TeamDisplayMetadata}
            />
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-2">
                <p className="text-[28px] font-extrabold tracking-tight text-ink">
                  {team.name}
                </p>
                <TeamStatusPill status={team.status} />
                {overview.pending_item_count > 0 ? (
                  <StatusPill tone="warn">待审批 {overview.pending_item_count}</StatusPill>
                ) : null}
              </div>
              <p className="mt-1 text-[13px] text-ink-2">
                {team.slug} / 负责人 {teamOwnerLabel(team)}
              </p>
              <div className="mt-3 flex flex-wrap items-center gap-3">
                <TeamHumanMembersChrome apiOptions={apiOptions} teamId={team.id} />
                <p className="text-[12px] tabular-nums text-ink-3">
                  {teamHeaderMetaLabel({
                    capabilityCount: overview.capability_count,
                    digitalEmployeeCount: overview.digital_employee_count
                  })}
                </p>
              </div>
            </div>
          </div>

          <div className="flex shrink-0 flex-wrap items-center gap-2">
            {canConfigure ? (
              <Button asChild size="sm" type="button" variant="outline">
                <Link params={{ teamId: team.id }} to="/teams/$teamId/config">
                  <Settings data-icon="inline-start" />
                  配置团队
                </Link>
              </Button>
            ) : null}
          </div>
        </div>
      </div>

      <TeamRosterPanel
        allowedActions={overview.allowed_actions}
        apiBaseUrl={apiOptions.baseUrl}
        fetcher={apiOptions.fetcher}
        readOnly
        teamId={team.id}
      />

      <section aria-label="团队能力" id="team-section-capabilities">
        <TeamCapabilitiesSummary apiOptions={apiOptions} teamId={team.id} />
      </section>

      <section aria-label="团队治理" id="team-section-constitution">
        <TeamConstitutionSummary constitution={team.constitution} teamId={team.id} />
      </section>

      <TeamRecentChanges
        allowedActions={overview.allowed_actions}
        apiOptions={apiOptions}
        teamId={team.id}
      />
    </div>
  );
}

function teamOwnerLabel(team: TeamOverview["team"]) {
  if (team.human_owners && team.human_owners.length > 0) {
    const owner = team.human_owners[0];
    return owner.display_name || owner.username || owner.email || owner.user_id;
  }
  return team.human_owner_user_ids?.join(", ") || "未设置";
}

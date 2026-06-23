import { Archive, Plus, RotateCcw, ShieldCheck, UserPlus, UsersRound } from "lucide-react";
import { IconTile, StatusPill, V3Button, V3Tabs } from "@/components/superteam";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type { ApiClientOptions } from "@/lib/api/client";
import type { TeamOverview, TeamStatus } from "@/lib/api/teams";
import { TeamAuditTab } from "./team-audit-tab";
import { TeamCapabilitiesTab } from "./team-capabilities-tab";
import { TeamGovernanceTab } from "./team-governance-tab";
import { TeamLendingTab } from "./team-lending-tab";
import { TeamOverviewTab } from "./team-overview-tab";

function TeamStatusPill({ status }: { status: TeamStatus }) {
  const label: Record<TeamStatus, string> = {
    active: "活跃",
    archived: "已归档",
    disabled: "已禁用",
  };
  const tone: Record<TeamStatus, "ok" | "mute" | "warn"> = {
    active: "ok",
    archived: "mute",
    disabled: "warn",
  };

  return (
    <StatusPill tone={tone[status]}>
      {label[status]}
    </StatusPill>
  );
}

type TeamDetailLayoutProps = {
  apiOptions: ApiClientOptions;
  currentRevision?: TeamOverview["current_revision"];
  onArchiveTeam?: () => void;
  onDisableTeam?: () => void;
  onRestoreTeam?: () => void;
  overview: TeamOverview;
};

export function TeamDetailLayout({
  apiOptions,
  currentRevision,
  onArchiveTeam,
  onDisableTeam,
  onRestoreTeam,
  overview,
}: TeamDetailLayoutProps) {
  const team = overview.team;
  const isActive = team.status === "active";
  const canAddMember = isActive && overview.allowed_actions.includes("team.member.add");
  const canCreateGovernance = isActive && overview.allowed_actions.includes("team.governance.edit");
  const canApproveGovernance = isActive && overview.allowed_actions.includes("team.governance.approve");
  const canEditLending = isActive && overview.allowed_actions.includes("team.lending.policy.edit");
  const canDecideLending = isActive && overview.allowed_actions.includes("team.lending.request.decide");
  const canDisable = isActive && overview.allowed_actions.includes("team.disable");
  const canArchive = team.status !== "archived" && overview.allowed_actions.includes("team.archive");
  const canRestore = team.status !== "active" && overview.allowed_actions.includes("team.restore");

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-col gap-4 rounded-v3-card bg-v3-card p-5 shadow-v3 lg:flex-row lg:items-start lg:justify-between">
        <div className="flex min-w-0 items-start gap-3">
          <IconTile tone="info" size="lg">
            <UsersRound />
          </IconTile>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="text-[28px] font-extrabold tracking-tight text-v3-ink">
                {team.name}
              </h1>
              <TeamStatusPill status={team.status} />
            </div>
            <p className="mt-1 text-[13px] text-v3-ink-2">
              {team.slug} / 负责人 {teamOwnerLabel(team)}
            </p>
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          {canAddMember ? (
            <V3Button disabled size="sm" variant="outline">
              <UserPlus data-icon="inline-start" />
              添加成员
            </V3Button>
          ) : null}
          {canCreateGovernance ? (
            <V3Button disabled size="sm">
              <Plus data-icon="inline-start" />
              创建治理草案
            </V3Button>
          ) : null}
          {canDisable ? (
            <V3Button onClick={onDisableTeam} size="sm" variant="outline">
              <ShieldCheck data-icon="inline-start" />
              禁用团队
            </V3Button>
          ) : null}
          {canArchive ? (
            <V3Button onClick={onArchiveTeam} size="sm" variant="outline">
              <Archive data-icon="inline-start" />
              归档团队
            </V3Button>
          ) : null}
          {canRestore ? (
            <V3Button onClick={onRestoreTeam} size="sm" variant="outline">
              <RotateCcw data-icon="inline-start" />
              恢复团队
            </V3Button>
          ) : null}
        </div>
      </div>

      <Tabs defaultValue="overview">
        <V3Tabs className="mb-4">
          <TabsList className="h-auto bg-transparent p-0 text-v3-ink-2">
            <TabsTrigger
              className="rounded-[10px] px-4 py-2 text-[13px] font-semibold data-[state=active]:bg-v3-brand-soft data-[state=active]:text-v3-brand-deep data-[state=active]:shadow-none"
              value="overview"
            >
              概览
            </TabsTrigger>
            <TabsTrigger
              className="rounded-[10px] px-4 py-2 text-[13px] font-semibold data-[state=active]:bg-v3-brand-soft data-[state=active]:text-v3-brand-deep data-[state=active]:shadow-none"
              value="capabilities"
            >
              能力与知识
            </TabsTrigger>
            <TabsTrigger
              className="rounded-[10px] px-4 py-2 text-[13px] font-semibold data-[state=active]:bg-v3-brand-soft data-[state=active]:text-v3-brand-deep data-[state=active]:shadow-none"
              value="governance"
            >
              治理策略
            </TabsTrigger>
            <TabsTrigger
              className="rounded-[10px] px-4 py-2 text-[13px] font-semibold data-[state=active]:bg-v3-brand-soft data-[state=active]:text-v3-brand-deep data-[state=active]:shadow-none"
              value="lending"
            >
              借调
            </TabsTrigger>
            <TabsTrigger
              className="rounded-[10px] px-4 py-2 text-[13px] font-semibold data-[state=active]:bg-v3-brand-soft data-[state=active]:text-v3-brand-deep data-[state=active]:shadow-none"
              value="audit"
            >
              审计记录
            </TabsTrigger>
          </TabsList>
        </V3Tabs>
        <TabsContent className="mt-0" value="overview">
          <TeamOverviewTab
            allowedActions={overview.allowed_actions}
            apiBaseUrl={apiOptions.baseUrl}
            fetcher={apiOptions.fetcher}
            overview={overview}
            teamId={team.id}
          />
        </TabsContent>
        <TabsContent className="mt-0" value="capabilities">
          <TeamCapabilitiesTab
            apiOptions={apiOptions}
            canEdit={canCreateGovernance}
            currentRevision={currentRevision ?? overview.current_revision}
            teamId={team.id}
          />
        </TabsContent>
        <TabsContent className="mt-0" value="governance">
          <TeamGovernanceTab
            apiOptions={apiOptions}
            canApprove={canApproveGovernance}
            canEdit={canCreateGovernance}
            currentRevision={currentRevision ?? overview.current_revision}
            teamId={team.id}
          />
        </TabsContent>
        <TabsContent className="mt-0" value="lending">
          <TeamLendingTab
            apiOptions={apiOptions}
            canDecide={canDecideLending}
            canEdit={canEditLending}
            teamId={team.id}
          />
        </TabsContent>
        <TabsContent className="mt-0" value="audit">
          <TeamAuditTab apiBaseUrl={apiOptions.baseUrl} fetcher={apiOptions.fetcher} teamId={team.id} />
        </TabsContent>
      </Tabs>
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

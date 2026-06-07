import { Archive, Plus, RotateCcw, ShieldCheck, UserPlus, UsersRound } from "lucide-react";
import { LiquidTabsList, LiquidTabsTrigger, SemanticIconTile } from "@/components/superteam";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent } from "@/components/ui/tabs";
import type { ApiClientOptions } from "@/lib/api/client";
import type { TeamOverview, TeamStatus } from "@/lib/api/teams";
import { TeamAuditTab } from "./team-audit-tab";
import { TeamCapabilitiesTab } from "./team-capabilities-tab";
import { TeamGovernanceTab } from "./team-governance-tab";
import { TeamOverviewTab } from "./team-overview-tab";

function TeamStatusBadge({ status }: { status: TeamStatus }) {
  const label: Record<TeamStatus, string> = {
    active: "活跃",
    archived: "已归档",
    disabled: "已禁用",
  };

  return (
    <Badge variant={status === "active" ? "default" : "secondary"}>
      {label[status]}
    </Badge>
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
  const canDisable = isActive && overview.allowed_actions.includes("team.disable");
  const canArchive = team.status !== "archived" && overview.allowed_actions.includes("team.archive");
  const canRestore = team.status !== "active" && overview.allowed_actions.includes("team.restore");

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-col gap-4 border-b pb-4 lg:flex-row lg:items-start lg:justify-between">
        <div className="flex min-w-0 items-start gap-3">
          <SemanticIconTile tone="info" size="lg">
            <UsersRound />
          </SemanticIconTile>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="text-2xl font-semibold">{team.name}</h1>
              <TeamStatusBadge status={team.status} />
            </div>
            <p className="mt-1 text-sm text-muted-foreground">
              {team.slug} / 负责人 {teamOwnerLabel(team)}
            </p>
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          {canAddMember ? (
            <Button disabled size="sm" variant="outline">
              <UserPlus data-icon="inline-start" />
              添加成员
            </Button>
          ) : null}
          {canCreateGovernance ? (
            <Button disabled size="sm">
              <Plus data-icon="inline-start" />
              创建治理草案
            </Button>
          ) : null}
          {canDisable ? (
            <Button onClick={onDisableTeam} size="sm" variant="outline">
              <ShieldCheck data-icon="inline-start" />
              禁用团队
            </Button>
          ) : null}
          {canArchive ? (
            <Button onClick={onArchiveTeam} size="sm" variant="outline">
              <Archive data-icon="inline-start" />
              归档团队
            </Button>
          ) : null}
          {canRestore ? (
            <Button onClick={onRestoreTeam} size="sm" variant="outline">
              <RotateCcw data-icon="inline-start" />
              恢复团队
            </Button>
          ) : null}
        </div>
      </div>

      <Tabs defaultValue="overview">
        <LiquidTabsList>
          <LiquidTabsTrigger value="overview">概览</LiquidTabsTrigger>
          <LiquidTabsTrigger value="capabilities">能力与知识</LiquidTabsTrigger>
          <LiquidTabsTrigger value="governance">治理策略</LiquidTabsTrigger>
          <LiquidTabsTrigger value="audit">审计记录</LiquidTabsTrigger>
        </LiquidTabsList>
        <TabsContent value="overview">
          <TeamOverviewTab
            allowedActions={overview.allowed_actions}
            apiBaseUrl={apiOptions.baseUrl}
            fetcher={apiOptions.fetcher}
            overview={overview}
            teamId={team.id}
          />
        </TabsContent>
        <TabsContent value="capabilities">
          <TeamCapabilitiesTab
            apiOptions={apiOptions}
            canEdit={canCreateGovernance}
            currentRevision={currentRevision ?? overview.current_revision}
            teamId={team.id}
          />
        </TabsContent>
        <TabsContent value="governance">
          <TeamGovernanceTab
            apiOptions={apiOptions}
            canApprove={canApproveGovernance}
            canEdit={canCreateGovernance}
            currentRevision={currentRevision ?? overview.current_revision}
            teamId={team.id}
          />
        </TabsContent>
        <TabsContent value="audit">
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

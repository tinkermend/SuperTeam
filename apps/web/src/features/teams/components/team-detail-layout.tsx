import { Archive, RotateCcw, ShieldCheck, Trash2, UsersRound } from "lucide-react";
import { useEffect, useState } from "react";
import { useLocation } from "@tanstack/react-router";
import { IconTile, StatusPill, V3Button, V3Tabs } from "@/components/superteam";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type { ApiClientOptions } from "@/lib/api/client";
import type { TeamOverview, TeamStatus } from "@/lib/api/teams";
import { teamStatusLabel } from "@/lib/status-labels";
import { TeamCapabilitiesTab } from "./team-capabilities-tab";
import { TeamConstitutionTab } from "./team-constitution-tab";
import { TeamOverviewTab } from "./team-overview-tab";

function TeamStatusPill({ status }: { status: TeamStatus }) {
  const tone: Record<TeamStatus, "ok" | "mute" | "warn"> = {
    active: "ok",
    archived: "mute",
    disabled: "warn",
  };

  return (
    <StatusPill tone={tone[status]}>
      {teamStatusLabel(status)}
    </StatusPill>
  );
}

type TeamDetailLayoutProps = {
  apiOptions: ApiClientOptions;
  onArchiveTeam?: () => void;
  onDeleteTeam?: () => void;
  onDisableTeam?: () => void;
  onRestoreTeam?: () => void;
  onTeamChanged?: () => void;
  overview: TeamOverview;
};

export function TeamDetailLayout({
  apiOptions,
  onArchiveTeam,
  onDeleteTeam,
  onDisableTeam,
  onRestoreTeam,
  onTeamChanged,
  overview,
}: TeamDetailLayoutProps) {
  const location = useLocation();
  const team = overview.team;
  const isActive = team.status === "active";
  const canEditConstitution = isActive && overview.allowed_actions.includes("team.governance.edit");
  const canDisable = isActive && overview.allowed_actions.includes("team.disable");
  const canArchive = team.status !== "archived" && overview.allowed_actions.includes("team.archive");
  const canRestore = team.status !== "active" && overview.allowed_actions.includes("team.restore");
  const canDelete = overview.allowed_actions.includes("team.delete");
  const [activeTab, setActiveTab] = useState(() => resolveTeamDetailTab(location.hash));

  useEffect(() => {
    setActiveTab(resolveTeamDetailTab(location.hash));
  }, [location.hash]);

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-col gap-4 rounded-v3-card bg-v3-card p-5 shadow-v3 lg:flex-row lg:items-start lg:justify-between">
        <div className="flex min-w-0 items-start gap-3">
          <IconTile tone="info" size="lg">
            <UsersRound />
          </IconTile>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <p className="text-[28px] font-extrabold tracking-tight text-v3-ink">
                {team.name}
              </p>
              <TeamStatusPill status={team.status} />
            </div>
            <p className="mt-1 text-[13px] text-v3-ink-2">
              {team.slug} / 负责人 {teamOwnerLabel(team)}
            </p>
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
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
          {canDelete ? (
            <AlertDialog>
              <AlertDialogTrigger asChild>
                <V3Button size="sm" variant="danger">
                  <Trash2 data-icon="inline-start" />
                  删除团队
                </V3Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>确认删除团队</AlertDialogTitle>
                  <AlertDialogDescription>
                    删除后，所有绑定的数字员工将失去团队归属，团队的技能绑定与能力（MCP）绑定将一并解除，操作不可逆。
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>取消</AlertDialogCancel>
                  <AlertDialogAction onClick={onDeleteTeam}>
                    确认删除
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          ) : null}
        </div>
      </div>

      <Tabs onValueChange={setActiveTab} value={activeTab}>
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
              能力
            </TabsTrigger>
            <TabsTrigger
              className="rounded-[10px] px-4 py-2 text-[13px] font-semibold data-[state=active]:bg-v3-brand-soft data-[state=active]:text-v3-brand-deep data-[state=active]:shadow-none"
              value="constitution"
            >
              宪法
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
            canEdit={canEditConstitution}
            teamId={team.id}
          />
        </TabsContent>
        <TabsContent className="mt-0" value="constitution">
          <TeamConstitutionTab
            apiOptions={apiOptions}
            canEdit={canEditConstitution}
            constitution={team.constitution}
            onSaved={onTeamChanged}
            teamId={team.id}
          />
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

function resolveTeamDetailTab(hash: string) {
  switch (hash) {
    case "capabilities":
    case "constitution":
      return hash;
    default:
      return "overview";
  }
}

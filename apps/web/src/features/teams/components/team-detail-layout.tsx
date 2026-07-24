import { Settings, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { useLocation } from "@tanstack/react-router";
import { TeamIconTile, type TeamDisplayMetadata } from "@/components/superteam/team-icon-tile";
import { StatusPill, Button } from "@/components/superteam";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle
} from "@/components/ui/alert-dialog";
import type { ApiClientOptions } from "@/lib/api/client";
import type { TeamOverview, TeamStatus } from "@/lib/api/teams";
import { teamStatusLabel } from "@/lib/status-labels";
import { TeamCapabilitiesTab } from "./team-capabilities-tab";
import { TeamConstitutionTab } from "./team-constitution-tab";
import {
  canConfigureTeamMembers,
  TeamHumanMembersChrome,
  teamHeaderMetaLabel
} from "./team-human-members";
import { TeamOverviewTab } from "./team-overview-tab";

// 团队生命周期收敛：存活团队唯一状态 active，退出只有删除一条路（软删+审计）。
function TeamStatusPill({ status }: { status: TeamStatus }) {
  return <StatusPill tone="ok">{teamStatusLabel(status)}</StatusPill>;
}

type TeamDetailLayoutProps = {
  apiOptions: ApiClientOptions;
  onDeleteTeam?: () => void;
  onTeamChanged?: () => void;
  overview: TeamOverview;
};

export function TeamDetailLayout({
  apiOptions,
  onDeleteTeam,
  onTeamChanged,
  overview
}: TeamDetailLayoutProps) {
  const location = useLocation();
  const team = overview.team;
  const canEditConstitution = overview.allowed_actions.includes("team.governance.edit");
  const canDelete = overview.allowed_actions.includes("team.delete");
  const canConfigure = canConfigureTeamMembers(overview.allowed_actions);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [configOpen, setConfigOpen] = useState(false);

  useEffect(() => {
    const sectionId = resolveTeamSectionId(location.hash);
    if (!sectionId) {
      return;
    }
    // 创建后 #constitution / 深链 #capabilities：滚到对应分区，不再切 Tab。
    requestAnimationFrame(() => {
      document.getElementById(sectionId)?.scrollIntoView({ behavior: "smooth", block: "start" });
    });
  }, [location.hash]);

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
                <TeamHumanMembersChrome
                  allowedActions={overview.allowed_actions}
                  apiOptions={apiOptions}
                  configOpen={configOpen}
                  onConfigOpenChange={setConfigOpen}
                  teamId={team.id}
                />
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
              <Button
                onClick={() => setConfigOpen(true)}
                size="sm"
                type="button"
                variant="outline"
              >
                <Settings data-icon="inline-start" />
                配置团队
              </Button>
            ) : null}
            {canDelete ? (
              <>
                <Button
                  onClick={() => setDeleteOpen(true)}
                  size="sm"
                  type="button"
                  variant="ghost"
                  className="text-danger hover:bg-danger-soft hover:text-danger"
                >
                  <Trash2 data-icon="inline-start" />
                  删除团队
                </Button>
                <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
                  <AlertDialogContent>
                    <AlertDialogHeader>
                      <AlertDialogTitle>确认删除团队</AlertDialogTitle>
                      <AlertDialogDescription>
                        删除后，所有绑定的数字员工将失去团队归属（进入候岗大厅），技能与能力（MCP）绑定一并解除；团队进入"待确认删除"状态，管理员可在团队管理页恢复，或确认后彻底删除。
                      </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                      <AlertDialogCancel>取消</AlertDialogCancel>
                      <AlertDialogAction onClick={onDeleteTeam}>确认删除</AlertDialogAction>
                    </AlertDialogFooter>
                  </AlertDialogContent>
                </AlertDialog>
              </>
            ) : null}
          </div>
        </div>
      </div>

      <TeamOverviewTab
        allowedActions={overview.allowed_actions}
        apiBaseUrl={apiOptions.baseUrl}
        fetcher={apiOptions.fetcher}
        teamId={team.id}
      />

      <section aria-label="团队能力" id="team-section-capabilities">
        <TeamCapabilitiesTab
          apiOptions={apiOptions}
          canEdit={canEditConstitution}
          teamId={team.id}
        />
      </section>

      <section aria-label="团队治理" id="team-section-constitution">
        <TeamConstitutionTab
          apiOptions={apiOptions}
          canEdit={canEditConstitution}
          constitution={team.constitution}
          onSaved={onTeamChanged}
          teamId={team.id}
        />
      </section>
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

function resolveTeamSectionId(hash: string) {
  switch (hash.replace(/^#/, "")) {
    case "capabilities":
      return "team-section-capabilities";
    case "constitution":
      return "team-section-constitution";
    default:
      return null;
  }
}

import { Archive, FileText, Plus, RotateCcw, ShieldCheck, UserPlus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type { TeamOverview } from "@/lib/api/teams";
import { TeamStatusBadge } from "./team-list-table";
import { TeamOverviewTab } from "./team-overview-tab";

type TeamDetailLayoutProps = {
  onArchiveTeam?: () => void;
  onDisableTeam?: () => void;
  onRestoreTeam?: () => void;
  overview: TeamOverview;
};

export function TeamDetailLayout({ onArchiveTeam, onDisableTeam, onRestoreTeam, overview }: TeamDetailLayoutProps) {
  const team = overview.team;
  const isActive = team.status === "active";
  const canAddMember = isActive && overview.allowed_actions.includes("team.member.add");
  const canCreateGovernance = isActive && overview.allowed_actions.includes("team.governance.edit");
  const canDisable = isActive && overview.allowed_actions.includes("team.disable");
  const canArchive = team.status !== "archived" && overview.allowed_actions.includes("team.archive");
  const canRestore = team.status !== "active" && overview.allowed_actions.includes("team.restore");

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-col gap-4 border-b pb-4 lg:flex-row lg:items-start lg:justify-between">
        <div className="flex min-w-0 items-start gap-3">
          <div className="flex size-12 shrink-0 items-center justify-center rounded-md border bg-muted">
            <ShieldCheck />
          </div>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="text-2xl font-semibold">{team.name}</h1>
              <TeamStatusBadge status={team.status} />
            </div>
            <p className="mt-1 text-sm text-muted-foreground">
              {team.slug} / 负责人 {team.human_owner_user_id ?? "未设置"}
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
        <TabsList className="w-full justify-start overflow-x-auto">
          <TabsTrigger value="overview">概览</TabsTrigger>
          <TabsTrigger value="members">成员</TabsTrigger>
          <TabsTrigger value="employees">数字员工</TabsTrigger>
          <TabsTrigger value="capabilities">能力与知识</TabsTrigger>
          <TabsTrigger value="governance">治理策略</TabsTrigger>
          <TabsTrigger value="audit">审计记录</TabsTrigger>
        </TabsList>
        <TabsContent value="overview">
          <TeamOverviewTab overview={overview} />
        </TabsContent>
        <TabsContent value="members">
          <ScopedPlaceholder text="Plan 2 会接入成员与角色管理。" />
        </TabsContent>
        <TabsContent value="employees">
          <ScopedPlaceholder text="Plan 4 会接入团队数字员工列表和快速创建。" />
        </TabsContent>
        <TabsContent value="capabilities">
          <ScopedPlaceholder text="Plan 3 会接入 Skills、MCP、知识库和外部能力绑定。" />
        </TabsContent>
        <TabsContent value="governance">
          <ScopedPlaceholder text="Plan 3 会接入宪法、审批策略和治理草案。" />
        </TabsContent>
        <TabsContent value="audit">
          <ScopedPlaceholder text="Plan 4 会接入团队审计记录。" />
        </TabsContent>
      </Tabs>
    </div>
  );
}

function ScopedPlaceholder({ text }: { text: string }) {
  return (
    <div className="flex min-h-32 items-center gap-3 rounded-md border p-4 text-sm text-muted-foreground">
      <FileText />
      <span>{text}</span>
    </div>
  );
}

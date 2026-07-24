import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { Plus } from "lucide-react";
import { Main } from "@/components/layout/main";
import {
  ShellPageHeader,
  ShellPageHeaderBack
} from "@/components/layout/shell-page-header";
import {
  Button,
  ErrorState,
  LoadingState,
  DataTable,
  Td,
  Th,
  Tr,
  WorkSurface
} from "@/components/superteam";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger
} from "@/components/ui/alert-dialog";
import {
  confirmTeamDelete,
  deleteTeam,
  getTeamOverview,
  listPendingDeleteTeams,
  listTeamSummaries,
  restorePendingDeleteTeam
} from "@/lib/api/teams";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { CreateTeamView } from "./components/create-team-page";
import {
  TeamManagementToolbar,
  type TeamListFilters
} from "./components/team-management-toolbar";
import { TeamDetailLayout } from "./components/team-detail-layout";
import { TeamCardGrid } from "./components/team-card-grid";

export function TeamsPage() {
  const apiBaseUrl = resolveControlPlaneUrl();

  return <TeamsView apiBaseUrl={apiBaseUrl} />;
}

export function TeamDetailPage({ teamId }: { teamId: string }) {
  const apiBaseUrl = resolveControlPlaneUrl();

  return <TeamDetailView apiBaseUrl={apiBaseUrl} teamId={teamId} />;
}

export function CreateTeamPage() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const navigate = useNavigate();

  return (
    <>
      <ShellPageHeader
        back={<ShellPageHeaderBack ariaLabel="返回团队管理" to="/teams" />}
        title="新建团队"
        subtitle="配置团队负责人、成员和初始治理边界。"
      />
      <Main width="canvas" fixed className="min-w-0 overflow-x-hidden py-4">
        <CreateTeamView
          apiBaseUrl={apiBaseUrl}
          showHeading={false}
          onCancel={() => void navigate({ to: "/teams" })}
          onCreated={(overview, { goToConstitution }) =>
            void navigate({
              hash: goToConstitution ? "constitution" : undefined,
              params: { teamId: overview.team.id },
              to: "/teams/$teamId"
})
          }
        />
      </Main>
    </>
  );
}

type TeamsViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
};

export function TeamsView({ apiBaseUrl, fetcher }: TeamsViewProps) {
  const [filters, setFilters] = useState<TeamListFilters>({ q: "" });
  const teams = useQuery({
    queryKey: ["team-summaries", filters],
    queryFn: () =>
      listTeamSummaries(
        { baseUrl: apiBaseUrl, fetcher },
        {
          governance_status: filters.governance_status,
          q: filters.q,
          status: filters.status
},
      )
});

  const isFiltered = Boolean(
    filters.q.trim() || filters.status || filters.governance_status,
  );

  return (
    <>
      <ShellPageHeader
        icon={
          <img
            alt=""
            className="size-8 object-contain"
            decoding="async"
            height={32}
            src="/images/team-role-icons/general-team.webp"
            width={32}
          />
        }
        iconTone="info"
        title="团队管理"
        subtitle="团队负责人、治理配置和协作边界。"
      />
      <Main width="wide" className="min-w-0 overflow-x-hidden">
        <div className="flex min-w-0 flex-col gap-5">
          <div className="flex flex-wrap items-center justify-start gap-2 sm:justify-end">
            <Button asChild className="self-start sm:self-auto">
              <Link to="/teams/new">
                <Plus data-icon="inline-start" />
                新建团队
              </Link>
            </Button>
          </div>
          <TeamManagementToolbar
            filters={filters}
            onChange={setFilters}
            onReset={() => setFilters({ q: "" })}
          />
          <TeamCardGrid
            isError={teams.isError}
            isFiltered={isFiltered}
            isLoading={teams.isLoading}
            teams={teams.data ?? []}
          />
          <PendingDeleteTeamsSection apiBaseUrl={apiBaseUrl} fetcher={fetcher} />
        </div>
      </Main>
    </>
  );
}

// 待确认删除队列(生命周期收敛 P2):删除后的团队滞留于此,管理员恢复或确认物理删除;
// 永不因超时自动删除,滞留超 7 天会收到收件箱催办。
function PendingDeleteTeamsSection({ apiBaseUrl, fetcher }: TeamsViewProps) {
  const apiOptions = { baseUrl: apiBaseUrl, fetcher };
  const queryClient = useQueryClient();
  const pending = useQuery({
    queryKey: ["pending-delete-teams"],
    queryFn: () => listPendingDeleteTeams(apiOptions)
});
  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["pending-delete-teams"] });
    void queryClient.invalidateQueries({ queryKey: ["team-summaries"] });
  };
  const restoreMutation = useMutation({
    mutationFn: (teamId: string) => restorePendingDeleteTeam(apiOptions, teamId),
    onSuccess: invalidate
});
  const confirmMutation = useMutation({
    mutationFn: (teamId: string) => confirmTeamDelete(apiOptions, teamId),
    onSuccess: invalidate
});
  // 防御非数组响应(网关错误页等):队列语义是"有才显示",异常一律隐藏不阻塞主列表。
  const items = Array.isArray(pending.data) ? pending.data : [];
  if (pending.isLoading || pending.isError || items.length === 0) return null;
  return (
    <WorkSurface data-pending-delete-teams>
      <div className="flex items-center justify-between gap-3 px-4 pt-4">
        <h2 className="text-sm font-semibold text-ink">待确认删除</h2>
        <span className="text-xs text-ink-3">恢复或确认后生效；未确认前不会物理删除</span>
      </div>
      <DataTable aria-label="待确认删除团队">
        <thead>
          <tr>
            <Th>团队</Th>
            <Th>删除时间</Th>
            <Th>滞留</Th>
            <Th aria-label="操作" />
          </tr>
        </thead>
        <tbody>
          {items.map((team) => {
            const stalledDays = Math.max(0, Math.floor((Date.now() - new Date(team.deleted_at).getTime()) / 86_400_000));
            return (
              <Tr key={team.id}>
                <Td>
                  <span className="font-medium text-ink">{team.name}</span>
                  <span className="ml-2 text-xs text-ink-3">{team.slug}</span>
                </Td>
                <Td className="tabular-nums">{new Date(team.deleted_at).toLocaleString("zh-CN", { hour12: false })}</Td>
                <Td className="tabular-nums">{stalledDays} 天</Td>
                <Td>
                  <div className="flex justify-end gap-2">
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={restoreMutation.isPending}
                      onClick={() => restoreMutation.mutate(team.id)}
                    >
                      恢复
                    </Button>
                    <AlertDialog>
                      <AlertDialogTrigger asChild>
                        <Button size="sm" variant="danger" disabled={confirmMutation.isPending}>
                          确认删除
                        </Button>
                      </AlertDialogTrigger>
                      <AlertDialogContent>
                        <AlertDialogHeader>
                          <AlertDialogTitle>确认物理删除团队</AlertDialogTitle>
                          <AlertDialogDescription>
                            将彻底删除「{team.name}」及其成员关系、技能与能力绑定等归属数据，操作不可逆；删除记录会保留在审计日志中。
                          </AlertDialogDescription>
                        </AlertDialogHeader>
                        <AlertDialogFooter>
                          <AlertDialogCancel>取消</AlertDialogCancel>
                          <AlertDialogAction onClick={() => confirmMutation.mutate(team.id)}>彻底删除</AlertDialogAction>
                        </AlertDialogFooter>
                      </AlertDialogContent>
                    </AlertDialog>
                  </div>
                </Td>
              </Tr>
            );
          })}
        </tbody>
      </DataTable>
    </WorkSurface>
  );
}

type TeamDetailViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  teamId: string;
};

export function TeamDetailView({
  apiBaseUrl,
  fetcher,
  teamId
}: TeamDetailViewProps) {
  const navigate = useNavigate();
  const apiOptions = { baseUrl: apiBaseUrl, fetcher };
  const overview = useQuery({
    queryKey: ["team-overview", teamId],
    queryFn: () => getTeamOverview(apiOptions, teamId)
});
  const deleteMutation = useMutation({
    mutationFn: () => deleteTeam(apiOptions, teamId),
    onSuccess: () => {
      void navigate({ to: "/teams" });
    }
});

  return (
    <>
      <ShellPageHeader
        back={<ShellPageHeaderBack ariaLabel="返回团队管理" to="/teams" />}
        title={overview.data?.team.name ?? "团队详情"}
        subtitle={overview.data ? `${overview.data.team.slug} / 团队治理和协作边界` : "加载团队详情"}
      />
      <Main width="wide">
        {overview.isLoading ? (
          <LoadingState label="团队概览加载中" />
        ) : null}
        {overview.isError ? (
          <ErrorState title="团队概览加载失败" />
        ) : null}
        {overview.data ? (
          <TeamDetailLayout
            apiOptions={apiOptions}
            onDeleteTeam={() => deleteMutation.mutate()}
            onTeamChanged={() => void overview.refetch()}
            overview={overview.data}
          />
        ) : null}
      </Main>
    </>
  );
}

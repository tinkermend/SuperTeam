import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { Plus, UsersRound } from "lucide-react";
import { Main } from "@/components/layout/main";
import {
  ShellPageHeader,
  ShellPageHeaderBack,
} from "@/components/layout/shell-page-header";
import {
  V3Button,
  V3ErrorState,
  V3LoadingState,
} from "@/components/superteam";
import {
  archiveTeam,
  deleteTeam,
  disableTeam,
  getTeamOverview,
  listTeamSummaries,
  restoreTeam,
} from "@/lib/api/teams";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { CreateTeamView } from "./components/create-team-page";
import {
  TeamManagementToolbar,
  type TeamListFilters,
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
      <Main className="min-w-0 overflow-x-hidden">
        <CreateTeamView
          apiBaseUrl={apiBaseUrl}
          showHeading={false}
          onCancel={() => void navigate({ to: "/teams" })}
          onCreated={(overview, { goToConstitution }) =>
            void navigate({
              hash: goToConstitution ? "constitution" : undefined,
              params: { teamId: overview.team.id },
              to: "/teams/$teamId",
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
          status: filters.status,
        },
      ),
  });

  return (
    <>
      <ShellPageHeader
        icon={<UsersRound />}
        iconTone="info"
        title="团队管理"
        subtitle="团队负责人、治理配置和协作边界。"
      />
      <Main className="min-w-0 overflow-x-hidden">
        <div className="flex min-w-0 flex-col gap-5">
          <div className="flex flex-wrap items-center justify-start gap-2 sm:justify-end">
            <V3Button asChild className="self-start sm:self-auto">
              <Link to="/teams/new">
                <Plus data-icon="inline-start" />
                新建团队
              </Link>
            </V3Button>
          </div>
          <TeamManagementToolbar
            filters={filters}
            onChange={setFilters}
            onReset={() => setFilters({ q: "" })}
          />
          <TeamCardGrid
            apiBaseUrl={apiBaseUrl}
            fetcher={fetcher}
            isError={teams.isError}
            isLoading={teams.isLoading}
            teams={teams.data ?? []}
          />
        </div>
      </Main>
    </>
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
  teamId,
}: TeamDetailViewProps) {
  const navigate = useNavigate();
  const apiOptions = { baseUrl: apiBaseUrl, fetcher };
  const overview = useQuery({
    queryKey: ["team-overview", teamId],
    queryFn: () => getTeamOverview(apiOptions, teamId),
  });
  const disableMutation = useMutation({
    mutationFn: () => disableTeam(apiOptions, teamId),
    onSuccess: () => {
      void overview.refetch();
    },
  });
  const archiveMutation = useMutation({
    mutationFn: () => archiveTeam(apiOptions, teamId),
    onSuccess: () => {
      void overview.refetch();
    },
  });
  const restoreMutation = useMutation({
    mutationFn: () => restoreTeam(apiOptions, teamId),
    onSuccess: () => {
      void overview.refetch();
    },
  });
  const deleteMutation = useMutation({
    mutationFn: () => deleteTeam(apiOptions, teamId),
    onSuccess: () => {
      void navigate({ to: "/teams" });
    },
  });

  return (
    <>
      <ShellPageHeader
        back={<ShellPageHeaderBack ariaLabel="返回团队管理" to="/teams" />}
        title={overview.data?.team.name ?? "团队详情"}
        subtitle={overview.data ? `${overview.data.team.slug} / 团队治理和协作边界` : "加载团队详情"}
      />
      <Main>
        {overview.isLoading ? (
          <V3LoadingState label="团队概览加载中" />
        ) : null}
        {overview.isError ? (
          <V3ErrorState title="团队概览加载失败" />
        ) : null}
        {overview.data ? (
          <TeamDetailLayout
            apiOptions={apiOptions}
            onArchiveTeam={() => archiveMutation.mutate()}
            onDeleteTeam={() => deleteMutation.mutate()}
            onDisableTeam={() => disableMutation.mutate()}
            onRestoreTeam={() => restoreMutation.mutate()}
            onTeamChanged={() => void overview.refetch()}
            overview={overview.data}
          />
        ) : null}
      </Main>
    </>
  );
}

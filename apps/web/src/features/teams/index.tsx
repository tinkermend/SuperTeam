import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { Plus, UsersRound } from "lucide-react";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import {
  IconTile,
  V3Button,
  V3ErrorState,
  V3LoadingState,
} from "@/components/superteam";
import { ThemeSwitch } from "@/components/theme-switch";
import {
  archiveTeam,
  disableTeam,
  getCurrentTeamGovernance,
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
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main className="min-w-0 overflow-x-hidden">
        <CreateTeamView
          apiBaseUrl={apiBaseUrl}
          onCancel={() => void navigate({ to: "/teams" })}
          onCreated={(overview, { goToGovernance }) =>
            void navigate({
              hash: goToGovernance ? "governance" : undefined,
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
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main className="min-w-0 overflow-x-hidden">
        <div className="mb-4 flex min-w-0 flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-3">
            <IconTile tone="info" size="lg">
              <UsersRound />
            </IconTile>
            <div>
              <h1 className="text-[28px] font-extrabold tracking-tight text-v3-ink">
                团队管理
              </h1>
              <p className="text-[13px] text-v3-ink-2">
                团队负责人、治理配置和协作边界。
              </p>
            </div>
          </div>
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
  const apiOptions = { baseUrl: apiBaseUrl, fetcher };
  const overview = useQuery({
    queryKey: ["team-overview", teamId],
    queryFn: () => getTeamOverview(apiOptions, teamId),
  });
  const currentGovernance = useQuery({
    queryKey: ["team-governance-current", teamId],
    queryFn: () => getCurrentTeamGovernance(apiOptions, teamId),
  });
  const disableMutation = useMutation({
    mutationFn: () => disableTeam(apiOptions, teamId),
    onSuccess: () => {
      void overview.refetch();
      void currentGovernance.refetch();
    },
  });
  const archiveMutation = useMutation({
    mutationFn: () => archiveTeam(apiOptions, teamId),
    onSuccess: () => {
      void overview.refetch();
      void currentGovernance.refetch();
    },
  });
  const restoreMutation = useMutation({
    mutationFn: () => restoreTeam(apiOptions, teamId),
    onSuccess: () => {
      void overview.refetch();
      void currentGovernance.refetch();
    },
  });

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
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
            currentRevision={
              currentGovernance.data ?? overview.data.current_revision
            }
            onArchiveTeam={() => archiveMutation.mutate()}
            onDisableTeam={() => disableMutation.mutate()}
            onRestoreTeam={() => restoreMutation.mutate()}
            overview={overview.data}
          />
        ) : null}
      </Main>
    </>
  );
}

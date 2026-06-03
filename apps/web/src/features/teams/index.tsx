import { useMutation, useQuery } from "@tanstack/react-query";
import { UsersRound } from "lucide-react";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import { archiveTeam, disableTeam, getTeamOverview, listTeamSummaries, restoreTeam } from "@/lib/api/teams";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { TeamDetailLayout } from "./components/team-detail-layout";
import { TeamListTable } from "./components/team-list-table";

export function TeamsPage() {
  const apiBaseUrl = resolveControlPlaneUrl();

  return <TeamsView apiBaseUrl={apiBaseUrl} />;
}

export function TeamDetailPage({ teamId }: { teamId: string }) {
  const apiBaseUrl = resolveControlPlaneUrl();

  return <TeamDetailView apiBaseUrl={apiBaseUrl} teamId={teamId} />;
}

type TeamsViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
};

export function TeamsView({ apiBaseUrl, fetcher }: TeamsViewProps) {
  const teams = useQuery({
    queryKey: ["team-summaries"],
    queryFn: () => listTeamSummaries({ baseUrl: apiBaseUrl, fetcher }),
  });

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main>
        <div className="mb-4 flex items-center gap-3">
          <div className="flex items-center gap-3">
            <div className="flex size-10 items-center justify-center rounded-md border bg-muted">
              <UsersRound />
            </div>
            <div>
              <h1 className="text-2xl font-bold tracking-tight">团队管理</h1>
              <p className="text-sm text-muted-foreground">团队负责人、治理配置和协作边界。</p>
            </div>
          </div>
        </div>

        <TeamListTable isError={teams.isError} isLoading={teams.isLoading} teams={teams.data ?? []} />
      </Main>
    </>
  );
}

type TeamDetailViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  teamId: string;
};

export function TeamDetailView({ apiBaseUrl, fetcher, teamId }: TeamDetailViewProps) {
  const overview = useQuery({
    queryKey: ["team-overview", teamId],
    queryFn: () => getTeamOverview({ baseUrl: apiBaseUrl, fetcher }, teamId),
  });
  const lifecycleOptions = { baseUrl: apiBaseUrl, fetcher };
  const disableMutation = useMutation({
    mutationFn: () => disableTeam(lifecycleOptions, teamId),
    onSuccess: () => {
      void overview.refetch();
    },
  });
  const archiveMutation = useMutation({
    mutationFn: () => archiveTeam(lifecycleOptions, teamId),
    onSuccess: () => {
      void overview.refetch();
    },
  });
  const restoreMutation = useMutation({
    mutationFn: () => restoreTeam(lifecycleOptions, teamId),
    onSuccess: () => {
      void overview.refetch();
    },
  });

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main>
        {overview.isLoading ? <p className="text-sm text-muted-foreground">加载中</p> : null}
        {overview.isError ? <p className="text-sm text-destructive">团队概览加载失败</p> : null}
        {overview.data ? (
          <TeamDetailLayout
            apiBaseUrl={apiBaseUrl}
            fetcher={fetcher}
            onArchiveTeam={() => archiveMutation.mutate()}
            onDisableTeam={() => disableMutation.mutate()}
            onRestoreTeam={() => restoreMutation.mutate()}
            overview={overview.data}
            teamId={teamId}
          />
        ) : null}
      </Main>
    </>
  );
}

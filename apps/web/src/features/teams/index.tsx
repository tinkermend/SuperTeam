import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, UsersRound } from "lucide-react";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { SemanticIconTile } from "@/components/superteam/liquid-components";
import { ThemeSwitch } from "@/components/theme-switch";
import { Button } from "@/components/ui/button";
import {
  archiveTeam,
  createTeam,
  disableTeam,
  getCurrentTeamGovernance,
  getTeamOverview,
  listTeamSummaries,
  restoreTeam,
} from "@/lib/api/teams";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import {
  CreateTeamDrawer,
  type CreateTeamDraft,
} from "./components/create-team-drawer";
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

type TeamsViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
};

export function TeamsView({ apiBaseUrl, fetcher }: TeamsViewProps) {
  const queryClient = useQueryClient();
  const [filters, setFilters] = useState<TeamListFilters>({ q: "" });
  const [createOpen, setCreateOpen] = useState(false);
  const [highlightedTeamId, setHighlightedTeamId] = useState<string>();
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
  const createMutation = useMutation({
    mutationFn: (draft: CreateTeamDraft) =>
      createTeam(
        { baseUrl: apiBaseUrl, fetcher },
        {
          name: draft.name.trim(),
          slug: draft.slug.trim(),
          human_owner_user_ids: draft.owner?.id ? [draft.owner.id] : [],
          initial_members: draft.initial_members,
          metadata: {
            display: {
              color_tone: draft.display.color_tone,
              icon_key: draft.display.icon_key,
            },
          },
        },
      ),
    onSuccess: (overview) => {
      setCreateOpen(false);
      setHighlightedTeamId(overview.team.id);
      void queryClient.invalidateQueries({
        queryKey: ["team-summaries"],
      });
    },
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
            <SemanticIconTile tone="info" size="lg">
              <UsersRound />
            </SemanticIconTile>
            <div>
              <h1 className="text-2xl font-bold tracking-tight">团队管理</h1>
              <p className="text-sm text-muted-foreground">
                团队负责人、治理配置和协作边界。
              </p>
            </div>
          </div>
          <Button className="self-start sm:self-auto" onClick={() => setCreateOpen(true)} type="button">
            <Plus data-icon="inline-start" />
            新建团队
          </Button>
        </div>

        <TeamManagementToolbar
          filters={filters}
          onChange={setFilters}
          onReset={() => setFilters({ q: "" })}
        />
        <TeamCardGrid
          apiBaseUrl={apiBaseUrl}
          fetcher={fetcher}
          highlightedTeamId={highlightedTeamId}
          isError={teams.isError}
          isLoading={teams.isLoading}
          teams={teams.data ?? []}
        />
        <CreateTeamDrawer
          apiBaseUrl={apiBaseUrl}
          fetcher={fetcher}
          isSubmitting={createMutation.isPending}
          onOpenChange={(open) => {
            createMutation.reset();
            setCreateOpen(open);
          }}
          onSubmit={(draft) => createMutation.mutate(draft)}
          open={createOpen}
          submitError={
            createMutation.error instanceof Error
              ? createMutation.error.message
              : undefined
          }
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
          <p className="text-sm text-muted-foreground">加载中</p>
        ) : null}
        {overview.isError ? (
          <p className="text-sm text-destructive">团队概览加载失败</p>
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

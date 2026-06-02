import { useQuery } from "@tanstack/react-query";
import { ShieldCheck, UsersRound } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import type { Team, TeamConfigRevision } from "@/lib/api/teams";
import { getCurrentTeamConfigRevision, listTeams } from "@/lib/api/teams";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";

export function TeamsPage() {
  const apiBaseUrl = resolveControlPlaneUrl();

  return <TeamsView apiBaseUrl={apiBaseUrl} />;
}

type TeamsViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
};

export function TeamsView({ apiBaseUrl, fetcher }: TeamsViewProps) {
  const teams = useQuery({
    queryKey: ["teams"],
    queryFn: () => listTeams({ baseUrl: apiBaseUrl, fetcher }),
  });

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main>
        <div className="mb-4 flex items-center gap-3">
          <div className="flex size-10 items-center justify-center rounded-md border bg-muted">
            <UsersRound />
          </div>
          <div>
            <h1 className="text-2xl font-bold tracking-tight">团队管理</h1>
            <p className="text-sm text-muted-foreground">团队负责人、治理配置和协作边界。</p>
          </div>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>团队列表</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex flex-col gap-3">
              {teams.isLoading ? <p className="text-sm text-muted-foreground">加载中</p> : null}
              {teams.isError ? <p className="text-sm text-destructive">团队列表加载失败</p> : null}
              {(teams.data ?? []).map((team) => (
                <TeamRow key={team.id} apiBaseUrl={apiBaseUrl} fetcher={fetcher} team={team} />
              ))}
              {!teams.isLoading && (teams.data ?? []).length === 0 ? (
                <p className="text-sm text-muted-foreground">暂无团队</p>
              ) : null}
            </div>
          </CardContent>
        </Card>
      </Main>
    </>
  );
}

type TeamRowProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  team: Team;
};

function TeamRow({ apiBaseUrl, fetcher, team }: TeamRowProps) {
  const currentConfig = useQuery({
    queryKey: ["team-config-revision-current", team.id],
    queryFn: () => getCurrentTeamConfigRevision({ baseUrl: apiBaseUrl, fetcher }, team.id),
    retry: false,
  });

  return (
    <div className="rounded-md border p-4">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="flex min-w-0 items-start gap-3">
          <div className="mt-1 flex size-8 shrink-0 items-center justify-center rounded-md border bg-muted">
            <ShieldCheck className="size-4" />
          </div>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <p className="truncate font-medium">{team.name}</p>
              <Badge variant={team.status === "active" ? "default" : "secondary"}>{team.status}</Badge>
            </div>
            <p className="mt-1 text-sm text-muted-foreground">
              {team.slug} / {team.status}
            </p>
            <p className="mt-1 text-xs text-muted-foreground">
              人类负责人 {team.human_owner_user_id || "未设置"}
            </p>
          </div>
        </div>
        <div className="text-left text-xs text-muted-foreground md:text-right">
          {currentConfig.data ? <p>修订号 {currentConfig.data.revision_number}</p> : null}
          <p className="mt-1">{currentConfig.data?.status ?? "治理配置"}</p>
        </div>
      </div>

      <div className="mt-4 border-t pt-3">
        {currentConfig.isLoading ? <p className="text-sm text-muted-foreground">治理配置加载中</p> : null}
        {currentConfig.isError ? <p className="text-sm text-muted-foreground">当前治理配置未就绪</p> : null}
        {currentConfig.data ? <TeamConfigSummary revision={currentConfig.data} /> : null}
      </div>
    </div>
  );
}

function TeamConfigSummary({ revision }: { revision: TeamConfigRevision }) {
  const hardRules = readStringList(revision.constitution, "hard_rules");
  const allowedSkills = readStringList(revision.capability_policy, "allowed_skills");
  const automaticRounds =
    readNumber(revision.internal_collaboration_policy, "max_auto_rounds") ??
    readNumber(revision.internal_collaboration_policy, "automatic_rounds");

  return (
    <div className="grid gap-3 md:grid-cols-3">
      <PolicySection label="宪法硬性规则" values={hardRules} emptyText="未配置硬性规则" />
      <PolicySection label="能力边界" values={allowedSkills} emptyText="未配置能力边界" />
      <div>
        <p className="text-xs font-medium text-muted-foreground">内部协作</p>
        <p className="mt-1 text-sm">内部协作自动轮次 {automaticRounds ?? "未配置"}</p>
      </div>
    </div>
  );
}

function PolicySection({ emptyText, label, values }: { emptyText: string; label: string; values: string[] }) {
  return (
    <div>
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <div className="mt-1 flex flex-wrap gap-2">
        {values.length > 0 ? (
          values.map((value) => (
            <Badge
              key={value}
              variant="secondary"
              className="max-w-full shrink whitespace-normal break-words text-left"
            >
              {value}
            </Badge>
          ))
        ) : (
          <span className="text-sm text-muted-foreground">{emptyText}</span>
        )}
      </div>
    </div>
  );
}

function readStringList(record: Record<string, unknown>, key: string): string[] {
  const value = record[key];

  if (!Array.isArray(value)) {
    return [];
  }

  return value.filter((item): item is string => typeof item === "string" && item.length > 0);
}

function readNumber(record: Record<string, unknown>, key: string): number | undefined {
  const value = record[key];

  return typeof value === "number" ? value : undefined;
}

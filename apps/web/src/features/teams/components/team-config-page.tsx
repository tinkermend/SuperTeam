import { useEffect, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useLocation, useNavigate } from "@tanstack/react-router";
import { Main } from "@/components/layout/main";
import {
  ShellPageHeader,
  ShellPageHeaderBack
} from "@/components/layout/shell-page-header";
import {
  ErrorState,
  LoadingState,
  SoftTabs,
  SoftTabsContent,
  SoftTabsList,
  SoftTabsTrigger
} from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { deleteTeam, getTeamOverview } from "@/lib/api/teams";
import { TeamCapabilitiesTab } from "./team-capabilities-tab";
import { TeamCapabilityReadiness } from "./team-capability-readiness";
import { TeamConfigAudit } from "./team-config-audit";
import { TeamConfigIdentity } from "./team-config-identity";
import { TeamConfigMembers } from "./team-config-members";
import { TeamConstitutionTab } from "./team-constitution-tab";
import { TeamRosterPanel } from "./team-roster-panel";

export function TeamConfigPage({ teamId }: { teamId: string }) {
  return <TeamConfigView apiBaseUrl={resolveControlPlaneUrl()} teamId={teamId} />;
}

type TeamConfigViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  teamId: string;
};

// 团队配置是团队唯一的写入口（与员工 /employees/$id/config、项目
// /projects/$id/config 对齐）；详情页只做观察。分区：编制 / 能力 / 约束 /
// 身份与生命周期 / 审计。
export function TeamConfigView({ apiBaseUrl, fetcher, teamId }: TeamConfigViewProps) {
  const navigate = useNavigate();
  const location = useLocation();
  const apiOptions: ApiClientOptions = { baseUrl: apiBaseUrl, fetcher };
  // 详情页的「去配置 / 查看全部」带 hash 深链到具体分区。Tab 必须受控：硬加载时
  // 首帧的 location.hash 可能还没解析出来，用 defaultValue 会永远停在第一个分区。
  const [tab, setTab] = useState<TeamConfigTab>(() => tabFromHash(location.hash) ?? "roster");
  useEffect(() => {
    const fromHash = tabFromHash(location.hash);
    if (fromHash) {
      setTab(fromHash);
    }
  }, [location.hash]);

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

  const team = overview.data?.team;

  return (
    <>
      <ShellPageHeader
        back={
          <ShellPageHeaderBack
            ariaLabel="返回团队详情"
            params={{ teamId }}
            to="/teams/$teamId"
          />
        }
        title={team ? `配置 ${team.name}` : "团队配置"}
        subtitle="编制、能力基线、行为约束与团队身份的唯一写入口"
      />
      <Main width="wide">
        {overview.isLoading ? <LoadingState label="团队配置加载中" /> : null}
        {overview.isError ? <ErrorState title="团队配置加载失败" /> : null}
        {overview.data ? (
          <SoftTabs
            className="flex w-full min-w-0 flex-col gap-4"
            onValueChange={(value) => setTab(tabFromHash(value) ?? "roster")}
            value={tab}
          >
            <div className="w-full min-w-0 max-w-full overflow-x-auto overflow-y-hidden pb-1">
              <SoftTabsList
                aria-label="团队配置分区"
                className="h-auto w-max min-w-full justify-start rounded-[14px] bg-card p-1.5 text-ink shadow-card"
              >
                <SoftTabsTrigger value="roster">编制</SoftTabsTrigger>
                <SoftTabsTrigger value="capabilities">能力</SoftTabsTrigger>
                <SoftTabsTrigger value="constitution">约束</SoftTabsTrigger>
                <SoftTabsTrigger value="identity">身份与生命周期</SoftTabsTrigger>
                <SoftTabsTrigger value="audit">审计</SoftTabsTrigger>
              </SoftTabsList>
            </div>

            <SoftTabsContent value="roster">
              <div className="flex flex-col gap-4">
                <TeamConfigMembers
                  allowedActions={overview.data.allowed_actions}
                  apiOptions={apiOptions}
                  teamId={teamId}
                />
                <TeamRosterPanel
                  allowedActions={overview.data.allowed_actions}
                  apiBaseUrl={apiBaseUrl}
                  fetcher={fetcher}
                  teamId={teamId}
                />
              </div>
            </SoftTabsContent>

            <SoftTabsContent value="capabilities">
              <div className="flex flex-col gap-4">
                <TeamCapabilitiesTab
                  allowedActions={overview.data.allowed_actions}
                  apiOptions={apiOptions}
                  teamId={teamId}
                />
                {overview.data.allowed_actions.includes("team.capability.manage") ? (
                  <TeamCapabilityReadiness apiOptions={apiOptions} teamId={teamId} />
                ) : null}
              </div>
            </SoftTabsContent>

            <SoftTabsContent value="constitution">
              <TeamConstitutionTab
                apiOptions={apiOptions}
                canEdit={overview.data.allowed_actions.includes("team.governance.edit")}
                constitution={overview.data.team.constitution}
                onSaved={() => void overview.refetch()}
                teamId={teamId}
              />
            </SoftTabsContent>

            <SoftTabsContent value="identity">
              <TeamConfigIdentity
                allowedActions={overview.data.allowed_actions}
                apiOptions={apiOptions}
                onDeleteTeam={() => deleteMutation.mutate()}
                onSaved={() => void overview.refetch()}
                overview={overview.data}
              />
            </SoftTabsContent>

            <SoftTabsContent value="audit">
              <TeamConfigAudit
                allowedActions={overview.data.allowed_actions}
                apiOptions={apiOptions}
                teamId={teamId}
              />
            </SoftTabsContent>
          </SoftTabs>
        ) : null}
      </Main>
    </>
  );
}

const TAB_VALUES = ["roster", "capabilities", "constitution", "identity", "audit"] as const;

type TeamConfigTab = (typeof TAB_VALUES)[number];

/** hash 命中分区就返回它；无 hash / 不认识的 hash 返回 undefined，由调用方保留当前分区。 */
function tabFromHash(hash: string): TeamConfigTab | undefined {
  const normalized = hash.replace(/^#/, "");
  return (TAB_VALUES as readonly string[]).includes(normalized)
    ? (normalized as TeamConfigTab)
    : undefined;
}

import { Boxes, KeyRound, Network } from "lucide-react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  Button,
  DataTable,
  ErrorState,
  IconTile,
  LoadingState,
  StatusPill,
  Td,
  Th,
  Tr,
  WorkSurface,
  type Tone
} from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import { listTeamMcpBindings } from "@/lib/api/capabilities";
import { listTeamSkills } from "@/lib/api/skills";
import { statusLabel } from "@/lib/status-labels";

// 观察面：只展示团队当前的能力基线，安装/绑定动作在团队配置页。
export function TeamCapabilitiesSummary({
  apiOptions,
  teamId
}: {
  apiOptions: ApiClientOptions;
  teamId: string;
}) {
  const teamSkills = useQuery({
    queryKey: ["team-skills", teamId],
    queryFn: () => listTeamSkills(apiOptions, teamId),
    placeholderData: keepPreviousData
  });
  const mcpBindings = useQuery({
    queryKey: ["team-mcp-bindings", teamId],
    queryFn: () => listTeamMcpBindings(apiOptions, teamId),
    placeholderData: keepPreviousData
  });

  return (
    <div className="grid gap-4 xl:grid-cols-2">
      <WorkSurface className="min-w-0">
        <SummaryHeader
          icon={<Boxes />}
          meta={`${teamSkills.data?.length ?? 0} 个已安装`}
          teamId={teamId}
          title="公共技能"
          tone="artifact"
        />
        <div className="p-4">
          {teamSkills.isLoading ? <LoadingState label="公共技能加载中" /> : null}
          {teamSkills.isError ? <ErrorState title="公共技能加载失败" /> : null}
          {!teamSkills.isLoading && !teamSkills.isError && (teamSkills.data?.length ?? 0) === 0 ? (
            <p className="py-2 text-[13px] text-ink-2">暂无已安装技能</p>
          ) : null}
          {(teamSkills.data?.length ?? 0) > 0 ? (
            <DataTable>
              <thead>
                <tr>
                  <Th>技能</Th>
                  <Th>风险</Th>
                </tr>
              </thead>
              <tbody>
                {(teamSkills.data ?? []).map((skill) => (
                  <Tr key={skill.id}>
                    <Td>
                      <p className="truncate text-sm font-medium text-ink">{skill.name}</p>
                      <p className="truncate text-xs text-ink-2">{skill.description}</p>
                    </Td>
                    <Td>
                      <StatusPill tone={skillRiskTone(skill.risk_level)}>
                        {skillRiskLabel(skill.risk_level)}
                      </StatusPill>
                    </Td>
                  </Tr>
                ))}
              </tbody>
            </DataTable>
          ) : null}
        </div>
      </WorkSurface>

      <WorkSurface className="min-w-0">
        <SummaryHeader
          icon={<Network />}
          meta={`${mcpBindings.data?.length ?? 0} 个绑定`}
          teamId={teamId}
          title="公共 MCP"
          tone="info"
        />
        <div className="p-4">
          {mcpBindings.isLoading ? <LoadingState label="公共 MCP 加载中" /> : null}
          {mcpBindings.isError ? <ErrorState title="公共 MCP 加载失败" /> : null}
          {!mcpBindings.isLoading && !mcpBindings.isError && (mcpBindings.data?.length ?? 0) === 0 ? (
            <p className="py-2 text-[13px] text-ink-2">暂无公共 MCP</p>
          ) : null}
          {(mcpBindings.data?.length ?? 0) > 0 ? (
            <DataTable>
              <thead>
                <tr>
                  <Th>服务</Th>
                  <Th>凭据环境变量</Th>
                  <Th>状态</Th>
                </tr>
              </thead>
              <tbody>
                {(mcpBindings.data ?? []).map((binding) => (
                  <Tr key={binding.id}>
                    <Td>
                      <p className="truncate text-sm font-medium text-ink">
                        {binding.server_name ?? binding.server_key}
                      </p>
                      <p className="truncate font-mono text-xs text-ink-2">
                        {binding.url ?? binding.server_key}
                      </p>
                    </Td>
                    <Td>
                      {binding.credential_env_var ? (
                        <span className="flex min-w-0 items-center gap-1 font-mono text-xs text-ink-2">
                          <KeyRound className="size-3 shrink-0" />
                          <span className="truncate">{binding.credential_env_var}</span>
                        </span>
                      ) : (
                        <span className="text-xs text-ink-3">无</span>
                      )}
                    </Td>
                    <Td>
                      <StatusPill tone={binding.status === "active" ? "ok" : "warn"}>
                        {statusLabel(binding.status)}
                      </StatusPill>
                    </Td>
                  </Tr>
                ))}
              </tbody>
            </DataTable>
          ) : null}
        </div>
      </WorkSurface>
    </div>
  );
}

function SummaryHeader({
  icon,
  meta,
  teamId,
  title,
  tone
}: {
  icon: React.ReactNode;
  meta: string;
  teamId: string;
  title: string;
  tone: Extract<Tone, "artifact" | "info">;
}) {
  return (
    <div className="flex min-w-0 items-center justify-between gap-3 border-b border-line px-5 py-4">
      <div className="flex min-w-0 items-center gap-3">
        <IconTile tone={tone} size="sm">
          {icon}
        </IconTile>
        <h2 className="truncate text-base font-bold text-ink">{title}</h2>
        <StatusPill tone={tone}>{meta}</StatusPill>
      </div>
      <Button asChild size="sm" variant="ghost">
        <Link hash="capabilities" params={{ teamId }} to="/teams/$teamId/config">
          去配置
        </Link>
      </Button>
    </div>
  );
}

function skillRiskTone(riskLevel: string): Tone {
  if (riskLevel === "high") return "danger";
  if (riskLevel === "medium") return "warn";
  return "mute";
}

function skillRiskLabel(riskLevel: string) {
  const labels: Record<string, string> = {
    high: "高风险",
    low: "低风险",
    medium: "中风险"
  };
  return labels[riskLevel] ?? riskLevel;
}

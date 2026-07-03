import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { BookOpen, Boxes, KeyRound, Network, ScrollText } from "lucide-react";
import { IconTile, SoftCard, StatusPill, V3Button } from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import { ApiRequestError } from "@/lib/api/client";
import {
  getCurrentDigitalEmployeeEffectiveConfig,
  listEmployeeEnvironmentVariables,
  type DigitalEmployee,
  type DigitalEmployeeExecutionInstance,
} from "@/lib/api/employees";
import { listEffectiveMcpConfig } from "@/lib/api/capabilities";
import { listEmployeeSkills } from "@/lib/api/skills";

type EffectiveContextPanelProps = {
  apiOptions: ApiClientOptions;
  employeeId: string;
  employee: DigitalEmployee;
  executionInstance: DigitalEmployeeExecutionInstance | undefined;
  onManageCapabilities: () => void;
};

export function EffectiveContextPanel({
  apiOptions,
  employeeId,
  employee,
  executionInstance,
  onManageCapabilities,
}: EffectiveContextPanelProps) {
  const effectiveConfig = useQuery({
    queryKey: ["digital-employee-effective-config", employeeId],
    queryFn: () => getCurrentDigitalEmployeeEffectiveConfig(apiOptions, employeeId),
    retry: false,
  });
  const skills = useQuery({
    queryKey: ["employee-skills", employeeId],
    queryFn: () => listEmployeeSkills(apiOptions, employeeId),
  });
  const mcpServers = useQuery({
    queryKey: ["employee-effective-mcp", employeeId],
    queryFn: () => listEffectiveMcpConfig(apiOptions, employeeId),
  });
  const envVars = useQuery({
    queryKey: ["employee-environment-variables", employeeId],
    queryFn: () => listEmployeeEnvironmentVariables(apiOptions, employeeId),
  });

  const noApprovedConfig =
    effectiveConfig.error instanceof ApiRequestError && effectiveConfig.error.status === 404;
  const personalSkillCount = skills.data?.filter((skill) => !skill.inherited).length ?? 0;
  const inheritedSkillCount = skills.data?.filter((skill) => skill.inherited).length ?? 0;
  const personalMcpCount = mcpServers.data?.filter((server) => server.source_scope !== "team").length ?? 0;
  const inheritedMcpCount = mcpServers.data?.filter((server) => server.source_scope === "team").length ?? 0;
  const configuredEnvCount = envVars.data?.filter((item) => item.configured).length ?? 0;
  const missingEnvVars = envVars.data?.filter((item) => !item.configured) ?? [];

  return (
    <SoftCard className="flex flex-col gap-5 p-5">
      <div className="flex items-center justify-between">
        <h2 className="text-base font-semibold text-v3-ink">生效上下文</h2>
        <V3Button asChild size="sm" variant="ghost">
          <Link params={{ employeeId }} to="/employees/$employeeId/config">
            编辑
          </Link>
        </V3Button>
      </div>

      <section className="space-y-2">
        <p className="text-xs font-semibold text-v3-ink-3">基本信息</p>
        <div className="grid grid-cols-2 gap-2 text-sm">
          <InfoItem label="Provider" value={executionInstance?.provider_type ?? "未绑定"} />
          <InfoItem label="角色" value={employee.role} />
          <InfoItem label="状态" value={employee.status} />
          <InfoItem label="工作目录" value={executionInstance?.agent_home_dir ?? "未配置"} />
        </div>
      </section>

      <section className="space-y-2">
        <div className="flex items-center justify-between">
          <p className="flex items-center gap-1.5 text-xs font-semibold text-v3-ink-3">
            <IconTile size="sm" tone="brand">
              <Boxes />
            </IconTile>
            技能
          </p>
          <Link className="text-xs text-v3-brand" to="/skills">
            查看全部
          </Link>
        </div>
        {skills.isLoading ? (
          <p className="text-xs text-v3-ink-3">加载中</p>
        ) : skills.isError ? (
          <p className="text-xs text-destructive">技能加载失败</p>
        ) : (
          <p className="text-xs text-v3-ink-2">
            个人技能 {personalSkillCount} · 团队继承技能 {inheritedSkillCount} · 生效总数 {skills.data?.length ?? 0}
          </p>
        )}
      </section>

      <section className="space-y-2">
        <div className="flex items-center justify-between">
          <p className="flex items-center gap-1.5 text-xs font-semibold text-v3-ink-3">
            <IconTile size="sm" tone="info">
              <Network />
            </IconTile>
            MCP
          </p>
          <Link className="text-xs text-v3-brand" to="/mcp">
            查看全部
          </Link>
        </div>
        {mcpServers.isLoading ? (
          <p className="text-xs text-v3-ink-3">加载中</p>
        ) : mcpServers.isError ? (
          <p className="text-xs text-destructive">MCP 加载失败</p>
        ) : (
          <p className="text-xs text-v3-ink-2">
            个人 MCP {personalMcpCount} · 团队 MCP {inheritedMcpCount} · 生效总数 {mcpServers.data?.length ?? 0}
          </p>
        )}
      </section>

      <section className="space-y-2">
        <p className="flex items-center gap-1.5 text-xs font-semibold text-v3-ink-3">
          <IconTile size="sm" tone="artifact">
            <ScrollText />
          </IconTile>
          宪法与记忆
        </p>
        {effectiveConfig.isLoading ? (
          <p className="text-xs text-v3-ink-3">加载中</p>
        ) : noApprovedConfig ? (
          <p className="text-xs text-v3-ink-3">尚无已批准的生效配置</p>
        ) : effectiveConfig.isError ? (
          <p className="text-xs text-destructive">生效配置加载失败</p>
        ) : (
          <p className="text-xs text-v3-ink-2">宪法层级：团队 + 个人补充（2 层）</p>
        )}
        <div className="flex items-center gap-2">
          <IconTile size="sm" tone="mute">
            <BookOpen />
          </IconTile>
          <StatusPill showDot={false} tone="mute">
            记忆：待接入
          </StatusPill>
        </div>
      </section>

      <section className="space-y-2">
        <div className="flex items-center justify-between">
          <p className="flex items-center gap-1.5 text-xs font-semibold text-v3-ink-3">
            <IconTile size="sm" tone="warn">
              <KeyRound />
            </IconTile>
            环境变量
          </p>
          <V3Button onClick={onManageCapabilities} size="sm" variant="ghost">
            查看详情
          </V3Button>
        </div>
        {envVars.isLoading ? (
          <p className="text-xs text-v3-ink-3">加载中</p>
        ) : envVars.isError ? (
          <p className="text-xs text-destructive">环境变量加载失败</p>
        ) : (
          <>
            <p className="text-xs text-v3-ink-2">
              已配置 {configuredEnvCount} · 缺失 {missingEnvVars.length} · 总数 {envVars.data?.length ?? 0}
            </p>
            {missingEnvVars.length ? (
              <div className="flex flex-wrap gap-1.5">
                {missingEnvVars.map((item) => (
                  <StatusPill key={item.name} tone="danger">
                    {item.name}
                  </StatusPill>
                ))}
              </div>
            ) : null}
          </>
        )}
      </section>
    </SoftCard>
  );
}

function InfoItem({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <p className="text-[11px] text-v3-ink-3">{label}</p>
      <p className="truncate text-sm font-medium text-v3-ink">{value}</p>
    </div>
  );
}

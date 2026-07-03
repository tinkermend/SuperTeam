import { Link } from "@tanstack/react-router";
import { BookOpen, Boxes, KeyRound, Network, ScrollText } from "lucide-react";
import { IconTile, SoftCard, StatusPill, V3Button } from "@/components/superteam";
import type { DigitalEmployee, DigitalEmployeeExecutionInstance } from "@/lib/api/employees";

type EffectiveContextPanelProps = {
  employee: DigitalEmployee;
  executionInstance: DigitalEmployeeExecutionInstance | undefined;
  employeeId: string;
  effectiveConfig: { isLoading: boolean; isError: boolean; noApprovedConfig: boolean };
  skills: { isLoading: boolean; isError: boolean; personalCount: number; inheritedCount: number; totalCount: number };
  mcp: { isLoading: boolean; isError: boolean; personalCount: number; inheritedCount: number; totalCount: number };
  envVars: {
    isLoading: boolean;
    isError: boolean;
    configuredCount: number;
    totalCount: number;
    missingNames: string[];
  };
  onManageCapabilities: () => void;
};

export function EffectiveContextPanel({
  employee,
  executionInstance,
  employeeId,
  effectiveConfig,
  skills,
  mcp,
  envVars,
  onManageCapabilities,
}: EffectiveContextPanelProps) {
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
            个人技能 {skills.personalCount} · 团队继承技能 {skills.inheritedCount} · 生效总数 {skills.totalCount}
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
        {mcp.isLoading ? (
          <p className="text-xs text-v3-ink-3">加载中</p>
        ) : mcp.isError ? (
          <p className="text-xs text-destructive">MCP 加载失败</p>
        ) : (
          <p className="text-xs text-v3-ink-2">
            个人 MCP {mcp.personalCount} · 团队 MCP {mcp.inheritedCount} · 生效总数 {mcp.totalCount}
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
        ) : effectiveConfig.noApprovedConfig ? (
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
              已配置 {envVars.configuredCount} · 缺失 {envVars.missingNames.length} · 总数 {envVars.totalCount}
            </p>
            {envVars.missingNames.length ? (
              <div className="flex flex-wrap gap-1.5">
                {envVars.missingNames.map((name) => (
                  <StatusPill key={name} tone="danger">
                    {name}
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

import { Link } from "@tanstack/react-router";
import { AlertTriangle, CheckCircle2, Info, Network } from "lucide-react";
import { IconTile, SoftCard, StatusPill, Button, type Tone } from "@/components/superteam";
import type {
  BudgetPolicy,
  DigitalEmployee,
  DigitalEmployeeSchedulingReadiness,
  SchedulingReadinessCheck
} from "@/lib/api/employees";
import { projectStatusLabel } from "@/lib/status-labels";

type CountState = {
  isLoading: boolean;
  isError: boolean;
  personalCount: number;
  inheritedCount: number;
  totalCount: number;
};

type EnvState = {
  isLoading: boolean;
  isError: boolean;
  configuredCount: number;
  totalCount: number;
  missingNames: string[];
};

type EmployeeCapabilityRailProps = {
  employee: DigitalEmployee;
  employeeId: string;
  skills: CountState;
  mcp: CountState;
  envVars: EnvState;
  readiness: DigitalEmployeeSchedulingReadiness | undefined;
  readinessLoading: boolean;
  readinessError: boolean;
  onRetryReadiness: () => void;
};

type ReadinessCheckStatus = SchedulingReadinessCheck["status"];

const checkTone = {
  passed: "ok",
  warning: "warn",
  blocked: "danger",
  info: "info"
} as const satisfies Record<ReadinessCheckStatus, Tone>;

const checkLabel = {
  passed: "通过",
  warning: "需关注",
  blocked: "阻塞",
  info: "信息"
} as const satisfies Record<ReadinessCheckStatus, string>;


const statusIcon = {
  passed: CheckCircle2,
  warning: AlertTriangle,
  blocked: AlertTriangle,
  info: Info
};

/** 右栏单卡：可调度能力摘要 + 折叠就绪度，避免能力/就绪两卡叠高。 */
export function EmployeeCapabilityRail({
  employee,
  employeeId,
  skills,
  mcp,
  envVars,
  readiness,
  readinessLoading,
  readinessError,
  onRetryReadiness
}: EmployeeCapabilityRailProps) {
  const persona = employee.persona_memory_markdown?.trim();
  const ready = readiness?.ready_for_project_scheduling === true;
  const blocked =
    readiness?.checks.some((check) => check.status === "blocked" || check.status === "warning") ??
    false;
  const expandChecks = readinessError || (!readinessLoading && readiness != null && !ready) || blocked;
  let headlineTone: Tone = "danger";
  if (readinessError) {
    headlineTone = "danger";
  } else if (readinessLoading) {
    headlineTone = "mute";
  } else if (ready) {
    headlineTone = "ok";
  }
  const headline = ready ? "可进入项目调度池" : "暂不可进入项目调度池";
  const projectSummary = employee.project_summary;
  const boundProjects = projectSummary?.projects ?? [];
  const projectCount = projectSummary?.project_count ?? boundProjects.length;

  return (
    <SoftCard className="flex max-h-full min-h-0 flex-col gap-4 overflow-y-auto p-4">
      <div className="flex items-center justify-between gap-2">
        <div className="min-w-0">
          <h2 className="text-sm font-semibold text-ink">可调度能力</h2>
          <p className="mt-0.5 text-[11px] text-ink-3">技能 / MCP / 环境构成调度边界。</p>
        </div>
        <Button asChild size="sm" variant="ghost">
          <Link params={{ employeeId }} to="/employees/$employeeId/config">
            编辑
          </Link>
        </Button>
      </div>

      <div className="space-y-2.5 text-xs text-ink-2">
        <CapabilityLine
          isError={skills.isError}
          isLoading={skills.isLoading}
          label="技能"
          value={`个人 ${skills.personalCount} · 继承 ${skills.inheritedCount} · 生效 ${skills.totalCount}`}
        />
        <CapabilityLine
          isError={mcp.isError}
          isLoading={mcp.isLoading}
          label="MCP"
          value={`个人 ${mcp.personalCount} · 团队 ${mcp.inheritedCount} · 生效 ${mcp.totalCount}`}
        />
        <CapabilityLine
          isError={envVars.isError}
          isLoading={envVars.isLoading}
          label="环境变量"
          value={`已配置 ${envVars.configuredCount} · 缺失 ${envVars.missingNames.length} · 总数 ${envVars.totalCount}`}
        />
        {(envVars.missingNames.length > 0 || (readiness?.capabilities.environment_variables.missing_names.length ?? 0) > 0) ? (
          <div className="flex flex-wrap gap-1">
            {[
              ...new Set([
                ...envVars.missingNames,
                ...(readiness?.capabilities.environment_variables.missing_names ?? []),
              ]),
            ].map((name) => (
              <StatusPill key={name} tone="danger">
                {name}
              </StatusPill>
            ))}
          </div>
        ) : null}
        {(readiness?.capabilities.skills.missing_required.length ?? 0) > 0 ? (
          <div className="flex flex-wrap gap-1">
            {readiness?.capabilities.skills.missing_required.map((name) => (
              <StatusPill key={name} tone="danger">
                {name}
              </StatusPill>
            ))}
          </div>
        ) : null}
        <div className="flex items-start justify-between gap-2">
          <span className="shrink-0 text-ink-3">人格记忆</span>
          <div className="min-w-0 text-end">
            <StatusPill showDot={false} tone={persona ? "info" : "mute"}>
              {persona ? "有内容" : "未设置"}
            </StatusPill>
            {persona ? (
              <p className="mt-1 line-clamp-2 text-start text-[11px] leading-4 text-ink-3">{persona}</p>
            ) : null}
          </div>
        </div>
        <div className="flex items-center justify-between gap-2">
          <span className="text-ink-3">每日 Token</span>
          <span className="font-medium tabular-nums text-ink">{budgetLimitLabel(employee.budget_policy)}</span>
        </div>
      </div>

      <div className="border-t border-line pt-3" id="employee-bound-projects">
        <p className="text-sm font-semibold text-ink">绑定项目</p>
        <p className="mt-0.5 text-[11px] text-ink-3">成员身份或任务分派均计入关联。</p>
        {projectCount === 0 ? (
          <p className="mt-2 text-xs text-ink-3">未绑定任何项目</p>
        ) : (
          <ul className="mt-2 space-y-1.5">
            {boundProjects.map((project) => (
              <li key={project.project_id}>
                <Link
                  className="block rounded-lg border border-line bg-card-soft px-2.5 py-1.5 hover:border-brand/40"
                  params={{ projectId: project.project_id }}
                  to="/projects/$projectId"
                >
                  <span className="block truncate text-xs font-medium text-ink">{project.name}</span>
                  <span className="mt-0.5 block text-[11px] text-ink-3">
                    {projectStatusLabel(project.status)}
                    {project.active_task_count > 0
                      ? ` · 活跃任务 ${project.active_task_count}`
                      : null}
                  </span>
                </Link>
              </li>
            ))}
            {projectCount > boundProjects.length ? (
              <li className="text-[11px] text-ink-3">
                另有 {projectCount - boundProjects.length} 个关联项目未展示
              </li>
            ) : null}
          </ul>
        )}
      </div>

      <div className="border-t border-line pt-3">
        <div className="flex items-start justify-between gap-2">
          <div className="flex min-w-0 items-start gap-2">
            <IconTile size="sm" tone={headlineTone}>
              <Network />
            </IconTile>
            <div className="min-w-0">
              <p className="text-sm font-semibold text-ink">项目调度</p>
              <p className="mt-0.5 text-[11px] text-ink-3">Runtime 由项目运行准备决定。</p>
            </div>
          </div>
          <StatusPill tone={headlineTone}>
            {readinessLoading ? "检查中" : readinessError ? "加载失败" : headline}
          </StatusPill>
        </div>

        {readinessLoading ? (
          <p className="mt-2 text-xs text-ink-3">正在检查调度就绪度</p>
        ) : readinessError ? (
          <div className="mt-2 space-y-2">
            <p className="text-xs text-destructive">调度就绪度加载失败</p>
            <Button onClick={onRetryReadiness} size="sm" type="button" variant="outline">
              重试
            </Button>
          </div>
        ) : readiness ? (
          <div className="mt-2 space-y-2">
            {expandChecks ? (
              <div className="max-h-40 space-y-1.5 overflow-y-auto">
                {readiness.checks.map((check) => (
                  <ReadinessCheckRow check={check} key={check.code} />
                ))}
              </div>
            ) : (
              <p className="text-xs text-ink-2">当前已满足项目调度条件，可纳入项目调度池。</p>
            )}
            {projectCount > 0 ? (
              <a
                className="inline-flex text-xs font-medium text-brand underline-offset-2 hover:underline"
                href="#employee-bound-projects"
              >
                查看绑定项目
              </a>
            ) : null}
          </div>
        ) : (
          <p className="mt-2 text-xs text-ink-3">暂无调度就绪度数据</p>
        )}
      </div>
    </SoftCard>
  );
}

function CapabilityLine({
  label,
  value,
  isLoading,
  isError
}: {
  label: string;
  value: string;
  isLoading: boolean;
  isError: boolean;
}) {
  return (
    <div className="flex items-center justify-between gap-2">
      <span className="shrink-0 text-ink-3">{label}</span>
      {isLoading ? (
        <span className="text-ink-3">加载中</span>
      ) : isError ? (
        <span className="text-destructive">加载失败</span>
      ) : (
        <span className="text-end tabular-nums">{value}</span>
      )}
    </div>
  );
}

function ReadinessCheckRow({ check }: { check: SchedulingReadinessCheck }) {
  const Icon = statusIcon[check.status];
  return (
    <div className="flex items-start gap-2 rounded-xl border border-line bg-card-soft px-2.5 py-1.5">
      <IconTile className="mt-0.5" size="sm" tone={checkTone[check.status]}>
        <Icon />
      </IconTile>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-1.5">
          <p className="text-xs font-semibold text-ink">{check.label}</p>
          <StatusPill tone={checkTone[check.status]}>{checkLabel[check.status]}</StatusPill>
        </div>
        <p className="mt-0.5 text-[11px] leading-4 text-ink-2">{check.message}</p>
      </div>
    </div>
  );
}

function budgetLimitLabel(budgetPolicy?: BudgetPolicy): string {
  const limit = budgetPolicy?.daily_token_limit;
  return typeof limit === "number" ? limit.toLocaleString("zh-CN") : "未设置";
}

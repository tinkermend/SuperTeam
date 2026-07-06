import { Link } from "@tanstack/react-router";
import { AlertTriangle, CheckCircle2, Info, Network } from "lucide-react";
import { IconTile, SoftCard, StatusPill, V3Button, type V3Tone } from "@/components/superteam";
import type {
  DigitalEmployeeSchedulingReadiness,
  SchedulingReadinessCheck,
} from "@/lib/api/employees";

type SchedulingReadinessPanelProps = {
  readiness: DigitalEmployeeSchedulingReadiness | undefined;
  isLoading: boolean;
  isError: boolean;
  onRetry: () => void;
};

const checkTone: Record<SchedulingReadinessCheck["status"], V3Tone> = {
  passed: "ok",
  warning: "warn",
  blocked: "danger",
  info: "info",
};

const checkLabel: Record<SchedulingReadinessCheck["status"], string> = {
  passed: "通过",
  warning: "需关注",
  blocked: "阻塞",
  info: "信息",
};

const statusIcon = {
  passed: CheckCircle2,
  warning: AlertTriangle,
  blocked: AlertTriangle,
  info: Info,
};

export function SchedulingReadinessPanel({
  readiness,
  isLoading,
  isError,
  onRetry,
}: SchedulingReadinessPanelProps) {
  const ready = readiness?.ready_for_project_scheduling === true;
  const headline = ready ? "可进入项目调度池" : "暂不可进入项目调度池";
  const headlineTone: V3Tone = isError ? "danger" : isLoading ? "mute" : ready ? "ok" : "danger";
  const capabilities = readiness?.capabilities;
  const skillCount =
    (capabilities?.skills.personal_count ?? 0) + (capabilities?.skills.inherited_count ?? 0);
  const mcpCount =
    (capabilities?.mcp_servers.personal_count ?? 0) +
    (capabilities?.mcp_servers.inherited_count ?? 0);
  const envCount = capabilities?.environment_variables.configured_count ?? 0;
  const missingSkillNames = capabilities?.skills.missing_required ?? [];
  const missingEnvNames = capabilities?.environment_variables.missing_names ?? [];

  return (
    <SoftCard className="flex flex-col gap-5 p-5">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-3">
          <IconTile size="sm" tone={headlineTone}>
            <Network />
          </IconTile>
          <div className="min-w-0">
            <h2 className="text-base font-semibold text-v3-ink">项目调度就绪度</h2>
            <p className="mt-1 text-xs leading-5 text-v3-ink-2">
              员工只提供可调度能力与上下文，实际 Runtime 选择留在项目运行时检查中处理。
            </p>
          </div>
        </div>
        <StatusPill tone={headlineTone}>{isLoading ? "检查中" : isError ? "加载失败" : headline}</StatusPill>
      </div>

      {isLoading ? (
        <p className="text-sm text-v3-ink-2">正在检查项目调度就绪度</p>
      ) : isError ? (
        <div className="space-y-3">
          <p className="text-sm text-destructive">项目调度就绪度加载失败</p>
          <V3Button onClick={onRetry} size="sm" type="button" variant="outline">
            重试
          </V3Button>
        </div>
      ) : readiness ? (
        <>
          <section className="space-y-2">
            <p className="text-xs font-semibold text-v3-ink-3">检查结果</p>
            <div className="space-y-2">
              {readiness.checks.map((check) => (
                <ReadinessCheckRow check={check} key={check.code} />
              ))}
            </div>
          </section>

          <section className="space-y-2">
            <p className="text-xs font-semibold text-v3-ink-3">可调度能力</p>
            <div className="flex flex-wrap gap-2">
              <StatusPill showDot={false} tone="info">
                技能 {skillCount}
              </StatusPill>
              <StatusPill showDot={false} tone="info">
                MCP {mcpCount}
              </StatusPill>
              <StatusPill showDot={false} tone={missingEnvNames.length ? "warn" : "info"}>
                环境变量 {envCount}
              </StatusPill>
            </div>
            {missingSkillNames.length ? (
              <MissingNames label="缺失必需技能" names={missingSkillNames} />
            ) : null}
            {missingEnvNames.length ? (
              <MissingNames label="缺失环境变量" names={missingEnvNames} />
            ) : null}
          </section>

          <section className="rounded-2xl border border-v3-line bg-v3-card-soft p-3">
            <p className="text-sm font-semibold text-v3-ink">
              {ready ? "当前已满足项目调度条件" : "当前尚未满足项目调度条件"}
            </p>
            <p className="mt-1 text-xs leading-5 text-v3-ink-2">
              {ready
                ? "可以把该员工纳入项目调度池；实际执行来源会在项目运行时就绪度中选择 Runtime 节点。"
                : "需先处理阻塞检查项，再把该员工纳入项目调度池；Runtime 节点仍由项目运行时就绪度决定。"}
            </p>
            <V3Button asChild className="mt-3" size="sm" variant="outline">
              <Link to="/projects">进入项目</Link>
            </V3Button>
          </section>
        </>
      ) : (
        <p className="text-sm text-v3-ink-2">暂无项目调度就绪度数据</p>
      )}
    </SoftCard>
  );
}

function ReadinessCheckRow({ check }: { check: SchedulingReadinessCheck }) {
  const Icon = statusIcon[check.status];

  return (
    <div className="flex items-start gap-2 rounded-2xl border border-v3-line bg-v3-card-soft px-3 py-2">
      <IconTile className="mt-0.5" size="sm" tone={checkTone[check.status]}>
        <Icon />
      </IconTile>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <p className="text-sm font-semibold text-v3-ink">{check.label}</p>
          <StatusPill tone={checkTone[check.status]}>{checkLabel[check.status]}</StatusPill>
        </div>
        <p className="mt-1 text-xs leading-5 text-v3-ink-2">{check.message}</p>
      </div>
    </div>
  );
}

function MissingNames({ label, names }: { label: string; names: string[] }) {
  return (
    <div className="space-y-1.5">
      <p className="text-xs text-v3-ink-3">{label}</p>
      <div className="flex flex-wrap gap-1.5">
        {names.map((name) => (
          <StatusPill key={name} tone="danger">
            {name}
          </StatusPill>
        ))}
      </div>
    </div>
  );
}

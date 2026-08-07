import { Link } from "@tanstack/react-router";
import { GlassCard, StatusPill, Button } from "@/components/superteam";
import type { RuntimeOverviewEmployee, RuntimeOverviewTeam } from "../runtime-overview-model";
import { formatDurationSince, formatNumber, formatTime } from "../formatters";
import { providerDisplayName } from "@/features/employees/provider-label";
import {
  employeeStatusDotClass as statusDotClass,
  employeeStatusLabel as statusLabel,
  employeeStatusTone as statusTone
} from "../status-maps";

type EmployeeDetailCardProps = {
  employee: RuntimeOverviewEmployee;
  team?: RuntimeOverviewTeam;
};

const statusDurationVerb: Record<RuntimeOverviewEmployee["status"], string> = {
  error: "已持续",
  idle: "已空闲",
  needs_configuration: "已持续",
  queued: "已排队",
  unavailable: "已持续",
  waiting_human: "已等待",
  working: "已运行"
};

// 运行快照卡：10 秒驻留内回答"什么状态、为什么、持续多久、在为哪个项目干什么、正在发生什么、消耗如何"。
// 横向平铺在地图正下方；内部按卡片自身宽度自适应：宽三列、中两列、窄单列堆叠。
export function EmployeeDetailCard({ employee, team }: EmployeeDetailCardProps) {
  return (
    <GlassCard className="@container p-4" data-employee-detail-card>
      <div className="grid grid-cols-1 items-start gap-x-6 gap-y-5 @xl:grid-cols-2 @5xl:grid-cols-3">
        <IdentityBlock employee={employee} team={team} />
        <div className="flex min-w-0 flex-col gap-4">
          <StatusBlock employee={employee} />
          <CurrentTaskBlock employee={employee} />
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <EventsBlock employee={employee} />
          <UsageBlock employee={employee} />
        </div>
      </div>
    </GlassCard>
  );
}

function IdentityBlock({ employee, team }: { employee: RuntimeOverviewEmployee; team?: RuntimeOverviewTeam }) {
  return (
    <div className="flex min-w-0 flex-col gap-4">
      <div className="flex items-start gap-3">
        {employee.avatarAsset?.url ? (
          <img className="size-12 rounded-full object-cover" src={employee.avatarAsset.url} alt="" />
        ) : (
          <div className="grid size-12 place-items-center rounded-full bg-brand-soft font-semibold text-brand-deep">
            {employee.name.slice(0, 1)}
          </div>
        )}
        <div className="min-w-0 flex-1">
          <h2 className="truncate text-lg font-semibold text-ink">{employee.name}</h2>
          <p className="text-sm text-ink-2">{employee.roleLabel}</p>
        </div>
        <StatusPill tone={statusTone(employee.status)}>{statusLabel[employee.status]}</StatusPill>
      </div>
      <div className="flex flex-wrap items-center gap-2 text-sm">
        {team ? (
          <span className="rounded-lg bg-brand-soft px-2.5 py-1 font-semibold text-brand-deep">{team.name}</span>
        ) : (
          <span className="rounded-lg bg-card-soft px-2.5 py-1 font-medium text-ink-2">未归属团队</span>
        )}
        {employee.projectCount > 0 ? (
          <span className="text-ink-2 tabular-nums" data-employee-project-count>
            关联 {employee.projectCount} 个项目
          </span>
        ) : null}
      </div>
      <p className="text-xs text-ink-3">
        Runtime：{employee.runtime?.nodeId ?? "未绑定节点"} · {employee.runtime?.providerType ? providerDisplayName(employee.runtime.providerType) : "未配置 Provider"}
      </p>
      <div className="flex flex-wrap gap-2">
        <Button asChild size="sm" variant="glass">
          <Link params={{ employeeId: employee.employeeId }} to="/employees/$employeeId">
            查看员工详情
          </Link>
        </Button>
        {employee.runtime?.nodeId ? (
          <Button asChild size="sm" variant="glass">
            <Link search={{ node: employee.runtime.nodeId }} to="/runtime">
              查看 Runtime 节点
            </Link>
          </Button>
        ) : null}
      </div>
    </div>
  );
}

function StatusBlock({ employee }: { employee: RuntimeOverviewEmployee }) {
  const showError = employee.status === "error" && employee.latestRunErrorMessage;
  return (
    <div className="min-w-0">
      <div className="text-xs font-semibold text-ink-2">运行状态</div>
      <div className="glass-inner mt-2 px-3 py-3 text-sm" data-employee-status-block>
        <div className="flex flex-wrap items-center gap-2">
          <span className={`size-2.5 rounded-full ${statusDotClass[employee.status]}`} aria-hidden />
          <span className="font-semibold text-ink">{statusLabel[employee.status]}</span>
          {employee.statusSince ? (
            <span className="text-ink-3 tabular-nums">
              · {statusDurationVerb[employee.status]} {formatDurationSince(employee.statusSince)}
            </span>
          ) : null}
        </div>
        {employee.statusReasons.map((reason) => (
          <p key={reason} className="mt-1.5 text-ink-2">
            {reason}
          </p>
        ))}
        {showError ? <p className="mt-1.5 line-clamp-2 break-all text-danger">{employee.latestRunErrorMessage}</p> : null}
      </div>
    </div>
  );
}

// 当前任务所属项目:优先用后端权威 current_work(currentTask.projectId,保证与正在做的
// 那件任务同源),回退到项目聚合启发式(workingTaskCount>0 → activeTaskCount>0)。
function currentProjectOf(employee: RuntimeOverviewEmployee): { projectId: string; name: string } | undefined {
  if (employee.currentTask?.projectId) {
    return {
      projectId: employee.currentTask.projectId,
      name: employee.currentTask.projectName?.trim() || employee.currentTask.projectId
};
  }
  const heuristic =
    employee.projects.find((project) => project.workingTaskCount > 0) ??
    employee.projects.find((project) => project.activeTaskCount > 0);
  return heuristic ? { projectId: heuristic.projectId, name: heuristic.name } : undefined;
}

function CurrentTaskBlock({ employee }: { employee: RuntimeOverviewEmployee }) {
  const currentProject = currentProjectOf(employee);
  return (
    <div className="min-w-0">
      <div className="text-xs font-semibold text-ink-2">当前任务</div>
      <div className="glass-inner mt-2 px-3 py-3">
        <div className="flex items-start justify-between gap-3">
          <p className="min-w-0 text-sm font-semibold text-ink">{employee.currentTask?.title ?? "暂无进行中的任务"}</p>
          {employee.currentTask?.priority === "high" ? (
            <span className="shrink-0 rounded-lg bg-danger-soft px-2 py-1 text-xs font-semibold text-danger">优先级：高</span>
          ) : null}
        </div>
        {currentProject ? (
          <p className="mt-2 flex items-center gap-1.5 text-sm" data-employee-current-project>
            <span className="text-ink-3">所属项目</span>
            <Link
              params={{ projectId: currentProject.projectId }}
              to="/projects/$projectId"
              className="min-w-0 truncate font-medium text-brand hover:underline"
            >
              {currentProject.name}
            </Link>
          </p>
        ) : null}
      </div>
    </div>
  );
}

function EventsBlock({ employee }: { employee: RuntimeOverviewEmployee }) {
  return (
    <div className="min-w-0">
      <div className="text-xs font-semibold text-ink-2">命令 / 日志</div>
      <ul className="mt-2 space-y-2">
        {employee.recentEvents.slice(0, 3).map((event, index) => (
          <li key={`${event.label}-${index}`} className="flex items-center gap-2 rounded-lg bg-card-soft px-3 py-2 text-sm text-ink-2">
            <span className={`size-2 rounded-full ${statusDotClass[employee.status]}`} aria-hidden />
            <span className="min-w-0 flex-1 truncate">{event.label}</span>
            {event.occurredAt ? <span className="shrink-0 text-xs tabular-nums text-ink-3">{formatTime(event.occurredAt)}</span> : null}
          </li>
        ))}
      </ul>
    </div>
  );
}

function UsageBlock({ employee }: { employee: RuntimeOverviewEmployee }) {
  const dailyTokens = employee.usage?.dailyTokens ?? 0;
  const dailyLimit = employee.usage?.dailyTokenLimit ?? 0;
  const usagePercent = dailyLimit > 0 ? Math.min(100, Math.round((dailyTokens / dailyLimit) * 100)) : 0;
  return (
    <div className="min-w-0">
      <div className="text-xs font-semibold text-ink-2">消耗情况</div>
      <div className="glass-inner mt-2 px-3 py-3 text-sm text-ink-2">
        <div className="flex items-center justify-between">
          <span>今日累计</span>
          <span className="font-semibold text-ink tabular-nums">
            {formatNumber(dailyTokens)}
            {dailyLimit > 0 ? ` / ${formatNumber(dailyLimit)}` : ""} tokens
          </span>
        </div>
        {dailyLimit > 0 ? (
          <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-[color:var(--aurora-hairline)]">
            <span className="block h-full rounded-full bg-brand" style={{ width: `${usagePercent}%` }} />
          </div>
        ) : null}
      </div>
    </div>
  );
}

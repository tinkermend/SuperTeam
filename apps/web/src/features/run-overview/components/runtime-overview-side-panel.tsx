import { Link } from "@tanstack/react-router";
import { SoftCard, StatusPill } from "@/components/superteam";
import type { RuntimeOverviewDTO, RuntimeOverviewEmployee } from "../runtime-overview-model";

const statusLabel: Record<RuntimeOverviewEmployee["status"], string> = {
  error: "异常",
  idle: "空闲",
  needs_configuration: "待配置",
  queued: "排队",
  unavailable: "不可用",
  waiting_human: "待确认",
  working: "工作中",
};

const statusDotClass: Record<RuntimeOverviewEmployee["status"], string> = {
  error: "bg-v3-danger",
  idle: "bg-v3-mute",
  needs_configuration: "bg-v3-warn",
  queued: "bg-v3-info",
  unavailable: "bg-v3-mute",
  waiting_human: "bg-v3-warn",
  working: "bg-v3-ok",
};

function statusTone(status: RuntimeOverviewEmployee["status"]) {
  if (status === "error") return "danger";
  if (status === "working") return "ok";
  if (status === "waiting_human" || status === "needs_configuration" || status === "queued") return "warn";
  return "mute";
}

export function RuntimeOverviewSidePanel({ overview, selectedEmployeeId }: { overview: RuntimeOverviewDTO; selectedEmployeeId?: string }) {
  const selected = overview.employees.find((employee) => employee.employeeId === selectedEmployeeId) ?? overview.employees[0];
  const selectedTeam = selected ? overview.teams.find((team) => team.teamId === selected.teamId) : undefined;
  const activeFloor = overview.floors.find((floor) => floor.floorId === overview.activeFloorId);
  const otherFloors = overview.floors.filter((floor) => floor.floorId !== overview.activeFloorId && floor.summary.teamCount > 0);
  const dailyTokens = selected?.usage?.dailyTokens ?? 0;
  const dailyLimit = selected?.usage?.dailyTokenLimit ?? 0;
  const usagePercent = dailyLimit > 0 ? Math.min(100, Math.round((dailyTokens / dailyLimit) * 100)) : 0;
  return (
    <div className="flex min-w-0 flex-col gap-4">
      <SoftCard className="p-4">
        <div className="flex items-center justify-between gap-3">
          <h2 className="text-sm font-semibold text-v3-ink">运行概况</h2>
          <span className="rounded-full bg-v3-ok-soft px-2 py-1 text-xs font-semibold text-v3-ok">实时读取</span>
        </div>
        <div className="mt-4 grid grid-cols-2 gap-3">
          <Metric label="团队" value={overview.summary.teamCount} />
          <Metric label="数字员工" value={overview.summary.employeeCount} />
          <Metric label="容量使用" value={`${overview.summary.capacityUsed}/${overview.summary.capacityTotal}`} />
          <Metric label="异常" value={overview.summary.errorCount} tone={overview.summary.errorCount > 0 ? "danger" : undefined} />
        </div>
        <div className="mt-4 space-y-3">
          <div>
            <div className="mb-2 flex items-center justify-between text-xs text-v3-ink-2">
              <span>容量使用</span>
              <span className="font-semibold text-v3-ink tabular-nums">
                {overview.summary.capacityUsed} / {overview.summary.capacityTotal}
              </span>
            </div>
            <div className="h-1.5 overflow-hidden rounded-full bg-v3-card-soft">
              <span
                className="block h-full rounded-full bg-v3-brand"
                style={{
                  width: `${overview.summary.capacityTotal > 0 ? Math.min(100, Math.round((overview.summary.capacityUsed / overview.summary.capacityTotal) * 100)) : 0}%`,
                }}
              />
            </div>
          </div>
          <StatusRow label="正在工作" value={overview.summary.workingCount} status="working" />
          <StatusRow label="空闲" value={overview.summary.idleCount} status="idle" />
          <StatusRow label="待人工确认" value={overview.summary.waitingHumanCount} status="waiting_human" />
          <StatusRow label="异常" value={overview.summary.errorCount} status="error" />
        </div>
        {otherFloors.length > 0 ? (
          <div className="mt-4 border-t border-v3-line pt-4">
            <div className="mb-2 text-xs font-semibold text-v3-ink">其他楼层</div>
            <div className="space-y-2">
              {otherFloors.map((floor) => (
                <div key={floor.floorId} className="flex items-center justify-between text-sm text-v3-ink-2">
                  <span>{floor.label}</span>
                  <span className="tabular-nums">
                    {floor.summary.teamCount} 团队 · 异常 {floor.summary.errorCount}
                  </span>
                </div>
              ))}
            </div>
          </div>
        ) : activeFloor ? (
          <p className="mt-4 border-t border-v3-line pt-4 text-xs text-v3-ink-2">{activeFloor.label} 已展示当前可见团队。</p>
        ) : null}
      </SoftCard>
      {selected ? (
        <SoftCard className="p-4">
          <div className="flex items-start gap-3">
            {selected.avatarAsset?.url ? (
              <img className="size-12 rounded-full object-cover" src={selected.avatarAsset.url} alt="" />
            ) : (
              <div className="grid size-12 place-items-center rounded-full bg-v3-brand-soft font-semibold text-v3-brand-deep">
                {selected.name.slice(0, 1)}
              </div>
            )}
            <div className="min-w-0 flex-1">
              <h2 className="truncate text-lg font-semibold text-v3-ink">{selected.name}</h2>
              <p className="text-sm text-v3-ink-2">{selected.roleLabel}</p>
            </div>
            <StatusPill tone={statusTone(selected.status)}>{statusLabel[selected.status]}</StatusPill>
          </div>
          {selectedTeam ? (
            <div className="mt-4 flex flex-wrap items-center gap-2 text-sm">
              <span className="text-v3-ink-2">所在团队</span>
              <span className="rounded-lg bg-v3-brand-soft px-2.5 py-1 font-semibold text-v3-brand-deep">{selectedTeam.name}</span>
            </div>
          ) : null}
          <div className="mt-5">
            <div className="text-xs font-semibold text-v3-ink-2">当前任务</div>
            <div className="mt-2 rounded-v3-inner border border-v3-line bg-white px-3 py-3">
              <div className="flex items-start justify-between gap-3">
                <p className="min-w-0 text-sm font-semibold text-v3-ink">{selected.currentTask?.title ?? "暂无进行中的任务"}</p>
                {selected.currentTask?.priority === "high" ? <span className="shrink-0 rounded-lg bg-v3-danger-soft px-2 py-1 text-xs font-semibold text-v3-danger">优先级：高</span> : null}
              </div>
              {selected.currentTask?.taskId ? <p className="mt-2 truncate font-mono text-xs text-v3-ink-3">{selected.currentTask.taskId}</p> : null}
            </div>
          </div>
          <div className="mt-5">
            <div className="text-xs font-semibold text-v3-ink-2">Runtime</div>
            <p className="mt-1 text-sm text-v3-ink-2">
              {selected.runtime?.nodeId ?? "未绑定节点"} · {selected.runtime?.providerType ?? "未配置 Provider"}
            </p>
          </div>
          <div className="mt-4 flex flex-wrap gap-2">
            <Link
              className="inline-flex h-9 items-center rounded-v3-button border border-v3-line bg-white px-3 text-xs font-semibold text-v3-ink shadow-sm transition-colors hover:bg-v3-card-soft"
              params={{ employeeId: selected.employeeId }}
              to="/employees/$employeeId"
            >
              查看员工详情
            </Link>
            {selected.runtime?.nodeId ? (
              <Link
                className="inline-flex h-9 items-center rounded-v3-button border border-v3-line bg-white px-3 text-xs font-semibold text-v3-ink shadow-sm transition-colors hover:bg-v3-card-soft"
                search={{ node: selected.runtime.nodeId }}
                to="/runtime"
              >
                查看 Runtime 节点
              </Link>
            ) : null}
          </div>
          <div className="mt-5">
            <div className="text-xs font-semibold text-v3-ink-2">命令 / 日志</div>
            <ul className="mt-2 space-y-2">
              {selected.recentEvents.slice(0, 3).map((event, index) => (
                <li key={`${event.label}-${index}`} className="flex items-center gap-2 rounded-lg bg-v3-card-soft px-3 py-2 text-sm text-v3-ink-2">
                  <span className={`size-2 rounded-full ${statusDotClass[selected.status]}`} aria-hidden />
                  <span className="min-w-0 flex-1 truncate">{event.label}</span>
                  {event.occurredAt ? <span className="shrink-0 text-xs tabular-nums text-v3-ink-3">{formatTime(event.occurredAt)}</span> : null}
                </li>
              ))}
            </ul>
          </div>
          <div className="mt-5">
            <div className="text-xs font-semibold text-v3-ink-2">消耗情况</div>
            <div className="mt-2 rounded-v3-inner bg-v3-card-soft px-3 py-3 text-sm text-v3-ink-2">
              <div className="flex items-center justify-between">
                <span>今日累计</span>
                <span className="font-semibold text-v3-ink tabular-nums">
                  {formatNumber(dailyTokens)}
                  {dailyLimit > 0 ? ` / ${formatNumber(dailyLimit)}` : ""} tokens
                </span>
              </div>
              {dailyLimit > 0 ? (
                <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-white">
                  <span className="block h-full rounded-full bg-v3-brand" style={{ width: `${usagePercent}%` }} />
                </div>
              ) : null}
            </div>
          </div>
        </SoftCard>
      ) : null}
    </div>
  );
}

function Metric({ label, value, tone }: { label: string; value: number | string; tone?: "danger" }) {
  return (
    <div className="rounded-v3-inner border border-v3-line bg-v3-card-soft p-3">
      <div className="text-xs text-v3-ink-2">{label}</div>
      <div className={`mt-1 text-2xl font-semibold tabular-nums ${tone === "danger" ? "text-v3-danger" : "text-v3-ink"}`}>{value}</div>
    </div>
  );
}

function StatusRow({ label, status, value }: { label: string; status: RuntimeOverviewEmployee["status"]; value: number }) {
  return (
    <div className="flex items-center justify-between text-sm">
      <span className="inline-flex items-center gap-2 text-v3-ink-2">
        <span className={`size-2.5 rounded-full ${statusDotClass[status]}`} aria-hidden />
        {label}
      </span>
      <span className="font-semibold text-v3-ink tabular-nums">{value}</span>
    </div>
  );
}

function formatNumber(value: number) {
  return new Intl.NumberFormat("zh-CN").format(value);
}

function formatTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return new Intl.DateTimeFormat("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

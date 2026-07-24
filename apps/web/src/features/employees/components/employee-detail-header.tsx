import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { FileClock, MoreHorizontal, Settings, Trash2 } from "lucide-react";
import {
  SoftCard,
  StatusPill,
  Button,
  type Tone
} from "@/components/superteam";
import {
  Dialog,
  DialogContent,
  DialogTitle
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger
} from "@/components/ui/dropdown-menu";
import type { DigitalEmployee, DigitalEmployeeRunStats } from "@/lib/api/employees";
import { employeeStatusLabel } from "@/lib/status-labels";
import { EmployeeAvatar } from "../avatar";
import { employeeAvatarAsset } from "../avatar-library";
import { operationalStatusPresentation } from "../operational-status";
import { providerDisplayName } from "../provider-label";

type EmployeeDetailHeaderProps = {
  employee: DigitalEmployee;
  stats?: DigitalEmployeeRunStats;
  onDelete?: () => void;
};

const identityStatusTone: Record<string, Tone> = {
  active: "ok",
  ready: "info",
  disabled: "mute",
  archived: "mute",
  error: "danger"
};

export function EmployeeDetailHeader({
  employee,
  stats,
  onDelete
}: EmployeeDetailHeaderProps) {
  const [avatarPreviewOpen, setAvatarPreviewOpen] = useState(false);
  const avatarAsset = employeeAvatarAsset(employee);
  const previewUrl = avatarAsset.image_url || avatarAsset.thumbnail_url;
  const canDelete = Boolean(onDelete && employee.allowed_actions?.includes("employee.delete"));
  // 运行态为主状态(与列表/总览同源);身份生命周期降为次要标注,避免「就绪+异常」并排歧义。
  const operationalStatus = employee.operational_state
    ? operationalStatusPresentation(employee.operational_state.status)
    : null;
  const operationalReasons = employee.operational_state?.reasons ?? [];
  const showOperationalReasons =
    operationalStatus &&
    (employee.operational_state?.status === "error" ||
      employee.operational_state?.status === "waiting_human") &&
    operationalReasons.length > 0;

  return (
    <SoftCard className="flex shrink-0 flex-col gap-3 p-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex min-w-0 items-center gap-3">
          <button
            aria-label={`查看 ${employee.name} 的大图头像`}
            className="shrink-0 cursor-pointer rounded-full outline-none transition-[box-shadow,transform] hover:scale-[1.03] hover:ring-2 hover:ring-brand/40 focus-visible:ring-2 focus-visible:ring-brand"
            onClick={() => setAvatarPreviewOpen(true)}
            type="button"
          >
            <EmployeeAvatar asset={avatarAsset} name={employee.name} size="detail" />
          </button>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="truncate text-lg font-extrabold tracking-tight text-ink">
                {employee.name}
              </h2>
              {operationalStatus ? (
                <StatusPill tone={operationalStatus.tone}>
                  {`运行：${operationalStatus.label}`}
                </StatusPill>
              ) : null}
              <StatusPill tone={identityStatusTone[employee.status] ?? "mute"}>
                {`身份：${employeeStatusLabel(employee.status)}`}
              </StatusPill>
            </div>
            {showOperationalReasons ? (
              <p className="mt-1 text-[13px] text-danger">
                {operationalReasons.map((reason) => reason.message).join("；")}
              </p>
            ) : null}
            <p className="mt-1 text-[13px] text-ink-2">
              <span className="font-medium text-ink">{employee.role}</span>
              <span className="text-ink-3"> · Provider {providerDisplayName(employee.provider_type)}</span>
            </p>
            <p className="mt-0.5 text-[13px] text-ink-2">
              <span className="text-ink-3">团队 · </span>
              {employee.team_id && employee.team_name?.trim() ? (
                <Link
                  className="font-medium text-brand underline-offset-2 hover:underline"
                  params={{ teamId: employee.team_id }}
                  to="/teams/$teamId"
                >
                  {employee.team_name.trim()}
                </Link>
              ) : (
                <span className="font-medium text-ink">
                  {employee.team_name?.trim() || "无团队归属"}
                </span>
              )}
            </p>
            {employee.description ? (
              <p className="mt-0.5 line-clamp-1 text-[13px] leading-5 text-ink-3">{employee.description}</p>
            ) : (
              <p className="mt-0.5 text-[13px] text-ink-3">尚未填写职责说明，可在配置页补充。</p>
            )}
          </div>
        </div>
        <div className="flex shrink-0 flex-wrap items-center gap-2">
          <Button asChild variant="primary">
            <Link params={{ employeeId: employee.id }} to="/employees/$employeeId/config">
              <Settings className="size-4" />
              编辑配置
            </Link>
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button aria-label="更多员工操作" type="button" variant="outline">
                <MoreHorizontal className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem asChild>
                <Link to="/audit">
                  <FileClock className="size-4" />
                  查看审计
                </Link>
              </DropdownMenuItem>
              {canDelete ? (
                <DropdownMenuItem onSelect={onDelete} variant="destructive">
                  <Trash2 className="size-4" />
                  删除员工
                </DropdownMenuItem>
              ) : null}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      <dl
        aria-label="工作节奏摘要"
        className="flex flex-wrap gap-x-4 gap-y-1 border-t border-line pt-2.5 text-[12px] text-ink-2"
      >
        <div className="flex items-baseline gap-1.5">
          <dt className="text-ink-3">近7天</dt>
          <dd className="font-semibold tabular-nums text-ink">
            {stats ? stats.last_7d_count : "—"}
          </dd>
          {stats ? <span className="text-ink-3">{formatTrend(stats)}</span> : null}
        </div>
        <div className="flex items-baseline gap-1.5">
          <dt className="text-ink-3">成功率</dt>
          <dd className="font-semibold tabular-nums text-ink">
            {stats?.success_rate != null ? formatPercent(stats.success_rate) : "—"}
          </dd>
        </div>
        <div className="flex items-baseline gap-1.5">
          <dt className="text-ink-3">平均耗时</dt>
          <dd className="font-semibold tabular-nums text-ink">
            {stats?.avg_duration_sec != null ? formatDuration(stats.avg_duration_sec) : "—"}
          </dd>
        </div>
        <div className="flex items-baseline gap-1.5">
          <dt className="text-ink-3">失败</dt>
          <dd className="font-semibold tabular-nums text-ink">
            {stats ? stats.failed_count : "—"}
          </dd>
          {stats ? (
            <span className="text-ink-3">
              成功 {stats.succeeded_count} · 累计 {stats.total_count}
            </span>
          ) : null}
        </div>
      </dl>

      <Dialog onOpenChange={setAvatarPreviewOpen} open={avatarPreviewOpen}>
        <DialogContent
          aria-describedby={undefined}
          className="max-w-fit gap-3 border-none bg-transparent p-0 shadow-none sm:max-w-fit"
          showCloseButton
        >
          <DialogTitle className="sr-only">{employee.name} 的头像</DialogTitle>
          <img
            alt={`${employee.name} 的大图头像`}
            className="max-h-[min(80vh,36rem)] max-w-[min(90vw,28rem)] rounded-2xl object-cover shadow-card"
            src={previewUrl}
          />
        </DialogContent>
      </Dialog>
    </SoftCard>
  );
}

function formatPercent(ratio: number): string {
  return `${(ratio * 100).toFixed(1)}%`;
}

function formatDuration(seconds: number): string {
  const totalSeconds = Math.round(seconds);
  const minutes = Math.floor(totalSeconds / 60);
  const remainSeconds = totalSeconds % 60;
  return `${minutes}分${remainSeconds}秒`;
}

function formatTrend(stats: DigitalEmployeeRunStats): string {
  if (stats.prev_7d_count === 0) {
    return `较上周期 +${stats.last_7d_count}`;
  }
  const change = ((stats.last_7d_count - stats.prev_7d_count) / stats.prev_7d_count) * 100;
  const arrow = change >= 0 ? "↑" : "↓";
  return `较上周期 ${arrow}${Math.abs(change).toFixed(0)}%`;
}

import { Link } from "@tanstack/react-router";
import { ArrowLeft, FileClock, Play, Settings, Trash2 } from "lucide-react";
import { StatusPill, V3Button, type V3Tone } from "@/components/superteam";
import type { DigitalEmployee } from "@/lib/api/employees";
import { employeeStatusLabel } from "@/lib/status-labels";
import { EmployeeAvatar } from "../avatar";
import { employeeAvatarAsset } from "../avatar-library";
import { operationalStatusPresentation } from "../operational-status";
import { providerDisplayName } from "../provider-label";

type EmployeeDetailHeaderProps = {
  employee: DigitalEmployee;
  onDelete?: () => void;
  onStartTask: () => void;
};

const identityStatusTone: Record<string, V3Tone> = {
  active: "ok",
  ready: "info",
  disabled: "mute",
  archived: "mute",
  error: "danger",
};

export function EmployeeDetailHeader({
  employee,
  onDelete,
  onStartTask,
}: EmployeeDetailHeaderProps) {
  const avatarAsset = employeeAvatarAsset(employee);
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
    <div className="flex flex-col gap-4 rounded-v3-card border border-v3-line bg-v3-card p-5 shadow-sm lg:flex-row lg:items-center lg:justify-between">
      <div className="flex min-w-0 items-center gap-4">
        <EmployeeAvatar asset={avatarAsset} name={employee.name} size="lg" />
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="truncate text-[22px] font-extrabold tracking-tight text-v3-ink">
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
            <p className="mt-1 text-[13px] text-v3-danger">
              {operationalReasons.map((reason) => reason.message).join("；")}
            </p>
          ) : null}
          <p className="mt-1 truncate text-[13px] text-v3-ink-2">
            数字员工身份 · Provider {providerDisplayName(employee.provider_type)} · 生效上下文与历史执行记录
          </p>
          <p className="mt-1 truncate text-xs text-v3-ink-3">
            角色 {employee.role}
            {employee.description ? ` · ${employee.description}` : ""}
          </p>
        </div>
      </div>
      <div className="flex shrink-0 flex-wrap gap-2">
        <V3Button asChild variant="outline">
          <Link to="/employees">
            <ArrowLeft className="size-4" />
            返回列表
          </Link>
        </V3Button>
        <V3Button asChild variant="outline">
          <Link params={{ employeeId: employee.id }} to="/employees/$employeeId/config">
            <Settings className="size-4" />
            编辑员工配置
          </Link>
        </V3Button>
        <V3Button asChild variant="outline">
          <Link to="/audit">
            <FileClock className="size-4" />
            查看审计
          </Link>
        </V3Button>
        {canDelete ? (
          <V3Button onClick={onDelete} type="button" variant="danger">
            <Trash2 className="size-4" />
            删除员工
          </V3Button>
        ) : null}
        <V3Button onClick={onStartTask} type="button" variant="outline">
          <Play className="size-4" />
          开始任务
        </V3Button>
      </div>
    </div>
  );
}

import type { RuntimeOverviewEmployee } from "../runtime-overview-model";
import { employeeStatusDotClass, employeeStatusLabel } from "../status-maps";

const statusRingClass: Record<RuntimeOverviewEmployee["status"], string> = {
  error: "border-danger",
  idle: "border-white",
  needs_configuration: "border-warn/70",
  queued: "border-info/70",
  unavailable: "border-white",
  waiting_human: "border-warn",
  working: "border-ok"
};

// 光晕仅作运行状态反馈：工作中呼吸脉冲、异常快速脉冲，其余状态保持静态。
const statusHaloClass: Record<RuntimeOverviewEmployee["status"], string | null> = {
  error: "bg-danger/45 motion-safe:animate-ping",
  idle: null,
  needs_configuration: null,
  queued: "bg-info/30 motion-safe:animate-pulse",
  unavailable: null,
  waiting_human: "bg-warn/35 motion-safe:animate-pulse",
  working: "bg-ok/40 motion-safe:animate-ping motion-safe:[animation-duration:2.2s]"
};

// 项目透镜下的头像态：participant 正常、stop 加脉冲标记（当前停留环节）、dimmed 降暗（非参与者）。
export type EmployeeLensState = "participant" | "stop" | "dimmed";

type EmployeeAvatarNodeProps = {
  employee: RuntimeOverviewEmployee;
  selected: boolean;
  x: number;
  y: number;
  onSelect: (employeeId: string) => void;
  lensState?: EmployeeLensState;
};

export function EmployeeAvatarNode({ employee, lensState, onSelect, selected, x, y }: EmployeeAvatarNodeProps) {
  const halo = statusHaloClass[employee.status];
  const dimmed = lensState === "dimmed";
  return (
    <button
      type="button"
      aria-label={`${employee.name}，${employee.roleLabel}，${employeeStatusLabel[employee.status]}`}
      className={`absolute z-40 grid size-12 place-items-center rounded-full border-2 bg-card shadow-card transition-transform hover:scale-105 focus:outline-none focus:ring-2 focus:ring-brand ${statusRingClass[employee.status]} ${dimmed ? "opacity-35 saturate-50" : ""}`}
      style={{ left: x - 24, top: y - 58 }}
      data-employee-id={employee.employeeId}
      data-employee-status={employee.status}
      data-lens-state={lensState}
      onClick={() => onSelect(employee.employeeId)}
    >
      {halo && !dimmed ? <span data-status-halo className={`absolute -inset-1 rounded-full ${halo}`} aria-hidden /> : null}
      {lensState === "stop" ? (
        <span data-lens-stop-marker className="absolute -inset-1.5 rounded-full border-2 border-brand/70 motion-safe:animate-pulse" aria-hidden />
      ) : null}
      <span className={selected ? "absolute -inset-1 rounded-full bg-brand/25" : "absolute -inset-1 rounded-full bg-transparent"} />
      {employee.avatarAsset?.url ? (
        <img className="relative size-10 rounded-full object-cover" src={employee.avatarAsset.url} alt="" />
      ) : (
        <span className="relative grid size-10 place-items-center rounded-full bg-brand-soft text-sm font-semibold text-brand-deep">
          {employee.avatarAsset?.fallbackLabel ?? employee.name.slice(0, 1)}
        </span>
      )}
      <span
        className={`absolute -bottom-0.5 -right-0.5 size-3 rounded-full border-2 border-white ${employeeStatusDotClass[employee.status]}`}
        aria-hidden
      />
      <span className="sr-only">{employeeStatusLabel[employee.status]}</span>
    </button>
  );
}

import type { RuntimeOverviewEmployee } from "../runtime-overview-model";
import { employeeStatusDotClass, employeeStatusLabel } from "../status-maps";

const statusRingClass: Record<RuntimeOverviewEmployee["status"], string> = {
  error: "border-v3-danger",
  idle: "border-white",
  needs_configuration: "border-v3-warn/70",
  queued: "border-v3-info/70",
  unavailable: "border-white",
  waiting_human: "border-v3-warn",
  working: "border-v3-ok",
};

// 光晕仅作运行状态反馈：工作中呼吸脉冲、异常快速脉冲，其余状态保持静态。
const statusHaloClass: Record<RuntimeOverviewEmployee["status"], string | null> = {
  error: "bg-v3-danger/45 motion-safe:animate-ping",
  idle: null,
  needs_configuration: null,
  queued: "bg-v3-info/30 motion-safe:animate-pulse",
  unavailable: null,
  waiting_human: "bg-v3-warn/35 motion-safe:animate-pulse",
  working: "bg-v3-ok/40 motion-safe:animate-ping motion-safe:[animation-duration:2.2s]",
};

type EmployeeAvatarNodeProps = {
  employee: RuntimeOverviewEmployee;
  selected: boolean;
  x: number;
  y: number;
  onSelect: (employeeId: string) => void;
};

export function EmployeeAvatarNode({ employee, onSelect, selected, x, y }: EmployeeAvatarNodeProps) {
  const halo = statusHaloClass[employee.status];
  return (
    <button
      type="button"
      aria-label={`${employee.name}，${employee.roleLabel}，${employeeStatusLabel[employee.status]}`}
      className={`absolute z-40 grid size-12 place-items-center rounded-full border-2 bg-v3-card shadow-v3 transition-transform hover:scale-105 focus:outline-none focus:ring-2 focus:ring-v3-brand ${statusRingClass[employee.status]}`}
      style={{ left: x - 24, top: y - 58 }}
      data-employee-id={employee.employeeId}
      data-employee-status={employee.status}
      onClick={() => onSelect(employee.employeeId)}
    >
      {halo ? <span data-status-halo className={`absolute -inset-1 rounded-full ${halo}`} aria-hidden /> : null}
      <span className={selected ? "absolute -inset-1 rounded-full bg-v3-brand/25" : "absolute -inset-1 rounded-full bg-transparent"} />
      {employee.avatarAsset?.url ? (
        <img className="relative size-10 rounded-full object-cover" src={employee.avatarAsset.url} alt="" />
      ) : (
        <span className="relative grid size-10 place-items-center rounded-full bg-v3-brand-soft text-sm font-semibold text-v3-brand-deep">
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

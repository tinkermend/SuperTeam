import type { RuntimeOverviewEmployee } from "../runtime-overview-model";

const statusClass: Record<RuntimeOverviewEmployee["status"], string> = {
  error: "bg-v3-danger",
  idle: "bg-v3-mute",
  needs_configuration: "bg-v3-warn",
  queued: "bg-v3-info",
  unavailable: "bg-v3-mute",
  waiting_human: "bg-v3-warn",
  working: "bg-v3-ok",
};

const statusLabel: Record<RuntimeOverviewEmployee["status"], string> = {
  error: "异常",
  idle: "空闲",
  needs_configuration: "待配置",
  queued: "排队",
  unavailable: "不可用",
  waiting_human: "待确认",
  working: "工作中",
};

type EmployeeAvatarNodeProps = {
  employee: RuntimeOverviewEmployee;
  selected: boolean;
  x: number;
  y: number;
  onSelect: (employeeId: string) => void;
};

export function EmployeeAvatarNode({ employee, onSelect, selected, x, y }: EmployeeAvatarNodeProps) {
  return (
    <button
      type="button"
      aria-label={`${employee.name}，${employee.roleLabel}，${statusLabel[employee.status]}`}
      className="absolute z-40 grid size-12 place-items-center rounded-full border border-white bg-v3-card shadow-v3 transition-transform hover:scale-105 focus:outline-none focus:ring-2 focus:ring-v3-brand"
      style={{ left: x - 24, top: y - 58 }}
      data-employee-id={employee.employeeId}
      onClick={() => onSelect(employee.employeeId)}
    >
      <span className={selected ? "absolute -inset-1 rounded-full bg-v3-brand/25" : "absolute -inset-1 rounded-full bg-transparent"} />
      {employee.avatarAsset?.url ? (
        <img className="relative size-10 rounded-full object-cover" src={employee.avatarAsset.url} alt="" />
      ) : (
        <span className="relative grid size-10 place-items-center rounded-full bg-v3-brand-soft text-sm font-semibold text-v3-brand-deep">
          {employee.avatarAsset?.fallbackLabel ?? employee.name.slice(0, 1)}
        </span>
      )}
      <span className={`absolute right-1 bottom-1 size-3 rounded-full border-2 border-white ${statusClass[employee.status]}`} />
      <span className="sr-only">{statusLabel[employee.status]}</span>
    </button>
  );
}

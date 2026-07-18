import type { RuntimeOverviewEmployee, RuntimeOverviewTeamWorkspace } from "../runtime-overview-model";
import { EmployeeAvatarNode, type EmployeeLensState } from "./employee-avatar-node";

type LobbyWorkspaceRendererProps = {
  employees: RuntimeOverviewEmployee[];
  onSelectEmployee: (employeeId: string) => void;
  selectedEmployeeId?: string;
  workspace: RuntimeOverviewTeamWorkspace;
  lensStateFor?: (employeeId: string) => EmployeeLensState | undefined;
};

// 候岗区：未归属团队的员工落座区。区别于团队工位——无容量/超员语义、虚线卡面保持安静；
// 座位不够时溢出员工聚合成 +N 徽标（仍可经轮播/侧栏动态选中，只是地图上不占座）。
export function LobbyWorkspaceRenderer({
  employees,
  lensStateFor,
  onSelectEmployee,
  selectedEmployeeId,
  workspace,
}: LobbyWorkspaceRendererProps) {
  if (employees.length === 0) return null;
  const employeesBySeat = new Map(employees.map((employee) => [employee.seatId, employee]));
  const overflowCount = employees.filter((employee) => !employee.seatId).length;
  const lastSeat = workspace.seats[workspace.seats.length - 1];
  return (
    <div className="absolute inset-0">
      <article
        data-runtime-lobby-callout
        className="absolute z-30 w-[196px] rounded-[14px] border border-dashed border-v3-line bg-white/88 p-3 shadow-v3 backdrop-blur"
        style={{ left: workspace.cardAnchor.x, top: workspace.cardAnchor.y }}
      >
        <h3 className="text-sm font-semibold text-v3-ink">候岗区</h3>
        <p className="mt-1 text-xs font-medium text-v3-ink-2">
          待编入团队 <span className="font-semibold text-v3-ink tabular-nums">{employees.length}</span> 名
        </p>
      </article>
      {workspace.seats.map((seat) => (
        <div key={seat.seatId} className="absolute z-20 size-1" style={{ left: seat.x, top: seat.y }} data-runtime-seat="unassigned">
          <span className="sr-only">候岗区工位</span>
        </div>
      ))}
      {workspace.seats.map((seat) => {
        const employee = employeesBySeat.get(seat.seatId);
        if (!employee) return null;
        return (
          <EmployeeAvatarNode
            key={employee.employeeId}
            employee={employee}
            lensState={lensStateFor?.(employee.employeeId)}
            selected={employee.employeeId === selectedEmployeeId}
            x={seat.x}
            y={seat.y}
            onSelect={onSelectEmployee}
          />
        );
      })}
      {overflowCount > 0 && lastSeat ? (
        <span
          data-runtime-lobby-overflow
          className="absolute z-40 grid h-8 min-w-8 place-items-center rounded-full border border-v3-line bg-white/92 px-1.5 text-xs font-semibold text-v3-ink-2 shadow-v3 tabular-nums"
          style={{ left: lastSeat.x - 16, top: lastSeat.y + 26 }}
        >
          +{overflowCount}
        </span>
      ) : null}
    </div>
  );
}

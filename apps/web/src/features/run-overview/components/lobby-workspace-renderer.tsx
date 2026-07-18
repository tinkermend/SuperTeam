import type { RuntimeOverviewEmployee, RuntimeOverviewTeamWorkspace } from "../runtime-overview-model";
import { runtimeOverviewLobbyPositions as lobbyPositions } from "../runtime-overview-layout";
import { EmployeeAvatarNode, type EmployeeLensState } from "./employee-avatar-node";

type LobbyWorkspaceRendererProps = {
  employees: RuntimeOverviewEmployee[];
  onSelectEmployee: (employeeId: string) => void;
  selectedEmployeeId?: string;
  workspace: RuntimeOverviewTeamWorkspace;
  lensStateFor?: (employeeId: string) => EmployeeLensState | undefined;
};

export function LobbyWorkspaceRenderer({
  employees,
  lensStateFor,
  onSelectEmployee,
  selectedEmployeeId,
  workspace,
}: LobbyWorkspaceRendererProps) {
  if (employees.length === 0) return null;
  const visibleEmployees = employees.slice(0, lobbyPositions.length);
  const positions = lobbyPositions.slice(0, visibleEmployees.length);
  const overflowCount = employees.length - visibleEmployees.length;
  const lastPosition = positions[positions.length - 1];
  return (
    <div className="absolute inset-0">
      <article
        data-runtime-lobby-callout
        className="absolute z-30 w-[196px] rounded-[14px] border border-dashed border-v3-line bg-white/88 p-3 shadow-v3 backdrop-blur"
        style={{ left: workspace.cardAnchor.x, top: workspace.cardAnchor.y }}
      >
        <h3 className="text-sm font-semibold text-v3-ink">候岗区</h3>
        <p className="mt-1 text-xs font-medium text-v3-ink-2">
          待编组 <span className="font-semibold text-v3-ink tabular-nums">{employees.length}</span> 名
        </p>
      </article>
      {positions.map((position, index) => (
        <div key={`lobby-position-${index + 1}`} className="absolute z-20 size-1" style={{ left: position.x, top: position.y }} data-runtime-seat="unassigned">
          <span className="sr-only">候岗区工位</span>
        </div>
      ))}
      {visibleEmployees.map((employee, index) => {
        const position = positions[index];
        if (!position) return null;
        return (
          <EmployeeAvatarNode
            key={employee.employeeId}
            employee={employee}
            lensState={lensStateFor?.(employee.employeeId)}
            selected={employee.employeeId === selectedEmployeeId}
            x={position.x}
            y={position.y}
            onSelect={onSelectEmployee}
          />
        );
      })}
      {overflowCount > 0 && lastPosition ? (
        <span
          data-runtime-lobby-overflow
          className="absolute z-40 grid h-8 min-w-8 place-items-center rounded-full border border-v3-line bg-white/92 px-1.5 text-xs font-semibold text-v3-ink-2 shadow-v3 tabular-nums"
          style={{ left: lastPosition.x - 16, top: lastPosition.y + 26 }}
        >
          +{overflowCount}
        </span>
      ) : null}
    </div>
  );
}

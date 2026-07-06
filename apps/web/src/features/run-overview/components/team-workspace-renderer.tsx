import { StatusPill } from "@/components/superteam";
import type { RuntimeOverviewEmployee, RuntimeOverviewTeam, RuntimeOverviewTeamWorkspace } from "../runtime-overview-model";
import { EmployeeAvatarNode } from "./employee-avatar-node";

type TeamWorkspaceRendererProps = {
  employees: RuntimeOverviewEmployee[];
  onSelectEmployee: (employeeId: string) => void;
  selectedEmployeeId?: string;
  selectedTeamId?: string;
  team: RuntimeOverviewTeam;
  workspace: RuntimeOverviewTeamWorkspace;
};

export function TeamWorkspaceRenderer({
  employees,
  onSelectEmployee,
  selectedEmployeeId,
  selectedTeamId,
  team,
  workspace,
}: TeamWorkspaceRendererProps) {
  const employeesBySeat = new Map(employees.map((employee) => [employee.seatId, employee]));
  const selected = selectedTeamId === team.teamId;
  return (
    <div className="absolute inset-0">
      <article
        data-runtime-team-callout={team.teamId}
        className={`absolute z-30 w-[196px] rounded-[14px] border bg-white/94 p-3 shadow-v3 backdrop-blur transition ${selected ? "border-v3-brand ring-2 ring-v3-brand/20" : "border-v3-line opacity-88"}`}
        style={{ left: workspace.cardAnchor.x, top: workspace.cardAnchor.y }}
      >
        <div className="flex items-start justify-between gap-3">
          <h3 className="min-w-0 text-sm font-semibold text-v3-ink">
            <span className="block truncate">{team.name}</span>
            <span className="mt-1 block text-xs font-medium text-v3-ink-2">
              工位 {team.capacityUsed}/{team.capacity}
            </span>
          </h3>
          {team.overCapacity ? <StatusPill tone="danger">超员</StatusPill> : <StatusPill tone="ok">正常</StatusPill>}
        </div>
        <p className="mt-2 text-[11px] text-v3-ink-2">
          异常 <span className="font-semibold text-v3-danger">{team.errorCount}</span> · 工作中{" "}
          <span className="font-semibold text-v3-ok">{team.workingCount}</span> · 待确认{" "}
          <span className="font-semibold text-v3-warn">{team.waitingHumanCount}</span>
        </p>
      </article>
      {workspace.seats.map((seat) => (
        <div key={seat.seatId} className="absolute z-20 size-1" style={{ left: seat.x, top: seat.y }} data-runtime-seat={team.teamId}>
          <span className="sr-only">{team.name} 工位</span>
        </div>
      ))}
      {workspace.seats.map((seat) => {
        const employee = employeesBySeat.get(seat.seatId);
        if (!employee) return null;
        return (
          <EmployeeAvatarNode
            key={employee.employeeId}
            employee={employee}
            selected={employee.employeeId === selectedEmployeeId}
            x={seat.x}
            y={seat.y}
            onSelect={onSelectEmployee}
          />
        );
      })}
    </div>
  );
}

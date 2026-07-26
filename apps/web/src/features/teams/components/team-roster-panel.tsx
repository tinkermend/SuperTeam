import type { ReactNode } from "react";
import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Plus, UserPlus } from "lucide-react";
import {
  StatusPill,
  Button,
  LoadingState,
  Pagination,
  DataTable,
  Td,
  Th,
  Tr,
  WorkSurface
} from "@/components/superteam";
import { Label } from "@/components/ui/label";
import type { ApiClientOptions } from "@/lib/api/client";
import type { AllowedTeamAction } from "@/lib/api/teams";
import { bindTeamDigitalEmployee } from "@/lib/api/teams";
import type { DigitalEmployee } from "@/lib/api/employees";
import { listDigitalEmployees } from "@/lib/api/employees";
import { EmployeeAvatar } from "@/features/employees/avatar";
import { employeeAvatarAsset } from "@/features/employees/avatar-library";
import { employeeStatusLabel } from "@/lib/status-labels";
import {
  isTeamDigitalEmployeePageSize,
  readTeamDigitalEmployeePageSize,
  TEAM_DIGITAL_EMPLOYEE_PAGE_SIZE_OPTIONS,
  writeTeamDigitalEmployeePageSize,
  type TeamDigitalEmployeePageSize
} from "../lib/team-digital-employees-page-size";
import { TeamDigitalEmployeeDetachActions } from "./team-digital-employee-detach";

type TeamRosterPanelProps = {
  allowedActions: AllowedTeamAction[];
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  /** 只读模式（团队详情观察面）：不渲染收编面板与移出/换队动作，仅保留详情链接。 */
  readOnly?: boolean;
  teamId: string;
};

// 团队数字员工编制面板。写操作只在团队配置页出现；详情页以 readOnly 复用同一张表，
// 避免两处各维护一套列与空状态。
export function TeamRosterPanel({
  allowedActions,
  apiBaseUrl,
  fetcher,
  readOnly = false,
  teamId
}: TeamRosterPanelProps) {
  const apiOptions = useMemo(() => ({ baseUrl: apiBaseUrl, fetcher }), [apiBaseUrl, fetcher]);
  const queryClient = useQueryClient();
  const canManageTeam = !readOnly && allowedActions.includes("team.update");

  const digitalEmployeesQuery = useQuery({
    queryKey: ["team-digital-employees", teamId],
    queryFn: () => listDigitalEmployees(apiOptions, { team_id: teamId })
});

  // 候岗（无归属）数字员工——团队归属参与门禁的归队入口，收编后才可参与项目。
  const unassignedEmployeesQuery = useQuery({
    enabled: canManageTeam,
    queryKey: ["unassigned-digital-employees", teamId],
    queryFn: () => listDigitalEmployees(apiOptions, { assignment: "unassigned" })
});

  // 编制变动同时影响头卡计数（team-overview 的 digital_employee_count），只刷两个
  // 员工列表会让表格与头卡对不上（表格 11 条、头卡还写 12 数字员工）。
  // 收编与移出/换队共用这一个出口。
  const refetchRoster = () => {
    void digitalEmployeesQuery.refetch();
    void unassignedEmployeesQuery.refetch();
    void queryClient.invalidateQueries({ queryKey: ["team-overview", teamId] });
    void queryClient.invalidateQueries({ queryKey: ["team-summaries"] });
  };

  const bindEmployeeMutation = useMutation({
    mutationFn: (employeeId: string) => bindTeamDigitalEmployee(apiOptions, teamId, employeeId),
    onSuccess: refetchRoster
});

  return (
    <DigitalEmployeesSection
      apiOptions={apiOptions}
      canManageTeam={canManageTeam}
      employees={digitalEmployeesQuery.data ?? []}
      isLoading={digitalEmployeesQuery.isLoading}
      onRosterChanged={refetchRoster}
      teamId={teamId}
      bindPanel={
        canManageTeam ? (
          <BindUnassignedEmployeePanel
            employees={unassignedEmployeesQuery.data ?? []}
            error={bindEmployeeMutation.error}
            isPending={bindEmployeeMutation.isPending}
            onBind={(employeeId) => bindEmployeeMutation.mutate(employeeId)}
          />
        ) : null
      }
    />
  );
}

function DigitalEmployeesSection({
  apiOptions,
  bindPanel,
  canManageTeam,
  employees,
  isLoading,
  onRosterChanged,
  teamId
}: {
  apiOptions: ApiClientOptions;
  bindPanel?: ReactNode;
  canManageTeam: boolean;
  employees: DigitalEmployee[];
  isLoading: boolean;
  onRosterChanged: () => void;
  teamId: string;
}) {
  const empty = !isLoading && employees.length === 0;
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState<TeamDigitalEmployeePageSize>(() =>
    readTeamDigitalEmployeePageSize(),
  );

  const pageCount = Math.max(1, Math.ceil(employees.length / pageSize));
  const activePage = Math.min(page, pageCount);
  const pagedEmployees = useMemo(
    () => employees.slice((activePage - 1) * pageSize, activePage * pageSize),
    [activePage, employees, pageSize],
  );

  useEffect(() => {
    if (page !== activePage) {
      setPage(activePage);
    }
  }, [activePage, page]);

  const handlePageSizeChange = (size: number) => {
    if (!isTeamDigitalEmployeePageSize(size)) {
      return;
    }
    setPageSize(size);
    setPage(1);
    writeTeamDigitalEmployeePageSize(size);
  };

  return (
    <WorkSurface>
      <div className="flex flex-col gap-3 border-b border-line px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 className="text-base font-bold text-ink">数字员工</h2>
          <p className="mt-1 text-[13px] text-ink-2">
            {empty ? "尚未绑定数字员工" : "团队当前绑定的数字员工。"}
          </p>
        </div>
        <Button asChild size="sm" variant="ghost">
          <Link to="/employees/new">
            <Plus data-icon="inline-start" className="size-3.5" />
            新建数字员工
          </Link>
        </Button>
      </div>
      {bindPanel}
      {isLoading ? (
        <div className="px-5 py-4">
          <LoadingState label="加载数字员工" />
        </div>
      ) : empty ? null : (
        <>
          <DataTable>
            <thead>
              <tr>
                <Th>数字员工</Th>
                <Th>职能</Th>
                <Th>状态</Th>
                <Th className="text-right">操作</Th>
              </tr>
            </thead>
            <tbody>
              {pagedEmployees.map((employee) => (
                <Tr key={employee.id}>
                  <Td>
                    <div className="flex min-w-0 items-center gap-3">
                      <EmployeeAvatar
                        asset={employeeAvatarAsset(employee)}
                        name={employee.name}
                        size="md"
                      />
                      <div className="min-w-0">
                        <p className="truncate font-medium leading-none text-ink">
                          {employee.name}
                        </p>
                        <p className="mt-1.5 truncate text-sm text-ink-2">
                          {employee.description || "执行代理"}
                        </p>
                      </div>
                    </div>
                  </Td>
                  <Td>
                    <StatusPill tone="info">{employee.role || "未设置"}</StatusPill>
                  </Td>
                  <Td>
                    <StatusPill tone={employee.status === "active" ? "ok" : "warn"}>
                      {employeeStatusLabel(employee.status)}
                    </StatusPill>
                  </Td>
                  <Td className="text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Button asChild size="sm" variant="ghost">
                        <Link to="/employees/$employeeId" params={{ employeeId: employee.id }}>
                          详情
                        </Link>
                      </Button>
                      {canManageTeam ? (
                        <TeamDigitalEmployeeDetachActions
                          apiOptions={apiOptions}
                          employee={employee}
                          onChanged={onRosterChanged}
                          teamId={teamId}
                        />
                      ) : null}
                    </div>
                  </Td>
                </Tr>
              ))}
            </tbody>
          </DataTable>
          <Pagination
            onPageChange={setPage}
            onPageSizeChange={handlePageSizeChange}
            page={activePage}
            pageCount={pageCount}
            pageSize={pageSize}
            pageSizeOptions={[...TEAM_DIGITAL_EMPLOYEE_PAGE_SIZE_OPTIONS]}
            total={employees.length}
          />
        </>
      )}
    </WorkSurface>
  );
}

// 收编候岗数字员工：团队归属参与门禁的归队入口。仅无归属员工可收编，
// 收编后员工才具备参与项目的资格。
function BindUnassignedEmployeePanel({
  employees,
  error,
  isPending,
  onBind
}: {
  employees: DigitalEmployee[];
  error: unknown;
  isPending: boolean;
  onBind: (employeeId: string) => void;
}) {
  const [selectedEmployeeId, setSelectedEmployeeId] = useState("");

  useEffect(() => {
    if (selectedEmployeeId && !employees.some((employee) => employee.id === selectedEmployeeId)) {
      setSelectedEmployeeId("");
    }
  }, [employees, selectedEmployeeId]);

  if (employees.length === 0) {
    return null;
  }

  return (
    <div className="border-b border-line bg-card-soft px-5 py-4">
      <form
        className="flex flex-col gap-3 sm:flex-row sm:items-end"
        onSubmit={(event) => {
          event.preventDefault();
          if (selectedEmployeeId) {
            onBind(selectedEmployeeId);
            setSelectedEmployeeId("");
          }
        }}
      >
        <div className="flex min-w-0 flex-1 flex-col gap-2">
          <Label htmlFor="bind-unassigned-employee">收编候岗数字员工</Label>
          <select
            className="h-9 rounded-xl border border-line-strong bg-card px-3 text-sm text-ink"
            disabled={isPending}
            id="bind-unassigned-employee"
            onChange={(event) => setSelectedEmployeeId(event.target.value)}
            value={selectedEmployeeId}
          >
            <option value="">选择候岗员工（{employees.length} 名待归队）</option>
            {employees.map((employee) => (
              <option key={employee.id} value={employee.id}>
                {employee.name} · {employee.role || "未设置职能"}
              </option>
            ))}
          </select>
          <p className="text-xs text-ink-3">
            无团队归属的数字员工无法参与项目，收编后即可被项目引用。
          </p>
          {error ? (
            <p className="text-xs text-danger">
              {error instanceof Error ? error.message : "收编失败，请重试"}
            </p>
          ) : null}
        </div>
        <Button disabled={!selectedEmployeeId || isPending} size="sm" type="submit" variant="ghost">
          <UserPlus data-icon="inline-start" className="mr-1" />
          收编进本团队
        </Button>
      </form>
    </div>
  );
}

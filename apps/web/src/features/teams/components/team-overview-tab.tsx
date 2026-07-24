import type { ReactNode } from "react";
import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Plus, UserPlus } from "lucide-react";
import {
  StatusPill,
  V3Button,
  V3LoadingState,
  V3Pagination,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  WorkSurface,
} from "@/components/superteam";
import { Label } from "@/components/ui/label";
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
  type TeamDigitalEmployeePageSize,
} from "../lib/team-digital-employees-page-size";

type TeamOverviewTabProps = {
  allowedActions: AllowedTeamAction[];
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  teamId: string;
};

export function TeamOverviewTab({ allowedActions, apiBaseUrl, fetcher, teamId }: TeamOverviewTabProps) {
  const apiOptions = useMemo(() => ({ baseUrl: apiBaseUrl, fetcher }), [apiBaseUrl, fetcher]);
  const canManageTeam = allowedActions.includes("team.update");

  const digitalEmployeesQuery = useQuery({
    queryKey: ["team-digital-employees", teamId],
    queryFn: () => listDigitalEmployees(apiOptions, { team_id: teamId }),
  });

  // 候岗（无归属）数字员工——团队归属参与门禁的归队入口，收编后才可参与项目。
  const unassignedEmployeesQuery = useQuery({
    enabled: canManageTeam,
    queryKey: ["unassigned-digital-employees", teamId],
    queryFn: () => listDigitalEmployees(apiOptions, { assignment: "unassigned" }),
  });

  const bindEmployeeMutation = useMutation({
    mutationFn: (employeeId: string) => bindTeamDigitalEmployee(apiOptions, teamId, employeeId),
    onSuccess: () => {
      void digitalEmployeesQuery.refetch();
      void unassignedEmployeesQuery.refetch();
    },
  });

  return (
    <DigitalEmployeesSection
      employees={digitalEmployeesQuery.data ?? []}
      isLoading={digitalEmployeesQuery.isLoading}
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
  bindPanel,
  employees,
  isLoading,
}: {
  bindPanel?: ReactNode;
  employees: DigitalEmployee[];
  isLoading: boolean;
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
      <div className="flex flex-col gap-3 border-b border-v3-line px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 className="text-base font-bold text-v3-ink">数字员工</h2>
          <p className="mt-1 text-[13px] text-v3-ink-2">
            {empty ? "尚未绑定数字员工" : "团队当前绑定的数字员工。"}
          </p>
        </div>
        <V3Button asChild size="sm" variant="ghost">
          <Link to="/employees/new">
            <Plus data-icon="inline-start" className="size-3.5" />
            新建数字员工
          </Link>
        </V3Button>
      </div>
      {bindPanel}
      {isLoading ? (
        <div className="px-5 py-4">
          <V3LoadingState label="加载数字员工" />
        </div>
      ) : empty ? null : (
        <>
          <V3Table>
            <thead>
              <tr>
                <V3Th>数字员工</V3Th>
                <V3Th>职能</V3Th>
                <V3Th>状态</V3Th>
                <V3Th className="text-right">操作</V3Th>
              </tr>
            </thead>
            <tbody>
              {pagedEmployees.map((employee) => (
                <V3Tr key={employee.id}>
                  <V3Td>
                    <div className="flex min-w-0 items-center gap-3">
                      <EmployeeAvatar
                        asset={employeeAvatarAsset(employee)}
                        name={employee.name}
                        size="md"
                      />
                      <div className="min-w-0">
                        <p className="truncate font-medium leading-none text-v3-ink">
                          {employee.name}
                        </p>
                        <p className="mt-1.5 truncate text-sm text-v3-ink-2">
                          {employee.description || "执行代理"}
                        </p>
                      </div>
                    </div>
                  </V3Td>
                  <V3Td>
                    <StatusPill tone="info">{employee.role || "未设置"}</StatusPill>
                  </V3Td>
                  <V3Td>
                    <StatusPill tone={employee.status === "active" ? "ok" : "warn"}>
                      {employeeStatusLabel(employee.status)}
                    </StatusPill>
                  </V3Td>
                  <V3Td className="text-right">
                    <V3Button asChild size="sm" variant="ghost">
                      <Link to="/employees/$employeeId" params={{ employeeId: employee.id }}>
                        详情
                      </Link>
                    </V3Button>
                  </V3Td>
                </V3Tr>
              ))}
            </tbody>
          </V3Table>
          <V3Pagination
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
  onBind,
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
    <div className="border-b border-v3-line bg-v3-card-soft px-5 py-4">
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
            className="h-9 rounded-xl border border-v3-line-strong bg-v3-card px-3 text-sm text-v3-ink"
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
          <p className="text-xs text-v3-ink-3">
            无团队归属的数字员工无法参与项目，收编后即可被项目引用。
          </p>
          {error ? (
            <p className="text-xs text-v3-danger">
              {error instanceof Error ? error.message : "收编失败，请重试"}
            </p>
          ) : null}
        </div>
        <V3Button disabled={!selectedEmployeeId || isPending} size="sm" type="submit" variant="ghost">
          <UserPlus data-icon="inline-start" className="mr-1" />
          收编进本团队
        </V3Button>
      </form>
    </div>
  );
}

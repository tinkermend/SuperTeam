import { useQueries, useQuery } from "@tanstack/react-query";
import { Bot, CheckCircle2, FileText, PlugZap, TriangleAlert, type LucideIcon } from "lucide-react";
import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { ApiRequestError } from "@/lib/api/client";
import type { DigitalEmployee, DigitalEmployeeExecutionInstance } from "@/lib/api/employees";
import {
  getDigitalEmployeeExecutionInstance,
  listDigitalEmployees,
} from "@/lib/api/employees";

const PAGE_SIZE = 10;

type TeamDigitalEmployeesTabProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  teamId: string;
};

type InstanceState = {
  error?: unknown;
  instance?: DigitalEmployeeExecutionInstance;
  isLoading: boolean;
};

export function TeamDigitalEmployeesTab({ apiBaseUrl, fetcher, teamId }: TeamDigitalEmployeesTabProps) {
  const [page, setPage] = useState(1);
  const options = { baseUrl: apiBaseUrl, fetcher };
  const employeesQueryKey = ["team-digital-employees", teamId];
  const employees = useQuery({
    queryKey: employeesQueryKey,
    queryFn: () => listDigitalEmployees(options, { team_id: teamId }),
  });
  const employeeRows = employees.data ?? [];
  const totalPages = Math.max(1, Math.ceil(employeeRows.length / PAGE_SIZE));
  const currentPage = Math.min(page, totalPages);
  const firstVisibleIndex = (currentPage - 1) * PAGE_SIZE;
  const visibleEmployeeRows = employeeRows.slice(firstVisibleIndex, firstVisibleIndex + PAGE_SIZE);
  const instanceQueries = useQueries({
    queries: visibleEmployeeRows.map((employee) => ({
      queryKey: ["team-digital-employee-execution-instance", teamId, employee.id],
      queryFn: () => getDigitalEmployeeExecutionInstance(options, employee.id),
      retry: false,
    })),
  });
  const instances = new Map<string, InstanceState>(
    visibleEmployeeRows.map((employee, index) => {
      const query = instanceQueries[index];

      return [
        employee.id,
        {
          error: query?.error,
          instance: query?.data,
          isLoading: Boolean(query?.isLoading),
        },
      ];
    }),
  );
  const unboundRuntimeCount = Array.from(instances.values()).filter((instance) => isNotFound(instance.error)).length;
  const staleConfigCount = employeeRows.filter((employee) => employee.metadata?.effective_config_status === "stale").length;
  const activeCount = employeeRows.filter((employee) => employee.status === "active").length;
  const draftCount = employeeRows.filter((employee) => employee.status === "draft").length;

  return (
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_320px]">
      <div className="flex min-w-0 flex-col gap-4">
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
          <MetricTile icon={Bot} label="数字员工" value={employeeRows.length} detail="总数" />
          <MetricTile icon={CheckCircle2} label="active" value={activeCount} detail="正常运行" />
          <MetricTile icon={FileText} label="draft" value={draftCount} detail="未发布" />
          <MetricTile icon={TriangleAlert} label="继承配置过期" value={staleConfigCount} detail="需更新" />
          <MetricTile icon={PlugZap} label="未绑定 Runtime" value={unboundRuntimeCount} detail="当前页需绑定" />
        </div>

        <Card>
          <CardHeader className="border-b">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <CardTitle>团队数字员工</CardTitle>
            </div>
          </CardHeader>
          <CardContent className="pt-4">
            {employees.isLoading ? <p className="text-sm text-muted-foreground">加载中</p> : null}
            {employees.isError ? <p className="text-sm text-destructive">数字员工加载失败</p> : null}
            {!employees.isLoading && !employees.isError && employeeRows.length === 0 ? (
              <p className="text-sm text-muted-foreground">暂无数字员工</p>
            ) : null}
            {employeeRows.length > 0 ? (
              <div className="flex flex-col gap-3">
                <div className="w-full overflow-x-auto rounded-md border">
                  <Table className="min-w-[920px]">
                    <TableHeader>
                      <TableRow>
                        <TableHead className="min-w-56">数字员工</TableHead>
                        <TableHead>角色</TableHead>
                        <TableHead>状态</TableHead>
                        <TableHead>风险</TableHead>
                        <TableHead>生效配置</TableHead>
                        <TableHead>执行实例</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {visibleEmployeeRows.map((employee) => (
                        <EmployeeTableRow
                          employee={employee}
                          instanceState={instances.get(employee.id) ?? { isLoading: true }}
                          key={employee.id}
                        />
                      ))}
                    </TableBody>
                  </Table>
                </div>
                <div className="flex flex-col gap-2 text-sm text-muted-foreground sm:flex-row sm:items-center sm:justify-between">
                  <span>
                    第 {currentPage} / {totalPages} 页，每页 {PAGE_SIZE} 条
                  </span>
                  <div className="flex gap-2">
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={currentPage <= 1}
                      onClick={() => setPage((value) => Math.max(1, value - 1))}
                    >
                      上一页
                    </Button>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={currentPage >= totalPages}
                      onClick={() => setPage((value) => Math.min(totalPages, value + 1))}
                    >
                      下一页
                    </Button>
                  </div>
                </div>
              </div>
            ) : null}
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>创建入口</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <p className="text-sm text-muted-foreground">
            新数字员工需要选择专业类型、能力、治理和 Runtime 绑定，请进入创建向导完成。
          </p>
          <Button asChild type="button">
            <a href="/employees/new">
              <Bot data-icon="inline-start" />
              从此团队创建数字员工
            </a>
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}

function MetricTile({
  detail,
  icon: Icon,
  label,
  value,
}: {
  detail: string;
  icon: LucideIcon;
  label: string;
  value: number;
}) {
  return (
    <Card aria-label={`${label} ${value} ${detail}`} className="gap-3 py-4" role="group">
      <CardContent className="flex items-center gap-3 px-4">
        <div className="flex size-9 shrink-0 items-center justify-center rounded-md border bg-muted">
          <Icon />
        </div>
        <div className="min-w-0">
          <p className="truncate text-sm text-muted-foreground">{label}</p>
          <p className="text-xl font-semibold">{value}</p>
          <p className="truncate text-xs text-muted-foreground">{detail}</p>
        </div>
      </CardContent>
    </Card>
  );
}

function EmployeeTableRow({
  employee,
  instanceState,
}: {
  employee: DigitalEmployee;
  instanceState: InstanceState;
}) {
  const isUnbound = isNotFound(instanceState.error);
  const runtimeText = instanceState.instance?.runtime_node_id ?? (isUnbound ? "未绑定" : "实例加载中");
  const instanceText = instanceState.instance
    ? `${instanceState.instance.provider_type} · ${instanceState.instance.status}`
    : runtimeText;

  return (
    <TableRow>
      <TableCell>
        <div className="flex min-w-0 items-start gap-3">
          <div className="mt-1 flex size-8 shrink-0 items-center justify-center rounded-md border bg-muted">
            <Bot />
          </div>
          <div className="min-w-0">
            <p className="truncate font-medium">{employee.name}</p>
            <p className="mt-1 max-w-64 truncate text-xs text-muted-foreground">{employee.description || employee.id}</p>
          </div>
        </div>
      </TableCell>
      <TableCell>{employee.role}</TableCell>
      <TableCell>
        <Badge variant={employee.status === "active" ? "default" : "secondary"}>{employeeStatusLabel(employee.status)}</Badge>
      </TableCell>
      <TableCell>{employee.risk_level ?? "medium"}</TableCell>
      <TableCell>
        <div className="flex flex-col gap-1">
          <span>{employee.metadata?.effective_config_label ?? "未配置"}</span>
          {employee.metadata?.effective_config_status === "stale" ? (
            <Badge variant="outline">需更新</Badge>
          ) : null}
        </div>
      </TableCell>
      <TableCell>
        <div className="flex flex-col gap-1">
          <span>{runtimeText}</span>
          <span className="text-xs text-muted-foreground">{instanceText}</span>
          {instanceState.error && !isUnbound ? <span className="text-xs text-destructive">执行实例加载失败</span> : null}
        </div>
      </TableCell>
    </TableRow>
  );
}

function isNotFound(error: unknown) {
  return error instanceof ApiRequestError && error.status === 404;
}

function employeeStatusLabel(status: DigitalEmployee["status"]) {
  const labels: Record<DigitalEmployee["status"], string> = {
    active: "活跃中",
    disabled: "已禁用",
    draft: "草稿",
    error: "异常",
    ready: "就绪",
  };

  return labels[status] ?? status;
}

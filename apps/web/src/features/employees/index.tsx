import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Bot, Plus } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import { ApiRequestError } from "@/lib/api/client";
import type { DigitalEmployee } from "@/lib/api/employees";
import {
  getDigitalEmployeeExecutionInstance,
  listDigitalEmployees,
} from "@/lib/api/employees";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";

export function EmployeesPage() {
  const apiBaseUrl = resolveControlPlaneUrl();

  return <EmployeesView apiBaseUrl={apiBaseUrl} />;
}

type EmployeesViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
};

export function EmployeesView({ apiBaseUrl, fetcher }: EmployeesViewProps) {
  const employees = useQuery({
    queryKey: ["digital-employees"],
    queryFn: () => listDigitalEmployees({ baseUrl: apiBaseUrl, fetcher }),
  });

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main>
        <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-3">
            <div className="flex size-10 items-center justify-center rounded-md border bg-muted">
              <Bot />
            </div>
            <div>
              <h1 className="text-2xl font-bold tracking-tight">数字员工</h1>
              <p className="text-sm text-muted-foreground">业务身份、执行实例和运行状态。</p>
            </div>
          </div>
          <Button asChild type="button">
            <Link to="/employees/new">
              <Plus data-icon="inline-start" />
              创建数字员工
            </Link>
          </Button>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>员工列表</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {employees.isLoading ? <p className="text-sm text-muted-foreground">加载中</p> : null}
            {employees.isError ? <p className="text-sm text-destructive">员工列表加载失败</p> : null}
            {(employees.data ?? []).map((employee) => (
              <EmployeeRow key={employee.id} apiBaseUrl={apiBaseUrl} employee={employee} fetcher={fetcher} />
            ))}
            {!employees.isLoading && (employees.data ?? []).length === 0 ? (
              <p className="text-sm text-muted-foreground">暂无数字员工</p>
            ) : null}
          </CardContent>
        </Card>
      </Main>
    </>
  );
}

type EmployeeRowProps = {
  apiBaseUrl: string;
  employee: DigitalEmployee;
  fetcher?: typeof fetch;
};

function EmployeeRow({ apiBaseUrl, employee, fetcher }: EmployeeRowProps) {
  const instance = useQuery({
    queryKey: ["digital-employee-execution-instance", employee.id],
    queryFn: () => getDigitalEmployeeExecutionInstance({ baseUrl: apiBaseUrl, fetcher }, employee.id),
    retry: false,
  });
  const isUnbound = instance.error instanceof ApiRequestError && instance.error.status === 404;
  const runtimeText = instance.data?.runtime_node_id ?? (isUnbound ? "未绑定 Runtime" : "实例加载中");
  const instanceText = instance.data ? `${instance.data.provider_type} · ${instance.data.status}` : null;

  return (
    <div className="flex flex-col gap-3 rounded-md border p-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="flex min-w-0 items-start gap-3">
        <div className="mt-1 flex size-8 shrink-0 items-center justify-center rounded-md border bg-muted">
          <Bot className="size-4" />
        </div>
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <p className="truncate font-medium">{employee.name}</p>
            <Badge variant={employee.status === "active" ? "default" : "secondary"}>{employee.status}</Badge>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">{employee.description || employee.role}</p>
          {instanceText ? <p className="mt-1 text-xs text-muted-foreground">{instanceText}</p> : null}
          {instance.isError && !isUnbound ? <p className="mt-1 text-xs text-destructive">执行实例加载失败</p> : null}
        </div>
      </div>
      <div className="flex flex-col gap-2 text-left text-xs text-muted-foreground sm:items-end sm:text-right">
        <div>
          <p>{employee.risk_level || "medium"}</p>
          <p className="mt-1 max-w-full truncate">{runtimeText}</p>
        </div>
        <Button asChild size="sm" type="button" variant="outline">
          <Link params={{ employeeId: employee.id }} to="/employees/$employeeId">
            详情
          </Link>
        </Button>
      </div>
    </div>
  );
}

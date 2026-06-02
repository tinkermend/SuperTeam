import { useQuery } from "@tanstack/react-query";
import { Bot } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import { listDigitalEmployees } from "@/lib/api/employees";
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
        <div className="mb-4 flex items-center gap-3">
          <div className="flex size-10 items-center justify-center rounded-md border bg-muted">
            <Bot />
          </div>
          <div>
            <h1 className="text-2xl font-bold tracking-tight">数字员工</h1>
            <p className="text-sm text-muted-foreground">业务身份、执行实例和运行状态。</p>
          </div>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>员工列表</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {employees.isLoading ? <p className="text-sm text-muted-foreground">加载中</p> : null}
            {employees.isError ? <p className="text-sm text-destructive">员工列表加载失败</p> : null}
            {(employees.data ?? []).map((employee) => (
              <div key={employee.id} className="flex flex-col gap-3 rounded-md border p-3 sm:flex-row sm:items-center sm:justify-between">
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
                    {employee.execution_instance ? (
                      <p className="mt-1 text-xs text-muted-foreground">
                        {employee.execution_instance.provider_type} · {employee.execution_instance.status}
                      </p>
                    ) : null}
                  </div>
                </div>
                <div className="text-left text-xs text-muted-foreground sm:text-right">
                  <p>{employee.risk_level || "medium"}</p>
                  <p className="mt-1 max-w-full truncate">{employee.execution_instance?.runtime_node_id || "未绑定 Runtime"}</p>
                </div>
              </div>
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

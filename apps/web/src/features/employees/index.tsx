import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bot, Plus } from "lucide-react";
import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import { ApiRequestError } from "@/lib/api/client";
import type { DigitalEmployee } from "@/lib/api/employees";
import {
  createDigitalEmployee,
  createDigitalEmployeeConfigRevision,
  getDigitalEmployeeExecutionInstance,
  listDigitalEmployees,
  previewDigitalEmployeeEffectiveConfig,
} from "@/lib/api/employees";
import { getCurrentTeamConfigRevision, listTeams } from "@/lib/api/teams";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";

export function EmployeesPage() {
  const apiBaseUrl = resolveControlPlaneUrl();

  return <EmployeesView apiBaseUrl={apiBaseUrl} />;
}

type EmployeesViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
};

type CreatedDraft = {
  id: string;
  name: string;
};

export function EmployeesView({ apiBaseUrl, fetcher }: EmployeesViewProps) {
  const queryClient = useQueryClient();
  const [isCreatePanelOpen, setIsCreatePanelOpen] = useState(false);
  const [name, setName] = useState("");
  const [role, setRole] = useState("");
  const [selectedTeamId, setSelectedTeamId] = useState("");
  const [createdDraft, setCreatedDraft] = useState<CreatedDraft | null>(null);
  const [draftPreviewFailed, setDraftPreviewFailed] = useState(false);
  const employees = useQuery({
    queryKey: ["digital-employees"],
    queryFn: () => listDigitalEmployees({ baseUrl: apiBaseUrl, fetcher }),
  });
  const teams = useQuery({
    queryKey: ["teams"],
    queryFn: () => listTeams({ baseUrl: apiBaseUrl, fetcher }),
  });
  const teamOptions = teams.data ?? [];
  const effectiveTeamId = selectedTeamId || teamOptions[0]?.id || "";
  const trimmedName = name.trim();
  const trimmedRole = role.trim();
  const createPreview = useMutation({
    mutationFn: async () => {
      setDraftPreviewFailed(false);
      const teamConfig = await getCurrentTeamConfigRevision({ baseUrl: apiBaseUrl, fetcher }, effectiveTeamId);
      const employee = await createDigitalEmployee(
        { baseUrl: apiBaseUrl, fetcher },
        {
          name: trimmedName,
          role: trimmedRole,
          team_id: effectiveTeamId,
        },
      );
      setCreatedDraft({ id: employee.id, name: employee.name });
      void queryClient.invalidateQueries({ queryKey: ["digital-employees"] });

      try {
        const employeeConfig = await createDigitalEmployeeConfigRevision(
          { baseUrl: apiBaseUrl, fetcher },
          employee.id,
          {
            role_profile: { role: trimmedRole },
            capability_selection: { enabled_skills: [] },
            status: "draft",
          },
        );

        return await previewDigitalEmployeeEffectiveConfig({ baseUrl: apiBaseUrl, fetcher }, employee.id, {
          team_config: { id: teamConfig.id },
          employee_config: { id: employeeConfig.id },
        });
      } catch (error) {
        setDraftPreviewFailed(true);
        throw error;
      }
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["digital-employees"] });
    },
  });
  const blockingErrors = createPreview.data?.validation.blocking_errors ?? [];

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
          <Button
            type="button"
            disabled={teams.isLoading || teamOptions.length === 0}
            onClick={() => setIsCreatePanelOpen(true)}
          >
            <Plus className="size-4" />
            创建数字员工
          </Button>
        </div>
        {teams.isError ? <p className="mb-4 text-sm text-destructive">团队列表加载失败</p> : null}
        {!teams.isLoading && !teams.isError && teamOptions.length === 0 ? (
          <p className="mb-4 text-sm text-muted-foreground">暂无团队，需先创建团队并配置治理版本</p>
        ) : null}

        {isCreatePanelOpen ? (
          <Card className="mb-4">
            <CardHeader>
              <CardTitle>创建数字员工</CardTitle>
            </CardHeader>
            <CardContent>
              <form
                className="grid gap-4 md:grid-cols-3"
                onSubmit={(event) => {
                  event.preventDefault();
                  createPreview.mutate();
                }}
              >
                <div className="space-y-2">
                  <Label htmlFor="employee-name">名称</Label>
                  <Input
                    id="employee-name"
                    value={name}
                    onChange={(event) => setName(event.target.value)}
                    placeholder="数据库运维员工"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="employee-role">角色</Label>
                  <Input
                    id="employee-role"
                    value={role}
                    onChange={(event) => setRole(event.target.value)}
                    placeholder="database_operator"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="employee-team">归属团队</Label>
                  <select
                    id="employee-team"
                    value={effectiveTeamId}
                    onChange={(event) => setSelectedTeamId(event.target.value)}
                    className="h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-xs outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50"
                  >
                    {teamOptions.map((team) => (
                      <option key={team.id} value={team.id}>
                        {team.name}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="flex flex-col gap-2 md:col-span-3 md:flex-row md:items-center">
                  <Button
                    type="submit"
                    disabled={
                      createPreview.isPending ||
                      Boolean(createdDraft) ||
                      !trimmedName ||
                      !trimmedRole ||
                      !effectiveTeamId
                    }
                  >
                    预览生效配置
                  </Button>
                  {createdDraft ? (
                    <p className="text-sm text-muted-foreground">已创建草稿：{createdDraft.name}</p>
                  ) : null}
                  {createPreview.isPending ? <p className="text-sm text-muted-foreground">预览中</p> : null}
                  {createPreview.isError ? (
                    <p className="text-sm text-destructive">
                      {draftPreviewFailed ? "草稿已创建但预览失败" : "生效配置预览失败"}
                    </p>
                  ) : null}
                  {createPreview.data ? (
                    blockingErrors.length === 0 ? (
                      <Badge variant="default">可提交负责人确认</Badge>
                    ) : (
                      <Badge variant="destructive">存在阻断错误</Badge>
                    )
                  ) : null}
                </div>
              </form>
            </CardContent>
          </Card>
        ) : null}

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
      <div className="text-left text-xs text-muted-foreground sm:text-right">
        <p>{employee.risk_level || "medium"}</p>
        <p className="mt-1 max-w-full truncate">{runtimeText}</p>
      </div>
    </div>
  );
}

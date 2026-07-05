import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Main } from "@/components/layout/main";
import {
  ShellPageHeader,
  ShellPageHeaderBack,
} from "@/components/layout/shell-page-header";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { ApiRequestError } from "@/lib/api/client";
import { listEffectiveMcpConfig } from "@/lib/api/capabilities";
import {
  createDigitalEmployeeRun,
  getCurrentDigitalEmployeeEffectiveConfig,
  getDigitalEmployee,
  getDigitalEmployeeExecutionInstance,
  getDigitalEmployeeRunStats,
  listDigitalEmployeeRuns,
  listEmployeeEnvironmentVariables,
  type DigitalEmployeeRun,
  type DigitalEmployeeRunListItem,
  type DigitalEmployeeRunStatus,
} from "@/lib/api/employees";
import { listEmployeeSkills } from "@/lib/api/skills";
import { getRuntimeOverview } from "@/lib/api/runtime";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { ContextInjectionChain } from "./components/context-injection-chain";
import { EffectiveContextPanel } from "./components/effective-context-panel";
import { EmployeeCapabilitiesPanel } from "./components/employee-capabilities-panel";
import { EmployeeDetailHeader } from "./components/employee-detail-header";
import { EmployeeMetricsStrip } from "./components/employee-metrics-strip";
import { EmployeeRunHistoryTable } from "./components/employee-run-history-table";
import { RunDetailDrawer } from "./components/run-detail-drawer";
import { StartTaskDrawer } from "./components/start-task-drawer";

const activeRunStatuses = new Set<DigitalEmployeeRunStatus>([
  "queued",
  "dispatching",
  "running",
  "cancelling",
]);
const PAGE_SIZE = 10;

export function EmployeeDetailPage({ employeeId }: { employeeId: string }) {
  const apiBaseUrl = resolveControlPlaneUrl();
  return <EmployeeDetailView apiBaseUrl={apiBaseUrl} employeeId={employeeId} />;
}

type EmployeeDetailViewProps = {
  apiBaseUrl: string;
  employeeId: string;
  fetcher?: typeof fetch;
};

export function EmployeeDetailView({ apiBaseUrl, employeeId, fetcher }: EmployeeDetailViewProps) {
  const apiOptions = { baseUrl: apiBaseUrl, fetcher };
  const queryClient = useQueryClient();
  const [page, setPage] = useState(1);
  const [statusFilter, setStatusFilter] = useState<DigitalEmployeeRunStatus | undefined>(undefined);
  const [selectedRun, setSelectedRun] = useState<DigitalEmployeeRunListItem | undefined>(undefined);
  const [runDrawerOpen, setRunDrawerOpen] = useState(false);
  const [startTaskOpen, setStartTaskOpen] = useState(false);
  const [capabilitiesOpen, setCapabilitiesOpen] = useState(false);

  const employee = useQuery({
    queryKey: ["digital-employee", employeeId],
    queryFn: () => getDigitalEmployee(apiOptions, employeeId),
  });
  const instance = useQuery({
    queryKey: ["digital-employee-execution-instance", employeeId],
    queryFn: () => getDigitalEmployeeExecutionInstance(apiOptions, employeeId),
    retry: false,
  });
  const runStats = useQuery({
    queryKey: ["digital-employee-run-stats", employeeId],
    queryFn: () => getDigitalEmployeeRunStats(apiOptions, employeeId),
  });
  const runs = useQuery({
    queryKey: ["digital-employee-runs", employeeId, { page, statusFilter }] as const,
    queryFn: () =>
      listDigitalEmployeeRuns(apiOptions, employeeId, {
        limit: PAGE_SIZE,
        offset: (page - 1) * PAGE_SIZE,
        status: statusFilter ? [statusFilter] : undefined,
      }),
    refetchInterval: (query) =>
      query.state.data?.items.some((item) => isActiveRun(item.status)) ? 2500 : false,
  });
  const runtimeOverview = useQuery({
    queryKey: ["runtime-overview"],
    queryFn: () => getRuntimeOverview(apiOptions),
    refetchInterval: 5000,
  });

  // Lifted from EffectiveContextPanel (Task 11) so detail.tsx can feed both the
  // panel (as computed props) and ContextInjectionChain (real counts). The panel
  // is now a pure presentational component.
  const effectiveConfigQuery = useQuery({
    queryKey: ["digital-employee-effective-config", employeeId],
    queryFn: () => getCurrentDigitalEmployeeEffectiveConfig(apiOptions, employeeId),
    retry: false,
  });
  const skillsQuery = useQuery({
    queryKey: ["employee-skills", employeeId],
    queryFn: () => listEmployeeSkills(apiOptions, employeeId),
  });
  const mcpQuery = useQuery({
    queryKey: ["employee-effective-mcp", employeeId],
    queryFn: () => listEffectiveMcpConfig(apiOptions, employeeId),
  });
  const envVarsQuery = useQuery({
    queryKey: ["employee-environment-variables", employeeId],
    queryFn: () => listEmployeeEnvironmentVariables(apiOptions, employeeId),
  });

  const instanceNotFound =
    instance.error instanceof ApiRequestError && instance.error.status === 404;
  const noApprovedConfig =
    effectiveConfigQuery.error instanceof ApiRequestError &&
    effectiveConfigQuery.error.status === 404;

  // EffectiveEmployeeSkill carries both `inherited` and `source_scope`; `inherited`
  // is the canonical flag for skill counting (matches Task 11 semantics).
  const personalSkillCount = skillsQuery.data?.filter((skill) => !skill.inherited).length ?? 0;
  const inheritedSkillCount = skillsQuery.data?.filter((skill) => skill.inherited).length ?? 0;
  // EffectiveMcpServer exposes `source_scope` (no `inherited` boolean). Team scope
  // is the inherited leg; anything else is personal.
  const personalMcpCount = mcpQuery.data?.filter((server) => server.source_scope !== "team").length ?? 0;
  const inheritedMcpCount = mcpQuery.data?.filter((server) => server.source_scope === "team").length ?? 0;
  const configuredEnvCount = envVarsQuery.data?.filter((item) => item.configured).length ?? 0;
  const missingEnvVars = envVarsQuery.data?.filter((item) => !item.configured) ?? [];

  const hasActiveRun = runs.data?.items.some((item) => isActiveRun(item.status)) ?? false;
  const employeeCanRun = employee.data?.status === "ready" || employee.data?.status === "active";
  const executionInstanceCanRun =
    instance.isSuccess && (instance.data.status === "ready" || instance.data.status === "active");
  const executionRuntimeNodeId = instance.data?.runtime_node_id;
  const runtimeNode = runtimeOverview.data?.nodes.find(
    (node) => node.runtime_node_id === executionRuntimeNodeId,
  );
  const runtimeCommandChannelDisconnected =
    runtimeOverview.isSuccess && runtimeNode?.command_channel_connected === false;
  const canStartTask =
    employeeCanRun &&
    executionInstanceCanRun &&
    runs.isSuccess &&
    !hasActiveRun &&
    !runtimeCommandChannelDisconnected;

  const disabledReasons: string[] = [];
  if (hasActiveRun) disabledReasons.push("当前已有活跃运行");
  if (!executionInstanceCanRun && instance.isSuccess) disabledReasons.push("执行实例当前不可执行");
  if (runtimeCommandChannelDisconnected) disabledReasons.push("Runtime 命令通道未连接，暂不能开始任务");
  if (instanceNotFound) disabledReasons.push("未绑定 Runtime，暂不能开始任务");
  if (runs.isError) disabledReasons.push("运行列表加载失败，暂不能开始新任务");

  const refreshRunFacts = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["digital-employee-runs", employeeId] }),
      queryClient.invalidateQueries({ queryKey: ["digital-employee-run-stats", employeeId] }),
    ]);
  };

  const createRun = useMutation({
    mutationFn: (input: { objective: string; prompt: string }) =>
      createDigitalEmployeeRun(apiOptions, employeeId, {
        objective: input.objective,
        prompt: input.prompt,
      }),
    onSuccess: async () => {
      setStartTaskOpen(false);
      await refreshRunFacts();
    },
  });

  const handleStopped = async (_run: DigitalEmployeeRun) => {
    await refreshRunFacts();
  };

  return (
    <>
      <ShellPageHeader
        back={<ShellPageHeaderBack ariaLabel="返回数字员工列表" to="/employees" />}
        title={employee.data?.name ?? "数字员工详情"}
        subtitle="执行实例、运行事件、结果和人工停止。"
      />
      <Main className="min-w-0 overflow-x-hidden">
        {employee.isLoading ? <p className="text-sm text-v3-ink-2">加载中</p> : null}
        {employee.isError ? <p className="text-sm text-destructive">数字员工加载失败</p> : null}

        {employee.data ? (
          <div className="flex flex-col gap-4">
            <EmployeeDetailHeader
              employee={employee.data}
              onManageCapabilities={() => setCapabilitiesOpen(true)}
              onStartTask={() => setStartTaskOpen(true)}
            />

            <EmployeeMetricsStrip
              commandChannelConnected={runtimeNode?.command_channel_connected ?? false}
              currentStatusLabel={employee.data.status}
              providerType={instance.data?.provider_type ?? "未绑定"}
              runtimeNodeLabel={instance.data?.runtime_node_id ?? "未绑定"}
              stats={runStats.data}
            />

            <section className="grid gap-4 lg:grid-cols-[minmax(0,1.2fr)_minmax(320px,0.8fr)]">
              <EmployeeRunHistoryTable
                error={runs.error}
                isError={runs.isError}
                isLoading={runs.isLoading}
                onPageChange={setPage}
                onRetry={() => runs.refetch()}
                onRowClick={(item) => {
                  setSelectedRun(item);
                  setRunDrawerOpen(true);
                }}
                onStatusFilterChange={(status) => {
                  setStatusFilter(status);
                  setPage(1);
                }}
                page={page}
                pageSize={PAGE_SIZE}
                result={runs.data}
                statusFilter={statusFilter}
              />
              <EffectiveContextPanel
                effectiveConfig={{
                  isLoading: effectiveConfigQuery.isLoading,
                  isError: effectiveConfigQuery.isError && !noApprovedConfig,
                  noApprovedConfig,
                }}
                employee={employee.data}
                employeeId={employeeId}
                envVars={{
                  isLoading: envVarsQuery.isLoading,
                  isError: envVarsQuery.isError,
                  configuredCount: configuredEnvCount,
                  totalCount: envVarsQuery.data?.length ?? 0,
                  missingNames: missingEnvVars.map((item) => item.name),
                }}
                executionInstance={instance.data}
                mcp={{
                  isLoading: mcpQuery.isLoading,
                  isError: mcpQuery.isError,
                  personalCount: personalMcpCount,
                  inheritedCount: inheritedMcpCount,
                  totalCount: mcpQuery.data?.length ?? 0,
                }}
                onManageCapabilities={() => setCapabilitiesOpen(true)}
                skills={{
                  isLoading: skillsQuery.isLoading,
                  isError: skillsQuery.isError,
                  personalCount: personalSkillCount,
                  inheritedCount: inheritedSkillCount,
                  totalCount: skillsQuery.data?.length ?? 0,
                }}
              />
            </section>

            <ContextInjectionChain
              envConfiguredCount={configuredEnvCount}
              envTotalCount={envVarsQuery.data?.length ?? 0}
              inheritedSkillCount={inheritedSkillCount}
              mcpCount={mcpQuery.data?.length ?? 0}
              personalSkillCount={personalSkillCount}
              roleLabel={employee.data.role}
            />
          </div>
        ) : null}
      </Main>

      <StartTaskDrawer
        canStartTask={canStartTask}
        disabledReasons={disabledReasons}
        isError={createRun.isError}
        isPending={createRun.isPending}
        onOpenChange={setStartTaskOpen}
        onSubmit={(input) => createRun.mutate(input)}
        open={startTaskOpen}
      />

      <RunDetailDrawer
        apiOptions={apiOptions}
        employeeId={employeeId}
        onOpenChange={setRunDrawerOpen}
        onStopped={handleStopped}
        open={runDrawerOpen}
        run={selectedRun}
      />

      <Sheet onOpenChange={setCapabilitiesOpen} open={capabilitiesOpen}>
        <SheetContent className="w-full overflow-y-auto sm:max-w-2xl" side="right">
          <SheetHeader>
            <SheetTitle>管理技能与 MCP</SheetTitle>
          </SheetHeader>
          <div className="px-4 pb-6">
            <EmployeeCapabilitiesPanel apiOptions={apiOptions} employeeId={employeeId} />
          </div>
        </SheetContent>
      </Sheet>
    </>
  );
}

function isActiveRun(status: DigitalEmployeeRunStatus) {
  return activeRunStatuses.has(status);
}

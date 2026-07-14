import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { AlertTriangle } from "lucide-react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Main } from "@/components/layout/main";
import {
  ShellPageHeader,
  ShellPageHeaderBack,
} from "@/components/layout/shell-page-header";
import { SoftCard, StatusPill } from "@/components/superteam";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { ApiRequestError } from "@/lib/api/client";
import { listEffectiveMcpConfig } from "@/lib/api/capabilities";
import {
  createDigitalEmployeeRun,
  deleteDigitalEmployee,
  getDigitalEmployee,
  getDigitalEmployeeExecutionInstance,
  getDigitalEmployeeSchedulingReadiness,
  getDigitalEmployeeRunStats,
  listDigitalEmployeeRuns,
  listEmployeeEnvironmentVariables,
  type DigitalEmployee,
  type DigitalEmployeeDeleteBlockedErrorResponse,
  type DigitalEmployeeDeleteBlocker,
  type DigitalEmployeeExecutionInstance,
  type DigitalEmployeeRun,
  type DigitalEmployeeRunKind,
  type DigitalEmployeeRunListItem,
  type DigitalEmployeeRunStatus,
} from "@/lib/api/employees";
import { listEmployeeSkills } from "@/lib/api/skills";
import { getRuntimeOverview } from "@/lib/api/runtime";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { EffectiveContextPanel } from "./components/effective-context-panel";
import { EmployeeCapabilitiesPanel } from "./components/employee-capabilities-panel";
import { EmployeeDetailHeader } from "./components/employee-detail-header";
import { EmployeeMetricsStrip } from "./components/employee-metrics-strip";
import { EmployeeRunHistoryTable } from "./components/employee-run-history-table";
import { RunDetailDrawer } from "./components/run-detail-drawer";
import { SchedulingReadinessPanel } from "./components/scheduling-readiness-panel";
import { StartTaskDrawer } from "./components/start-task-drawer";
import { providerDisplayName } from "./provider-label";

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
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [page, setPage] = useState(1);
  const [statusFilter, setStatusFilter] = useState<DigitalEmployeeRunStatus | undefined>(undefined);
  const [runKindFilter, setRunKindFilter] = useState<DigitalEmployeeRunKind | undefined>(undefined);
  const [selectedRun, setSelectedRun] = useState<DigitalEmployeeRunListItem | undefined>(undefined);
  const [runDrawerOpen, setRunDrawerOpen] = useState(false);
  const [startTaskOpen, setStartTaskOpen] = useState(false);
  const [capabilitiesOpen, setCapabilitiesOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteConfirmation, setDeleteConfirmation] = useState("");
  const [deleteBlocked, setDeleteBlocked] =
    useState<DigitalEmployeeDeleteBlockedErrorResponse | undefined>(undefined);

  const employee = useQuery({
    queryKey: ["digital-employee", employeeId],
    queryFn: () => getDigitalEmployee(apiOptions, employeeId),
  });
  const instance = useQuery({
    queryKey: ["digital-employee-execution-instance", employeeId],
    queryFn: () => getDigitalEmployeeExecutionInstance(apiOptions, employeeId),
    retry: false,
  });
  const schedulingReadiness = useQuery({
    queryKey: ["digital-employee-scheduling-readiness", employeeId],
    queryFn: () => getDigitalEmployeeSchedulingReadiness(apiOptions, employeeId),
    retry: false,
  });
  const runStats = useQuery({
    queryKey: ["digital-employee-run-stats", employeeId],
    queryFn: () => getDigitalEmployeeRunStats(apiOptions, employeeId),
  });
  const runs = useQuery({
    queryKey: ["digital-employee-runs", employeeId, { page, statusFilter, runKindFilter }] as const,
    queryFn: () =>
      listDigitalEmployeeRuns(apiOptions, employeeId, {
        limit: PAGE_SIZE,
        offset: (page - 1) * PAGE_SIZE,
        status: statusFilter ? [statusFilter] : undefined,
        run_kind: runKindFilter,
      }),
    refetchInterval: (query) =>
      query.state.data?.items.some((item) => isActiveRun(item.status)) ? 2500 : false,
  });
  const runtimeOverview = useQuery({
    queryKey: ["runtime-overview"],
    queryFn: () => getRuntimeOverview(apiOptions),
    refetchInterval: 5000,
  });

  // Lifted from EffectiveContextPanel (Task 11) so detail.tsx can feed the panel
  // (as computed props). The panel is now a pure presentational component.
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
  if (instanceNotFound) {
    disabledReasons.push("项目运行时就绪度会决定 Runtime 节点，当前不能从员工详情直接开始任务");
  }
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

  const deleteEmployee = useMutation({
    mutationFn: () => deleteDigitalEmployee(apiOptions, employeeId),
    onMutate: () => {
      setDeleteBlocked(undefined);
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["digital-employees"] }),
        queryClient.invalidateQueries({ queryKey: ["digital-employee", employeeId] }),
        queryClient.invalidateQueries({ queryKey: ["digital-employee-overview"] }),
        queryClient.invalidateQueries({ queryKey: ["digital-employees-overview"] }),
        queryClient.invalidateQueries({ queryKey: ["unassigned-digital-employees"] }),
        queryClient.invalidateQueries({ queryKey: ["digital-employee-create-options"] }),
        queryClient.invalidateQueries({ queryKey: ["projects"] }),
      ]);
      await navigate({ to: "/employees" });
    },
    onError: (error) => {
      if (isDeleteBlockedError(error)) {
        setDeleteBlocked(error.payload);
      }
    },
  });

  const handleStopped = async (_run: DigitalEmployeeRun) => {
    await refreshRunFacts();
  };

  const handleDeleteDialogOpenChange = (open: boolean) => {
    setDeleteDialogOpen(open);
    if (!open) {
      setDeleteConfirmation("");
      setDeleteBlocked(undefined);
      deleteEmployee.reset();
    }
  };

  const employeeName = employee.data?.name ?? "";
  const deleteConfirmReady = deleteConfirmation === employeeName;
  const genericDeleteError =
    deleteEmployee.isError && !deleteBlocked ? getDeleteErrorMessage(deleteEmployee.error) : undefined;

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
            {runtimeCommandChannelDisconnected ? (
              <Alert className="border-v3-danger/30 bg-v3-danger-soft text-v3-danger" variant="destructive">
                <AlertTriangle className="size-4" />
                <AlertTitle>Runtime 命令通道未连接</AlertTitle>
                <AlertDescription>当前无法开始新任务，请检查 Runtime Agent 连接状态后重试。</AlertDescription>
              </Alert>
            ) : null}

            <EmployeeDetailHeader
              employee={employee.data}
              onDelete={() => setDeleteDialogOpen(true)}
              onManageCapabilities={() => setCapabilitiesOpen(true)}
              onStartTask={() => setStartTaskOpen(true)}
            />

            <EmployeeMetricsStrip
              providerType={providerDisplayName(employee.data.provider_type)}
              stats={runStats.data}
            />

            <section className="grid gap-4 lg:grid-cols-[minmax(0,1.2fr)_minmax(320px,0.8fr)]">
              <EmployeeRunHistoryTable
                employeeId={employeeId}
                error={runs.error}
                isError={runs.isError}
                isLoading={runs.isLoading}
                onPageChange={setPage}
                onRetry={() => runs.refetch()}
                onRowClick={(item) => {
                  setSelectedRun(item);
                  setRunDrawerOpen(true);
                }}
                onRunKindFilterChange={(runKind) => {
                  setRunKindFilter(runKind);
                  setPage(1);
                }}
                onStatusFilterChange={(status) => {
                  setStatusFilter(status);
                  setPage(1);
                }}
                page={page}
                pageSize={PAGE_SIZE}
                result={runs.data}
                runKindFilter={runKindFilter}
                statusFilter={statusFilter}
              />
              <div className="flex flex-col gap-4">
                <SchedulingReadinessPanel
                  isError={schedulingReadiness.isError}
                  isLoading={schedulingReadiness.isLoading}
                  onRetry={() => schedulingReadiness.refetch()}
                  readiness={schedulingReadiness.data}
                />
                <EffectiveContextPanel
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
              </div>
            </section>

            <EmployeeConfigSnapshotSection
              employee={employee.data}
              executionInstance={instance.data}
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

      {employee.data ? (
        <ConfirmDialog
          cancelBtnText="取消"
          className="sm:max-w-xl"
          confirmText="确认删除"
          desc={
            <div className="space-y-2">
              <p>
                删除后该数字员工会从当前员工列表、团队候选和项目候选中隐藏；历史运行、项目任务、
                工件和审计记录会保留。Runtime 工作目录不会在本次操作中物理删除。
              </p>
              <p>
                如仍有排队、分发中、运行中、取消中运行，或排队、运行中、处理中项目任务，删除会被阻断。
              </p>
            </div>
          }
          destructive
          disabled={!deleteConfirmReady || deleteEmployee.isPending}
          form="delete-digital-employee-form"
          isLoading={deleteEmployee.isPending}
          onOpenChange={handleDeleteDialogOpenChange}
          open={deleteDialogOpen}
          title="删除数字员工"
        >
          <form
            className="space-y-4"
            id="delete-digital-employee-form"
            onSubmit={(event) => {
              event.preventDefault();
              if (deleteConfirmReady) {
                deleteEmployee.mutate();
              }
            }}
          >
            <div className="space-y-2">
              <Label htmlFor="delete-digital-employee-confirmation">输入员工名称确认删除</Label>
              <Input
                autoComplete="off"
                id="delete-digital-employee-confirmation"
                onChange={(event) => {
                  setDeleteConfirmation(event.currentTarget.value);
                  if (deleteBlocked) setDeleteBlocked(undefined);
                }}
                value={deleteConfirmation}
              />
            </div>
            {deleteBlocked ? <DeleteBlockedAlert blocked={deleteBlocked} /> : null}
            {genericDeleteError ? (
              <p className="text-sm text-v3-danger">{genericDeleteError}</p>
            ) : null}
          </form>
        </ConfirmDialog>
      ) : null}
    </>
  );
}

function isActiveRun(status: DigitalEmployeeRunStatus) {
  return activeRunStatuses.has(status);
}

function isDeleteBlockedError(
  error: unknown,
): error is ApiRequestError & { payload: DigitalEmployeeDeleteBlockedErrorResponse } {
  return (
    error instanceof ApiRequestError &&
    error.status === 409 &&
    error.code === "digital_employee_delete_blocked" &&
    isDeleteBlockedPayload(error.payload)
  );
}

function isDeleteBlockedPayload(payload: unknown): payload is DigitalEmployeeDeleteBlockedErrorResponse {
  if (!payload || typeof payload !== "object") return false;
  const value = payload as Partial<DigitalEmployeeDeleteBlockedErrorResponse>;
  return (
    value.code === "digital_employee_delete_blocked" &&
    typeof value.message === "string" &&
    Array.isArray(value.blockers)
  );
}

function getDeleteErrorMessage(error: unknown) {
  if (error instanceof ApiRequestError && error.detail) return error.detail;
  if (error instanceof Error) return error.message;
  return "删除失败，请稍后重试。";
}

function DeleteBlockedAlert({ blocked }: { blocked: DigitalEmployeeDeleteBlockedErrorResponse }) {
  return (
    <Alert className="border-v3-danger/30 bg-v3-danger-soft text-v3-danger" variant="destructive">
      <AlertTriangle className="size-4" />
      <AlertTitle>删除被阻断</AlertTitle>
      <AlertDescription>
        <p>{blocked.message}</p>
        <ul className="mt-3 space-y-2">
          {blocked.blockers.map((blocker) => (
            <DeleteBlockerItem blocker={blocker} key={`${blocker.type}:${blocker.id}`} />
          ))}
        </ul>
      </AlertDescription>
    </Alert>
  );
}

function DeleteBlockerItem({ blocker }: { blocker: DigitalEmployeeDeleteBlocker }) {
  return (
    <li className="rounded-v3-inner border border-v3-danger/25 bg-v3-card px-3 py-2 text-v3-ink">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-semibold">{blocker.title}</span>
        <StatusPill tone="danger">{`${blocker.type} · ${blocker.status}`}</StatusPill>
      </div>
      <p className="mt-1 break-all font-mono text-[11px] text-v3-ink-3">
        {blocker.project_id ? `project ${blocker.project_id} · ` : ""}
        {blocker.run_id ? `run ${blocker.run_id} · ` : ""}
        id {blocker.id}
      </p>
    </li>
  );
}

function EmployeeConfigSnapshotSection({
  employee,
  executionInstance,
}: {
  employee: DigitalEmployee;
  executionInstance?: DigitalEmployeeExecutionInstance;
}) {
  const metadata = employee.metadata ?? {};
  const runtimeState = {
    effective_config_label: metadata.effective_config_label,
    effective_config_status: metadata.effective_config_status,
    execution_instance_status: executionInstance?.status,
    provider_type: employee.provider_type,
    runtime_node_id: executionInstance?.runtime_node_id,
  };

  return (
    <section className="grid gap-4 lg:grid-cols-2">
      <ConfigSnapshotCard label="人格记忆.md" value={employee.persona_memory_markdown || "未设置"} />
      <ConfigSnapshotCard
        label="能力绑定"
        value={formatConfigSnapshotJson(employee.capability_bindings ?? {})}
      />
      <ConfigSnapshotCard label="预算策略" value={formatConfigSnapshotJson(employee.budget_policy ?? {})} />
      <ConfigSnapshotCard
        label="运行与缓存状态"
        value={hasRuntimeState(runtimeState) ? formatConfigSnapshotJson(runtimeState) : "暂无运行与缓存状态"}
      />
    </section>
  );
}

function ConfigSnapshotCard({ label, value }: { label: string; value: string }) {
  return (
    <SoftCard className="p-4">
      <div className="text-sm font-semibold text-v3-ink">{label}</div>
      <pre className="mt-3 overflow-x-auto whitespace-pre-wrap rounded-[14px] border border-v3-line bg-v3-card-soft p-3 font-mono text-xs text-v3-ink">
        {value}
      </pre>
    </SoftCard>
  );
}

function formatConfigSnapshotJson(value: Record<string, unknown>) {
  return JSON.stringify(value, null, 2);
}

function hasRuntimeState(value: Record<string, unknown>) {
  return Object.values(value).some((item) => item !== undefined && item !== "");
}

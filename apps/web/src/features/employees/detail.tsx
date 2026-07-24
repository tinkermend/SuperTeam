import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { AlertTriangle } from "lucide-react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Main } from "@/components/layout/main";
import {
  ShellPageHeader,
  ShellPageHeaderBack
} from "@/components/layout/shell-page-header";
import { MasterDetailLayout, StatusPill, Segmented } from "@/components/superteam";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ApiRequestError } from "@/lib/api/client";
import { listEffectiveMcpConfig } from "@/lib/api/capabilities";
import {
  deleteDigitalEmployee,
  getDigitalEmployee,
  getDigitalEmployeeRun,
  getDigitalEmployeeRunCalendar,
  getDigitalEmployeeSchedulingReadiness,
  getDigitalEmployeeRunStats,
  listDigitalEmployeeRuns,
  listEmployeeEnvironmentVariables,
  type DigitalEmployeeDeleteBlockedErrorResponse,
  type DigitalEmployeeDeleteBlocker,
  type DigitalEmployeeRun,
  type DigitalEmployeeRunCalendarItem,
  type DigitalEmployeeRunKind,
  type DigitalEmployeeRunListItem,
  type DigitalEmployeeRunStatus
} from "@/lib/api/employees";
import { listEmployeeSkills } from "@/lib/api/skills";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { EmployeeCapabilityRail } from "./components/employee-capability-rail";
import { EmployeeDetailHeader } from "./components/employee-detail-header";
import { EmployeeRunHistoryTable } from "./components/employee-run-history-table";
import {
  EmployeeWorkCalendar,
  employeeWeekQueryWindow,
  employeeWeekStart
} from "./components/employee-work-calendar";
import { RunDetailDrawer } from "./components/run-detail-drawer";
import { deleteBlockerTypeLabel, statusLabel } from "@/lib/status-labels";

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
  const [historyView, setHistoryView] = useState<"calendar" | "list">("calendar");
  const [weekStart, setWeekStart] = useState(() => employeeWeekStart(new Date()));
  const [selectedRun, setSelectedRun] = useState<DigitalEmployeeRunListItem | undefined>(undefined);
  const [runDrawerOpen, setRunDrawerOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteConfirmation, setDeleteConfirmation] = useState("");
  const [deleteBlocked, setDeleteBlocked] =
    useState<DigitalEmployeeDeleteBlockedErrorResponse | undefined>(undefined);
  const [calendarOpenError, setCalendarOpenError] = useState<string | undefined>(undefined);

  const employee = useQuery({
    queryKey: ["digital-employee", employeeId],
    queryFn: () => getDigitalEmployee(apiOptions, employeeId),
    retry: (failureCount, error) =>
      !(error instanceof ApiRequestError && error.status === 404) && failureCount < 3
});
  // 员工不存在（已删除或路由过期）时停掉所有从属查询，避免对已删资源持续打 404。
  const employeeNotFound =
    employee.error instanceof ApiRequestError && employee.error.status === 404;
  const schedulingReadiness = useQuery({
    enabled: !employeeNotFound,
    queryKey: ["digital-employee-scheduling-readiness", employeeId],
    queryFn: () => getDigitalEmployeeSchedulingReadiness(apiOptions, employeeId),
    retry: false
});
  const runStats = useQuery({
    enabled: !employeeNotFound,
    queryKey: ["digital-employee-run-stats", employeeId],
    queryFn: () => getDigitalEmployeeRunStats(apiOptions, employeeId)
});
  const runs = useQuery({
    enabled: !employeeNotFound && historyView === "list",
    queryKey: ["digital-employee-runs", employeeId, { page, statusFilter, runKindFilter }] as const,
    queryFn: () =>
      listDigitalEmployeeRuns(apiOptions, employeeId, {
        limit: PAGE_SIZE,
        offset: (page - 1) * PAGE_SIZE,
        status: statusFilter ? [statusFilter] : undefined,
        run_kind: runKindFilter
}),
    refetchInterval: (query) =>
      query.state.data?.items.some((item) => isActiveRun(item.status)) ? 2500 : false
});
  // Encode local Mon 00:00 → next Mon 00:00 as absolute UTC ISO so the API window
  // matches the calendar's local-day columns (do not send "UTC midnight" labels).
  const weekWindow = employeeWeekQueryWindow(weekStart);
  const calendar = useQuery({
    enabled: !employeeNotFound && historyView === "calendar",
    queryKey: [
      "digital-employee-run-calendar",
      employeeId,
      weekWindow.from,
      weekWindow.to,
    ] as const,
    queryFn: () =>
      getDigitalEmployeeRunCalendar(apiOptions, employeeId, {
        from: weekWindow.from,
        to: weekWindow.to
})
});
  // Lifted from EffectiveContextPanel (Task 11) so detail.tsx can feed the panel
  // (as computed props). The panel is now a pure presentational component.
  const skillsQuery = useQuery({
    enabled: !employeeNotFound,
    queryKey: ["employee-skills", employeeId],
    queryFn: () => listEmployeeSkills(apiOptions, employeeId)
});
  const mcpQuery = useQuery({
    enabled: !employeeNotFound,
    queryKey: ["employee-effective-mcp", employeeId],
    queryFn: () => listEffectiveMcpConfig(apiOptions, employeeId)
});
  const envVarsQuery = useQuery({
    enabled: !employeeNotFound,
    queryKey: ["employee-environment-variables", employeeId],
    queryFn: () => listEmployeeEnvironmentVariables(apiOptions, employeeId)
});

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

  const refreshRunFacts = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["digital-employee-runs", employeeId] }),
      queryClient.invalidateQueries({ queryKey: ["digital-employee-run-stats", employeeId] }),
      queryClient.invalidateQueries({ queryKey: ["digital-employee-run-calendar", employeeId] }),
    ]);
  };

  const openCalendarItem = async (item: DigitalEmployeeRunCalendarItem) => {
    setCalendarOpenError(undefined);
    try {
      const run = await getDigitalEmployeeRun(apiOptions, employeeId, item.id);
      setSelectedRun({
        ...run,
        task_title: item.task_title,
        project_id: item.project_id,
        project_name: item.project_name,
        work_product_count: Array.isArray(run.work_products) ? run.work_products.length : 0
});
      setRunDrawerOpen(true);
    } catch {
      setCalendarOpenError("打开运行详情失败，请稍后重试");
    }
  };

  const deleteEmployee = useMutation({
    mutationFn: () => deleteDigitalEmployee(apiOptions, employeeId),
    onMutate: () => {
      setDeleteBlocked(undefined);
    },
    onSuccess: async () => {
      // 先离开详情页再清缓存：详情页还挂着时 invalidate 会立刻重取已删员工，打出 404。
      await navigate({ to: "/employees" });
      for (const key of [
        "digital-employee",
        "digital-employee-scheduling-readiness",
        "digital-employee-run-stats",
        "digital-employee-runs",
        "digital-employee-run-calendar",
        "employee-skills",
        "employee-effective-mcp",
        "employee-environment-variables",
      ]) {
        queryClient.removeQueries({ queryKey: [key, employeeId] });
      }
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["digital-employees"] }),
        queryClient.invalidateQueries({ queryKey: ["digital-employee-overview"] }),
        queryClient.invalidateQueries({ queryKey: ["digital-employees-overview"] }),
        queryClient.invalidateQueries({ queryKey: ["unassigned-digital-employees"] }),
        queryClient.invalidateQueries({ queryKey: ["digital-employee-create-options"] }),
        queryClient.invalidateQueries({ queryKey: ["projects"] }),
      ]);
    },
    onError: (error) => {
      if (isDeleteBlockedError(error)) {
        setDeleteBlocked(error.payload);
      }
    }
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
        subtitle="身份与工作节奏。"
      />
      <Main width="wide" className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden py-4">
        {employee.isLoading ? <p className="text-sm text-ink-2">加载中</p> : null}
        {employeeNotFound ? (
          <p className="text-sm text-ink-2">该数字员工不存在或已被删除。</p>
        ) : employee.isError ? (
          <p className="text-sm text-destructive">数字员工加载失败</p>
        ) : null}

        {employee.data ? (
          <div className="flex min-h-[calc(100dvh-9.5rem)] flex-col gap-3">
            <EmployeeDetailHeader
              employee={employee.data}
              onDelete={() => setDeleteDialogOpen(true)}
              stats={runStats.data}
            />

            <MasterDetailLayout
              className="min-h-0 flex-1 [&>div]:h-full [&>div]:items-stretch [&>div]:gap-3"
              narrowDetail="stack"
              rail="md"
              master={
                <div className="flex h-full min-h-0 min-w-0 flex-col gap-2">
                  <div className="flex shrink-0 flex-wrap items-center justify-between gap-2">
                    <div>
                      <h3 className="text-sm font-semibold text-ink">工作节奏</h3>
                      <p className="mt-0.5 text-[11px] text-ink-3">按日查看做过什么；点条目打开运行详情。</p>
                    </div>
                    <Segmented
                      aria-label="工作节奏视图"
                      onChange={setHistoryView}
                      options={[
                        { value: "calendar", label: "日历" },
                        { value: "list", label: "列表" },
                      ]}
                      role="group"
                      value={historyView}
                    />
                  </div>
                  {calendarOpenError ? (
                    <p className="shrink-0 text-sm text-danger">{calendarOpenError}</p>
                  ) : null}
                  {historyView === "list" ? (
                    <div className="min-h-0 flex-1 overflow-auto">
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
                    </div>
                  ) : (
                    <EmployeeWorkCalendar
                      error={calendar.error}
                      isError={calendar.isError}
                      isLoading={calendar.isLoading}
                      items={calendar.data?.items ?? []}
                      onItemClick={(item) => {
                        void openCalendarItem(item);
                      }}
                      onRetry={() => calendar.refetch()}
                      onWeekChange={setWeekStart}
                      totalCount={calendar.data?.total_count ?? 0}
                      truncated={calendar.data?.truncated}
                      weekStart={weekStart}
                    />
                  )}
                </div>
              }
              detail={
                <EmployeeCapabilityRail
                  employee={employee.data}
                  employeeId={employeeId}
                  envVars={{
                    isLoading: envVarsQuery.isLoading,
                    isError: envVarsQuery.isError,
                    configuredCount: configuredEnvCount,
                    totalCount: envVarsQuery.data?.length ?? 0,
                    missingNames: missingEnvVars.map((item) => item.name)
}}
                  mcp={{
                    isLoading: mcpQuery.isLoading,
                    isError: mcpQuery.isError,
                    personalCount: personalMcpCount,
                    inheritedCount: inheritedMcpCount,
                    totalCount: mcpQuery.data?.length ?? 0
}}
                  onRetryReadiness={() => schedulingReadiness.refetch()}
                  readiness={schedulingReadiness.data}
                  readinessError={schedulingReadiness.isError}
                  readinessLoading={schedulingReadiness.isLoading}
                  skills={{
                    isLoading: skillsQuery.isLoading,
                    isError: skillsQuery.isError,
                    personalCount: personalSkillCount,
                    inheritedCount: inheritedSkillCount,
                    totalCount: skillsQuery.data?.length ?? 0
}}
                />
              }
            />
          </div>
        ) : null}
      </Main>

      <RunDetailDrawer
        apiOptions={apiOptions}
        employeeId={employeeId}
        onOpenChange={setRunDrawerOpen}
        onStopped={handleStopped}
        open={runDrawerOpen}
        run={selectedRun}
      />

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
              <p className="text-sm text-danger">{genericDeleteError}</p>
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
    <Alert className="border-danger/30 bg-danger-soft text-danger" variant="destructive">
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
    <li className="rounded-inner border border-danger/25 bg-card px-3 py-2 text-ink">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-semibold">{blocker.title}</span>
        <StatusPill tone="danger">{`${deleteBlockerTypeLabel(blocker.type)} · ${statusLabel(blocker.status)}`}</StatusPill>
      </div>
      <p className="mt-1 break-all font-mono text-[11px] text-ink-3">
        {blocker.project_id ? `project ${blocker.project_id} · ` : ""}
        {blocker.run_id ? `run ${blocker.run_id} · ` : ""}
        id {blocker.id}
      </p>
    </li>
  );
}

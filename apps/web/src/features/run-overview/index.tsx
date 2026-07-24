import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { Activity, Pause, Play, RefreshCw } from "lucide-react";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import { MasterDetailLayout, Button, ErrorState, LoadingState } from "@/components/superteam";
import { getDigitalEmployeeActivity, getDigitalEmployeeOverview } from "@/lib/api/employees";
import { getProjectTaskGraph, listProjectDemands } from "@/lib/api/projects";
import { listTeamSummaries } from "@/lib/api/teams";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { EmployeeDetailCard } from "./components/employee-detail-card";
import { RuntimeMapStage } from "./components/runtime-map-stage";
import { RuntimeOverviewSidePanel } from "./components/runtime-overview-side-panel";
import { buildRuntimeOverview } from "./runtime-overview-adapter";
import type { RuntimeOverviewFloorId } from "./runtime-overview-model";
import { buildProjectLens, lensParticipantFloorIds } from "./runtime-overview-project-lens";
import { useRuntimeFocusCarousel } from "./use-runtime-focus-carousel";

export function RunOverviewPage() {
  return <RunOverviewView apiBaseUrl={resolveControlPlaneUrl()} />;
}

type RunOverviewViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  // 测试注入用；生产默认用带凭据的原生 EventSource。
  eventSourceFactory?: (url: string) => EventSource;
};

export function RunOverviewView({ apiBaseUrl, fetcher, eventSourceFactory }: RunOverviewViewProps) {
  const search = useSearch({ strict: false }) as { employee?: string; project?: string };
  const navigate = useNavigate();
  const [activeFloorId, setActiveFloorId] = useState<RuntimeOverviewFloorId>("floor-1");
  const [selectedEmployeeId, setSelectedEmployeeId] = useState<string>();
  // 项目透镜：初始从 ?project= 深链进入；选中/退出会回写 URL 供分享与反向链入。
  const [selectedProjectId, setSelectedProjectId] = useState<string | undefined>(search.project || undefined);
  useEffect(() => {
    setSelectedProjectId(search.project || undefined);
  }, [search.project]);
  const employees = useQuery({
    queryKey: ["run-overview", "digital-employees"],
    queryFn: () => getDigitalEmployeeOverview({ baseUrl: apiBaseUrl, fetcher }, { limit: 100 }),
    refetchInterval: 10_000
});
  const teams = useQuery({
    queryKey: ["run-overview", "teams"],
    queryFn: () => listTeamSummaries({ baseUrl: apiBaseUrl, fetcher }, { limit: 100, status: "active" }),
    refetchInterval: 10_000
});
  // 跨员工动态流走专用 activity 端点（服务端标签映射 + 不受每员工 top3 截断）；失败时回退 overview 聚合。
  const activity = useQuery({
    queryKey: ["run-overview", "activity"],
    queryFn: () => getDigitalEmployeeActivity({ baseUrl: apiBaseUrl, fetcher }, { limit: 8 }),
    refetchInterval: 10_000,
    retry: false
});
  // 项目透镜的任务链路：仅选中项目时拉取，零常驻成本；与 overview 同频轮询。
  // task-graph 端点必须限定 demand 域。默认选最新 demand，但选定后"钉住"——
  // 新 demand 到达不抢当前视图（除非当前 demand 从列表消失），并提供显式切换。
  const projectDemands = useQuery({
    queryKey: ["run-overview", "project-demands", selectedProjectId],
    queryFn: () => listProjectDemands({ baseUrl: apiBaseUrl, fetcher }, selectedProjectId as string, { limit: 20 }),
    enabled: Boolean(selectedProjectId),
    refetchInterval: 10_000,
    retry: false
});
  const [selectedDemandId, setSelectedDemandId] = useState<string>();
  const demandList = useMemo(() => projectDemands.data ?? [], [projectDemands.data]);
  const demandIdsKey = demandList.map((demand) => demand.id).join("|");
  useEffect(() => {
    setSelectedDemandId(undefined);
  }, [selectedProjectId]);
  useEffect(() => {
    if (demandList.length === 0) return;
    setSelectedDemandId((current) =>
      current && demandList.some((demand) => demand.id === current) ? current : demandList[0].id,
    );
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [demandIdsKey]);
  const taskGraph = useQuery({
    queryKey: ["run-overview", "task-graph", selectedProjectId, selectedDemandId],
    queryFn: () =>
      getProjectTaskGraph({ baseUrl: apiBaseUrl, fetcher }, selectedProjectId as string, {
        demandId: selectedDemandId as string
}),
    enabled: Boolean(selectedProjectId) && Boolean(selectedDemandId),
    refetchInterval: 10_000,
    retry: false
});
  // SSE 秒级推送：activity/stream 推来新事件时立即刷新动态流与 overview（节流 2s），
  // 状态变化经既有轮询 diff 通道触发轮播插队；流断开由 EventSource 自动重连，10s 轮询兜底。
  const queryClient = useQueryClient();
  const lastStreamInvalidateRef = useRef(0);
  useEffect(() => {
    // 组件测试注入 fetcher 时默认不开真实流，避免连不上的重连噪音；显式给 factory 则照常开。
    if (fetcher && !eventSourceFactory) return;
    const factory =
      eventSourceFactory ?? ((url: string) => new EventSource(url, { withCredentials: true }));
    let source: EventSource | undefined;
    try {
      source = factory(`${apiBaseUrl}/api/v1/digital-employees/activity/stream`);
    } catch {
      return;
    }
    const onActivity = () => {
      const now = Date.now();
      if (now - lastStreamInvalidateRef.current < 2_000) return;
      lastStreamInvalidateRef.current = now;
      void queryClient.invalidateQueries({ queryKey: ["run-overview", "activity"] });
      void queryClient.invalidateQueries({ queryKey: ["run-overview", "digital-employees"] });
      void queryClient.invalidateQueries({ queryKey: ["run-overview", "project-demands"] });
      void queryClient.invalidateQueries({ queryKey: ["run-overview", "task-graph"] });
    };
    source.addEventListener("activity", onActivity);
    return () => {
      source?.removeEventListener("activity", onActivity);
      source?.close();
    };
  }, [apiBaseUrl, eventSourceFactory, fetcher, queryClient]);

  const recentActivity = useMemo(
    () =>
      activity.data?.items.map((item) => ({
        employeeId: item.digital_employee_id,
        employeeName: item.digital_employee_name,
        teamId: item.team_id ?? "unassigned",
        label: item.label,
        status: item.status,
        occurredAt: item.occurred_at,
        taskTitle: item.task_title || undefined,
        projectName: item.project_name || undefined
})),
    [activity.data],
  );
  const overview = useMemo(() => {
    if (!employees.data || !teams.data) return undefined;
    return buildRuntimeOverview({ activeFloorId, employees: employees.data, generatedAt: new Date().toISOString(), teams: teams.data });
  }, [activeFloorId, employees.data, teams.data]);
  const error = employees.error ?? teams.error;
  const lens = useMemo(
    () => (selectedProjectId && taskGraph.data ? buildProjectLens(selectedProjectId, taskGraph.data) : undefined),
    [selectedProjectId, taskGraph.data],
  );
  const lensFloorIds = useMemo(
    () => (lens && overview ? new Set(lensParticipantFloorIds(lens, overview)) : undefined),
    [lens, overview],
  );
  const carousel = useRuntimeFocusCarousel({
    employees: overview?.employees ?? [],
    initialInteracted: Boolean(search.employee),
    // 透镜态强制暂停轮播：链路阅读期间焦点不被抢走，退出透镜即恢复。
    forcePaused: Boolean(selectedProjectId)
});
  const searchEmployeeId =
    search.employee && overview?.employees.some((employee) => employee.employeeId === search.employee)
      ? search.employee
      : undefined;
  // 轮播运行时焦点即选中；被交互暂停时用户选择优先，超时自动回到轮播。
  const userSelectedEmployeeId = searchEmployeeId ?? selectedEmployeeId;
  const effectiveSelectedEmployeeId = carousel.isPaused
    ? (userSelectedEmployeeId ?? carousel.focusEmployeeId ?? overview?.selectedEmployeeId)
    : (carousel.focusEmployeeId ?? userSelectedEmployeeId ?? overview?.selectedEmployeeId);
  const selectedEmployee =
    overview?.employees.find((employee) => employee.employeeId === effectiveSelectedEmployeeId) ?? overview?.employees[0];
  const selectedTeam = selectedEmployee ? overview?.teams.find((team) => team.teamId === selectedEmployee.teamId) : undefined;

  // 楼层跟随：轮播驱动的焦点跨楼层时切换楼层；暂停期间尊重用户手动浏览。
  const carouselFocusFloorId = !carousel.isPaused
    ? overview?.employees.find((employee) => employee.employeeId === carousel.focusEmployeeId)?.floorId
    : undefined;
  useEffect(() => {
    if (carouselFocusFloorId && carouselFocusFloorId !== activeFloorId) {
      setActiveFloorId(carouselFocusFloorId);
    }
  }, [carouselFocusFloorId, activeFloorId]);

  const handleSelectEmployee = (employeeId: string) => {
    setSelectedEmployeeId(employeeId);
    carousel.notifyInteraction();
  };
  const handleSelectFloor = (floorId: RuntimeOverviewFloorId) => {
    setActiveFloorId(floorId);
    carousel.notifyInteraction();
  };
  const handleSelectProject = (projectId?: string) => {
    setSelectedProjectId(projectId);
    void navigate({
      to: "/run-overview",
      search: (previous: { employee?: string; project?: string }) => ({ ...previous, project: projectId || undefined }),
      replace: true
});
  };
  const handleRefresh = () => {
    void employees.refetch();
    void teams.refetch();
    if (selectedProjectId) {
      void projectDemands.refetch();
      void taskGraph.refetch();
    }
  };

  return (
    <>
      <ShellPageHeader
        icon={<Activity className="size-4" />}
        title="运行总览"
        subtitle="按楼层展示团队运行态、数字员工状态和容量占用。"
      />
      <Main width="wide" className="min-w-0">
      {employees.isPending || teams.isPending ? <LoadingState label="正在加载运行总览" /> : null}
      {error ? <ErrorState title="运行总览加载失败" description={error.message} /> : null}
      {overview ? (
        <MasterDetailLayout
          aria-label="运行总览地图"
          narrowDetail="stack"
          rail="md"
          role="region"
          master={
            <div className="min-w-0">
              <div data-runtime-overview-toolbar className="mb-4 flex flex-wrap items-center gap-3">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="inline-flex h-10 items-center rounded-inner border border-line bg-white/80 px-4 text-sm font-semibold text-ink-2 shadow-sm">
                    全部重点
                  </span>
                  {overview.floors.map((floor) => {
                    const lobbyCount =
                      floor.floorId === "lobby"
                        ? overview.employees.filter((employee) => employee.floorId === "lobby").length
                        : 0;
                    return (
                      <Button
                        key={floor.floorId}
                        type="button"
                        variant={activeFloorId === floor.floorId ? "primary" : "outline"}
                        onClick={() => handleSelectFloor(floor.floorId)}
                      >
                        {floor.label}
                        {lobbyCount > 0 ? (
                          <span data-runtime-lobby-count className="ml-1 tabular-nums">
                            · {lobbyCount}
                          </span>
                        ) : null}
                        {lensFloorIds?.has(floor.floorId) ? (
                          <span
                            data-runtime-lens-floor-dot={floor.floorId}
                            className="ml-1 size-2 rounded-full bg-brand"
                            aria-label="该楼层有选中项目的参与员工"
                          />
                        ) : null}
                      </Button>
                    );
                  })}
                  <span className="inline-flex h-10 items-center rounded-inner border border-line bg-white/80 px-4 text-sm font-semibold text-ink-2 shadow-sm">
                    异常优先
                  </span>
                </div>
                <Button type="button" variant="outline" aria-label="刷新运行总览" onClick={handleRefresh}>
                  <RefreshCw className="size-4" />
                  刷新
                </Button>
                <StatusLegend className="mx-auto" />
                {carousel.queue.length > 0 ? (
                  <div data-runtime-carousel-indicator className="flex items-center gap-2 text-sm text-ink-2">
                    {selectedProjectId ? (
                      <span className="tabular-nums">项目透镜聚焦中 · 轮播已暂停</span>
                    ) : (
                      <>
                        <span className="tabular-nums">
                          {carousel.isPaused
                            ? "轮播已暂停 · 稍后自动恢复"
                            : `焦点轮播 ${carousel.queueIndex >= 0 ? carousel.queueIndex + 1 : 1} / ${carousel.queue.length}`}
                        </span>
                        <Button
                          type="button"
                          size="sm"
                          variant="outline"
                          aria-label={carousel.isPaused ? "恢复轮播" : "暂停轮播"}
                          onClick={() => (carousel.isPaused ? carousel.resume() : carousel.notifyInteraction())}
                        >
                          {carousel.isPaused ? <Play className="size-3.5" /> : <Pause className="size-3.5" />}
                          {carousel.isPaused ? "恢复" : "暂停"}
                        </Button>
                      </>
                    )}
                  </div>
                ) : null}
              </div>
              <RuntimeMapStage
                activeFloorId={activeFloorId}
                lens={lens}
                overview={overview}
                selectedEmployeeId={effectiveSelectedEmployeeId}
                onSelectEmployee={handleSelectEmployee}
                onSelectFloor={handleSelectFloor}
              />
              {selectedEmployee ? (
                <div className="mt-5">
                  <EmployeeDetailCard employee={selectedEmployee} team={selectedTeam} />
                </div>
              ) : null}
            </div>
          }
          detail={
            <RuntimeOverviewSidePanel
              overview={overview}
              activity={recentActivity}
              selectedProjectId={selectedProjectId}
              onSelectProject={handleSelectProject}
              lens={lens}
              lensLoading={projectDemands.isLoading || taskGraph.isLoading}
              demands={demandList}
              selectedDemandId={selectedDemandId}
              onSelectDemand={setSelectedDemandId}
            />
          }
        />
      ) : null}
      </Main>
    </>
  );
}

function StatusLegend({ className }: { className?: string }) {
  const items = [
    { label: "异常", className: "bg-danger" },
    { label: "工作中", className: "bg-ok" },
    { label: "待确认", className: "bg-warn" },
    { label: "排队", className: "bg-info" },
    { label: "空闲", className: "bg-mute" },
  ];
  return (
    <div className={`flex flex-wrap items-center gap-3 rounded-inner border border-line bg-white/82 px-3 py-2 text-xs font-medium text-ink-2 shadow-sm ${className ?? ""}`}>
      {items.map((item) => (
        <span key={item.label} className="inline-flex items-center gap-1.5">
          <span className={`size-2.5 rounded-full ${item.className}`} aria-hidden />
          {item.label}
        </span>
      ))}
    </div>
  );
}

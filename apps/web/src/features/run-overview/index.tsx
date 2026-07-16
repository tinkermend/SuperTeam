import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useSearch } from "@tanstack/react-router";
import { Activity, Pause, Play, RefreshCw } from "lucide-react";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import { MasterDetailLayout, V3Button, V3ErrorState, V3LoadingState } from "@/components/superteam";
import { getDigitalEmployeeActivity, getDigitalEmployeeOverview } from "@/lib/api/employees";
import { listTeamSummaries } from "@/lib/api/teams";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { EmployeeDetailCard } from "./components/employee-detail-card";
import { RuntimeMapStage } from "./components/runtime-map-stage";
import { RuntimeOverviewSidePanel } from "./components/runtime-overview-side-panel";
import { buildRuntimeOverview } from "./runtime-overview-adapter";
import type { RuntimeOverviewFloorId } from "./runtime-overview-model";
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
  const [activeFloorId, setActiveFloorId] = useState<RuntimeOverviewFloorId>("floor-1");
  const [selectedEmployeeId, setSelectedEmployeeId] = useState<string>();
  const employees = useQuery({
    queryKey: ["run-overview", "digital-employees"],
    queryFn: () => getDigitalEmployeeOverview({ baseUrl: apiBaseUrl, fetcher }, { limit: 100 }),
    refetchInterval: 10_000,
  });
  const teams = useQuery({
    queryKey: ["run-overview", "teams"],
    queryFn: () => listTeamSummaries({ baseUrl: apiBaseUrl, fetcher }, { limit: 100, status: "active" }),
    refetchInterval: 10_000,
  });
  // 跨员工动态流走专用 activity 端点（服务端标签映射 + 不受每员工 top3 截断）；失败时回退 overview 聚合。
  const activity = useQuery({
    queryKey: ["run-overview", "activity"],
    queryFn: () => getDigitalEmployeeActivity({ baseUrl: apiBaseUrl, fetcher }, { limit: 8 }),
    refetchInterval: 10_000,
    retry: false,
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
        projectName: item.project_name || undefined,
      })),
    [activity.data],
  );
  const overview = useMemo(() => {
    if (!employees.data || !teams.data) return undefined;
    return buildRuntimeOverview({ activeFloorId, employees: employees.data, generatedAt: new Date().toISOString(), teams: teams.data });
  }, [activeFloorId, employees.data, teams.data]);
  const error = employees.error ?? teams.error;
  const carousel = useRuntimeFocusCarousel({
    employees: overview?.employees ?? [],
    initialInteracted: Boolean(search.employee),
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
  const handleRefresh = () => {
    void employees.refetch();
    void teams.refetch();
  };

  return (
    <>
      <ShellPageHeader
        icon={<Activity className="size-4" />}
        title="运行总览"
        subtitle="按楼层展示团队运行态、数字员工状态和容量占用。"
      />
      <Main width="wide" className="min-w-0">
      {employees.isPending || teams.isPending ? <V3LoadingState label="正在加载运行总览" /> : null}
      {error ? <V3ErrorState title="运行总览加载失败" description={error.message} /> : null}
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
                  <span className="inline-flex h-10 items-center rounded-v3-inner border border-v3-line bg-white/80 px-4 text-sm font-semibold text-v3-ink-2 shadow-sm">
                    全部重点
                  </span>
                  {overview.floors.map((floor) => (
                    <V3Button
                      key={floor.floorId}
                      type="button"
                      variant={activeFloorId === floor.floorId ? "primary" : "outline"}
                      onClick={() => handleSelectFloor(floor.floorId)}
                    >
                      {floor.label}
                    </V3Button>
                  ))}
                  <span className="inline-flex h-10 items-center rounded-v3-inner border border-v3-line bg-white/80 px-4 text-sm font-semibold text-v3-ink-2 shadow-sm">
                    异常优先
                  </span>
                </div>
                <V3Button type="button" variant="outline" aria-label="刷新运行总览" onClick={handleRefresh}>
                  <RefreshCw className="size-4" />
                  刷新
                </V3Button>
                <StatusLegend className="mx-auto" />
                {carousel.queue.length > 0 ? (
                  <div data-runtime-carousel-indicator className="flex items-center gap-2 text-sm text-v3-ink-2">
                    <span className="tabular-nums">
                      {carousel.isPaused
                        ? "轮播已暂停 · 稍后自动恢复"
                        : `焦点轮播 ${carousel.queueIndex >= 0 ? carousel.queueIndex + 1 : 1} / ${carousel.queue.length}`}
                    </span>
                    <V3Button
                      type="button"
                      size="sm"
                      variant="outline"
                      aria-label={carousel.isPaused ? "恢复轮播" : "暂停轮播"}
                      onClick={() => (carousel.isPaused ? carousel.resume() : carousel.notifyInteraction())}
                    >
                      {carousel.isPaused ? <Play className="size-3.5" /> : <Pause className="size-3.5" />}
                      {carousel.isPaused ? "恢复" : "暂停"}
                    </V3Button>
                  </div>
                ) : null}
              </div>
              <RuntimeMapStage activeFloorId={activeFloorId} overview={overview} selectedEmployeeId={effectiveSelectedEmployeeId} onSelectEmployee={handleSelectEmployee} />
              {selectedEmployee ? (
                <div className="mt-5">
                  <EmployeeDetailCard employee={selectedEmployee} team={selectedTeam} />
                </div>
              ) : null}
            </div>
          }
          detail={<RuntimeOverviewSidePanel overview={overview} activity={recentActivity} />}
        />
      ) : null}
      </Main>
    </>
  );
}

function StatusLegend({ className }: { className?: string }) {
  const items = [
    { label: "异常", className: "bg-v3-danger" },
    { label: "工作中", className: "bg-v3-ok" },
    { label: "待确认", className: "bg-v3-warn" },
    { label: "排队", className: "bg-v3-info" },
    { label: "空闲", className: "bg-v3-mute" },
  ];
  return (
    <div className={`flex flex-wrap items-center gap-3 rounded-v3-inner border border-v3-line bg-white/82 px-3 py-2 text-xs font-medium text-v3-ink-2 shadow-sm ${className ?? ""}`}>
      {items.map((item) => (
        <span key={item.label} className="inline-flex items-center gap-1.5">
          <span className={`size-2.5 rounded-full ${item.className}`} aria-hidden />
          {item.label}
        </span>
      ))}
    </div>
  );
}

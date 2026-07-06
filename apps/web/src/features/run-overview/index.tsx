import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Activity, RefreshCw } from "lucide-react";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import { V3Button, V3ErrorState, V3LoadingState } from "@/components/superteam";
import { getDigitalEmployeeOverview } from "@/lib/api/employees";
import { listTeamSummaries } from "@/lib/api/teams";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { RuntimeMapStage } from "./components/runtime-map-stage";
import { RuntimeOverviewSidePanel } from "./components/runtime-overview-side-panel";
import { buildRuntimeOverview } from "./runtime-overview-adapter";
import type { RuntimeOverviewFloorId } from "./runtime-overview-model";

export function RunOverviewPage() {
  return <RunOverviewView apiBaseUrl={resolveControlPlaneUrl()} />;
}

export function RunOverviewView({ apiBaseUrl, fetcher }: { apiBaseUrl: string; fetcher?: typeof fetch }) {
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
  const overview = useMemo(() => {
    if (!employees.data || !teams.data) return undefined;
    return buildRuntimeOverview({ activeFloorId, employees: employees.data, generatedAt: new Date().toISOString(), teams: teams.data });
  }, [activeFloorId, employees.data, teams.data]);
  const error = employees.error ?? teams.error;
  const effectiveSelectedEmployeeId = selectedEmployeeId ?? overview?.selectedEmployeeId;

  return (
    <Main className="min-w-0">
      <ShellPageHeader
        icon={<Activity className="size-4" />}
        title="运行总览"
        subtitle="按楼层展示团队运行态、数字员工状态和容量占用。"
        actions={
          <V3Button
            type="button"
            variant="outline"
            onClick={() => {
              void employees.refetch();
              void teams.refetch();
            }}
          >
            <RefreshCw className="size-4" />
            刷新
          </V3Button>
        }
      />
      {employees.isPending || teams.isPending ? <V3LoadingState label="正在加载运行总览" /> : null}
      {error ? <V3ErrorState title="运行总览加载失败" description={error.message} /> : null}
      {overview ? (
        <section aria-label="运行总览地图" className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_340px]">
          <div className="min-w-0">
            <div className="mb-4 flex flex-wrap items-center gap-3">
              <div className="flex flex-wrap items-center gap-2">
                <span className="inline-flex h-10 items-center rounded-v3-inner border border-v3-line bg-white/80 px-4 text-sm font-semibold text-v3-ink-2 shadow-sm">
                  全部重点
                </span>
                {overview.floors.map((floor) => (
                  <V3Button
                    key={floor.floorId}
                    type="button"
                    variant={activeFloorId === floor.floorId ? "primary" : "outline"}
                    onClick={() => setActiveFloorId(floor.floorId)}
                  >
                    {floor.label}
                  </V3Button>
                ))}
                <span className="inline-flex h-10 items-center rounded-v3-inner border border-v3-line bg-white/80 px-4 text-sm font-semibold text-v3-ink-2 shadow-sm">
                  异常优先
                </span>
              </div>
              <StatusLegend className="ml-auto" />
            </div>
            <div className="mb-3 flex flex-wrap items-center justify-between gap-2 text-sm text-v3-ink-2">
              <p>当前楼层：{overview.floors.find((floor) => floor.floorId === activeFloorId)?.label}</p>
              <p className="text-xs">每层最多显示 6 个团队</p>
            </div>
            <RuntimeMapStage activeFloorId={activeFloorId} overview={overview} selectedEmployeeId={effectiveSelectedEmployeeId} onSelectEmployee={setSelectedEmployeeId} />
            <p className="mt-3 text-sm text-v3-ink-2">
              当前选择：{overview.employees.find((employee) => employee.employeeId === effectiveSelectedEmployeeId)?.name ?? "未选择"}
            </p>
          </div>
          <RuntimeOverviewSidePanel overview={overview} selectedEmployeeId={effectiveSelectedEmployeeId} />
        </section>
      ) : null}
    </Main>
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

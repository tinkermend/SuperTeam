import { humanWaitLabel } from "@/lib/status-labels";
import { GlassCard } from "@/components/superteam";
import type { ProjectDemand } from "@/lib/api/projects";
import type { ProjectLens, ProjectRunBandOption } from "../runtime-overview-project-lens";

// 大屏项目镜头的摘要浮层(spec §3.2):无侧栏形态下,观众必须能看出当前镜头聚焦的是哪个项目。
// 数据复用运行带聚合与透镜投影,不新增请求。
export function DisplayProjectShotCard({
  option,
  lens,
  demand
}: {
  option?: ProjectRunBandOption;
  lens?: ProjectLens;
  demand?: ProjectDemand;
}) {
  if (!option) return null;
  return (
    <GlassCard className="p-4" data-display-project-shot-card>
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <span className="text-sm text-ink-2">项目镜头</span>
        <span className="text-2xl font-semibold text-ink">{option.name}</span>
        {demand ? <span className="min-w-0 truncate text-sm text-ink-3">需求 · {demand.title}</span> : null}
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-base text-ink-2 tabular-nums">
        {lens ? (
          <>
            <span>参与 {lens.participantEmployeeIds.length} 人</span>
            <span>交接 {lens.edges.length} 段</span>
            {lens.blockedTaskCount > 0 ? <span className="font-semibold text-danger">阻塞 {lens.blockedTaskCount}</span> : null}
          </>
        ) : (
          <span className="text-ink-3">正在加载任务链路…</span>
        )}
        {option.runningCount > 0 ? <span className="text-info-text">运行 {option.runningCount}</span> : null}
        {option.failedCount > 0 ? <span className="font-semibold text-danger">失败 {option.failedCount}</span> : null}
        {option.waitingHumanCount > 0 ? (
          <span className="font-semibold text-warn-text">{humanWaitLabel("run_overview_badge")} {option.waitingHumanCount}</span>
        ) : null}
        {option.unassignedCount > 0 ? <span className="text-ink-3">待派发 {option.unassignedCount}</span> : null}
      </div>
    </GlassCard>
  );
}

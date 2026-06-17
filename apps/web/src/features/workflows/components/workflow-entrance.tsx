import { AlertCircle, Loader2 } from "lucide-react";
import { LiquidCard, StatusBadge } from "@/components/superteam";
import type { WorkflowInstanceSummary } from "@/lib/api/projects";
import { WorkflowInstanceCard } from "./workflow-instance-card";

type WorkflowEntranceProps = {
  instances: WorkflowInstanceSummary[];
  isError: boolean;
  isLoading: boolean;
};

export function WorkflowEntrance({
  instances,
  isError,
  isLoading,
}: WorkflowEntranceProps) {
  if (isError && instances.length === 0) {
    return (
      <LiquidCard className="rounded-xl p-6">
        <div className="flex items-start gap-3">
          <AlertCircle className="mt-0.5 size-5 shrink-0 text-[color:var(--superteam-danger)]" />
          <div className="min-w-0">
            <h2 className="text-base font-semibold tracking-normal">流程实例加载失败</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              暂时无法读取流程编排入口，请稍后重试。
            </p>
          </div>
        </div>
      </LiquidCard>
    );
  }

  if (isLoading && instances.length === 0) {
    return (
      <LiquidCard className="rounded-xl p-6">
        <div className="flex items-center gap-3 text-sm text-muted-foreground">
          <Loader2 className="size-4 animate-spin text-[color:var(--superteam-info)]" />
          正在加载流程实例
        </div>
      </LiquidCard>
    );
  }

  if (instances.length === 0) {
    return (
      <LiquidCard className="rounded-xl p-6">
        <h2 className="text-base font-semibold tracking-normal">暂无可见流程实例</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          有需求进入协调线程后，会在这里显示全局流程状态。
        </p>
      </LiquidCard>
    );
  }

  return (
    <div className="flex min-w-0 flex-col gap-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div className="min-w-0">
          <h2 className="text-base font-semibold tracking-normal">流程实例</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {instances.length} 个可见实例，进入单个实例查看编排画布和阶段详情。
          </p>
        </div>
        <div className="flex shrink-0 flex-wrap gap-2">
          {isLoading ? <StatusBadge tone="info">同步中</StatusBadge> : null}
          {isError ? <StatusBadge tone="danger">刷新失败</StatusBadge> : null}
        </div>
      </div>

      <div className="grid min-w-0 gap-4 md:grid-cols-2 2xl:grid-cols-3">
        {instances.map((instance) => (
          <WorkflowInstanceCard instance={instance} key={instance.demand_id} />
        ))}
      </div>
    </div>
  );
}

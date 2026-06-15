import { Link } from "@tanstack/react-router";
import { Clock3 } from "lucide-react";
import { LiquidCard, StatusBadge } from "@/components/superteam";
import { cn } from "@/lib/utils";
import type { WorkflowInstanceSummary } from "@/lib/api/projects";
import { workflowStatusLabel, workflowStatusTone } from "../workflow-status";

type WorkflowInstanceListProps = {
  instances: WorkflowInstanceSummary[];
  selectedDemandId?: string;
};

export function WorkflowInstanceList({
  instances,
  selectedDemandId,
}: WorkflowInstanceListProps) {
  if (instances.length === 0) {
    return (
      <LiquidCard className="rounded-xl p-5 text-sm text-muted-foreground">
        暂无可见流程实例
      </LiquidCard>
    );
  }

  return (
    <LiquidCard className="overflow-hidden rounded-xl">
      <div className="border-b px-4 py-3">
        <h2 className="text-sm font-semibold tracking-normal">流程实例</h2>
        <p className="text-xs text-muted-foreground">{instances.length} 个可见需求</p>
      </div>
      <div className="divide-y">
        {instances.map((instance) => {
          const isSelected = instance.demand_id === selectedDemandId;
          const completed = instance.progress.completed_nodes;
          const total = instance.progress.total_nodes;

          return (
            <Link
              aria-current={isSelected ? "page" : undefined}
              className={cn(
                "block px-4 py-3 transition-colors hover:bg-[color:var(--superteam-sidebar-hover)]",
                isSelected && "bg-[color:var(--superteam-sidebar-active)]",
              )}
              key={instance.demand_id}
              params={{ demandId: instance.demand_id }}
              to="/workflows/$demandId"
            >
              <div className="flex min-w-0 items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="line-clamp-2 text-sm font-semibold tracking-normal">
                    {instance.title}
                  </p>
                  <p className="mt-1 truncate text-xs text-muted-foreground">
                    {instance.project_name}
                  </p>
                </div>
                <StatusBadge tone={workflowStatusTone(instance.status)}>
                  {workflowStatusLabel(instance.status)}
                </StatusBadge>
              </div>
              <p className="mt-2 line-clamp-2 text-xs text-muted-foreground">
                {instance.status_reason || "等待更新"}
              </p>
              <div className="mt-3 flex items-center gap-2 text-xs text-muted-foreground">
                <Clock3 className="size-3.5" />
                <span>
                  {completed}/{total} 已完成
                </span>
              </div>
            </Link>
          );
        })}
      </div>
    </LiquidCard>
  );
}

import type { ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import {
  CheckCircle2,
  FileText,
  FolderKanban,
  GitBranch,
  ShieldCheck,
  Workflow,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { LiquidCard, SemanticIconTile, StatusBadge, type Tone } from "@/components/superteam";
import type { ProjectDemandLaunchDetail } from "@/lib/api/projects";

type FactItem = {
  id: string;
  meta?: string;
  status: string;
  subtitle?: string;
  title: string;
};

type FactListProps = {
  icon: ReactNode;
  items: FactItem[];
  title: string;
  tone: Tone;
};

export function TaskLaunchDetail({ detail }: { detail: ProjectDemandLaunchDetail }) {
  const hasFacts =
    detail.coordination_jobs.length > 0 ||
    detail.route_decisions.length > 0 ||
    detail.project_tasks.length > 0 ||
    detail.decision_requests.length > 0;
  const reviewerText =
    detail.reviewer?.display_name ||
    detail.reviewer?.reviewer_user_id ||
    detail.demand.reviewer?.display_name ||
    detail.demand.reviewer?.reviewer_user_id ||
    "未解析";

  return (
    <div className="grid items-start gap-4 xl:grid-cols-[360px_minmax(0,1fr)]">
      <LiquidCard className="rounded-xl p-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="text-xs font-medium text-muted-foreground">发起需求</p>
            <h2 className="mt-1 line-clamp-2 text-lg font-semibold tracking-normal">
              {detail.demand.title}
            </h2>
          </div>
          <StatusBadge tone={demandStatusTone(detail.demand.status)}>
            {detail.demand.status}
          </StatusBadge>
        </div>

        <p className="mt-3 line-clamp-4 text-sm leading-6 text-muted-foreground">
          {detail.demand.content || "暂无需求正文"}
        </p>

        <div className="mt-4 grid gap-2 border-y py-3 text-sm">
          <SummaryRow label="项目" value={detail.project.name} />
          <SummaryRow label="审核人" value={reviewerText} />
          <SummaryRow label="审核角色" value={detail.reviewer?.project_role ?? "未设置"} />
          <SummaryRow label="需求 ID" value={detail.demand.id} />
        </div>

        <Button asChild variant="outline" className="mt-4 w-full justify-start">
          <Link to="/projects/$projectId" params={{ projectId: detail.project.id }}>
            <FolderKanban className="size-4" />
            进入项目
          </Link>
        </Button>
      </LiquidCard>

      <div className="grid gap-4">
        {!hasFacts ? (
          <LiquidCard className="rounded-xl p-6 text-sm text-muted-foreground">
            等待项目协调线程处理
          </LiquidCard>
        ) : null}

        <FactList
          icon={<Workflow />}
          items={detail.coordination_jobs.map((job) => ({
            id: job.id,
            meta: job.workflow_id,
            status: job.status,
            subtitle: job.workflow_id,
            title: job.job_type,
          }))}
          title="协调 Job"
          tone="info"
        />
        <FactList
          icon={<GitBranch />}
          items={detail.route_decisions.map((decision) => ({
            id: decision.id,
            meta: decision.requires_human_review ? "需要人工确认" : "可分发",
            status: decision.requires_human_review ? "review_required" : "dispatchable",
            subtitle:
              decision.selected_digital_employee_ids.length > 0
                ? `已选数字员工：${decision.selected_digital_employee_ids.join(", ")}`
                : "尚未选定数字员工",
            title: decision.reason,
          }))}
          title="路由决策"
          tone="primary"
        />
        <FactList
          icon={<CheckCircle2 />}
          items={detail.project_tasks.map((task) => ({
            id: task.id,
            meta: task.requires_human_approval ? "需审批" : task.assigned_digital_employee_id,
            status: task.status,
            subtitle: task.summary || task.assigned_digital_employee_id || "暂无摘要",
            title: task.title,
          }))}
          title="项目任务"
          tone="artifact"
        />
        <FactList
          icon={<ShieldCheck />}
          items={detail.decision_requests.map((decision) => ({
            id: decision.id,
            meta: decision.target_user_id,
            status: decision.status_snapshot,
            subtitle: decision.summary_snapshot || decision.target_user_id,
            title: decision.title_snapshot,
          }))}
          title="人类决策请求"
          tone="decision"
        />
      </div>
    </div>
  );
}

function SummaryRow({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="grid grid-cols-[72px_minmax(0,1fr)] gap-3">
      <span className="text-muted-foreground">{label}</span>
      <span className="min-w-0 truncate font-medium text-foreground">{value}</span>
    </div>
  );
}

function FactList({ icon, items, title, tone }: FactListProps) {
  if (items.length === 0) {
    return null;
  }

  return (
    <LiquidCard className="rounded-xl">
      <div className="flex items-center justify-between gap-3 border-b p-4">
        <div className="flex min-w-0 items-center gap-2">
          <SemanticIconTile tone={tone} size="sm">
            {icon}
          </SemanticIconTile>
          <div className="min-w-0">
            <h3 className="text-sm font-semibold tracking-normal">{title}</h3>
            <p className="text-xs text-muted-foreground">{items.length} 条事实</p>
          </div>
        </div>
        <StatusBadge tone="neutral">{items.length}</StatusBadge>
      </div>
      <div className="divide-y">
        {items.map((item) => (
          <div className="flex items-start justify-between gap-3 p-4" key={item.id}>
            <div className="flex min-w-0 gap-3">
              <FileText className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
              <div className="min-w-0">
                <p className="line-clamp-2 text-sm font-medium">{item.title}</p>
                {item.subtitle ? (
                  <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                    {item.subtitle}
                  </p>
                ) : null}
                {item.meta ? (
                  <p className="mt-1 truncate text-xs text-muted-foreground">{item.meta}</p>
                ) : null}
              </div>
            </div>
            <StatusBadge tone={factStatusTone(item.status)}>{item.status}</StatusBadge>
          </div>
        ))}
      </div>
    </LiquidCard>
  );
}

function demandStatusTone(status: string): Tone {
  if (status === "cancelled") {
    return "danger";
  }
  if (status === "planning_pending") {
    return "warning";
  }
  return "info";
}

function factStatusTone(status: string): Tone {
  if (["completed", "accepted", "approved", "done", "success"].includes(status)) {
    return "success";
  }
  if (["failed", "rejected", "cancelled", "blocked"].includes(status)) {
    return "danger";
  }
  if (["pending", "waiting", "review_required", "planning_pending"].includes(status)) {
    return "warning";
  }
  if (["dispatchable", "running", "in_progress"].includes(status)) {
    return "info";
  }
  return "neutral";
}

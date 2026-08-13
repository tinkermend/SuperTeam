import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { Link } from "@tanstack/react-router";
import { EmptyState, ErrorState, LoadingState, StatusPill } from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  listWorkflowInstances,
  type WorkflowInstanceScope,
  type WorkflowInstanceSummary,
} from "@/lib/api/projects";
import { formatRelativeTime } from "@/lib/format-time";
import { statusLabel } from "@/lib/status-labels";
import type { WorkflowInstancesFilters } from "./workflow-instances-view";
import { workflowStatusLabel, workflowStatusTone } from "./workflow-status";

type TaskHubInstanceRailProps = {
  apiOptions: ApiClientOptions;
  filters: WorkflowInstancesFilters;
  onFiltersChange: (next: WorkflowInstancesFilters) => void;
  onSelect: (instance: WorkflowInstanceSummary | null) => void;
  searchDebounceMs?: number;
  selectedDemandId?: string | null;
};

function instanceStatusText(status: WorkflowInstanceSummary["status"]): string {
  const mapped = statusLabel(status);
  return mapped === status ? workflowStatusLabel(status) : mapped;
}

export function TaskHubInstanceRail({
  apiOptions,
  filters,
  onFiltersChange,
  onSelect,
  searchDebounceMs = 300,
  selectedDemandId = null,
}: TaskHubInstanceRailProps) {
  const scope = filters.scope ?? "active";
  const [searchInput, setSearchInput] = useState(filters.q ?? "");
  const filtersRef = useRef(filters);
  filtersRef.current = filters;
  const onFiltersChangeRef = useRef(onFiltersChange);
  onFiltersChangeRef.current = onFiltersChange;

  useEffect(() => {
    const trimmed = searchInput.trim();
    if ((filtersRef.current.q ?? "") === trimmed) {
      return;
    }
    const timer = setTimeout(() => {
      onFiltersChangeRef.current({
        ...filtersRef.current,
        q: trimmed || undefined,
      });
    }, searchDebounceMs);
    return () => clearTimeout(timer);
  }, [searchInput, searchDebounceMs]);

  const listQuery = useQuery({
    placeholderData: keepPreviousData,
    queryFn: () =>
      listWorkflowInstances(apiOptions, {
        limit: 50,
        offset: 0,
        projectId: filters.projectId,
        q: filters.q,
        scope,
      }),
    queryKey: [
      "workflow-instances",
      apiOptions.baseUrl,
      scope,
      filters.q ?? "",
      filters.projectId ?? "",
    ],
    refetchInterval: 5000,
  });

  const instances = listQuery.data ?? [];

  return (
    <aside className="hub-rail hub-rail-left" aria-label="在途流程实例">
      <div className="hub-session-head">
        <p className="hub-rail-label">在途实例</p>
        <span className="hub-rail-count">{instances.length}</span>
      </div>
      <div className="hub-session-filters" role="group" aria-label="实例范围">
        <button
          className={scope === "active" ? "hub-filter is-active" : "hub-filter"}
          onClick={() => onFiltersChange({ ...filters, scope: "active" })}
          type="button"
        >
          运行中
        </button>
        <button
          className={scope === "archived" ? "hub-filter is-active" : "hub-filter"}
          onClick={() => onFiltersChange({ ...filters, scope: "archived" })}
          type="button"
        >
          已结束
        </button>
      </div>
      <div className="hub-instance-search">
        <input
          aria-label="搜索流程实例"
          onChange={(event) => setSearchInput(event.target.value)}
          placeholder="搜索需求标题"
          type="search"
          value={searchInput}
        />
      </div>
      {listQuery.isLoading ? (
        <LoadingState label="加载实例…" />
      ) : listQuery.isError ? (
        <ErrorState
          description="无法加载在途实例"
          onRetry={() => {
            void listQuery.refetch();
          }}
        />
      ) : instances.length === 0 ? (
        <p className="hub-rail-empty">
          {filters.projectId ? "当前项目没有在途实例" : "选择项目后显示在途实例"}
        </p>
      ) : (
        <ul className="hub-session-list">
          {instances.map((instance) => {
            const active = instance.demand_id === selectedDemandId;
            return (
              <li key={instance.demand_id}>
                <button
                  className={active ? "hub-session is-active" : "hub-session"}
                  onClick={() => onSelect(active ? null : instance)}
                  type="button"
                >
                  <span className="hub-session-title">{instance.title}</span>
                  <span className="hub-session-meta">
                    <StatusPill tone={workflowStatusTone(instance.status)}>
                      {instanceStatusText(instance.status)}
                    </StatusPill>
                    <span className="hub-session-time">
                      {formatRelativeTime(instance.updated_at || instance.created_at)}
                    </span>
                  </span>
                  <span className="hub-session-sum">
                    {instance.submitted_by_display_name
                      ? `提交 ${instance.submitted_by_display_name}`
                      : "提交人未投影"}
                    {instance.current_blocker?.title
                      ? ` · ${instance.current_blocker.title}`
                      : ""}
                  </span>
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </aside>
  );
}

export function HubInstanceSummary({
  instance,
  onBack,
}: {
  instance: WorkflowInstanceSummary;
  onBack: () => void;
}) {
  return (
    <div className="hub-task-summary">
      <div className="hub-task-summary-head">
        <h2 className="hub-task-summary-title">{instance.title}</h2>
        <StatusPill tone={workflowStatusTone(instance.status)}>
          {instanceStatusText(instance.status)}
        </StatusPill>
      </div>
      <p className="hub-task-summary-meta">
        提交 {instance.submitted_by_display_name || "未投影"}
        {" · "}
        {formatRelativeTime(instance.created_at)}
      </p>
      {instance.current_blocker?.title ? (
        <p className="hub-task-summary-block">阻塞：{instance.current_blocker.title}</p>
      ) : (
        <p className="hub-task-summary-meta">当前无人工阻塞</p>
      )}
      {instance.progress.total_nodes > 0 ? (
        <p className="hub-task-summary-meta">
          进度 {instance.progress.completed_nodes}/{instance.progress.total_nodes}
          {instance.progress.waiting_human_nodes
            ? ` · 待人工 ${instance.progress.waiting_human_nodes}`
            : ""}
        </p>
      ) : instance.progress.waiting_human_nodes ? (
        <p className="hub-task-summary-meta">
          待人工 {instance.progress.waiting_human_nodes}
        </p>
      ) : null}
      <div className="hub-task-summary-actions">
        <button className="hub-ghost-btn" onClick={onBack} type="button">
          发起新任务
        </button>
        <Link
          className="hub-send"
          params={{ projectId: instance.project_id }}
          search={{ demand: instance.demand_id, tab: "tasks" }}
          to="/projects/$projectId"
        >
          在项目中打开
        </Link>
      </div>
    </div>
  );
}

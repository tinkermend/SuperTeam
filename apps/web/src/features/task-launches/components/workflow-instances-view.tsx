import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  listProjects,
  listWorkflowInstances,
  type WorkflowInstanceScope
} from "@/lib/api/projects";
import { WorkflowRiverView } from "./workflow-river-view";

export type WorkflowInstancesFilters = {
  /** 服务端关键词（已防抖落定的值，来自 URL query）。 */
  q?: string;
  /** 服务端项目过滤（项目 id，来自 URL query）。 */
  projectId?: string;
  /** 河道口径页签：active（默认，运行中）/ archived（已结束）。 */
  scope?: WorkflowInstanceScope;
};

type WorkflowInstancesViewProps = {
  apiOptions: ApiClientOptions;
  filters: WorkflowInstancesFilters;
  /** 过滤条件变化出口：由页面层写回 URL query，保证深链可达。 */
  onFiltersChange: (next: WorkflowInstancesFilters) => void;
  /** 搜索防抖毫秒数；测试可置 0。 */
  searchDebounceMs?: number;
};

/**
 * 任务中枢「流程实例」页签容器：持有列表/项目查询与搜索防抖，
 * 过滤本身全部走服务端（listWorkflowInstances 的 q / project_id / scope），
 * 不做前端二次过滤。
 */
export function WorkflowInstancesView({
  apiOptions,
  filters,
  onFiltersChange,
  searchDebounceMs = 300
}: WorkflowInstancesViewProps) {
  const scope = filters.scope ?? "active";
  const [searchInput, setSearchInput] = useState(filters.q ?? "");
  const filtersRef = useRef(filters);
  filtersRef.current = filters;
  const onFiltersChangeRef = useRef(onFiltersChange);
  onFiltersChangeRef.current = onFiltersChange;

  // 输入即时回显，落定值防抖后经 onFiltersChange 写回（进而驱动服务端查询）。
  useEffect(() => {
    const trimmed = searchInput.trim();
    if ((filtersRef.current.q ?? "") === trimmed) {
      return;
    }
    const timer = setTimeout(() => {
      onFiltersChangeRef.current({
        ...filtersRef.current,
        q: trimmed || undefined
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
        scope
      }),
    queryKey: [
      "workflow-instances",
      apiOptions.baseUrl,
      scope,
      filters.q ?? "",
      filters.projectId ?? ""
    ],
    refetchInterval: 5000
  });

  const projectsQuery = useQuery({
    placeholderData: keepPreviousData,
    queryFn: () => listProjects(apiOptions, { limit: 50, offset: 0 }),
    queryKey: ["task-launch-projects", apiOptions.baseUrl]
  });

  return (
    <WorkflowRiverView
      instances={listQuery.data ?? []}
      isError={listQuery.isError}
      isLoading={listQuery.isLoading}
      onProjectFilterChange={(projectId) =>
        onFiltersChange({ ...filters, projectId })
      }
      onScopeChange={(nextScope) => onFiltersChange({ ...filters, scope: nextScope })}
      onSearchChange={setSearchInput}
      projectFilter={filters.projectId}
      projects={projectsQuery.data ?? []}
      scope={scope}
      searchValue={searchInput}
    />
  );
}

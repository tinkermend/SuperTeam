import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import {
  Chip,
  DataTable,
  EmptyState,
  ErrorState,
  ObjectIdChip,
  ObjectRef,
  RelativeTime,
  StatusPill,
  TableSkeleton,
  Td,
  Th,
  Tr,
  type Density,
} from "@/components/superteam";
import {
  listOperationLogs,
  type OperationLogRecord,
  type OperationLogResult,
} from "@/lib/api/auth";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { logActionLabel, logModuleLabel } from "@/lib/status-labels";
import {
  DEFAULT_LOG_SINCE,
  LOG_PAGE_SIZE,
  LogDensityToggle,
  LogDetailPanel,
  LogDetailSection,
  LogFilterChips,
  LogInfoRow,
  LogListToolbar,
  LogPagination,
  LogSinceSegmented,
  LogToolbarSearch,
  LogWorkbench,
  formatLogDateTime,
  logEmptyCopy,
  logRowClassName,
  logTableDensityClass,
  resourceCaption,
  sinceQueryValue,
  type LogSinceWindow,
} from "../-shared";

export const Route = createFileRoute("/_authenticated/logs/operation/")({
  component: OperationLogsRoute,
});

const chipOptions = [
  { label: logModuleLabel("auth"), value: "auth" },
  { label: logModuleLabel("teams"), value: "teams" },
  { label: logModuleLabel("employees"), value: "employees" },
  { label: logModuleLabel("projects"), value: "projects" },
  { label: logModuleLabel("skills"), value: "skills" },
  { label: logModuleLabel("system_config"), value: "system_config" },
  { label: logModuleLabel("scenario_templates"), value: "scenario_templates" },
  { label: logModuleLabel("authz"), value: "authz" },
];

type OperationLogFilters = {
  module?: string;
  action?: string;
  result?: OperationLogResult;
};

function operationSummary(record: OperationLogRecord): string {
  const actor = record.username?.trim() || "未知用户";
  const action = logActionLabel(record.action);
  const resource = resourceCaption(record.resource_type, record.resource_id, record.resource_name);
  return resource ? `${actor} ${action} · ${resource}` : `${actor} ${action}`;
}

function OperationLogsRoute() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const [filters, setFilters] = useState<OperationLogFilters>({});
  const [sinceWindow, setSinceWindow] = useState<LogSinceWindow>(DEFAULT_LOG_SINCE);
  const [density, setDensity] = useState<Density>("comfortable");
  const [offset, setOffset] = useState(0);
  const [selected, setSelected] = useState<OperationLogRecord | null>(null);

  const logsQuery = useQuery({
    queryKey: ["web-operation-logs", filters, sinceWindow, offset],
    queryFn: () =>
      listOperationLogs({
        baseUrl: apiBaseUrl,
        limit: LOG_PAGE_SIZE,
        offset,
        since: sinceQueryValue(sinceWindow),
        exclude_module: filters.module ? undefined : "authz",
        ...filters,
      }),
    placeholderData: keepPreviousData,
  });

  const updateFilter = <K extends keyof OperationLogFilters>(key: K, value: OperationLogFilters[K]) => {
    setOffset(0);
    setFilters((prev) => ({ ...prev, [key]: value }));
  };

  const records = logsQuery.data?.items ?? [];
  const hasExtraFilter = Boolean(filters.module || filters.action || filters.result);
  const empty = logEmptyCopy({
    fallbackDescription: "控制台管理操作产生后会显示在这里。",
    hasExtraFilter,
    noun: "操作日志",
    sinceWindow,
  });

  return (
    <LogWorkbench
      detailLabel="操作详情"
      onDetailDismiss={() => setSelected(null)}
      toolbar={
        <LogListToolbar
          search={
            <LogToolbarSearch
              id="operation-log-action"
              onCommit={(v) => updateFilter("action", v)}
              placeholder="按动作筛选，回车生效"
              value={filters.action}
            />
          }
          filters={
            <>
              <LogFilterChips
                options={chipOptions}
                value={filters.module}
                onValueChange={(v) => updateFilter("module", v)}
              />
              <Chip
                active={filters.result === "succeeded"}
                onClick={() =>
                  updateFilter("result", filters.result === "succeeded" ? undefined : "succeeded")
                }
                type="button"
              >
                成功
              </Chip>
              <Chip
                active={filters.result === "failed"}
                onClick={() =>
                  updateFilter("result", filters.result === "failed" ? undefined : "failed")
                }
                type="button"
              >
                失败
              </Chip>
            </>
          }
          since={
            <LogSinceSegmented
              value={sinceWindow}
              onValueChange={(value) => {
                setOffset(0);
                setSinceWindow(value);
              }}
            />
          }
          actions={<LogDensityToggle value={density} onChange={setDensity} />}
        />
      }
      body={
        logsQuery.isLoading && !logsQuery.data ? (
          <TableSkeleton className="m-4" cols={3} rows={8} />
        ) : logsQuery.isError ? (
          <ErrorState title="操作日志加载失败" description="请稍后重试，或确认当前账号仍有访问权限。" />
        ) : records.length === 0 ? (
          <EmptyState title={empty.title} description={empty.description} />
        ) : (
          <DataTable className={logTableDensityClass(density)}>
            <thead>
              <Tr>
                <Th className="w-[7.5rem]">时间</Th>
                <Th>摘要</Th>
                <Th className="w-20">状态</Th>
              </Tr>
            </thead>
            <tbody>
              {records.map((record: OperationLogRecord) => (
                <Tr
                  aria-selected={selected?.id === record.id}
                  className={logRowClassName({
                    failed: record.result === "failed",
                    selected: selected?.id === record.id,
                  })}
                  key={record.id}
                  onClick={() => setSelected(record)}
                >
                  <Td className="whitespace-nowrap text-xs">
                    <RelativeTime value={record.created_at} />
                  </Td>
                  <Td className="min-w-0">
                    <p className="truncate text-sm font-medium text-ink">{operationSummary(record)}</p>
                    <p className="truncate text-xs text-ink-3">{logModuleLabel(record.module)}</p>
                  </Td>
                  <Td>
                    <StatusPill tone={record.result === "succeeded" ? "ok" : "danger"}>
                      {record.result === "succeeded" ? "成功" : "失败"}
                    </StatusPill>
                  </Td>
                </Tr>
              ))}
            </tbody>
          </DataTable>
        )
      }
      pagination={
        <LogPagination
          isFetching={logsQuery.isFetching}
          itemCount={records.length}
          offset={offset}
          onOffsetChange={setOffset}
          pageSize={LOG_PAGE_SIZE}
        />
      }
      detail={
        selected ? (
          <LogDetailPanel
            kicker={<span className="text-xs text-ink-3">{logModuleLabel(selected.module)}</span>}
            onClose={() => setSelected(null)}
            status={
              <StatusPill tone={selected.result === "succeeded" ? "ok" : "danger"}>
                {selected.result === "succeeded" ? "成功" : "失败"}
              </StatusPill>
            }
            title={operationSummary(selected)}
          >
            <LogDetailSection title="操作信息">
              <div className="flex flex-col gap-2.5">
                <LogInfoRow label="时间">{formatLogDateTime(selected.created_at)}</LogInfoRow>
                <LogInfoRow label="用户">
                  {selected.username ? (
                    <ObjectRef id={selected.user_id} name={selected.username} />
                  ) : (
                    "-"
                  )}
                </LogInfoRow>
                <LogInfoRow label="来源 IP">{selected.client_ip || "-"}</LogInfoRow>
                <LogInfoRow label="请求 ID">
                  {selected.request_id ? <ObjectIdChip full id={selected.request_id} /> : "-"}
                </LogInfoRow>
                <LogInfoRow label="UA">{selected.user_agent || "-"}</LogInfoRow>
                <LogInfoRow label="资源">
                  {selected.resource_type || selected.resource_id || selected.resource_name ? (
                    <ObjectRef
                      id={
                        selected.resource_type === "system_config" ? undefined : selected.resource_id
                      }
                      name={
                        selected.resource_name ||
                        (selected.resource_type === "system_config"
                          ? selected.resource_id
                          : undefined) ||
                        resourceCaption(selected.resource_type, selected.resource_id)
                      }
                    />
                  ) : (
                    "-"
                  )}
                </LogInfoRow>
              </div>
            </LogDetailSection>
            {selected.details && Object.keys(selected.details).length > 0 ? (
              <LogDetailSection title="详情">
                <pre className="max-h-64 overflow-auto rounded-lg bg-card-soft p-3 text-[11px] leading-relaxed text-ink">
                  {JSON.stringify(selected.details, null, 2)}
                </pre>
              </LogDetailSection>
            ) : null}
          </LogDetailPanel>
        ) : undefined
      }
    />
  );
}

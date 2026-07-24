import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import {
  StatusPill,
  EmptyState,
  ErrorState,
  LoadingState,
  DataTable,
  Td,
  Th,
  Tr,
  WorkSurface
} from "@/components/superteam";
import {
  listOperationLogs,
  type OperationLogRecord,
  type OperationLogResult
} from "@/lib/api/auth";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import {
  LOG_PAGE_SIZE,
  LogChips,
  LogFilterBar,
  LogPagination,
  LogSelectFilter,
  LogTextFilter,
  formatLogDateTime
} from "../-shared";

export const Route = createFileRoute("/_authenticated/logs/operation/")({
  component: OperationLogsRoute
});

const chipOptions = [
  { label: "authz", value: "authz" },
  { label: "users", value: "users" },
  { label: "teams", value: "teams" },
  { label: "projects", value: "projects" },
  { label: "skills", value: "skills" },
];

const resultOptions = [
  { label: "成功", value: "succeeded" },
  { label: "失败", value: "failed" },
];

type OperationLogFilters = {
  module?: string;
  action?: string;
  result?: OperationLogResult;
};

function OperationLogsRoute() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const [filters, setFilters] = useState<OperationLogFilters>({});
  const [offset, setOffset] = useState(0);

  const logsQuery = useQuery({
    queryKey: ["web-operation-logs", filters, offset],
    queryFn: () =>
      listOperationLogs({ baseUrl: apiBaseUrl, limit: LOG_PAGE_SIZE, offset, ...filters }),
    placeholderData: keepPreviousData
});

  const updateFilter = <K extends keyof OperationLogFilters>(key: K, value: OperationLogFilters[K]) => {
    setOffset(0);
    setFilters((prev) => ({ ...prev, [key]: value }));
  };

  const records = logsQuery.data?.items ?? [];
  const hasFilter = Boolean(filters.module || filters.action || filters.result);

  return (
    <>
      <WorkSurface>
        <LogChips
          options={chipOptions}
          value={filters.module}
          onValueChange={(v) => updateFilter("module", v)}
        />
        <LogFilterBar>
          <LogTextFilter
            id="operation-log-module"
            label="模块"
            placeholder="如 authz、users"
            value={filters.module}
            onCommit={(v) => updateFilter("module", v)}
          />
          <LogTextFilter
            id="operation-log-action"
            label="动作"
            placeholder="如 user.create"
            value={filters.action}
            onCommit={(v) => updateFilter("action", v)}
          />
          <LogSelectFilter
            id="operation-log-result"
            label="结果"
            options={resultOptions}
            value={filters.result}
            onValueChange={(v) => updateFilter("result", v as OperationLogResult | undefined)}
          />
        </LogFilterBar>

        {logsQuery.isLoading && !logsQuery.data ? (
          <LoadingState label="正在加载操作日志…" />
        ) : logsQuery.isError ? (
          <ErrorState title="操作日志加载失败" description="请稍后重试，或确认当前账号仍有访问权限。" />
        ) : records.length === 0 ? (
          <EmptyState
            title={hasFilter ? "筛选后无操作日志" : "暂无操作日志"}
            description="控制台管理操作产生后会显示在这里。"
          />
        ) : (
          <DataTable>
            <thead>
              <Tr>
                <Th className="min-w-[150px]">时间</Th>
                <Th>模块</Th>
                <Th>动作</Th>
                <Th>结果</Th>
                <Th>用户</Th>
                <Th className="min-w-[180px]">资源</Th>
                <Th>来源 IP</Th>
              </Tr>
            </thead>
            <tbody>
              {records.map((record: OperationLogRecord) => (
                <Tr key={record.id}>
                  <Td className="whitespace-nowrap text-xs text-ink-2 tabular-nums">
                    {formatLogDateTime(record.created_at)}
                  </Td>
                  <Td className="whitespace-nowrap text-sm">{record.module}</Td>
                  <Td><StatusPill tone="mute">{record.action}</StatusPill></Td>
                  <Td>
                    <StatusPill tone={record.result === "succeeded" ? "ok" : "danger"}>
                      {record.result === "succeeded" ? "成功" : "失败"}
                    </StatusPill>
                  </Td>
                  <Td className="max-w-[160px] truncate text-sm">{record.username || "-"}</Td>
                  <Td className="max-w-[220px] truncate font-mono text-xs text-ink-3">
                    {record.resource_type ? `${record.resource_type}:${record.resource_id || "-"}` : "-"}
                  </Td>
                  <Td className="whitespace-nowrap font-mono text-xs">{record.client_ip || "-"}</Td>
                </Tr>
              ))}
            </tbody>
          </DataTable>
        )}

        <LogPagination
          isFetching={logsQuery.isFetching}
          itemCount={records.length}
          offset={offset}
          onOffsetChange={setOffset}
          pageSize={LOG_PAGE_SIZE}
        />
      </WorkSurface>
    </>
  );
}

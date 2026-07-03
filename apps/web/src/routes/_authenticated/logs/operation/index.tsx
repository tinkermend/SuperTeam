import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { ClipboardList } from "lucide-react";
import { useState } from "react";
import {
  StatusPill,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  WorkSurface,
} from "@/components/superteam";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import {
  listOperationLogs,
  type OperationLogRecord,
  type OperationLogResult,
} from "@/lib/api/auth";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import {
  LOG_PAGE_SIZE,
  LogFilterBar,
  LogPagination,
  LogSelectFilter,
  LogTextFilter,
  formatLogDateTime,
} from "../-shared";

export const Route = createFileRoute("/_authenticated/logs/operation/")({
  component: OperationLogsRoute,
});

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
      listOperationLogs({
        baseUrl: apiBaseUrl,
        limit: LOG_PAGE_SIZE,
        offset,
        ...filters,
      }),
    placeholderData: keepPreviousData,
  });

  const updateFilter = <Key extends keyof OperationLogFilters>(
    key: Key,
    value: OperationLogFilters[Key],
  ) => {
    setOffset(0);
    setFilters((prev) => ({ ...prev, [key]: value }));
  };

  const records = logsQuery.data?.items ?? [];
  const hasFilter = Boolean(filters.module || filters.action || filters.result);

  return (
    <>
      <ShellPageHeader
        icon={<ClipboardList />}
        iconTone="mute"
        title="操作日志"
        subtitle="控制台管理操作（增删改）记录"
      />
      <Main className="min-w-0 overflow-x-hidden">
        <div className="mx-auto flex w-full max-w-6xl flex-col gap-4 text-v3-ink">
          <WorkSurface>
            <LogFilterBar>
              <LogTextFilter
                id="operation-log-module"
                label="模块"
                placeholder="如 authz、users"
                value={filters.module}
                onCommit={(value) => updateFilter("module", value)}
              />
              <LogTextFilter
                id="operation-log-action"
                label="动作"
                placeholder="如 user.create"
                value={filters.action}
                onCommit={(value) => updateFilter("action", value)}
              />
              <LogSelectFilter
                id="operation-log-result"
                label="结果"
                options={resultOptions}
                value={filters.result}
                onValueChange={(value) =>
                  updateFilter("result", value as OperationLogResult | undefined)
                }
              />
            </LogFilterBar>

            {logsQuery.isLoading && !logsQuery.data ? (
              <V3LoadingState label="正在加载操作日志…" />
            ) : logsQuery.isError ? (
              <V3ErrorState
                title="操作日志加载失败"
                description="请稍后重试，或确认当前账号仍有访问权限。"
              />
            ) : records.length === 0 ? (
              <V3EmptyState
                title={hasFilter ? "筛选后无操作日志" : "暂无操作日志"}
                description="控制台管理操作产生后会显示在这里。"
              />
            ) : (
              <V3Table>
                <thead>
                  <V3Tr>
                    <V3Th className="min-w-[150px]">时间</V3Th>
                    <V3Th>模块</V3Th>
                    <V3Th>动作</V3Th>
                    <V3Th>结果</V3Th>
                    <V3Th>用户</V3Th>
                    <V3Th className="min-w-[180px]">资源</V3Th>
                    <V3Th>来源 IP</V3Th>
                  </V3Tr>
                </thead>
                <tbody>
                  {records.map((record: OperationLogRecord) => (
                    <V3Tr key={record.id}>
                      <V3Td className="whitespace-nowrap text-xs text-v3-ink-2 tabular-nums">
                        {formatLogDateTime(record.created_at)}
                      </V3Td>
                      <V3Td className="whitespace-nowrap text-sm">{record.module}</V3Td>
                      <V3Td>
                        <StatusPill tone="mute">{record.action}</StatusPill>
                      </V3Td>
                      <V3Td>
                        <StatusPill tone={record.result === "succeeded" ? "ok" : "danger"}>
                          {record.result === "succeeded" ? "成功" : "失败"}
                        </StatusPill>
                      </V3Td>
                      <V3Td className="max-w-[160px] truncate text-sm">{record.username || "-"}</V3Td>
                      <V3Td className="max-w-[220px] truncate font-mono text-xs text-v3-ink-3">
                        {record.resource_type
                          ? `${record.resource_type}:${record.resource_id || "-"}`
                          : "-"}
                      </V3Td>
                      <V3Td className="whitespace-nowrap font-mono text-xs">
                        {record.client_ip || "-"}
                      </V3Td>
                    </V3Tr>
                  ))}
                </tbody>
              </V3Table>
            )}

            <LogPagination
              isFetching={logsQuery.isFetching}
              itemCount={records.length}
              offset={offset}
              onOffsetChange={setOffset}
              pageSize={LOG_PAGE_SIZE}
            />
          </WorkSurface>
        </div>
      </Main>
    </>
  );
}

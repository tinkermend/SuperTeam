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
  listLoginLogs,
  type LoginLogEventType,
  type LoginLogRecord,
  type LoginLogResult
} from "@/lib/api/auth";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import {
  LOG_PAGE_SIZE,
  LogChips,
  LogFilterBar,
  LogPagination,
  LogSelectFilter,
  formatLogDateTime
} from "../-shared";

export const Route = createFileRoute("/_authenticated/logs/login/")({
  component: LoginLogsRoute
});

const eventTypeLabel: Record<LoginLogEventType, string> = {
  login_succeeded: "登录成功",
  login_failed: "登录失败",
  logout_succeeded: "登出成功"
};

const chipOptions = [
  { label: "登录失败", value: "login_failed" },
  { label: "登录成功", value: "login_succeeded" },
  { label: "登出成功", value: "logout_succeeded" },
];

const resultOptions = [
  { label: "成功", value: "succeeded" },
  { label: "失败", value: "failed" },
];

type LoginLogFilters = {
  event_type?: LoginLogEventType;
  result?: LoginLogResult;
};

function LoginLogsRoute() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const [filters, setFilters] = useState<LoginLogFilters>({});
  const [offset, setOffset] = useState(0);

  const logsQuery = useQuery({
    queryKey: ["web-login-logs", filters, offset],
    queryFn: () =>
      listLoginLogs({ baseUrl: apiBaseUrl, limit: LOG_PAGE_SIZE, offset, ...filters }),
    placeholderData: keepPreviousData
});

  const updateFilter = <K extends keyof LoginLogFilters>(key: K, value: LoginLogFilters[K]) => {
    setOffset(0);
    setFilters((prev) => ({ ...prev, [key]: value }));
  };

  const records = logsQuery.data?.items ?? [];
  const hasFilter = Boolean(filters.event_type || filters.result);

  return (
    <>
      <WorkSurface>
        <LogChips
          options={chipOptions}
          value={filters.event_type}
          onValueChange={(v) => updateFilter("event_type", v as LoginLogEventType | undefined)}
        />
        <LogFilterBar>
          <LogSelectFilter
            id="login-log-event-type"
            label="事件类型"
            options={chipOptions}
            value={filters.event_type}
            onValueChange={(v) => updateFilter("event_type", v as LoginLogEventType | undefined)}
          />
          <LogSelectFilter
            id="login-log-result"
            label="结果"
            options={resultOptions}
            value={filters.result}
            onValueChange={(v) => updateFilter("result", v as LoginLogResult | undefined)}
          />
        </LogFilterBar>

        {logsQuery.isLoading && !logsQuery.data ? (
          <LoadingState label="正在加载登录日志…" />
        ) : logsQuery.isError ? (
          <ErrorState title="登录日志加载失败" description="请稍后重试，或确认当前账号仍有访问权限。" />
        ) : records.length === 0 ? (
          <EmptyState
            title={hasFilter ? "筛选后无登录日志" : "暂无登录日志"}
            description="账号登录后会显示在这里。"
          />
        ) : (
          <DataTable>
            <thead>
              <Tr>
                <Th className="min-w-[150px]">时间</Th>
                <Th>事件类型</Th>
                <Th>结果</Th>
                <Th>用户</Th>
                <Th>来源 IP</Th>
                <Th className="min-w-[180px]">失败原因</Th>
              </Tr>
            </thead>
            <tbody>
              {records.map((record: LoginLogRecord) => (
                <Tr key={record.id}>
                  <Td className="whitespace-nowrap text-xs text-ink-2 tabular-nums">
                    {formatLogDateTime(record.created_at)}
                  </Td>
                  <Td className="whitespace-nowrap text-sm">
                    {eventTypeLabel[record.event_type] ?? record.event_type}
                  </Td>
                  <Td>
                    <StatusPill tone={record.result === "succeeded" ? "ok" : "danger"}>
                      {record.result === "succeeded" ? "成功" : "失败"}
                    </StatusPill>
                  </Td>
                  <Td className="max-w-[200px] truncate text-sm">{record.username}</Td>
                  <Td className="whitespace-nowrap font-mono text-xs">{record.client_ip || "-"}</Td>
                  <Td className="max-w-[240px] truncate text-xs text-ink-3">
                    {record.failure_reason || "-"}
                  </Td>
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

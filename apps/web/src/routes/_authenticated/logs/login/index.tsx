import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { LogIn } from "lucide-react";
import { useState } from "react";
import {
  IconTile,
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
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import {
  listLoginLogs,
  type LoginLogEventType,
  type LoginLogRecord,
  type LoginLogResult,
} from "@/lib/api/auth";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import {
  LOG_PAGE_SIZE,
  LogFilterBar,
  LogPagination,
  LogSelectFilter,
  formatLogDateTime,
} from "../-shared";

export const Route = createFileRoute("/_authenticated/logs/login/")({
  component: LoginLogsRoute,
});

const eventTypeLabel: Record<LoginLogEventType, string> = {
  login_succeeded: "登录成功",
  login_failed: "登录失败",
  logout_succeeded: "登出成功",
};

const eventTypeOptions = [
  { label: "登录成功", value: "login_succeeded" },
  { label: "登录失败", value: "login_failed" },
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
      listLoginLogs({
        baseUrl: apiBaseUrl,
        limit: LOG_PAGE_SIZE,
        offset,
        ...filters,
      }),
    placeholderData: keepPreviousData,
  });

  const updateFilter = <Key extends keyof LoginLogFilters>(key: Key, value: LoginLogFilters[Key]) => {
    setOffset(0);
    setFilters((prev) => ({ ...prev, [key]: value }));
  };

  const records = logsQuery.data?.items ?? [];
  const hasFilter = Boolean(filters.event_type || filters.result);

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main className="min-w-0 overflow-x-hidden">
        <div className="mx-auto flex w-full max-w-6xl flex-col gap-4 text-v3-ink">
          <div className="flex items-center gap-3">
            <IconTile tone="mute" size="sm">
              <LogIn />
            </IconTile>
            <div className="min-w-0">
              <h1 className="text-lg font-bold text-v3-ink">登录日志</h1>
              <p className="truncate text-sm text-v3-ink-2">
                控制台账号登录、登出与失败记录
              </p>
            </div>
          </div>

          <WorkSurface>
            <LogFilterBar>
              <LogSelectFilter
                id="login-log-event-type"
                label="事件类型"
                options={eventTypeOptions}
                value={filters.event_type}
                onValueChange={(value) =>
                  updateFilter("event_type", value as LoginLogEventType | undefined)
                }
              />
              <LogSelectFilter
                id="login-log-result"
                label="结果"
                options={resultOptions}
                value={filters.result}
                onValueChange={(value) => updateFilter("result", value as LoginLogResult | undefined)}
              />
            </LogFilterBar>

            {logsQuery.isLoading && !logsQuery.data ? (
              <V3LoadingState label="正在加载登录日志…" />
            ) : logsQuery.isError ? (
              <V3ErrorState
                title="登录日志加载失败"
                description="请稍后重试，或确认当前账号仍有访问权限。"
              />
            ) : records.length === 0 ? (
              <V3EmptyState
                title={hasFilter ? "筛选后无登录日志" : "暂无登录日志"}
                description="账号登录后会显示在这里。"
              />
            ) : (
              <V3Table>
                <thead>
                  <V3Tr>
                    <V3Th className="min-w-[150px]">时间</V3Th>
                    <V3Th>事件类型</V3Th>
                    <V3Th>结果</V3Th>
                    <V3Th>用户</V3Th>
                    <V3Th>来源 IP</V3Th>
                    <V3Th className="min-w-[180px]">失败原因</V3Th>
                  </V3Tr>
                </thead>
                <tbody>
                  {records.map((record: LoginLogRecord) => (
                    <V3Tr key={record.id}>
                      <V3Td className="whitespace-nowrap text-xs text-v3-ink-2 tabular-nums">
                        {formatLogDateTime(record.created_at)}
                      </V3Td>
                      <V3Td className="whitespace-nowrap text-sm">
                        {eventTypeLabel[record.event_type] ?? record.event_type}
                      </V3Td>
                      <V3Td>
                        <StatusPill tone={record.result === "succeeded" ? "ok" : "danger"}>
                          {record.result === "succeeded" ? "成功" : "失败"}
                        </StatusPill>
                      </V3Td>
                      <V3Td className="max-w-[200px] truncate text-sm">{record.username}</V3Td>
                      <V3Td className="whitespace-nowrap font-mono text-xs">
                        {record.client_ip || "-"}
                      </V3Td>
                      <V3Td className="max-w-[240px] truncate text-xs text-v3-ink-3">
                        {record.failure_reason || "-"}
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

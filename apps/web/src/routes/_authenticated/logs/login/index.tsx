import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import {
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
  listLoginLogs,
  type LoginLogEventType,
  type LoginLogRecord,
} from "@/lib/api/auth";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { loginFailureReasonLabel, loginLogEventLabel } from "@/lib/status-labels";
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
  LogWorkbench,
  formatLogDateTime,
  logEmptyCopy,
  logRowClassName,
  logTableDensityClass,
  sinceQueryValue,
  type LogSinceWindow,
} from "../-shared";

export const Route = createFileRoute("/_authenticated/logs/login/")({
  component: LoginLogsRoute,
});

const chipOptions = [
  { label: loginLogEventLabel("login_failed"), value: "login_failed" },
  { label: loginLogEventLabel("login_succeeded"), value: "login_succeeded" },
  { label: loginLogEventLabel("logout_succeeded"), value: "logout_succeeded" },
];

type LoginLogFilters = {
  event_type?: LoginLogEventType;
};

function loginSummary(record: LoginLogRecord): string {
  const actor = record.username?.trim() || "未知用户";
  const event = loginLogEventLabel(record.event_type);
  const reason = record.failure_reason ? loginFailureReasonLabel(record.failure_reason) : "";
  return reason ? `${actor} ${event} · ${reason}` : `${actor} ${event}`;
}

function LoginLogsRoute() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const [filters, setFilters] = useState<LoginLogFilters>({});
  const [sinceWindow, setSinceWindow] = useState<LogSinceWindow>(DEFAULT_LOG_SINCE);
  const [density, setDensity] = useState<Density>("comfortable");
  const [offset, setOffset] = useState(0);
  const [selected, setSelected] = useState<LoginLogRecord | null>(null);

  const logsQuery = useQuery({
    queryKey: ["web-login-logs", filters, sinceWindow, offset],
    queryFn: () =>
      listLoginLogs({
        baseUrl: apiBaseUrl,
        limit: LOG_PAGE_SIZE,
        offset,
        since: sinceQueryValue(sinceWindow),
        ...filters,
      }),
    placeholderData: keepPreviousData,
  });

  const updateFilter = <K extends keyof LoginLogFilters>(key: K, value: LoginLogFilters[K]) => {
    setOffset(0);
    setFilters((prev) => ({ ...prev, [key]: value }));
  };

  const records = logsQuery.data?.items ?? [];
  const hasExtraFilter = Boolean(filters.event_type);
  const empty = logEmptyCopy({
    fallbackDescription: "账号登录后会显示在这里。",
    hasExtraFilter,
    noun: "登录日志",
    sinceWindow,
  });

  return (
    <LogWorkbench
      detailLabel="登录详情"
      onDetailDismiss={() => setSelected(null)}
      toolbar={
        <LogListToolbar
          filters={
            <LogFilterChips
              options={chipOptions}
              value={filters.event_type}
              onValueChange={(v) => updateFilter("event_type", v as LoginLogEventType | undefined)}
            />
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
          <TableSkeleton className="m-4" cols={4} rows={8} />
        ) : logsQuery.isError ? (
          <ErrorState title="登录日志加载失败" description="请稍后重试，或确认当前账号仍有访问权限。" />
        ) : records.length === 0 ? (
          <EmptyState title={empty.title} description={empty.description} />
        ) : (
          <DataTable className={logTableDensityClass(density)}>
            <thead>
              <Tr>
                <Th className="w-[7.5rem]">时间</Th>
                <Th>摘要</Th>
                <Th className="w-20">状态</Th>
                <Th className="w-32">来源 IP</Th>
              </Tr>
            </thead>
            <tbody>
              {records.map((record: LoginLogRecord) => (
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
                    <p className="truncate text-sm font-medium text-ink">{loginSummary(record)}</p>
                  </Td>
                  <Td>
                    <StatusPill tone={record.result === "succeeded" ? "ok" : "danger"}>
                      {record.result === "succeeded" ? "成功" : "失败"}
                    </StatusPill>
                  </Td>
                  <Td className="whitespace-nowrap font-mono text-xs">{record.client_ip || "-"}</Td>
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
            kicker={<span className="text-xs text-ink-3">{loginLogEventLabel(selected.event_type)}</span>}
            onClose={() => setSelected(null)}
            status={
              <StatusPill tone={selected.result === "succeeded" ? "ok" : "danger"}>
                {selected.result === "succeeded" ? "成功" : "失败"}
              </StatusPill>
            }
            title={loginSummary(selected)}
          >
            <LogDetailSection title="事件信息">
              <div className="flex flex-col gap-2.5">
                <LogInfoRow label="时间">{formatLogDateTime(selected.created_at)}</LogInfoRow>
                <LogInfoRow label="来源 IP">{selected.client_ip || "-"}</LogInfoRow>
                <LogInfoRow label="用户">
                  <ObjectRef id={selected.user_id} name={selected.username} />
                </LogInfoRow>
                <LogInfoRow label="会话">
                  {selected.session_id ? <ObjectIdChip id={selected.session_id} /> : "-"}
                </LogInfoRow>
                <LogInfoRow label="UA">{selected.user_agent || "-"}</LogInfoRow>
                {selected.failure_reason ? (
                  <LogInfoRow label="失败原因">{loginFailureReasonLabel(selected.failure_reason)}</LogInfoRow>
                ) : null}
              </div>
            </LogDetailSection>
          </LogDetailPanel>
        ) : undefined
      }
    />
  );
}

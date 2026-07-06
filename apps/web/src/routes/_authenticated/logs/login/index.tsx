import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
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
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  listLoginLogs,
  type LoginLogEventType,
  type LoginLogRecord,
  type LoginLogResult,
} from "@/lib/api/auth";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import {
  LOG_PAGE_SIZE,
  LogChips,
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
  const [selectedRecord, setSelectedRecord] = useState<LoginLogRecord | null>(null);

  const logsQuery = useQuery({
    queryKey: ["web-login-logs", filters, offset],
    queryFn: () =>
      listLoginLogs({ baseUrl: apiBaseUrl, limit: LOG_PAGE_SIZE, offset, ...filters }),
    placeholderData: keepPreviousData,
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
          <V3LoadingState label="正在加载登录日志…" />
        ) : logsQuery.isError ? (
          <V3ErrorState title="登录日志加载失败" description="请稍后重试，或确认当前账号仍有访问权限。" />
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
                <V3Tr
                  key={record.id}
                  className="cursor-pointer"
                  onClick={() => setSelectedRecord(record)}
                >
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
                  <V3Td className="whitespace-nowrap font-mono text-xs">{record.client_ip || "-"}</V3Td>
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

      <Sheet open={selectedRecord !== null} onOpenChange={(open) => { if (!open) setSelectedRecord(null); }}>
        <SheetContent side="right" className="w-[420px] max-w-[45vw]">
          {selectedRecord && (
            <>
              <SheetHeader>
                <SheetTitle className="text-base font-semibold text-v3-ink">
                  {eventTypeLabel[selectedRecord.event_type] ?? selectedRecord.event_type}
                </SheetTitle>
              </SheetHeader>
              <div className="mt-4 flex flex-col gap-3 text-sm">
                <DetailRow label="结果">
                  <StatusPill tone={selectedRecord.result === "succeeded" ? "ok" : "danger"}>
                    {selectedRecord.result === "succeeded" ? "成功" : "失败"}
                  </StatusPill>
                </DetailRow>
                <DetailRow label="用户名">{selectedRecord.username || "—"}</DetailRow>
                <DetailRow label="来源 IP">{selectedRecord.client_ip || "—"}</DetailRow>
                <DetailRow label="失败原因">{selectedRecord.failure_reason || "—"}</DetailRow>
                <DetailRow label="时间">{formatLogDateTime(selectedRecord.created_at)}</DetailRow>
              </div>
            </>
          )}
        </SheetContent>
      </Sheet>
    </>
  );
}

function DetailRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start gap-3">
      <span className="w-20 shrink-0 text-xs text-v3-ink-2">{label}</span>
      <span className="min-w-0 break-all text-v3-ink">{children}</span>
    </div>
  );
}

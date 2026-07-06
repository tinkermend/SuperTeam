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
  listOperationLogs,
  type OperationLogRecord,
  type OperationLogResult,
} from "@/lib/api/auth";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import {
  LOG_PAGE_SIZE,
  LogChips,
  LogFilterBar,
  LogPagination,
  LogSelectFilter,
  LogTextFilter,
  formatLogDateTime,
} from "../-shared";

export const Route = createFileRoute("/_authenticated/logs/operation/")({
  component: OperationLogsRoute,
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
  const [selectedRecord, setSelectedRecord] = useState<OperationLogRecord | null>(null);

  const logsQuery = useQuery({
    queryKey: ["web-operation-logs", filters, offset],
    queryFn: () =>
      listOperationLogs({ baseUrl: apiBaseUrl, limit: LOG_PAGE_SIZE, offset, ...filters }),
    placeholderData: keepPreviousData,
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
          <V3LoadingState label="正在加载操作日志…" />
        ) : logsQuery.isError ? (
          <V3ErrorState title="操作日志加载失败" description="请稍后重试，或确认当前账号仍有访问权限。" />
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
                <V3Tr
                  key={record.id}
                  className="cursor-pointer"
                  onClick={() => setSelectedRecord(record)}
                >
                  <V3Td className="whitespace-nowrap text-xs text-v3-ink-2 tabular-nums">
                    {formatLogDateTime(record.created_at)}
                  </V3Td>
                  <V3Td className="whitespace-nowrap text-sm">{record.module}</V3Td>
                  <V3Td><StatusPill tone="mute">{record.action}</StatusPill></V3Td>
                  <V3Td>
                    <StatusPill tone={record.result === "succeeded" ? "ok" : "danger"}>
                      {record.result === "succeeded" ? "成功" : "失败"}
                    </StatusPill>
                  </V3Td>
                  <V3Td className="max-w-[160px] truncate text-sm">{record.username || "-"}</V3Td>
                  <V3Td className="max-w-[220px] truncate font-mono text-xs text-v3-ink-3">
                    {record.resource_type ? `${record.resource_type}:${record.resource_id || "-"}` : "-"}
                  </V3Td>
                  <V3Td className="whitespace-nowrap font-mono text-xs">{record.client_ip || "-"}</V3Td>
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
                  {selectedRecord.module} · {selectedRecord.action}
                </SheetTitle>
              </SheetHeader>
              <div className="mt-4 flex flex-col gap-3 text-sm">
                <DetailRow label="结果">
                  <StatusPill tone={selectedRecord.result === "succeeded" ? "ok" : "danger"}>
                    {selectedRecord.result === "succeeded" ? "成功" : "失败"}
                  </StatusPill>
                </DetailRow>
                <DetailRow label="操作者">{selectedRecord.username || "—"}</DetailRow>
                <DetailRow label="资源">
                  {selectedRecord.resource_type
                    ? `${selectedRecord.resource_type}:${selectedRecord.resource_id || "-"}`
                    : "—"}
                </DetailRow>
                <DetailRow label="来源 IP">{selectedRecord.client_ip || "—"}</DetailRow>
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

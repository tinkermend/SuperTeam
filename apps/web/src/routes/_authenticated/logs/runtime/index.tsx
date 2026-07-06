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
  type V3Tone,
  WorkSurface,
} from "@/components/superteam";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  listRuntimeEvents,
  type RuntimeEvent,
  type RuntimeEventSeverity,
} from "@/lib/api/runtime";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import {
  LOG_PAGE_SIZE,
  LogChips,
  LogFilterBar,
  LogPagination,
  LogTextFilter,
  formatLogDateTime,
} from "../-shared";

export const Route = createFileRoute("/_authenticated/logs/runtime/")({
  component: RuntimeEventLogsRoute,
});

const severityLabel: Record<RuntimeEventSeverity, string> = {
  error: "错误",
  info: "信息",
  success: "成功",
  warning: "预警",
};

const severityTone: Record<RuntimeEventSeverity, V3Tone> = {
  error: "danger",
  info: "info",
  success: "ok",
  warning: "warn",
};

const chipOptions = [
  { label: "错误", value: "error" },
  { label: "预警", value: "warning" },
  { label: "成功", value: "success" },
  { label: "信息", value: "info" },
];

type RuntimeEventFilters = {
  severity?: RuntimeEventSeverity;
  event_type?: string;
  node_id?: string;
};

function RuntimeEventLogsRoute() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const [filters, setFilters] = useState<RuntimeEventFilters>({});
  const [offset, setOffset] = useState(0);
  const [selectedEvent, setSelectedEvent] = useState<RuntimeEvent | null>(null);

  const eventsQuery = useQuery({
    queryKey: ["web-runtime-event-logs", filters, offset],
    queryFn: () =>
      listRuntimeEvents({
        baseUrl: apiBaseUrl,
        limit: LOG_PAGE_SIZE,
        offset,
        ...filters,
      }),
    placeholderData: keepPreviousData,
  });

  const updateFilter = <Key extends keyof RuntimeEventFilters>(
    key: Key,
    value: RuntimeEventFilters[Key],
  ) => {
    setOffset(0);
    setFilters((prev) => ({ ...prev, [key]: value }));
  };

  const events = eventsQuery.data?.items ?? [];
  const hasFilter = Boolean(filters.severity || filters.event_type || filters.node_id);

  return (
    <>
      <WorkSurface>
        <LogChips
          options={chipOptions}
          value={filters.severity}
          onValueChange={(v) => updateFilter("severity", v as RuntimeEventSeverity | undefined)}
        />
        <LogFilterBar>
          <LogTextFilter
            id="runtime-log-event-type"
            label="事件类型"
            placeholder="如 node_offline"
            value={filters.event_type}
            onCommit={(value) => updateFilter("event_type", value)}
          />
          <LogTextFilter
            id="runtime-log-node"
            label="节点"
            placeholder="node_id"
            value={filters.node_id}
            onCommit={(value) => updateFilter("node_id", value)}
          />
        </LogFilterBar>

        {eventsQuery.isLoading && !eventsQuery.data ? (
          <V3LoadingState label="正在加载平台事件…" />
        ) : eventsQuery.isError ? (
          <V3ErrorState
            title="平台事件加载失败"
            description="请稍后重试，或确认当前账号仍有访问权限。"
          />
        ) : events.length === 0 ? (
          <V3EmptyState
            title={hasFilter ? "筛选后无平台事件" : "暂无平台事件"}
            description="平台或 Runtime 事件产生后会显示在这里。"
          />
        ) : (
          <V3Table>
            <thead>
              <V3Tr>
                <V3Th className="min-w-[150px]">时间</V3Th>
                <V3Th>级别</V3Th>
                <V3Th>事件类型</V3Th>
                <V3Th>节点</V3Th>
                <V3Th className="min-w-[260px]">标题</V3Th>
              </V3Tr>
            </thead>
            <tbody>
              {events.map((event: RuntimeEvent) => (
                <V3Tr
                  key={event.id}
                  className="cursor-pointer"
                  onClick={() => setSelectedEvent(event)}
                >
                  <V3Td className="whitespace-nowrap text-xs text-v3-ink-2 tabular-nums">
                    {formatLogDateTime(event.created_at)}
                  </V3Td>
                  <V3Td>
                    <StatusPill tone={severityTone[event.severity] ?? "mute"}>
                      {severityLabel[event.severity] ?? event.severity}
                    </StatusPill>
                  </V3Td>
                  <V3Td className="whitespace-nowrap text-sm">{event.event_type}</V3Td>
                  <V3Td className="max-w-[160px] truncate font-mono text-xs">
                    {event.node_id || "-"}
                  </V3Td>
                  <V3Td className="max-w-[320px] truncate text-sm">
                    {event.title}
                    {event.description ? (
                      <span className="block truncate text-xs text-v3-ink-3">
                        {event.description}
                      </span>
                    ) : null}
                  </V3Td>
                </V3Tr>
              ))}
            </tbody>
          </V3Table>
        )}

        <LogPagination
          isFetching={eventsQuery.isFetching}
          itemCount={events.length}
          offset={offset}
          onOffsetChange={setOffset}
          pageSize={LOG_PAGE_SIZE}
        />
      </WorkSurface>

      <Sheet
        open={selectedEvent !== null}
        onOpenChange={(open) => {
          if (!open) setSelectedEvent(null);
        }}
      >
        <SheetContent side="right" className="w-[420px] max-w-[45vw]">
          {selectedEvent && (
            <>
              <SheetHeader>
                <SheetTitle className="text-base font-semibold text-v3-ink">
                  {selectedEvent.event_type}
                </SheetTitle>
              </SheetHeader>
              <div className="mt-4 flex flex-col gap-3 text-sm">
                <DetailRow label="级别">
                  <StatusPill tone={severityTone[selectedEvent.severity] ?? "mute"}>
                    {severityLabel[selectedEvent.severity] ?? selectedEvent.severity}
                  </StatusPill>
                </DetailRow>
                <DetailRow label="标题">{selectedEvent.title}</DetailRow>
                {selectedEvent.description && (
                  <DetailRow label="描述">{selectedEvent.description}</DetailRow>
                )}
                <DetailRow label="节点">{selectedEvent.node_id || "—"}</DetailRow>
                <DetailRow label="来源">{selectedEvent.source || "—"}</DetailRow>
                {selectedEvent.provider_type && (
                  <DetailRow label="Provider">{selectedEvent.provider_type}</DetailRow>
                )}
                {selectedEvent.correlation_id && (
                  <DetailRow label="关联 ID">
                    <span className="font-mono text-xs">
                      {selectedEvent.correlation_type
                        ? `${selectedEvent.correlation_type}:${selectedEvent.correlation_id}`
                        : selectedEvent.correlation_id}
                    </span>
                  </DetailRow>
                )}
                <DetailRow label="时间">{formatLogDateTime(selectedEvent.created_at)}</DetailRow>
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

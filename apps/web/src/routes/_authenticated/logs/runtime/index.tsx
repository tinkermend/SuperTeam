import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import {
  DataTable,
  EmptyState,
  ErrorState,
  RelativeTime,
  StatusPill,
  TableSkeleton,
  Td,
  Th,
  Tr,
  type Density,
  type Tone,
} from "@/components/superteam";
import {
  listRuntimeEvents,
  type RuntimeEvent,
  type RuntimeEventSeverity,
} from "@/lib/api/runtime";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import {
  runtimeEventSeverityLabel,
  runtimeEventSourceLabel,
  runtimeEventTypeLabel,
} from "@/lib/status-labels";
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
  LogToolbarSelect,
  LogWorkbench,
  formatLogDateTime,
  logEmptyCopy,
  logRowClassName,
  logTableDensityClass,
  shortenLogId,
  sinceQueryValue,
  type LogSinceWindow,
} from "../-shared";

export const Route = createFileRoute("/_authenticated/logs/runtime/")({
  component: RuntimeEventLogsRoute,
});

const severityTone: Record<RuntimeEventSeverity, Tone> = {
  error: "danger",
  info: "info",
  success: "ok",
  warning: "warn",
};

const severityChipOptions = [
  { label: runtimeEventSeverityLabel("error"), value: "error" },
  { label: runtimeEventSeverityLabel("warning"), value: "warning" },
  { label: runtimeEventSeverityLabel("success"), value: "success" },
  { label: runtimeEventSeverityLabel("info"), value: "info" },
];

/** 事件类型放进下拉（按调查意图分组），不跟级别 chips 并列抢注意力。 */
const eventTypeSelectGroups = [
  {
    label: "节点健康",
    options: [
      { label: runtimeEventTypeLabel("node_offline"), value: "node_offline" },
      { label: runtimeEventTypeLabel("node_online"), value: "node_online" },
      { label: runtimeEventTypeLabel("capability_degraded"), value: "capability_degraded" },
      { label: runtimeEventTypeLabel("capability_reported"), value: "capability_reported" },
    ],
  },
  {
    label: "命令执行",
    options: [
      { label: runtimeEventTypeLabel("command_failed"), value: "command_failed" },
      { label: runtimeEventTypeLabel("command_timed_out"), value: "command_timed_out" },
      { label: runtimeEventTypeLabel("command_completed"), value: "command_completed" },
      { label: runtimeEventTypeLabel("command_cancelled"), value: "command_cancelled" },
      { label: runtimeEventTypeLabel("command_event"), value: "command_event" },
    ],
  },
  {
    label: "节点注册",
    options: [
      { label: runtimeEventTypeLabel("enrollment_requested"), value: "enrollment_requested" },
      { label: runtimeEventTypeLabel("enrollment_approved"), value: "enrollment_approved" },
      { label: runtimeEventTypeLabel("enrollment_rejected"), value: "enrollment_rejected" },
      { label: runtimeEventTypeLabel("enrollment_revoked"), value: "enrollment_revoked" },
    ],
  },
  {
    label: "原生配置",
    options: [
      { label: runtimeEventTypeLabel("provider_native_config_pull"), value: "provider_native_config_pull" },
      { label: runtimeEventTypeLabel("provider_native_config_push"), value: "provider_native_config_push" },
    ],
  },
];

type RuntimeEventFilters = {
  severity?: RuntimeEventSeverity;
  event_type?: string;
  node_id?: string;
};

function runtimeSecondary(event: RuntimeEvent): string {
  const parts = [runtimeEventTypeLabel(event.event_type)];
  if (event.node_id) {
    parts.push(shortenLogId(event.node_id));
  }
  return parts.join(" · ");
}

function RuntimeEventLogsRoute() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const [filters, setFilters] = useState<RuntimeEventFilters>({});
  const [sinceWindow, setSinceWindow] = useState<LogSinceWindow>(DEFAULT_LOG_SINCE);
  const [density, setDensity] = useState<Density>("comfortable");
  const [offset, setOffset] = useState(0);
  const [selectedEvent, setSelectedEvent] = useState<RuntimeEvent | null>(null);

  const eventsQuery = useQuery({
    queryKey: ["web-runtime-event-logs", filters, sinceWindow, offset],
    queryFn: () =>
      listRuntimeEvents({
        baseUrl: apiBaseUrl,
        limit: LOG_PAGE_SIZE,
        offset,
        since: sinceQueryValue(sinceWindow),
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
  const hasExtraFilter = Boolean(filters.severity || filters.event_type || filters.node_id);
  const empty = logEmptyCopy({
    fallbackDescription: "平台或 Runtime 事件产生后会显示在这里。",
    hasExtraFilter,
    noun: "平台事件",
    sinceWindow,
  });

  return (
    <LogWorkbench
      detailLabel="事件详情"
      onDetailDismiss={() => setSelectedEvent(null)}
      toolbar={
        <LogListToolbar
          search={
            <LogToolbarSearch
              id="runtime-log-node"
              onCommit={(value) => updateFilter("node_id", value)}
              placeholder="按节点 ID 筛选，回车生效"
              value={filters.node_id}
            />
          }
          filters={
            <>
              <LogFilterChips
                options={severityChipOptions}
                value={filters.severity}
                onValueChange={(v) => updateFilter("severity", v as RuntimeEventSeverity | undefined)}
              />
              <LogToolbarSelect
                ariaLabel="事件类型"
                groups={eventTypeSelectGroups}
                onValueChange={(v) => updateFilter("event_type", v)}
                placeholder="全部类型"
                value={filters.event_type}
                widthClassName="w-[11.5rem]"
              />
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
        eventsQuery.isLoading && !eventsQuery.data ? (
          <TableSkeleton className="m-4" cols={3} rows={8} />
        ) : eventsQuery.isError ? (
          <ErrorState
            title="平台事件加载失败"
            description="请稍后重试，或确认当前账号仍有访问权限。"
          />
        ) : events.length === 0 ? (
          <EmptyState title={empty.title} description={empty.description} />
        ) : (
          <DataTable className={logTableDensityClass(density)}>
            <thead>
              <Tr>
                <Th className="w-[7.5rem]">时间</Th>
                <Th>摘要</Th>
                <Th className="w-20">级别</Th>
              </Tr>
            </thead>
            <tbody>
              {events.map((event: RuntimeEvent) => (
                <Tr
                  aria-selected={selectedEvent?.id === event.id}
                  className={logRowClassName({
                    failed: event.severity === "error",
                    selected: selectedEvent?.id === event.id,
                    warn: event.severity === "warning",
                  })}
                  key={event.id}
                  onClick={() => setSelectedEvent(event)}
                >
                  <Td className="whitespace-nowrap text-xs">
                    <RelativeTime value={event.created_at} />
                  </Td>
                  <Td className="min-w-0">
                    <p className="truncate text-sm font-medium text-ink">{event.title}</p>
                    <p className="truncate text-xs text-ink-3">{runtimeSecondary(event)}</p>
                  </Td>
                  <Td>
                    <StatusPill tone={severityTone[event.severity] ?? "mute"}>
                      {runtimeEventSeverityLabel(event.severity)}
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
          isFetching={eventsQuery.isFetching}
          itemCount={events.length}
          offset={offset}
          onOffsetChange={setOffset}
          pageSize={LOG_PAGE_SIZE}
        />
      }
      detail={
        selectedEvent ? (
          <LogDetailPanel
            kicker={
              <span className="text-xs text-ink-3">{runtimeEventTypeLabel(selectedEvent.event_type)}</span>
            }
            onClose={() => setSelectedEvent(null)}
            status={
              <StatusPill tone={severityTone[selectedEvent.severity] ?? "mute"}>
                {runtimeEventSeverityLabel(selectedEvent.severity)}
              </StatusPill>
            }
            title={selectedEvent.title}
          >
            {selectedEvent.description ? (
              <LogDetailSection title="说明">
                <p className="text-xs leading-relaxed text-ink-2">{selectedEvent.description}</p>
              </LogDetailSection>
            ) : null}
            <LogDetailSection title="事件信息">
              <div className="flex flex-col gap-2.5">
                <LogInfoRow label="来源">{runtimeEventSourceLabel(selectedEvent.source)}</LogInfoRow>
                <LogInfoRow label="时间">{formatLogDateTime(selectedEvent.created_at)}</LogInfoRow>
                {selectedEvent.provider_type ? (
                  <LogInfoRow label="Provider">{selectedEvent.provider_type}</LogInfoRow>
                ) : null}
                {selectedEvent.correlation_id ? (
                  <LogInfoRow label="关联 ID">
                    <span className="font-mono text-xs">
                      {selectedEvent.correlation_type
                        ? `${selectedEvent.correlation_type}:${selectedEvent.correlation_id}`
                        : selectedEvent.correlation_id}
                    </span>
                  </LogInfoRow>
                ) : null}
              </div>
            </LogDetailSection>
            {selectedEvent.node_id ? (
              <LogDetailSection title="节点">
                <div className="flex items-center gap-2 rounded-lg bg-card-soft px-3 py-2">
                  <span className="font-mono text-xs text-ink">{selectedEvent.node_id}</span>
                </div>
              </LogDetailSection>
            ) : null}
            {selectedEvent.payload ? (
              <LogDetailSection title="Payload">
                <pre className="max-h-64 overflow-auto rounded-lg bg-card-soft p-3 text-[11px] leading-relaxed text-ink">
                  {JSON.stringify(selectedEvent.payload, null, 2)}
                </pre>
              </LogDetailSection>
            ) : null}
          </LogDetailPanel>
        ) : undefined
      }
    />
  );
}

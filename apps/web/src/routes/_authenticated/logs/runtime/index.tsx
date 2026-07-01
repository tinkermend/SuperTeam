import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { ServerCog } from "lucide-react";
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
  type V3Tone,
  WorkSurface,
} from "@/components/superteam";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import {
  listRuntimeEvents,
  type RuntimeEvent,
  type RuntimeEventSeverity,
} from "@/lib/api/runtime";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import {
  LOG_PAGE_SIZE,
  LogFilterBar,
  LogPagination,
  LogSelectFilter,
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

const severityOptions = [
  { label: "信息", value: "info" },
  { label: "成功", value: "success" },
  { label: "预警", value: "warning" },
  { label: "错误", value: "error" },
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
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main className="min-w-0 overflow-x-hidden">
        <div className="mx-auto flex w-full max-w-6xl flex-col gap-4 text-v3-ink">
          <div className="flex items-center gap-3">
            <IconTile tone="mute" size="sm">
              <ServerCog />
            </IconTile>
            <div className="min-w-0">
              <h1 className="text-lg font-bold text-v3-ink">平台事件</h1>
              <p className="truncate text-sm text-v3-ink-2">
                Runtime 节点上线/掉线、能力降级与平台服务异常事件
              </p>
            </div>
          </div>

          <WorkSurface>
            <LogFilterBar>
              <LogSelectFilter
                id="runtime-log-severity"
                label="级别"
                options={severityOptions}
                value={filters.severity}
                onValueChange={(value) =>
                  updateFilter("severity", value as RuntimeEventSeverity | undefined)
                }
              />
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
                    <V3Tr key={event.id}>
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
        </div>
      </Main>
    </>
  );
}

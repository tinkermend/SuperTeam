import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { createFileRoute, useSearch } from "@tanstack/react-router";
import { FileClock } from "lucide-react";
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
import { buildApiUrl, parseJson } from "@/lib/api/client";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";

export const Route = createFileRoute("/_authenticated/audit/")({
  component: AuditRoute,
});

type AuditEvent = {
  id: string;
  tenant_id: string;
  event_type: string;
  actor_type: string;
  actor_id: string;
  resource_type: string;
  resource_id: string;
  action: string;
  details: Record<string, unknown>;
  ip_address: string;
  created_at?: string;
};

type ProjectAuditEventsResult = {
  projectId: string;
  events: AuditEvent[];
};

function AuditRoute() {
  const search = useSearch({ strict: false });
  const projectId = projectIDFromSearch(search);
  const apiBaseUrl = resolveControlPlaneUrl();
  const eventsQuery = useQuery({
    enabled: Boolean(projectId),
    queryKey: ["project-audit-events", projectId],
    queryFn: () => listProjectAuditEvents(apiBaseUrl, projectId as string),
    placeholderData: keepPreviousData,
  });
  const eventData = eventsQuery.data;
  let currentEvents: AuditEvent[] | undefined;
  if (eventData && eventData.projectId === projectId) {
    currentEvents = eventData.events;
  }

  return (
    <div className="mx-auto flex w-full max-w-6xl flex-col gap-4 bg-v3-bg p-4 text-v3-ink md:p-6">
      <div className="flex items-center gap-3">
        <IconTile tone="mute" size="sm">
          <FileClock />
        </IconTile>
        <div className="min-w-0">
          <h1 className="text-lg font-bold text-v3-ink">审计中心</h1>
          <p className="truncate text-sm text-v3-ink-2">
            {projectId ? `项目 ${projectId}` : "等待项目上下文"}
          </p>
        </div>
      </div>

      {!projectId ? (
        <AuditEmptyState />
      ) : (
        <ProjectAuditTable
          events={currentEvents ?? []}
          isError={eventsQuery.isError}
          isFetching={eventsQuery.isFetching}
          isLoading={eventsQuery.isLoading && !currentEvents}
        />
      )}
    </div>
  );
}

function projectIDFromSearch(search: unknown) {
  if (!search || typeof search !== "object" || !("project_id" in search)) {
    return undefined;
  }
  const value = search.project_id;
  return typeof value === "string" ? value.trim() : undefined;
}

async function listProjectAuditEvents(
  apiBaseUrl: string,
  projectId: string,
): Promise<ProjectAuditEventsResult> {
  const params = new URLSearchParams({
    limit: "50",
    resource_id: projectId,
    resource_type: "project",
  });
  const response = await fetch(
    buildApiUrl(apiBaseUrl, `/api/v1/audit/events?${params.toString()}`),
    {
      credentials: "include",
      headers: { accept: "application/json" },
      method: "GET",
    },
  );
  const events = await parseJson<AuditEvent[]>(response, "project audit events");
  return { projectId, events };
}

function AuditEmptyState() {
  return (
    <WorkSurface>
      <V3EmptyState
        icon={<FileClock />}
        title="请选择项目后查看审计"
        description="从项目详情进入审计中心，或在地址中提供 project_id。"
      />
    </WorkSurface>
  );
}

function ProjectAuditTable({
  events,
  isError,
  isFetching,
  isLoading,
}: {
  events: AuditEvent[];
  isError: boolean;
  isFetching: boolean;
  isLoading: boolean;
}) {
  if (isLoading) {
    return (
      <WorkSurface>
        <V3LoadingState label="正在加载项目审计事件…" />
      </WorkSurface>
    );
  }

  if (isError) {
    return (
      <V3ErrorState
        title="项目审计加载失败"
        description="请稍后重试，或确认当前账号仍有项目访问权限。"
      />
    );
  }

  return (
    <WorkSurface>
      <div className="flex items-start justify-between gap-3 border-b border-v3-line px-5 py-4">
        <div>
          <h2 className="text-base font-bold text-v3-ink">项目审计事件</h2>
          <p className="mt-1 text-sm text-v3-ink-2">
            按项目资源筛选最近 50 条操作记录
          </p>
        </div>
        <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
          {isFetching ? <StatusPill tone="info">刷新中</StatusPill> : null}
          <StatusPill tone="mute">{events.length} 条</StatusPill>
        </div>
      </div>
      {events.length === 0 ? (
        <V3EmptyState title="暂无项目审计事件" description="项目相关操作产生后会显示在这里。" />
      ) : (
        <V3Table>
          <thead>
            <V3Tr>
              <V3Th className="min-w-[150px]">时间</V3Th>
              <V3Th>动作</V3Th>
              <V3Th>事件类型</V3Th>
              <V3Th>Actor</V3Th>
              <V3Th>来源 IP</V3Th>
              <V3Th className="min-w-[220px]">详情</V3Th>
            </V3Tr>
          </thead>
          <tbody>
            {events.map((event) => (
              <V3Tr key={event.id}>
                <V3Td className="whitespace-nowrap text-xs text-v3-ink-2 tabular-nums">
                  {formatDateTime(event.created_at)}
                </V3Td>
                <V3Td>
                  <StatusPill tone="mute">{event.action}</StatusPill>
                </V3Td>
                <V3Td className="whitespace-nowrap text-sm">
                  {event.event_type}
                </V3Td>
                <V3Td className="max-w-[220px] truncate font-mono text-xs">
                  {event.actor_type}:{event.actor_id || "-"}
                </V3Td>
                <V3Td className="whitespace-nowrap font-mono text-xs">
                  {event.ip_address || "-"}
                </V3Td>
                <V3Td className="max-w-[280px] truncate font-mono text-xs text-v3-ink-3">
                  {formatDetails(event.details)}
                </V3Td>
              </V3Tr>
            ))}
          </tbody>
        </V3Table>
      )}
    </WorkSurface>
  );
}

function formatDateTime(value?: string) {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat("zh-CN", {
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    month: "2-digit",
    year: "numeric",
  }).format(date);
}

function formatDetails(value: Record<string, unknown>) {
  const keys = Object.keys(value);
  if (keys.length === 0) {
    return "-";
  }
  return JSON.stringify(value);
}

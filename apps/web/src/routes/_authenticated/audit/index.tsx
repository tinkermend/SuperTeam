import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { createFileRoute, useSearch } from "@tanstack/react-router";
import { FileClock } from "lucide-react";
import {
  LiquidCard,
  SemanticIconTile,
  StatusBadge,
} from "@/components/superteam";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
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

function AuditRoute() {
  const search = useSearch({ from: "/_authenticated/audit/" }) as {
    project_id?: string;
  };
  const projectId = search.project_id?.trim();
  const apiBaseUrl = resolveControlPlaneUrl();
  const eventsQuery = useQuery({
    enabled: Boolean(projectId),
    queryKey: ["project-audit-events", projectId],
    queryFn: () => listProjectAuditEvents(apiBaseUrl, projectId as string),
    placeholderData: keepPreviousData,
  });

  return (
    <div className="mx-auto flex w-full max-w-6xl flex-col gap-4 p-4 md:p-6">
      <div className="flex items-center gap-3">
        <SemanticIconTile tone="neutral" size="sm">
          <FileClock />
        </SemanticIconTile>
        <div className="min-w-0">
          <h1 className="text-lg font-semibold">审计中心</h1>
          <p className="truncate text-sm text-muted-foreground">
            {projectId ? `项目 ${projectId}` : "等待项目上下文"}
          </p>
        </div>
      </div>

      {!projectId ? (
        <AuditEmptyState />
      ) : (
        <ProjectAuditTable
          events={eventsQuery.data ?? []}
          isError={eventsQuery.isError}
          isFetching={eventsQuery.isFetching}
          isLoading={eventsQuery.isLoading && !eventsQuery.data}
        />
      )}
    </div>
  );
}

async function listProjectAuditEvents(
  apiBaseUrl: string,
  projectId: string,
): Promise<AuditEvent[]> {
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
  return parseJson<AuditEvent[]>(response, "project audit events");
}

function AuditEmptyState() {
  return (
    <LiquidCard className="rounded-xl p-5">
      <div className="flex items-center gap-3">
        <SemanticIconTile tone="neutral" size="sm">
          <FileClock />
        </SemanticIconTile>
        <div>
          <h2 className="font-semibold">请选择项目后查看审计</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            从项目详情进入审计中心，或在地址中提供 project_id。
          </p>
        </div>
      </div>
    </LiquidCard>
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
      <LiquidCard className="rounded-xl p-5 text-sm text-muted-foreground">
        正在加载项目审计事件...
      </LiquidCard>
    );
  }

  if (isError) {
    return (
      <LiquidCard className="rounded-xl p-5">
        <div className="flex items-center justify-between gap-3">
          <div>
            <h2 className="font-semibold">项目审计加载失败</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              请稍后重试，或确认当前账号仍有项目访问权限。
            </p>
          </div>
          <StatusBadge tone="danger">失败</StatusBadge>
        </div>
      </LiquidCard>
    );
  }

  return (
    <LiquidCard className="rounded-xl">
      <div className="flex items-center justify-between gap-3 border-b p-4">
        <div>
          <h2 className="font-semibold">项目审计事件</h2>
          <p className="text-xs text-muted-foreground">
            按项目资源筛选最近 50 条操作记录
          </p>
        </div>
        <div className="flex items-center gap-2">
          {isFetching ? <StatusBadge tone="info">刷新中</StatusBadge> : null}
          <StatusBadge tone="neutral">{events.length} 条</StatusBadge>
        </div>
      </div>
      <div className="overflow-x-auto p-4">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="min-w-[150px]">时间</TableHead>
              <TableHead>动作</TableHead>
              <TableHead>事件类型</TableHead>
              <TableHead>Actor</TableHead>
              <TableHead>来源 IP</TableHead>
              <TableHead className="min-w-[220px]">详情</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {events.length === 0 ? (
              <TableRow>
                <TableCell
                  className="h-24 text-center text-sm text-muted-foreground"
                  colSpan={6}
                >
                  暂无项目审计事件
                </TableCell>
              </TableRow>
            ) : (
              events.map((event) => (
                <TableRow key={event.id}>
                  <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                    {formatDateTime(event.created_at)}
                  </TableCell>
                  <TableCell>
                    <StatusBadge tone="neutral">{event.action}</StatusBadge>
                  </TableCell>
                  <TableCell className="whitespace-nowrap text-sm">
                    {event.event_type}
                  </TableCell>
                  <TableCell className="max-w-[220px] truncate font-mono text-xs">
                    {event.actor_type}:{event.actor_id || "-"}
                  </TableCell>
                  <TableCell className="whitespace-nowrap font-mono text-xs">
                    {event.ip_address || "-"}
                  </TableCell>
                  <TableCell className="max-w-[280px] truncate font-mono text-xs text-muted-foreground">
                    {formatDetails(event.details)}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
    </LiquidCard>
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

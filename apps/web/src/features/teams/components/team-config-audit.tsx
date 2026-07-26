import { useState } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import {
  DataTable,
  ErrorState,
  LoadingState,
  Pagination,
  StatusPill,
  Td,
  Th,
  Tr,
  WorkSurface
} from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import type { AllowedTeamAction, TeamAuditEvent } from "@/lib/api/teams";
import { listTeamAuditEvents } from "@/lib/api/teams";
import { teamAuditActionLabel } from "@/lib/status-labels";
import { formatDateTime, formatRelativeTime } from "@/lib/format-time";

const PAGE_SIZE = 20;

type TeamConfigAuditProps = {
  allowedActions: AllowedTeamAction[];
  apiOptions: ApiClientOptions;
  teamId: string;
};

export function TeamConfigAudit({ allowedActions, apiOptions, teamId }: TeamConfigAuditProps) {
  const canRead = allowedActions.includes("team.audit.read");
  const [page, setPage] = useState(1);

  const auditQuery = useQuery({
    enabled: canRead,
    queryKey: ["team-audit", teamId, page],
    queryFn: () =>
      listTeamAuditEvents(apiOptions, teamId, {
        limit: PAGE_SIZE,
        offset: (page - 1) * PAGE_SIZE
      }),
    placeholderData: keepPreviousData
  });

  if (!canRead) {
    return (
      <WorkSurface>
        <p className="p-5 text-[13px] text-ink-2">没有查看本团队审计流水的权限。</p>
      </WorkSurface>
    );
  }

  const events = auditQuery.data ?? [];
  // 端点按页返回，没有总数：拿到满页就允许翻下一页，空页则停在当前页。
  const hasNextPage = events.length === PAGE_SIZE;

  return (
    <WorkSurface>
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-5 py-4">
        <div className="min-w-0">
          <h2 className="text-base font-bold text-ink">团队变更流水</h2>
          <p className="mt-1 text-[13px] text-ink-2">
            成员、能力绑定、编制与身份变更的审计记录，按时间倒序。
          </p>
        </div>
        {auditQuery.isFetching ? <StatusPill tone="info">刷新中</StatusPill> : null}
      </div>
      <div className="p-4">
        {auditQuery.isLoading ? <LoadingState label="加载团队审计" /> : null}
        {auditQuery.isError ? <ErrorState title="团队审计加载失败" /> : null}
        {!auditQuery.isLoading && !auditQuery.isError && events.length === 0 ? (
          <p className="py-2 text-[13px] text-ink-2">
            {page > 1 ? "没有更多记录" : "暂无变更记录"}
          </p>
        ) : null}
        {events.length > 0 ? (
          <DataTable aria-label="团队变更流水">
            <thead>
              <tr>
                <Th>时间</Th>
                <Th>动作</Th>
                <Th>详情</Th>
              </tr>
            </thead>
            <tbody>
              {events.map((event) => (
                <Tr key={event.id}>
                  <Td className="whitespace-nowrap tabular-nums">
                    <span title={event.created_at ? formatDateTime(event.created_at) : undefined}>
                      {event.created_at ? formatRelativeTime(event.created_at) : "-"}
                    </span>
                  </Td>
                  <Td>
                    <span className="font-medium text-ink">
                      {teamAuditActionLabel(event.action)}
                    </span>
                  </Td>
                  <Td>
                    <AuditDetails event={event} />
                  </Td>
                </Tr>
              ))}
            </tbody>
          </DataTable>
        ) : null}
      </div>
      {(page > 1 || hasNextPage) && events.length > 0 ? (
        <Pagination
          onPageChange={setPage}
          page={page}
          pageCount={hasNextPage ? page + 1 : page}
          pageSize={PAGE_SIZE}
          total={(page - 1) * PAGE_SIZE + events.length}
        />
      ) : null}
    </WorkSurface>
  );
}

// details 是各动作自带的结构化载荷，键集合不固定；这里按 mono 小字展示原样键值，
// 属于技术详情区（宪法允许该区出现标识符）。
function AuditDetails({ event }: { event: TeamAuditEvent }) {
  const entries = Object.entries(event.details ?? {}).filter(
    ([, value]) => value !== null && value !== undefined && value !== "",
  );
  if (entries.length === 0) {
    return <span className="text-xs text-ink-3">-</span>;
  }
  return (
    <div className="flex flex-wrap gap-x-3 gap-y-1">
      {entries.map(([key, value]) => (
        <span key={key} className="font-mono text-[11px] text-ink-2">
          {key}=
          {typeof value === "object" ? JSON.stringify(value) : String(value)}
        </span>
      ))}
    </div>
  );
}

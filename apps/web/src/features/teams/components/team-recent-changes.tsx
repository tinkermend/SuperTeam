import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  Button,
  DataTable,
  ErrorState,
  LoadingState,
  Td,
  Th,
  Tr,
  WorkSurface
} from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import type { AllowedTeamAction } from "@/lib/api/teams";
import { listTeamAuditEvents } from "@/lib/api/teams";
import { teamAuditActionLabel } from "@/lib/status-labels";
import { formatDateTime, formatRelativeTime } from "@/lib/format-time";

const RECENT_LIMIT = 5;

// 观察面「最近变更」：团队审计流的前几条，完整流水在配置页的审计分区。
export function TeamRecentChanges({
  allowedActions,
  apiOptions,
  teamId
}: {
  allowedActions: AllowedTeamAction[];
  apiOptions: ApiClientOptions;
  teamId: string;
}) {
  const canRead = allowedActions.includes("team.audit.read");
  const auditQuery = useQuery({
    enabled: canRead,
    queryKey: ["team-audit", teamId, "recent"],
    queryFn: () => listTeamAuditEvents(apiOptions, teamId, { limit: RECENT_LIMIT, offset: 0 })
  });

  if (!canRead) {
    return null;
  }

  const events = auditQuery.data ?? [];

  return (
    <WorkSurface>
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-5 py-4">
        <div className="min-w-0">
          <h2 className="text-base font-bold text-ink">最近变更</h2>
          <p className="mt-1 text-[13px] text-ink-2">团队编制、能力与身份的最近改动。</p>
        </div>
        <Button asChild size="sm" variant="ghost">
          <Link hash="audit" params={{ teamId }} to="/teams/$teamId/config">
            查看全部
          </Link>
        </Button>
      </div>
      <div className="p-4">
        {auditQuery.isLoading ? <LoadingState label="加载最近变更" /> : null}
        {auditQuery.isError ? <ErrorState title="最近变更加载失败" /> : null}
        {!auditQuery.isLoading && !auditQuery.isError && events.length === 0 ? (
          <p className="py-2 text-[13px] text-ink-2">暂无变更记录</p>
        ) : null}
        {events.length > 0 ? (
          <DataTable aria-label="团队最近变更">
            <thead>
              <tr>
                <Th>时间</Th>
                <Th>动作</Th>
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
                  <Td>{teamAuditActionLabel(event.action)}</Td>
                </Tr>
              ))}
            </tbody>
          </DataTable>
        ) : null}
      </div>
    </WorkSurface>
  );
}

import {
  StatusPill,
  V3Chip,
  V3Pagination,
  V3StateSurface,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  WorkSurface,
  type V3Tone,
} from "@/components/superteam";
import type { DigitalEmployeeRunListItem, DigitalEmployeeRunListResult, DigitalEmployeeRunStatus } from "@/lib/api/employees";

type EmployeeRunHistoryTableProps = {
  result: DigitalEmployeeRunListResult | undefined;
  isLoading?: boolean;
  isError?: boolean;
  error?: unknown;
  page: number;
  pageSize: number;
  statusFilter: DigitalEmployeeRunStatus | undefined;
  onStatusFilterChange: (status: DigitalEmployeeRunStatus | undefined) => void;
  onPageChange: (page: number) => void;
  onRowClick: (item: DigitalEmployeeRunListItem) => void;
  onRetry: () => void;
};

const runStatusTone: Record<DigitalEmployeeRunStatus, V3Tone> = {
  queued: "mute",
  dispatching: "mute",
  running: "info",
  cancelling: "warn",
  completed: "ok",
  failed: "danger",
  cancelled: "warn",
  timed_out: "danger",
};

export function EmployeeRunHistoryTable({
  result,
  isLoading,
  isError,
  error,
  page,
  pageSize,
  statusFilter,
  onStatusFilterChange,
  onPageChange,
  onRowClick,
  onRetry,
}: EmployeeRunHistoryTableProps) {
  const items = result?.items ?? [];
  const total = result?.total_count ?? 0;
  const pageCount = Math.max(1, Math.ceil(total / pageSize));

  return (
    <WorkSurface>
      <div className="flex flex-wrap items-center gap-2 border-b border-v3-line px-4 py-3">
        <V3Chip active={statusFilter === undefined} onClick={() => onStatusFilterChange(undefined)} type="button">
          全部状态
        </V3Chip>
        {result?.filters.statuses.map((option) => (
          <V3Chip
            active={statusFilter === option.value}
            key={option.value}
            onClick={() => onStatusFilterChange(option.value as DigitalEmployeeRunStatus)}
            type="button"
          >
            {option.label}
          </V3Chip>
        ))}
      </div>
      <V3StateSurface empty={items.length === 0} error={error} isError={isError} isLoading={isLoading} onRetry={onRetry}>
        <V3Table>
          <thead>
            <tr>
              <V3Th>任务 / 项目</V3Th>
              <V3Th>会话 ID</V3Th>
              <V3Th>状态</V3Th>
              <V3Th>耗时</V3Th>
              <V3Th>工件</V3Th>
              <V3Th>时间</V3Th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <V3Tr
                className="cursor-pointer"
                key={item.id}
                onClick={() => onRowClick(item)}
                tone={item.status === "failed" || item.status === "timed_out" ? "danger" : undefined}
              >
                <V3Td>
                  <p className="truncate font-medium text-v3-ink">{item.task_title}</p>
                  <p className="truncate text-xs text-v3-ink-3">{item.project_name ?? "无关联项目"}</p>
                </V3Td>
                <V3Td className="font-mono text-xs text-v3-ink-2">{shortId(item.id)}</V3Td>
                <V3Td>
                  <StatusPill tone={runStatusTone[item.status]}>{item.status}</StatusPill>
                </V3Td>
                <V3Td className="tabular-nums">{item.duration_sec != null ? formatDuration(item.duration_sec) : "--"}</V3Td>
                <V3Td className="tabular-nums">{item.work_product_count}</V3Td>
                <V3Td className="text-xs text-v3-ink-3">{item.updated_at ?? item.created_at ?? "-"}</V3Td>
              </V3Tr>
            ))}
          </tbody>
        </V3Table>
      </V3StateSurface>
      <V3Pagination onPageChange={onPageChange} page={page} pageCount={pageCount} pageSize={pageSize} total={total} />
    </WorkSurface>
  );
}

function shortId(id: string): string {
  return id.slice(0, 8);
}

function formatDuration(seconds: number): string {
  const totalSeconds = Math.round(seconds);
  const minutes = Math.floor(totalSeconds / 60);
  const remainSeconds = totalSeconds % 60;
  return `${minutes}分${remainSeconds}秒`;
}

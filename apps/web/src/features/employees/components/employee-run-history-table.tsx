import {
  StatusPill,
  Button,
  Chip,
  Pagination,
  StateSurface,
  DataTable,
  Td,
  Th,
  Tr,
  WorkSurface,
  type Tone
} from "@/components/superteam";
import type {
  DigitalEmployeeRunKind,
  DigitalEmployeeRunListItem,
  DigitalEmployeeRunListResult,
  DigitalEmployeeRunStatus
} from "@/lib/api/employees";
import { formatDateTime } from "@/lib/format-time";
import { runStatusLabel, statusLabel } from "@/lib/status-labels";
import { Link } from "@tanstack/react-router";

type EmployeeRunHistoryTableProps = {
  employeeId: string;
  result: DigitalEmployeeRunListResult | undefined;
  isLoading?: boolean;
  isError?: boolean;
  error?: unknown;
  page: number;
  pageSize: number;
  statusFilter: DigitalEmployeeRunStatus | undefined;
  onStatusFilterChange: (status: DigitalEmployeeRunStatus | undefined) => void;
  runKindFilter: DigitalEmployeeRunKind | undefined;
  onRunKindFilterChange: (runKind: DigitalEmployeeRunKind | undefined) => void;
  onPageChange: (page: number) => void;
  onRowClick: (item: DigitalEmployeeRunListItem) => void;
  onRetry: () => void;
};

const runStatusTone: Record<DigitalEmployeeRunStatus, Tone> = {
  queued: "mute",
  dispatching: "mute",
  running: "info",
  cancelling: "warn",
  completed: "ok",
  failed: "danger",
  cancelled: "warn",
  timed_out: "danger"
};

const runKindLabel: Record<DigitalEmployeeRunKind, string> = {
  task: "任务",
  chat: "对话"
};

export function EmployeeRunHistoryTable({
  employeeId,
  result,
  isLoading,
  isError,
  error,
  page,
  pageSize,
  statusFilter,
  onStatusFilterChange,
  runKindFilter,
  onRunKindFilterChange,
  onPageChange,
  onRowClick,
  onRetry
}: EmployeeRunHistoryTableProps) {
  const items = result?.items ?? [];
  const total = result?.total_count ?? 0;
  const pageCount = Math.max(1, Math.ceil(total / pageSize));

  return (
    <WorkSurface>
      <div className="flex flex-col gap-3 border-b border-line px-4 py-3 lg:flex-row lg:items-center lg:justify-between">
        <div className="flex flex-col gap-2">
          <div className="flex flex-wrap items-center gap-2">
            <Chip active={statusFilter === undefined} onClick={() => onStatusFilterChange(undefined)} type="button">
              全部状态
            </Chip>
            {result?.filters.statuses.map((option) => (
              <Chip
                active={statusFilter === option.value}
                key={option.value}
                onClick={() => onStatusFilterChange(option.value as DigitalEmployeeRunStatus)}
                type="button"
              >
                {option.label}
              </Chip>
            ))}
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Chip active={runKindFilter === undefined} onClick={() => onRunKindFilterChange(undefined)} type="button">
              全部
            </Chip>
            <Chip active={runKindFilter === "task"} onClick={() => onRunKindFilterChange("task")} type="button">
              任务
            </Chip>
            <Chip active={runKindFilter === "chat"} onClick={() => onRunKindFilterChange("chat")} type="button">
              对话
            </Chip>
          </div>
        </div>
        <Button asChild size="sm" variant="outline">
          <Link search={{ employee: employeeId }} to="/run-overview">
            在运行总览查看
          </Link>
        </Button>
      </div>
      <StateSurface empty={items.length === 0} error={error} isError={isError} isLoading={isLoading} onRetry={onRetry}>
        <DataTable>
          <thead>
            <tr>
              <Th>任务 / 项目</Th>
              <Th>会话 ID</Th>
              <Th>类型</Th>
              <Th>状态</Th>
              <Th>耗时</Th>
              <Th>工件</Th>
              <Th>时间</Th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <Tr
                className="cursor-pointer"
                key={item.id}
                onClick={() => onRowClick(item)}
                tone={item.status === "failed" || item.status === "timed_out" ? "danger" : undefined}
              >
                <Td>
                  <p className="truncate font-medium text-ink">{item.task_title}</p>
                  <p className="truncate text-xs text-ink-3">
                    {item.project_name
                      ? item.project_deleted
                        ? `${item.project_name}（${statusLabel("deleted")}）`
                        : item.project_name
                      : "无关联项目"}
                  </p>
                </Td>
                <Td className="font-mono text-xs text-ink-2">{shortId(item.id)}</Td>
                <Td>
                  <StatusPill showDot={false} tone="mute">
                    {runKindLabel[item.run_kind]}
                  </StatusPill>
                </Td>
                <Td>
                  <StatusPill tone={runStatusTone[item.status]}>{runStatusLabel(item.status)}</StatusPill>
                </Td>
                <Td className="tabular-nums">{item.duration_sec != null ? formatDuration(item.duration_sec) : "--"}</Td>
                <Td className="tabular-nums">{item.work_product_count}</Td>
                <Td className="text-xs text-ink-3">{formatRowTime(item)}</Td>
              </Tr>
            ))}
          </tbody>
        </DataTable>
      </StateSurface>
      <Pagination onPageChange={onPageChange} page={page} pageCount={pageCount} pageSize={pageSize} total={total} />
    </WorkSurface>
  );
}

function shortId(id: string): string {
  return id.slice(0, 8);
}

function formatRowTime(item: DigitalEmployeeRunListItem) {
  const value = item.updated_at ?? item.created_at;
  return value ? formatDateTime(value) : "-";
}

function formatDuration(seconds: number): string {
  const totalSeconds = Math.round(seconds);
  const minutes = Math.floor(totalSeconds / 60);
  const remainSeconds = totalSeconds % 60;
  return `${minutes}分${remainSeconds}秒`;
}

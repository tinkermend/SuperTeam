import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { useMemo, useState } from "react";
import { RotateCcw } from "lucide-react";
import {
  Button,
  Callout,
  EmptyState,
  ErrorState,
  LoadingState,
  DataTable,
  StatusPill,
  Td,
  Th,
  Tr,
  WorkSurface,
  type Tone,
} from "@/components/superteam";
import { ApiRequestError } from "@/lib/api/client";
import {
  listFeishuOperationalOutbox,
  requeueFeishuOutbox,
  type FeishuOperationalOutboxItem,
} from "@/lib/api/channel-admin";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import {
  LOG_PAGE_SIZE,
  LogChips,
  LogFilterBar,
  LogPagination,
  LogSelectFilter,
  formatLogDateTime,
} from "../-shared";

export const Route = createFileRoute("/_authenticated/logs/delivery/")({
  component: DeliveryLogsRoute,
});

const statusChipOptions = [
  { label: "失败", value: "failed" },
  { label: "未绑定跳过", value: "skipped_unbound" },
];

const statusSelectOptions = [
  { label: "失败", value: "failed" },
  { label: "未绑定跳过", value: "skipped_unbound" },
];

type DeliveryFilters = {
  status?: string;
};

function statusLabel(status: string): string {
  switch (status) {
    case "failed":
      return "失败";
    case "skipped_unbound":
      return "未绑定跳过";
    case "pending":
      return "待投递";
    default:
      return status;
  }
}

function statusTone(status: string): Tone {
  switch (status) {
    case "failed":
      return "danger";
    case "skipped_unbound":
      return "warn";
    case "pending":
      return "info";
    default:
      return "mute";
  }
}

function shortenId(id: string): string {
  return id.length > 12 ? `${id.slice(0, 8)}…` : id;
}

function DeliveryLogsRoute() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const apiOptions = useMemo(() => ({ baseUrl: apiBaseUrl }), [apiBaseUrl]);
  const queryClient = useQueryClient();
  const [filters, setFilters] = useState<DeliveryFilters>({});
  const [offset, setOffset] = useState(0);
  const [actionError, setActionError] = useState<string | null>(null);

  const statusParam = filters.status || "failed,skipped_unbound";

  const logsQuery = useQuery({
    queryKey: ["web-delivery-outbox", filters, offset],
    queryFn: () =>
      listFeishuOperationalOutbox(apiOptions, {
        status: statusParam,
        limit: LOG_PAGE_SIZE,
        offset,
      }),
    placeholderData: keepPreviousData,
  });

  const requeueMutation = useMutation({
    mutationFn: (outboxId: string) => requeueFeishuOutbox(apiOptions, outboxId),
    onSuccess: async () => {
      setActionError(null);
      await queryClient.invalidateQueries({ queryKey: ["web-delivery-outbox"] });
    },
    onError: (error: unknown) => {
      setActionError(
        error instanceof ApiRequestError && error.detail
          ? error.detail
          : error instanceof Error
            ? error.message
            : "重推失败",
      );
    },
  });

  const updateFilter = <K extends keyof DeliveryFilters>(key: K, value: DeliveryFilters[K]) => {
    setOffset(0);
    setFilters((prev) => ({ ...prev, [key]: value }));
  };

  const records = logsQuery.data?.items ?? [];
  const total = logsQuery.data?.total ?? 0;
  const hasFilter = Boolean(filters.status);

  return (
    <>
      {actionError ? (
        <Callout className="mb-3" tone="danger" title="操作失败" description={actionError} />
      ) : null}

      <WorkSurface>
        <div className="flex flex-wrap items-start justify-between gap-3 border-b border-line px-5 py-4">
          <div className="min-w-0">
            <h2 className="text-sm font-semibold text-ink">消息投递台账</h2>
            <p className="mt-0.5 text-xs text-ink-2">
              飞书 outbox 失败与未绑定终态。失败行可重推；通道接入与服务凭据仍在{" "}
              <Link className="font-medium text-brand underline-offset-2 hover:underline" to="/system-config">
                系统配置 · 消息通道
              </Link>
              。
              {total > 0 ? ` 合计 ${total} 条。` : ""}
            </p>
          </div>
        </div>

        <LogChips
          options={statusChipOptions}
          value={filters.status}
          onValueChange={(v) => updateFilter("status", v)}
        />
        <LogFilterBar>
          <LogSelectFilter
            id="delivery-log-status"
            label="状态"
            options={statusSelectOptions}
            value={filters.status}
            onValueChange={(v) => updateFilter("status", v)}
          />
        </LogFilterBar>

        {logsQuery.isLoading && !logsQuery.data ? (
          <LoadingState label="正在加载投递台账…" />
        ) : logsQuery.isError ? (
          <ErrorState
            title="投递台账加载失败"
            description="请确认管理员权限，或稍后重试。"
          />
        ) : records.length === 0 ? (
          <EmptyState
            icon={<RotateCcw />}
            title={hasFilter ? "筛选后无投递记录" : "没有待处理投递"}
            description="失败与未绑定行会显示在这里；修复通道或绑定后可重推失败项。"
          />
        ) : (
          <DataTable>
            <thead>
              <Tr>
                <Th className="w-28">状态</Th>
                <Th className="w-32">类型</Th>
                <Th>资源</Th>
                <Th className="w-16">次数</Th>
                <Th>最近错误</Th>
                <Th className="min-w-[150px]">更新时间</Th>
                <Th className="w-24" aria-label="操作" />
              </Tr>
            </thead>
            <tbody>
              {records.map((item: FeishuOperationalOutboxItem) => (
                <Tr key={item.id} tone={item.status === "failed" ? "danger" : undefined}>
                  <Td>
                    <StatusPill tone={statusTone(item.status)}>{statusLabel(item.status)}</StatusPill>
                  </Td>
                  <Td className="font-mono text-xs">{item.kind}</Td>
                  <Td className="min-w-0">
                    <div className="truncate font-mono text-xs text-ink">
                      {item.resource_type}/{shortenId(item.resource_id)}
                    </div>
                    <div className="truncate text-xs text-ink-2">
                      收件人 {shortenId(item.recipient_user_id)}
                      {item.project_id ? ` · 项目 ${shortenId(item.project_id)}` : ""}
                    </div>
                  </Td>
                  <Td className="tabular-nums text-sm">{item.attempts}</Td>
                  <Td
                    className="max-w-[16rem] truncate font-mono text-xs text-ink-2"
                    title={item.last_error ?? undefined}
                  >
                    {item.last_error || "—"}
                  </Td>
                  <Td className="whitespace-nowrap text-xs tabular-nums text-ink-2">
                    {formatLogDateTime(item.updated_at)}
                  </Td>
                  <Td>
                    {item.status === "failed" ? (
                      <Button
                        disabled={requeueMutation.isPending}
                        onClick={() => requeueMutation.mutate(item.id)}
                        size="sm"
                        type="button"
                        variant="outline"
                      >
                        <RotateCcw data-icon="inline-start" />
                        重推
                      </Button>
                    ) : (
                      <span className="text-xs text-ink-2">需先绑定</span>
                    )}
                  </Td>
                </Tr>
              ))}
            </tbody>
          </DataTable>
        )}

        <LogPagination
          isFetching={logsQuery.isFetching}
          itemCount={records.length}
          offset={offset}
          onOffsetChange={setOffset}
          pageSize={LOG_PAGE_SIZE}
        />
      </WorkSurface>
    </>
  );
}

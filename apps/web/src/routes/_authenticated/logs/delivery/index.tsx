import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { useMemo, useState } from "react";
import { RotateCcw } from "lucide-react";
import {
  Button,
  Callout,
  DataTable,
  EmptyState,
  ErrorState,
  ObjectRef,
  RelativeTime,
  StatusPill,
  TableSkeleton,
  Td,
  Th,
  Tr,
  type Density,
  type Tone,
} from "@/components/superteam";
import { ApiRequestError } from "@/lib/api/client";
import {
  listFeishuOperationalOutbox,
  requeueFeishuOutbox,
  type FeishuOperationalOutboxItem,
} from "@/lib/api/channel-admin";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { deliveryOutboxKindLabel, deliveryOutboxStatusLabel } from "@/lib/status-labels";
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
  LogWorkbench,
  formatLogDateTime,
  logEmptyCopy,
  logRowClassName,
  logTableDensityClass,
  resourceCaption,
  sinceQueryValue,
  type LogSinceWindow,
} from "../-shared";

export const Route = createFileRoute("/_authenticated/logs/delivery/")({
  component: DeliveryLogsRoute,
});

const ALL_DELIVERY_STATUSES = "pending,sent,failed,skipped_unbound,superseded";

const statusChipOptions = [
  { label: deliveryOutboxStatusLabel("failed"), value: "failed" },
  { label: deliveryOutboxStatusLabel("skipped_unbound"), value: "skipped_unbound" },
  { label: deliveryOutboxStatusLabel("sent"), value: "sent" },
  { label: deliveryOutboxStatusLabel("pending"), value: "pending" },
  { label: deliveryOutboxStatusLabel("superseded"), value: "superseded" },
];

type DeliveryFilters = {
  status?: string;
};

function statusTone(status: string): Tone {
  switch (status) {
    case "failed":
      return "danger";
    case "skipped_unbound":
      return "warn";
    case "pending":
      return "info";
    case "sent":
      return "ok";
    default:
      return "mute";
  }
}

function deliverySummary(item: FeishuOperationalOutboxItem): string {
  const kind = deliveryOutboxKindLabel(item.kind);
  const resource = resourceCaption(
    item.resource_type,
    item.resource_id,
    item.resource_title,
  );
  const project = item.project_name?.trim();
  if (resource && project) {
    return `${kind} · ${resource} · ${project}`;
  }
  if (resource) {
    return `${kind} · ${resource}`;
  }
  if (project) {
    return `${kind} · ${project}`;
  }
  return kind;
}

function deliverySecondary(item: FeishuOperationalOutboxItem): string {
  if (item.last_error) {
    return item.last_error;
  }
  const recipient = item.recipient_display_name?.trim();
  if (recipient) {
    return `发给 ${recipient} · 尝试 ${item.attempts} 次`;
  }
  return `尝试 ${item.attempts} 次`;
}

function DeliveryLogsRoute() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const apiOptions = useMemo(() => ({ baseUrl: apiBaseUrl }), [apiBaseUrl]);
  const queryClient = useQueryClient();
  const [filters, setFilters] = useState<DeliveryFilters>({});
  const [sinceWindow, setSinceWindow] = useState<LogSinceWindow>(DEFAULT_LOG_SINCE);
  const [density, setDensity] = useState<Density>("comfortable");
  const [offset, setOffset] = useState(0);
  const [selected, setSelected] = useState<FeishuOperationalOutboxItem | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const statusParam = filters.status || ALL_DELIVERY_STATUSES;

  const logsQuery = useQuery({
    queryKey: ["web-delivery-outbox", filters, sinceWindow, offset],
    queryFn: () =>
      listFeishuOperationalOutbox(apiOptions, {
        status: statusParam,
        since: sinceQueryValue(sinceWindow),
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
  const hasExtraFilter = Boolean(filters.status);
  const empty = logEmptyCopy({
    fallbackDescription: "投递产生后会显示在这里；修复通道或绑定后可重推失败项。",
    hasExtraFilter,
    noun: "投递记录",
    sinceWindow,
  });

  return (
    <>
      {actionError ? (
        <Callout className="mb-3" tone="danger" title="操作失败" description={actionError} />
      ) : null}

      <LogWorkbench
        detailLabel="投递详情"
        onDetailDismiss={() => setSelected(null)}
        toolbar={
          <LogListToolbar
            filters={
              <LogFilterChips
                options={statusChipOptions}
                value={filters.status}
                onValueChange={(v) => updateFilter("status", v)}
              />
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
            actions={
              <>
                <LogDensityToggle value={density} onChange={setDensity} />
                <Button asChild size="sm" variant="ghost">
                  <Link to="/system-config">通道配置</Link>
                </Button>
              </>
            }
          />
        }
        body={
          logsQuery.isLoading && !logsQuery.data ? (
            <TableSkeleton className="m-4" cols={4} rows={8} />
          ) : logsQuery.isError ? (
            <ErrorState title="投递台账加载失败" description="请确认管理员权限，或稍后重试。" />
          ) : records.length === 0 ? (
            <EmptyState
              icon={<RotateCcw />}
              title={empty.title}
              description={empty.description}
            />
          ) : (
            <DataTable className={logTableDensityClass(density)}>
              <thead>
                <Tr>
                  <Th className="w-[7.5rem]">时间</Th>
                  <Th>摘要</Th>
                  <Th className="w-24">状态</Th>
                  <Th className="w-24" aria-label="操作" />
                </Tr>
              </thead>
              <tbody>
                {records.map((item: FeishuOperationalOutboxItem) => (
                  <Tr
                    aria-selected={selected?.id === item.id}
                    className={logRowClassName({
                      failed: item.status === "failed",
                      selected: selected?.id === item.id,
                      warn: item.status === "skipped_unbound",
                    })}
                    key={item.id}
                    onClick={() => setSelected(item)}
                  >
                    <Td className="whitespace-nowrap text-xs">
                      <RelativeTime value={item.updated_at} />
                    </Td>
                    <Td className="min-w-0">
                      <p className="truncate text-sm font-medium text-ink">{deliverySummary(item)}</p>
                      <p className="truncate text-xs text-ink-3">{deliverySecondary(item)}</p>
                    </Td>
                    <Td>
                      <StatusPill tone={statusTone(item.status)}>
                        {deliveryOutboxStatusLabel(item.status)}
                      </StatusPill>
                    </Td>
                    <Td>
                      {item.status === "failed" ? (
                        <Button
                          disabled={requeueMutation.isPending}
                          onClick={(event) => {
                            event.stopPropagation();
                            requeueMutation.mutate(item.id);
                          }}
                          size="sm"
                          type="button"
                          variant="outline"
                        >
                          <RotateCcw data-icon="inline-start" />
                          重推
                        </Button>
                      ) : item.status === "skipped_unbound" ? (
                        <span className="text-xs text-ink-2">需先绑定</span>
                      ) : (
                        <span className="text-xs text-ink-2">—</span>
                      )}
                    </Td>
                  </Tr>
                ))}
              </tbody>
            </DataTable>
          )
        }
        pagination={
          <LogPagination
            isFetching={logsQuery.isFetching}
            itemCount={records.length}
            offset={offset}
            onOffsetChange={setOffset}
            pageSize={LOG_PAGE_SIZE}
          />
        }
        detail={
          selected ? (
            <LogDetailPanel
              kicker={
                <span className="text-xs text-ink-3">{deliveryOutboxKindLabel(selected.kind)}</span>
              }
              onClose={() => setSelected(null)}
              status={
                <StatusPill tone={statusTone(selected.status)}>
                  {deliveryOutboxStatusLabel(selected.status)}
                </StatusPill>
              }
              title={deliverySummary(selected)}
            >
              <LogDetailSection title="投递信息">
                <div className="flex flex-col gap-2.5">
                  <LogInfoRow label="更新">{formatLogDateTime(selected.updated_at)}</LogInfoRow>
                  <LogInfoRow label="创建">{formatLogDateTime(selected.created_at)}</LogInfoRow>
                  <LogInfoRow label="资源">
                    <ObjectRef
                      id={selected.resource_id}
                      name={resourceCaption(
                        selected.resource_type,
                        selected.resource_id,
                        selected.resource_title,
                      )}
                    />
                  </LogInfoRow>
                  <LogInfoRow label="收件人">
                    <ObjectRef
                      id={selected.recipient_user_id}
                      name={selected.recipient_display_name || undefined}
                    />
                  </LogInfoRow>
                  <LogInfoRow label="次数">{String(selected.attempts)}</LogInfoRow>
                  {selected.project_id || selected.project_name ? (
                    <LogInfoRow label="项目">
                      <ObjectRef id={selected.project_id} name={selected.project_name || undefined} />
                    </LogInfoRow>
                  ) : null}
                  {selected.last_error ? (
                    <LogInfoRow label="错误">{selected.last_error}</LogInfoRow>
                  ) : null}
                </div>
              </LogDetailSection>
              {selected.status === "failed" ? (
                <LogDetailSection title="操作">
                  <Button
                    disabled={requeueMutation.isPending}
                    onClick={() => requeueMutation.mutate(selected.id)}
                    size="sm"
                    type="button"
                    variant="outline"
                  >
                    <RotateCcw data-icon="inline-start" />
                    重推
                  </Button>
                </LogDetailSection>
              ) : null}
            </LogDetailPanel>
          ) : undefined
        }
      />
    </>
  );
}

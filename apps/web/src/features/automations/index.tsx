import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CalendarClock, Plus } from "lucide-react";
import {
  MasterDetailLayout,
  MetricGrid,
  StatusPill,
  V3Button,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  V3MetricCard,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  WorkSurface,
  type V3Tone,
} from "@/components/superteam";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import { ApiRequestError } from "@/lib/api/client";
import {
  createAutomationRule,
  deleteAutomationRule,
  disableAutomationRule,
  enableAutomationRule,
  formatAutomationScheduleSummary,
  listAutomationFires,
  listAutomationRules,
  triggerAutomationRule,
  updateAutomationRule,
  type AutomationRule,
} from "@/lib/api/automations";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { statusLabel } from "@/lib/status-labels";
import { AutomationRuleDetail } from "./components/automation-rule-detail";
import { AutomationRuleFormSheet } from "./components/automation-rule-form-sheet";

function errorMessage(error: unknown): string {
  if (error instanceof ApiRequestError && error.detail) return error.detail;
  if (error instanceof Error) return error.message;
  return "操作失败";
}

function relativeTime(iso: string | undefined): string {
  if (!iso) return "尚未触发";
  const ts = Date.parse(iso);
  if (Number.isNaN(ts)) return "尚未触发";
  const delta = Date.now() - ts;
  const minutes = Math.round(delta / 60_000);
  if (Math.abs(minutes) < 1) return "刚刚";
  if (Math.abs(minutes) < 60) return `${minutes} 分钟前`;
  const hours = Math.round(minutes / 60);
  if (Math.abs(hours) < 24) return `${hours} 小时前`;
  const days = Math.round(hours / 24);
  return `${days} 天前`;
}

export function AutomationsPage() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const queryClient = useQueryClient();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [filter, setFilter] = useState<"all" | "enabled" | "disabled">("all");
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<AutomationRule | null>(null);
  const [pendingDelete, setPendingDelete] = useState<AutomationRule | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [formError, setFormError] = useState<string | null>(null);

  const rulesQuery = useQuery({
    queryKey: ["automation-rules", apiBaseUrl, filter],
    queryFn: () =>
      listAutomationRules(
        { baseUrl: apiBaseUrl },
        filter === "all" ? undefined : { enabled: filter === "enabled" },
      ),
  });

  const rows = rulesQuery.data?.items ?? [];
  const selected = rows.find((row) => row.id === selectedId) ?? null;

  const firesQuery = useQuery({
    queryKey: ["automation-fires", apiBaseUrl, selected?.id],
    queryFn: () => listAutomationFires({ baseUrl: apiBaseUrl }, selected!.id),
    enabled: Boolean(selected?.id),
  });

  const invalidate = async () => {
    await queryClient.invalidateQueries({ queryKey: ["automation-rules"] });
    await queryClient.invalidateQueries({ queryKey: ["automation-fires"] });
  };

  const createMutation = useMutation({
    mutationFn: (input: Parameters<typeof createAutomationRule>[1]) =>
      createAutomationRule({ baseUrl: apiBaseUrl }, input),
    onSuccess: async (rule) => {
      setFormOpen(false);
      setFormError(null);
      await invalidate();
      setSelectedId(rule.id);
    },
    onError: (error) => setFormError(errorMessage(error)),
  });

  const updateMutation = useMutation({
    mutationFn: ({
      ruleId,
      input,
    }: {
      ruleId: string;
      input: Parameters<typeof updateAutomationRule>[2];
    }) => updateAutomationRule({ baseUrl: apiBaseUrl }, ruleId, input),
    onSuccess: async () => {
      setFormOpen(false);
      setEditing(null);
      setFormError(null);
      await invalidate();
    },
    onError: (error) => setFormError(errorMessage(error)),
  });

  const enableMutation = useMutation({
    mutationFn: (ruleId: string) => enableAutomationRule({ baseUrl: apiBaseUrl }, ruleId),
    onSuccess: async () => {
      setActionError(null);
      await invalidate();
    },
    onError: (error) => setActionError(errorMessage(error)),
  });

  const disableMutation = useMutation({
    mutationFn: (ruleId: string) => disableAutomationRule({ baseUrl: apiBaseUrl }, ruleId),
    onSuccess: async () => {
      setActionError(null);
      await invalidate();
    },
    onError: (error) => setActionError(errorMessage(error)),
  });

  const triggerMutation = useMutation({
    mutationFn: (ruleId: string) => triggerAutomationRule({ baseUrl: apiBaseUrl }, ruleId),
    onSuccess: async () => {
      setActionError(null);
      await invalidate();
    },
    onError: (error) => setActionError(errorMessage(error)),
  });

  const deleteMutation = useMutation({
    mutationFn: (ruleId: string) => deleteAutomationRule({ baseUrl: apiBaseUrl }, ruleId),
    onSuccess: async () => {
      setPendingDelete(null);
      setSelectedId(null);
      await invalidate();
    },
    onError: (error) => setActionError(errorMessage(error)),
  });

  const metrics = useMemo(() => {
    const enabled = rows.filter((row) => row.enabled).length;
    const failedWindow = rows.filter(
      (row) => row.latest_fire?.status === "failed" || row.consecutive_failure_count > 0,
    ).length;
    return [
      { label: "规则总数", value: rows.length, tone: "info" as V3Tone },
      { label: "启用中", value: enabled, tone: "ok" as V3Tone },
      { label: "失败关注", value: failedWindow, tone: "warn" as V3Tone },
    ];
  }, [rows]);

  const busy =
    enableMutation.isPending ||
    disableMutation.isPending ||
    triggerMutation.isPending ||
    deleteMutation.isPending;

  const isInitialLoading = rulesQuery.isPending && rows.length === 0;
  const isBlockingError = rulesQuery.isError && rows.length === 0;

  return (
    <>
      <ShellPageHeader
        icon={<CalendarClock />}
        iconTone="brand"
        subtitle="按日程自动发起任务中枢需求或对话；人类闸门仍按项目逻辑处理（含飞书）"
        title="自动化任务"
      />
      <Main className="min-w-0 overflow-x-hidden" width="wide">
        <div className="flex min-w-0 flex-col gap-6">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="flex flex-wrap gap-2">
              {(
                [
                  ["all", "全部"],
                  ["enabled", "启用"],
                  ["disabled", "已禁用"],
                ] as const
              ).map(([value, label]) => (
                <V3Button
                  key={value}
                  size="sm"
                  type="button"
                  variant={filter === value ? "primary" : "secondary"}
                  onClick={() => setFilter(value)}
                >
                  {label}
                </V3Button>
              ))}
            </div>
            <V3Button
              className="h-11 px-5"
              onClick={() => {
                setEditing(null);
                setFormError(null);
                setFormOpen(true);
              }}
            >
              <Plus data-icon="inline-start" />
              新建规则
            </V3Button>
          </div>

          <MetricGrid aria-label="自动化规则指标">
            {metrics.map((metric) => (
              <V3MetricCard
                key={metric.label}
                icon={<CalendarClock />}
                iconTone={metric.tone}
                label={metric.label}
                value={metric.value}
              />
            ))}
          </MetricGrid>

          <MasterDetailLayout
            data-layout="table-governance"
            detail={
              selected ? (
                <AutomationRuleDetail
                  actionError={actionError}
                  busy={busy}
                  fires={firesQuery.data?.items ?? []}
                  firesLoading={firesQuery.isPending}
                  rule={selected}
                  onDelete={() => setPendingDelete(selected)}
                  onDisable={() => disableMutation.mutate(selected.id)}
                  onEdit={() => {
                    setEditing(selected);
                    setFormError(null);
                    setFormOpen(true);
                  }}
                  onEnable={() => enableMutation.mutate(selected.id)}
                  onTrigger={() => triggerMutation.mutate(selected.id)}
                />
              ) : undefined
            }
            master={
              <WorkSurface className="min-w-0">
                {isInitialLoading ? (
                  <V3LoadingState label="加载定时规则…" />
                ) : isBlockingError ? (
                  <V3ErrorState description="无法加载自动化规则" title="加载失败" />
                ) : rows.length === 0 ? (
                  <V3EmptyState
                    description="把例行需求做成定时规则。自动触发后验收仍可在 Console / 飞书处理。"
                    icon={<CalendarClock />}
                    title="还没有定时规则"
                  />
                ) : (
                  <V3Table>
                    <thead>
                      <tr>
                        <V3Th>名称 / 项目</V3Th>
                        <V3Th className="w-24">模式</V3Th>
                        <V3Th className="w-36">日程</V3Th>
                        <V3Th className="w-40">状态</V3Th>
                        <V3Th className="w-36">最近触发</V3Th>
                      </tr>
                    </thead>
                    <tbody>
                      {rows.map((row) => {
                        const selectedRow = row.id === selectedId;
                        const warn =
                          !row.enabled ||
                          row.latest_fire?.status === "failed" ||
                          row.consecutive_failure_count >= 3;
                        return (
                          <V3Tr
                            key={row.id}
                            aria-selected={selectedRow}
                            className="cursor-pointer"
                            tone={warn ? "warn" : undefined}
                            onClick={() => {
                              setActionError(null);
                              setSelectedId(row.id);
                            }}
                          >
                            <V3Td>
                              <div className="font-medium text-foreground">{row.name}</div>
                              <div className="truncate text-xs text-muted-foreground">
                                {row.project_name?.trim() || row.project_id}
                              </div>
                            </V3Td>
                            <V3Td>
                              <StatusPill tone="mute">
                                {statusLabel(row.coordination_mode)}
                              </StatusPill>
                            </V3Td>
                            <V3Td className="text-sm tabular-nums">
                              {formatAutomationScheduleSummary(row)}
                            </V3Td>
                            <V3Td>
                              <StatusPill tone={row.enabled ? "ok" : "mute"}>
                                {row.enabled ? "启用中" : "已禁用"}
                              </StatusPill>
                              {!row.enabled && row.disabled_reason ? (
                                <div className="mt-1 text-xs text-muted-foreground">
                                  {statusLabel(row.disabled_reason)}
                                </div>
                              ) : null}
                            </V3Td>
                            <V3Td className="text-sm text-muted-foreground tabular-nums">
                              {row.latest_fire ? (
                                <>
                                  <span className="mr-1 inline-block size-1.5 rounded-full bg-current align-middle" />
                                  {statusLabel(row.latest_fire.status)} ·{" "}
                                  {relativeTime(row.latest_fire.scheduled_fire_at)}
                                </>
                              ) : (
                                "尚未触发"
                              )}
                            </V3Td>
                          </V3Tr>
                        );
                      })}
                    </tbody>
                  </V3Table>
                )}
              </WorkSurface>
            }
            narrowDetail="stack"
            rail="md"
          />
        </div>
      </Main>

      <AutomationRuleFormSheet
        apiBaseUrl={apiBaseUrl}
        editing={editing}
        error={formError}
        open={formOpen}
        submitting={createMutation.isPending || updateMutation.isPending}
        onCreate={(input) => createMutation.mutate(input)}
        onOpenChange={(open) => {
          setFormOpen(open);
          if (!open) setEditing(null);
        }}
        onUpdate={(ruleId, input) => updateMutation.mutate({ ruleId, input })}
      />

      <ConfirmDialog
        confirmText="删除"
        desc="删除后日程停止，历史触发记录一并清理。此操作不可撤销。"
        destructive
        open={pendingDelete !== null}
        title={`删除规则 ${pendingDelete?.name ?? ""}`}
        handleConfirm={() => {
          if (pendingDelete) deleteMutation.mutate(pendingDelete.id);
        }}
        onOpenChange={(open) => {
          if (!open) setPendingDelete(null);
        }}
      />
    </>
  );
}

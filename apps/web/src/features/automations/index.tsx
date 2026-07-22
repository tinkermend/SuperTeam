import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CalendarClock, Plus, Search } from "lucide-react";
import {
  MasterDetailLayout,
  StatusPill,
  V3Button,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
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
import { Input } from "@/components/ui/input";
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
  type AutomationCoordinationMode,
  type AutomationRule,
} from "@/lib/api/automations";
import { getInboxBadge, listInboxItems } from "@/lib/api/inbox";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { formatRelativeFuture, formatRelativeTime } from "@/lib/format-time";
import { statusLabel } from "@/lib/status-labels";
import { cn } from "@/lib/utils";
import { AutomationDashboardRail, countUpcomingFires } from "./components/automation-dashboard-rail";
import { AutomationFactStrip } from "./components/automation-fact-strip";
import { AutomationRuleDetail } from "./components/automation-rule-detail";
import { AutomationRuleFormSheet } from "./components/automation-rule-form-sheet";
import {
  AUTOMATION_SCENARIO_TEMPLATES,
  type AutomationRuleDraft,
} from "./scenario-templates";
import { buildNextFireById } from "./schedule-next";
import { automationFireTone } from "./fire-tone";

function errorMessage(error: unknown): string {
  if (error instanceof ApiRequestError && error.detail) return error.detail;
  if (error instanceof Error) return error.message;
  return "操作失败";
}

function healthLabel(rule: AutomationRule): { text: string; tone: V3Tone } {
  if (!rule.enabled) {
    return {
      text: rule.disabled_reason ? statusLabel(rule.disabled_reason) : "已禁用",
      tone:
        rule.disabled_reason === "consecutive_fire_failures" ||
        rule.disabled_reason === "actor_removed_from_project" ||
        rule.disabled_reason === "actor_deactivated"
          ? "warn"
          : "mute",
    };
  }
  if (rule.latest_fire?.status === "failed" || rule.consecutive_failure_count > 0) {
    return {
      text:
        rule.consecutive_failure_count > 0
          ? `连败 ${rule.consecutive_failure_count}`
          : "最近失败",
      tone: "warn",
    };
  }
  return { text: "启用中", tone: "ok" };
}

export function AutomationsPage() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const queryClient = useQueryClient();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [filter, setFilter] = useState<"all" | "enabled" | "disabled">("all");
  const [modeFilter, setModeFilter] = useState<"all" | AutomationCoordinationMode>("all");
  const [search, setSearch] = useState("");
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<AutomationRule | null>(null);
  const [draft, setDraft] = useState<AutomationRuleDraft | null>(null);
  const [pendingDelete, setPendingDelete] = useState<AutomationRule | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [formError, setFormError] = useState<string | null>(null);

  const rulesQuery = useQuery({
    queryKey: ["automation-rules", apiBaseUrl],
    queryFn: () => listAutomationRules({ baseUrl: apiBaseUrl }),
  });

  const inboxQuery = useQuery({
    queryKey: ["inbox", "automations-rail", apiBaseUrl],
    queryFn: () =>
      listInboxItems(
        { baseUrl: apiBaseUrl },
        { limit: 8, status: "open", view: "mine" },
      ),
  });

  const inboxBadgeQuery = useQuery({
    queryKey: ["inbox", "automations-badge", apiBaseUrl],
    queryFn: () => getInboxBadge({ baseUrl: apiBaseUrl }),
  });

  const allRows = rulesQuery.data?.items ?? [];
  const selected = allRows.find((row) => row.id === selectedId) ?? null;

  const nextFireById = useMemo(() => buildNextFireById(allRows), [allRows]);

  const rows = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return allRows.filter((row) => {
      if (filter === "enabled" && !row.enabled) return false;
      if (filter === "disabled" && row.enabled) return false;
      if (modeFilter !== "all" && row.coordination_mode !== modeFilter) return false;
      if (!needle) return true;
      const hay = `${row.name} ${row.project_name ?? ""} ${row.project_id}`.toLowerCase();
      return hay.includes(needle);
    });
  }, [allRows, filter, modeFilter, search]);

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
      setDraft(null);
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
      setDraft(null);
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

  const enabledCount = allRows.filter((row) => row.enabled).length;
  const dueSoonCount = useMemo(
    () => countUpcomingFires(allRows, nextFireById),
    [allRows, nextFireById],
  );
  const attentionCount = allRows.filter(
    (row) =>
      row.latest_fire?.status === "failed" ||
      row.consecutive_failure_count > 0 ||
      (!row.enabled &&
        (row.disabled_reason === "consecutive_fire_failures" ||
          row.disabled_reason === "actor_removed_from_project" ||
          row.disabled_reason === "actor_deactivated")),
  ).length;
  // Prefer list summary when loaded so heading count matches rail items;
  // badge is fallback while the items query is still pending.
  const pendingDecisionCount = inboxQuery.isSuccess
    ? inboxQuery.data.summary.open_count
    : (inboxBadgeQuery.data?.mine_open_count ?? 0);

  const busy =
    enableMutation.isPending ||
    disableMutation.isPending ||
    triggerMutation.isPending ||
    deleteMutation.isPending;

  const isInitialLoading = rulesQuery.isPending && allRows.length === 0;
  const isBlockingError = rulesQuery.isError && allRows.length === 0;

  function openCreate(nextDraft?: AutomationRuleDraft | null) {
    setEditing(null);
    setDraft(nextDraft ?? null);
    setFormError(null);
    setFormOpen(true);
  }

  const rail = selected ? (
    <AutomationRuleDetail
      actionError={actionError}
      busy={busy}
      fires={firesQuery.data?.items ?? []}
      firesLoading={firesQuery.isPending}
      nextFireIso={nextFireById.get(selected.id) ?? null}
      rule={selected}
      onClose={() => setSelectedId(null)}
      onDelete={() => setPendingDelete(selected)}
      onDisable={() => disableMutation.mutate(selected.id)}
      onEdit={() => {
        setEditing(selected);
        setDraft(null);
        setFormError(null);
        setFormOpen(true);
      }}
      onEnable={() => enableMutation.mutate(selected.id)}
      onTrigger={() => triggerMutation.mutate(selected.id)}
    />
  ) : (
    <AutomationDashboardRail
      decisionItems={inboxQuery.data?.items ?? []}
      decisionOpenCount={pendingDecisionCount}
      decisionsLoading={inboxQuery.isPending}
      nextFireById={nextFireById}
      rules={allRows}
      onSelectRule={(ruleId) => {
        setActionError(null);
        setSelectedId(ruleId);
      }}
    />
  );

  return (
    <>
      <ShellPageHeader
        icon={<CalendarClock />}
        iconTone="brand"
        subtitle="定时发起任务中枢需求或对话；人类闸门仍按项目逻辑处理（含飞书）"
        title="自动化任务"
      />
      <Main className="min-w-0 overflow-x-hidden" width="wide">
        <div className="flex min-w-0 flex-col gap-5">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="max-w-2xl text-sm text-v3-ink-2">
              把例行需求做成日程规则。自动的是触发，不是跳过验收。
            </p>
            <V3Button
              className="h-11 px-5"
              onClick={() => openCreate()}
            >
              <Plus data-icon="inline-start" />
              新建规则
            </V3Button>
          </div>

          <AutomationFactStrip
            attentionCount={attentionCount}
            dueSoonCount={dueSoonCount}
            enabledCount={enabledCount}
            pendingDecisionCount={pendingDecisionCount}
          />

          <MasterDetailLayout
            data-layout="table-governance"
            detail={rail}
            master={
              <WorkSurface className="min-w-0" data-density="compact">
                <div className="flex min-w-0 flex-col gap-3 border-b border-v3-line bg-v3-card px-3 py-3">
                  <div className="flex min-w-0 flex-col gap-2 lg:flex-row lg:items-center lg:justify-between">
                    <div className="min-w-0">
                      <h2 className="text-sm font-extrabold text-v3-ink">定时规则</h2>
                      <p className="text-[12px] text-v3-ink-3">
                        {rows.length} / {allRows.length} 条 · 点击行查看详情与触发历史
                      </p>
                    </div>
                    <div className="relative w-full max-w-xs">
                      <Search
                        aria-hidden
                        className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-v3-ink-3"
                      />
                      <Input
                        aria-label="搜索规则或项目"
                        className="h-9 pl-8"
                        placeholder="搜索名称 / 项目"
                        value={search}
                        onChange={(e) => setSearch(e.target.value)}
                      />
                    </div>
                  </div>
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
                    <span aria-hidden className="mx-1 h-6 w-px bg-v3-line" />
                    {(
                      [
                        ["all", "所有模式"],
                        ["loop", "循环"],
                        ["plan", "计划"],
                        ["chat", "对话"],
                      ] as const
                    ).map(([value, label]) => (
                      <V3Button
                        key={value}
                        size="sm"
                        type="button"
                        variant={modeFilter === value ? "primary" : "secondary"}
                        onClick={() => setModeFilter(value)}
                      >
                        {label}
                      </V3Button>
                    ))}
                  </div>
                </div>

                {isInitialLoading ? (
                  <V3LoadingState label="加载定时规则…" />
                ) : isBlockingError ? (
                  <V3ErrorState description="无法加载自动化规则" title="加载失败" />
                ) : allRows.length === 0 ? (
                  <div className="space-y-4 p-6">
                    <V3EmptyState
                      description="把例行需求做成定时规则。自动触发后验收仍可在 Console / 飞书处理。"
                      icon={<CalendarClock />}
                      title="还没有定时规则"
                    />
                    <div className="grid gap-3 sm:grid-cols-3">
                      {AUTOMATION_SCENARIO_TEMPLATES.map((template) => (
                        <button
                          key={template.id}
                          className="rounded-[14px] border border-v3-line bg-v3-soft/60 p-4 text-left transition-colors hover:border-v3-brand/40 hover:bg-v3-brand-soft/40"
                          type="button"
                          onClick={() => openCreate(template.draft)}
                        >
                          <div className="text-sm font-semibold text-v3-ink">{template.title}</div>
                          <p className="mt-1.5 text-[12px] leading-5 text-v3-ink-3">
                            {template.description}
                          </p>
                          <span className="mt-3 inline-block text-[12px] font-medium text-v3-brand">
                            用此模板新建
                          </span>
                        </button>
                      ))}
                    </div>
                  </div>
                ) : rows.length === 0 ? (
                  <V3EmptyState
                    description="试试清空搜索或切换筛选条件。"
                    title="没有匹配的规则"
                  />
                ) : (
                  <V3Table>
                    <thead>
                      <tr>
                        <V3Th>名称 / 项目</V3Th>
                        <V3Th className="w-20">模式</V3Th>
                        <V3Th className="w-32">日程</V3Th>
                        <V3Th className="w-32">下次触发</V3Th>
                        <V3Th className="w-36">健康</V3Th>
                        <V3Th className="w-36">最近触发</V3Th>
                      </tr>
                    </thead>
                    <tbody>
                      {rows.map((row) => {
                        const selectedRow = row.id === selectedId;
                        const nextIso = nextFireById.get(row.id) ?? null;
                        const health = healthLabel(row);
                        const fireTone = row.latest_fire
                          ? automationFireTone(row.latest_fire.status)
                          : null;
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
                            <V3Td className="text-sm text-muted-foreground tabular-nums">
                              {nextIso ? (
                                <time dateTime={nextIso}>{formatRelativeFuture(nextIso)}</time>
                              ) : (
                                "—"
                              )}
                            </V3Td>
                            <V3Td>
                              <StatusPill tone={health.tone}>{health.text}</StatusPill>
                            </V3Td>
                            <V3Td className="text-sm text-muted-foreground">
                              {row.latest_fire && fireTone ? (
                                <span className="inline-flex items-center gap-1.5">
                                  <span
                                    aria-hidden
                                    className={cn(
                                      "size-1.5 rounded-full",
                                      fireTone === "ok" && "bg-v3-ok",
                                      fireTone === "danger" && "bg-v3-danger",
                                      fireTone === "warn" && "bg-v3-warn",
                                      fireTone === "info" && "bg-v3-info",
                                      fireTone === "mute" && "bg-v3-mute",
                                    )}
                                  />
                                  <span className="tabular-nums">
                                    {statusLabel(row.latest_fire.status)} ·{" "}
                                    {formatRelativeTime(row.latest_fire.scheduled_fire_at)}
                                  </span>
                                </span>
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
            onDetailDismiss={() => setSelectedId(null)}
            rail="lg"
          />
        </div>
      </Main>

      <AutomationRuleFormSheet
        apiBaseUrl={apiBaseUrl}
        draft={draft}
        editing={editing}
        error={formError}
        open={formOpen}
        submitting={createMutation.isPending || updateMutation.isPending}
        onCreate={(input) => createMutation.mutate(input)}
        onOpenChange={(open) => {
          setFormOpen(open);
          if (!open) {
            setEditing(null);
            setDraft(null);
          }
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

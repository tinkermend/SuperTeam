import { useEffect, useMemo, useState } from "react";
import { Link } from "@tanstack/react-router";
import {
  AlertTriangle,
  ArrowUpRight,
  CheckCircle2,
  Clock,
  FileQuestion,
  FileText,
  Inbox,
  ListChecks,
  RotateCcw,
  Route as RouteIcon,
  ShieldCheck,
  XCircle,
} from "lucide-react";
import {
  SoftCard,
  StatusPill,
  V3Button,
  V3MetricCard,
  V3StateSurface,
  V3Tabs,
  V3Tab,
} from "@/components/superteam";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import type {
  InboxAction,
  InboxItem,
  InboxItemType,
  InboxListFilters,
  InboxListResponse,
  InboxStatus,
  InboxViewMode,
} from "@/lib/api/inbox";
import { cn } from "@/lib/utils";
import { formatInboxActionLabel } from "./action-format";
import {
  formatContext,
  formatCurrentNode,
  formatDateTime,
  formatItemType,
  formatSourceType,
  InboxItemList,
  resolveInboxHref,
  riskLabel,
  riskTone,
} from "./inbox-item-list";

export type InboxFilterKey = "status" | "item_type" | "risk_level" | "project_id" | "target_user_id";
export type InboxUuidFilterKey = Extract<InboxFilterKey, "project_id" | "target_user_id">;
export type InboxUuidFilterDrafts = Record<InboxUuidFilterKey, string>;
export type InboxUuidFilterErrors = Partial<Record<InboxUuidFilterKey, string | undefined>>;
export type InboxFilterChangeValue<Key extends InboxFilterKey> = {
  item_type: InboxItemType | "all";
  project_id: string;
  risk_level: string;
  status: InboxStatus;
  target_user_id: string;
}[Key];
type InboxFilterChangeHandler = <Key extends InboxFilterKey>(
  key: Key,
  value: InboxFilterChangeValue<Key>,
) => void;

type InboxShellProps = {
  data?: InboxListResponse;
  error: Error | null;
  filters: InboxListFilters;
  isFetching: boolean;
  isLoading: boolean;
  mutationError: Error | null;
  onAction: (item: InboxItem, action: InboxAction) => void;
  onFilterChange: InboxFilterChangeHandler;
  onRetry: () => void;
  onResetFilters: () => void;
  onViewChange: (view: InboxViewMode) => void;
  uuidFilterDrafts: InboxUuidFilterDrafts;
  uuidFilterErrors: InboxUuidFilterErrors;
  view: InboxViewMode;
};

export function InboxShell({
  data,
  error,
  filters,
  isFetching,
  isLoading,
  mutationError,
  onAction,
  onFilterChange,
  onRetry,
  onResetFilters,
  onViewChange,
  uuidFilterDrafts,
  uuidFilterErrors,
  view,
}: InboxShellProps) {
  const [selectedItemId, setSelectedItemId] = useState<string | null>(null);
  const hasItems = Boolean(data?.items.length);
  const selectedItem = useMemo(() => {
    return data?.items.find((item) => item.id === selectedItemId) ?? null;
  }, [data?.items, selectedItemId]);

  useEffect(() => {
    if (selectedItemId && data && !data.items.some((item) => item.id === selectedItemId)) {
      setSelectedItemId(null);
    }
  }, [data, selectedItemId]);

  return (
    <>
      <ShellPageHeader
        title="收件箱"
        subtitle="需要你处理、确认或继续追踪的事项。"
        icon={<Inbox />}
        iconTone="brand"
      />
      <Main className="space-y-5 text-v3-ink">
        {data ? (
          <div className="grid gap-4 md:grid-cols-3">
            <V3MetricCard
              icon={<Inbox />}
              iconTone="info"
              label="开放事项"
              value={data.summary.open_count}
              meta={view === "mine" ? "我的待处理队列" : "团队待处理队列"}
            />
            <V3MetricCard
              icon={<AlertTriangle />}
              iconTone="danger"
              label="高风险"
              value={data.summary.high_risk_count}
              meta="需优先确认"
            />
            <V3MetricCard
              icon={<ShieldCheck />}
              iconTone="warn"
              label="阻断"
              value={data.summary.blocked_count}
              meta="等待人工判断"
            />
          </div>
        ) : null}

        <SoftCard className="flex flex-col gap-4 p-4">
          <V3Tabs role="tablist" aria-label="收件箱视图">
            <V3Tab
              type="button"
              role="tab"
              active={view === "mine"}
              aria-selected={view === "mine"}
              onClick={() => onViewChange("mine")}
            >
              我的待办
            </V3Tab>
            <V3Tab
              type="button"
              role="tab"
              active={view === "team"}
              aria-selected={view === "team"}
              onClick={() => onViewChange("team")}
            >
              团队待办
            </V3Tab>
          </V3Tabs>
          <InboxFilters
            filters={filters}
            onFilterChange={onFilterChange}
            onReset={onResetFilters}
            uuidFilterDrafts={uuidFilterDrafts}
            uuidFilterErrors={uuidFilterErrors}
          />
        </SoftCard>

        {mutationError ? (
          <div className="rounded-v3-inner bg-v3-danger-soft p-4 text-sm text-v3-danger" role="alert">
            <p className="font-bold">操作未完成</p>
            <p className="mt-1 text-v3-ink-2">{mutationError.message}</p>
          </div>
        ) : null}

        {isFetching && hasItems ? (
          <StatusPill tone="info" className="w-fit">正在刷新</StatusPill>
        ) : null}

        <V3StateSurface
          isLoading={isLoading && !data}
          isError={Boolean(error && !data)}
          error={error}
          empty={Boolean(data && !hasItems)}
          onRetry={onRetry}
          emptyState={
            <SoftCard>
              <div className="px-6 py-12 text-center text-sm text-v3-ink-2">
                当前没有需要处理的事项。
              </div>
            </SoftCard>
          }
        >
          {data && hasItems ? (
            <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_24rem] 2xl:grid-cols-[minmax(0,1fr)_26rem]">
              <InboxItemList
                items={data.items}
                onSelect={(item) => setSelectedItemId(item.id)}
                selectedItemId={selectedItemId}
              />
              <InboxDecisionPanel
                data={data}
                item={selectedItem}
                onAction={onAction}
                view={view}
              />
            </div>
          ) : null}
        </V3StateSurface>
      </Main>
    </>
  );
}

type InboxDecisionPanelProps = {
  data: InboxListResponse;
  item: InboxItem | null;
  onAction: (item: InboxItem, action: InboxAction) => void;
  view: InboxViewMode;
};

const actionToneVariant: Record<string, "primary" | "outline" | "danger"> = {
  danger: "danger",
  destructive: "danger",
  positive: "primary",
  primary: "primary",
  success: "outline",
  warning: "outline",
};

const actionToneClass: Record<string, string> = {
  primary: "",
  positive: "",
  success: "border-v3-ok text-v3-ok hover:bg-v3-ok-soft",
  warning: "border-v3-warn text-v3-warn hover:bg-v3-warn-soft",
};

function InboxDecisionPanel({ data, item, onAction, view }: InboxDecisionPanelProps) {
  if (!item) {
    return <InboxEmptyDetailPanel data={data} />;
  }

  const actions = Array.isArray(item.actions) ? item.actions : [];
  const detailHref = resolveInboxHref(item);

  return (
    <SoftCard className="overflow-hidden">
      <div className="border-b border-v3-line px-5 py-4">
        <div className="flex items-start gap-3">
          <div className="min-w-0 flex-1">
            <h2 className="text-lg font-extrabold text-v3-ink">{item.title}</h2>
            <div className="mt-2 flex flex-wrap gap-2">
              <StatusPill tone={item.item_type === "approval" ? "info" : "artifact"}>
                {formatItemType(item)}
              </StatusPill>
              {item.risk_level ? (
                <StatusPill tone={riskTone[item.risk_level] ?? "mute"}>
                  {riskLabel[item.risk_level] ?? item.risk_level}
                </StatusPill>
              ) : null}
              <StatusPill tone={view === "mine" ? "warn" : "mute"}>
                {view === "mine" ? "待我处理" : "团队只读"}
              </StatusPill>
            </div>
          </div>
        </div>
      </div>

      <section className="border-b border-v3-line px-5 py-4">
        <h3 className="text-[13px] font-extrabold text-v3-ink">为什么需要你处理</h3>
        <dl className="mt-3 grid grid-cols-[5.5rem_minmax(0,1fr)] gap-x-3 gap-y-2 text-[13px]">
          <dt className="font-semibold text-v3-ink-3">关联对象</dt>
          <dd className="min-w-0 font-semibold text-v3-ink">{formatContext(item) ?? item.source_id}</dd>
          <dt className="font-semibold text-v3-ink-3">当前节点</dt>
          <dd className="min-w-0 font-semibold text-v3-ink">{formatCurrentNode(item)}</dd>
          <dt className="font-semibold text-v3-ink-3">发起来源</dt>
          <dd className="min-w-0 font-semibold text-v3-ink">{formatSourceType(item)}</dd>
          <dt className="font-semibold text-v3-ink-3">更新时间</dt>
          <dd className="min-w-0 font-semibold text-v3-ink">{formatDateTime(item.last_activity_at)}</dd>
          {item.source_task_id ? (
            <>
              <dt className="font-semibold text-v3-ink-3">关联任务</dt>
              <dd className="min-w-0 truncate font-mono text-xs text-v3-ink-2">{item.source_task_id}</dd>
            </>
          ) : null}
        </dl>
        {item.summary ? (
          <p className="mt-3 rounded-v3-inner bg-v3-card-soft px-3 py-2 text-[13px] leading-5 text-v3-ink-2">
            {item.summary}
          </p>
        ) : null}
      </section>

      <section className="border-b border-v3-line bg-v3-card-soft px-5 py-4">
        {view === "mine" && actions.length > 0 ? (
          <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-1 2xl:grid-cols-2">
            {actions.map((action) => (
              <V3Button
                className={cn(actionToneClass[action.tone])}
                key={action.key}
                onClick={() => onAction(item, action)}
                type="button"
                variant={actionToneVariant[action.tone] ?? "outline"}
              >
                {formatInboxActionLabel(action)}
              </V3Button>
            ))}
          </div>
        ) : (
          <p className="text-sm font-semibold text-v3-ink-2">
            {view === "mine" ? "该事项暂无可执行动作。" : "团队视图仅用于查看上下文。"}
          </p>
        )}
      </section>

      <section className="grid gap-2 border-b border-v3-line px-5 py-4">
        <V3Button asChild variant="outline" className="justify-start">
          <Link to={detailHref}>
            <ArrowUpRight className="size-4" />
            查看完整详情
          </Link>
        </V3Button>
        <V3Button asChild variant="outline" className="justify-start">
          <Link to={resolveWorkflowInstanceHref(item)}>
            <RouteIcon className="size-4" />
            进入流程实例
          </Link>
        </V3Button>
        <V3Button asChild variant="outline" className="justify-start">
          <Link to={resolveWorkflowTemplateHref(item)}>
            <ListChecks className="size-4" />
            查看流程编排
          </Link>
        </V3Button>
      </section>

      <section className="border-b border-v3-line px-5 py-4">
        <h3 className="text-[13px] font-extrabold text-v3-ink">过程记录</h3>
        <div className="mt-3 space-y-3">
          {buildProcessRecords(item).map((record) => (
            <div className="flex gap-3" key={record.title}>
              <span
                className={cn(
                  "mt-0.5 grid size-5 shrink-0 place-items-center rounded-full",
                  record.current ? "bg-v3-brand-soft text-v3-brand" : "bg-v3-ok-soft text-v3-ok",
                )}
              >
                {record.current ? <Clock className="size-3.5" /> : <CheckCircle2 className="size-3.5" />}
              </span>
              <div className="min-w-0">
                <p className="text-[13px] font-bold text-v3-ink">{record.title}</p>
                <p className="text-xs text-v3-ink-3">{record.description}</p>
              </div>
            </div>
          ))}
        </div>
      </section>

      <section className="px-5 py-4">
        <h3 className="text-[13px] font-extrabold text-v3-ink">证据与上下文</h3>
        <div className="mt-3 grid gap-2">
          {buildEvidenceItems(item).map((evidence) => (
            <div
              className="flex min-w-0 items-center gap-2 rounded-v3-inner border border-v3-line bg-v3-card-soft px-3 py-2 text-[13px]"
              key={evidence.label}
            >
              <FileText className="size-4 shrink-0 text-v3-ink-3" />
              <span className="min-w-0 flex-1 truncate font-semibold text-v3-ink">{evidence.label}</span>
              <span className="shrink-0 text-xs text-v3-ink-3">{evidence.meta}</span>
            </div>
          ))}
        </div>
      </section>
    </SoftCard>
  );
}

function InboxEmptyDetailPanel({ data }: { data: InboxListResponse }) {
  return (
    <SoftCard className="overflow-hidden">
      <div className="border-b border-v3-line px-5 py-5">
        <h2 className="text-lg font-extrabold text-v3-ink">选择一条事项查看详情</h2>
        <p className="mt-2 text-[13px] leading-5 text-v3-ink-2">
          左侧列出所有需要你同意、审核、确认或验收的事项。选择任一事项后，这里会展示过程记录、证据、上下文和可执行动作。
        </p>
      </div>
      <section className="border-b border-v3-line px-5 py-4">
        <h3 className="text-[13px] font-extrabold text-v3-ink">今日待处理摘要</h3>
        <div className="mt-3 grid grid-cols-3 gap-2">
          <MiniSummary label="待我处理" value={data.summary.open_count} />
          <MiniSummary label="高风险" value={data.summary.high_risk_count} tone="danger" />
          <MiniSummary label="阻断" value={data.summary.blocked_count} tone="warn" />
        </div>
      </section>
      <section className="border-b border-v3-line px-5 py-4">
        <h3 className="text-[13px] font-extrabold text-v3-ink">处理顺序建议</h3>
        <ol className="mt-3 space-y-2 text-[13px] text-v3-ink-2">
          <li className="flex gap-2"><span className="font-bold text-v3-ink">1.</span>先处理临近超时事项。</li>
          <li className="flex gap-2"><span className="font-bold text-v3-ink">2.</span>再处理高风险审批与验收。</li>
          <li className="flex gap-2"><span className="font-bold text-v3-ink">3.</span>最后处理普通同意或复核事项。</li>
        </ol>
      </section>
      <section className="px-5 py-4">
        <h3 className="text-[13px] font-extrabold text-v3-ink">选择事项后可执行</h3>
        <div className="mt-3 grid gap-2 text-[13px] text-v3-ink-2">
          {["查看完整详情", "查看过程记录", "查看证据与上下文", "同意 / 驳回 / 要求补证", "进入流程实例 / 查看流程编排"].map((label) => (
            <div className="flex items-center gap-2" key={label}>
              <CheckCircle2 className="size-4 text-v3-ok" />
              <span>{label}</span>
            </div>
          ))}
        </div>
      </section>
    </SoftCard>
  );
}

function MiniSummary({
  label,
  tone = "info",
  value,
}: {
  label: string;
  tone?: "danger" | "info" | "warn";
  value: number;
}) {
  const valueToneClass = {
    danger: "text-v3-danger",
    info: "text-v3-brand",
    warn: "text-v3-warn",
  }[tone];

  return (
    <div className="rounded-v3-inner border border-v3-line bg-v3-card-soft px-3 py-2">
      <p className="text-[11px] font-bold text-v3-ink-2">{label}</p>
      <p className={cn("mt-1 text-2xl font-extrabold tabular-nums", valueToneClass)}>{value}</p>
    </div>
  );
}

function buildProcessRecords(item: InboxItem) {
  const context = formatContext(item) ?? formatSourceType(item);
  return [
    {
      description: "事项进入收件箱并等待人工处理。",
      title: `${formatSourceType(item)}创建`,
    },
    {
      description: context,
      title: "关联上下文已归档",
    },
    {
      description: "选择同意、驳回或要求补证后将推动流程进入下一节点。",
      current: true,
      title: "等待人类决策",
    },
  ];
}

function buildEvidenceItems(item: InboxItem) {
  const items = [
    {
      label: formatContext(item) ?? item.title,
      meta: "上下文",
    },
  ];

  if (item.source_task_id) {
    items.push({ label: `任务 ${item.source_task_id}`, meta: "任务" });
  }

  if (item.source_approval_request_id) {
    items.push({ label: `审批 ${item.source_approval_request_id}`, meta: "审批" });
  }

  items.push({ label: "操作将写入审计日志", meta: "审计" });
  return items;
}

function resolveWorkflowInstanceHref(item: InboxItem) {
  const route = readStringFromContext(item.context, [
    "workflow_instance_route",
    "workflow_run_route",
    "process_instance_route",
  ]);
  return isLocalPath(route) ? route : resolveInboxHref(item);
}

function resolveWorkflowTemplateHref(item: InboxItem) {
  const route = readStringFromContext(item.context, [
    "workflow_template_route",
    "workflow_route",
    "process_route",
  ]);
  return isLocalPath(route) ? route : "/workflows";
}

function readStringFromContext(context: Record<string, unknown>, keys: string[]) {
  for (const key of keys) {
    const value = context[key];
    if (typeof value === "string" && value.trim()) {
      return value.trim();
    }
  }
  return undefined;
}

function isLocalPath(value: string | undefined): value is string {
  return Boolean(value?.startsWith("/") && !value.startsWith("//") && !value.includes("\\"));
}

type InboxFiltersProps = {
  filters: InboxListFilters;
  onFilterChange: InboxFilterChangeHandler;
  onReset: () => void;
  uuidFilterDrafts: InboxUuidFilterDrafts;
  uuidFilterErrors: InboxUuidFilterErrors;
};

type SelectOption<Value extends string> = {
  label: string;
  value: Value;
};

const statusOptions = [
  { label: "开放", value: "open" },
  { label: "已处理", value: "resolved" },
  { label: "已取消", value: "cancelled" },
] satisfies Array<SelectOption<InboxStatus>>;

const itemTypeOptions = [
  { label: "全部类型", value: "all" },
  { label: "审批", value: "approval" },
  { label: "项目决策", value: "project_decision" },
] satisfies Array<SelectOption<InboxItemType | "all">>;

const riskOptions = [
  { label: "全部风险", value: "all" },
  { label: "阻断", value: "blocked" },
  { label: "高风险", value: "high" },
  { label: "中风险", value: "medium" },
  { label: "低风险", value: "low" },
] satisfies Array<SelectOption<string>>;

function InboxFilters({
  filters,
  onFilterChange,
  onReset,
  uuidFilterDrafts,
  uuidFilterErrors,
}: InboxFiltersProps) {
  const hasUuidFilterError = Boolean(
    uuidFilterErrors.project_id || uuidFilterErrors.target_user_id,
  );

  return (
    <div className="flex flex-col gap-3 xl:flex-row xl:items-end">
      <div className="flex flex-1 flex-col gap-2">
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
          <FilterSelect
            label="状态"
            options={statusOptions}
            value={filters.status ?? "open"}
            onValueChange={(value) => onFilterChange("status", value)}
          />
          <FilterSelect
            label="事项类型"
            options={itemTypeOptions}
            value={filters.item_type ?? "all"}
            onValueChange={(value) => onFilterChange("item_type", value)}
          />
          <FilterSelect
            label="风险等级"
            options={riskOptions}
            value={filters.risk_level ?? "all"}
            onValueChange={(value) => onFilterChange("risk_level", value)}
          />
          <FilterInput
            invalid={Boolean(uuidFilterErrors.project_id)}
            label="项目 ID"
            placeholder="精确匹配"
            value={uuidFilterDrafts.project_id}
            onValueChange={(value) => onFilterChange("project_id", value)}
          />
          <FilterInput
            invalid={Boolean(uuidFilterErrors.target_user_id)}
            label="目标用户 ID"
            placeholder="精确匹配"
            value={uuidFilterDrafts.target_user_id}
            onValueChange={(value) => onFilterChange("target_user_id", value)}
          />
        </div>
        {hasUuidFilterError ? (
          <p className="text-xs font-semibold text-v3-danger" role="alert">
            请输入有效 UUID
          </p>
        ) : null}
      </div>
      <V3Button
        className="shrink-0"
        onClick={onReset}
        type="button"
        variant="outline"
        size="sm"
      >
        <RotateCcw className="size-4" />
        重置筛选
      </V3Button>
    </div>
  );
}

type FilterSelectProps<Value extends string> = {
  label: string;
  onValueChange: (value: Value) => void;
  options: ReadonlyArray<SelectOption<Value>>;
  value: Value;
};

function FilterSelect<Value extends string>({
  label,
  onValueChange,
  options,
  value,
}: FilterSelectProps<Value>) {
  const selectId = `inbox-filter-${label}`;

  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <label className="text-[13px] font-semibold text-v3-ink-2" htmlFor={selectId}>
        {label}
      </label>
      <Select value={value} onValueChange={onValueChange}>
        <SelectTrigger
          id={selectId}
          aria-label={label}
          className="h-10 w-full rounded-xl border-v3-line bg-v3-card-soft text-v3-ink shadow-none"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {options.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </div>
  );
}

type FilterInputProps = {
  invalid?: boolean;
  label: string;
  onValueChange: (value: string) => void;
  placeholder: string;
  value: string;
};

function FilterInput({ invalid = false, label, onValueChange, placeholder, value }: FilterInputProps) {
  const inputId = `inbox-filter-${label}`;

  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <label className="text-[13px] font-semibold text-v3-ink-2" htmlFor={inputId}>
        {label}
      </label>
      <Input
        aria-invalid={invalid || undefined}
        id={inputId}
        className="h-10 rounded-xl border-v3-line bg-v3-card-soft text-v3-ink shadow-none placeholder:text-v3-ink-3 aria-invalid:border-v3-danger"
        onChange={(event) => onValueChange(event.target.value)}
        placeholder={placeholder}
        value={value}
      />
    </div>
  );
}

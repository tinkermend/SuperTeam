import { AlertTriangle, Inbox, RotateCcw, ShieldCheck } from "lucide-react";
import {
  SoftCard,
  StatusPill,
  V3Button,
  V3MetricCard,
  V3PageHeader,
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
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import type {
  InboxAction,
  InboxItem,
  InboxItemType,
  InboxListFilters,
  InboxListResponse,
  InboxStatus,
  InboxViewMode,
} from "@/lib/api/inbox";
import { InboxItemList } from "./inbox-item-list";

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
  const hasItems = Boolean(data?.items.length);

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main className="space-y-5 text-v3-ink">
        <SoftCard className="p-5">
          <V3PageHeader
            title="收件箱"
            subtitle="需要你处理、确认或继续追踪的事项。"
            icon={<Inbox />}
            iconTone="brand"
          />
        </SoftCard>

        {data ? (
          <div className="grid gap-4 md:grid-cols-3">
            <V3MetricCard
              icon={<Inbox />}
              iconTone="info"
              label="开放事项"
              value={data.summary.open_count}
              meta={view === "mine" ? "我的待处理队列" : "团队待处理队列"}
              loud={data.summary.open_count > 0}
            />
            <V3MetricCard
              icon={<AlertTriangle />}
              iconTone="danger"
              label="高风险"
              value={data.summary.high_risk_count}
              meta="需优先确认"
              loud={data.summary.high_risk_count > 0}
            />
            <V3MetricCard
              icon={<ShieldCheck />}
              iconTone="warn"
              label="阻断"
              value={data.summary.blocked_count}
              meta="等待人工判断"
              loud={data.summary.blocked_count > 0}
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
            <InboxItemList items={data.items} onAction={onAction} view={view} />
          ) : null}
        </V3StateSurface>
      </Main>
    </>
  );
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

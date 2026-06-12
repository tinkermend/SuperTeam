import { Inbox, RotateCcw } from "lucide-react";
import {
  LiquidCard,
  LiquidTabsList,
  LiquidTabsTrigger,
  SemanticIconTile,
  StatusBadge,
} from "@/components/superteam";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs } from "@/components/ui/tabs";
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
      <Main>
        <div className="flex min-w-0 flex-col gap-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex min-w-0 items-center gap-3">
              <SemanticIconTile tone="decision" size="lg">
                <Inbox />
              </SemanticIconTile>
              <div className="min-w-0">
                <h1 className="text-2xl font-bold tracking-normal">收件箱</h1>
                <p className="text-sm text-muted-foreground">需要你处理的事项</p>
              </div>
            </div>
            {data ? (
              <div className="flex flex-wrap items-center gap-2">
                <StatusBadge tone="decision">开放 {data.summary.open_count}</StatusBadge>
                <StatusBadge tone="danger">高风险 {data.summary.high_risk_count}</StatusBadge>
                <StatusBadge tone="warning">阻断 {data.summary.blocked_count}</StatusBadge>
              </div>
            ) : null}
          </div>

          <Tabs value={view} onValueChange={(value) => onViewChange(value as InboxViewMode)}>
            <LiquidTabsList className="max-w-md">
              <LiquidTabsTrigger value="mine">我的待办</LiquidTabsTrigger>
              <LiquidTabsTrigger value="team">团队待办</LiquidTabsTrigger>
            </LiquidTabsList>
          </Tabs>

          <InboxFilters
            filters={filters}
            onFilterChange={onFilterChange}
            onReset={onResetFilters}
            uuidFilterDrafts={uuidFilterDrafts}
            uuidFilterErrors={uuidFilterErrors}
          />

          {mutationError ? (
            <Alert variant="destructive">
              <AlertTitle>操作未完成</AlertTitle>
              <AlertDescription>{mutationError.message}</AlertDescription>
            </Alert>
          ) : null}

          {error && !data ? (
            <Alert variant="destructive">
              <AlertTitle>收件箱加载失败</AlertTitle>
              <AlertDescription className="flex flex-wrap items-center gap-3">
                <span>{error.message}</span>
                <Button type="button" variant="outline" size="sm" onClick={onRetry}>
                  重试
                </Button>
              </AlertDescription>
            </Alert>
          ) : null}

          {isFetching && hasItems ? (
            <div className="text-xs text-muted-foreground">正在刷新</div>
          ) : null}

          {isLoading && !data ? (
            <LiquidCard>
              <CardContent className="py-8 text-sm text-muted-foreground">
                加载收件箱中
              </CardContent>
            </LiquidCard>
          ) : null}

          {data ? (
            hasItems ? (
              <InboxItemList items={data.items} onAction={onAction} view={view} />
            ) : (
              <LiquidCard>
                <CardContent className="py-8 text-sm text-muted-foreground">
                  当前没有需要处理的事项。
                </CardContent>
              </LiquidCard>
            )
          ) : null}
        </div>
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
    <div className="flex flex-col gap-3 rounded-lg border bg-card/60 p-3 shadow-sm backdrop-blur-sm xl:flex-row xl:items-end">
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
          <p className="text-xs text-destructive" role="alert">
            请输入有效 UUID
          </p>
        ) : null}
      </div>
      <Button
        className="h-9 shrink-0"
        onClick={onReset}
        type="button"
        variant="outline"
      >
        <RotateCcw className="size-4" />
        重置筛选
      </Button>
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
      <label className="text-sm font-medium text-foreground" htmlFor={selectId}>
        {label}
      </label>
      <Select value={value} onValueChange={onValueChange}>
        <SelectTrigger
          id={selectId}
          aria-label={label}
          className="h-9 w-full rounded-full bg-background/70 shadow-none"
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
      <label className="text-sm font-medium text-foreground" htmlFor={inputId}>
        {label}
      </label>
      <Input
        aria-invalid={invalid || undefined}
        id={inputId}
        className="h-9 rounded-full bg-background/70 shadow-none"
        onChange={(event) => onValueChange(event.target.value)}
        placeholder={placeholder}
        value={value}
      />
    </div>
  );
}

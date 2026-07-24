import { type ReactNode, useEffect, useState } from "react";
import { Button } from "@/components/superteam";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue
} from "@/components/ui/select";

export const LOG_PAGE_SIZE = 20;

export function formatLogDateTime(value?: string): string {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat("zh-CN", {
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    month: "2-digit",
    second: "2-digit",
    year: "numeric"
}).format(date);
}

export type LogSelectOption = { label: string; value: string };

export function LogFilterBar({ children }: { children: ReactNode }) {
  return (
    <div className="grid gap-3 border-b border-line px-5 py-4 md:grid-cols-2 xl:grid-cols-4">
      {children}
    </div>
  );
}

export function LogSelectFilter({
  id,
  label,
  onValueChange,
  options,
  placeholder = "全部",
  value
}: {
  id: string;
  label: string;
  onValueChange: (value: string | undefined) => void;
  options: LogSelectOption[];
  placeholder?: string;
  value?: string;
}) {
  return (
    <div className="flex min-w-0 flex-col gap-2">
      <Label htmlFor={id} className="text-xs font-semibold text-ink-2">
        {label}
      </Label>
      <Select
        value={value ?? "all"}
        onValueChange={(nextValue) => onValueChange(nextValue === "all" ? undefined : nextValue)}
      >
        <SelectTrigger id={id} className="w-full border-line-strong bg-card text-ink shadow-none">
          <SelectValue placeholder={placeholder} />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            <SelectItem value="all">{placeholder}</SelectItem>
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

/**
 * Free-text filter that commits to the server only on Enter or blur, so typing
 * does not fire a request per keystroke.
 */
export function LogTextFilter({
  id,
  label,
  onCommit,
  placeholder,
  value
}: {
  id: string;
  label: string;
  onCommit: (value: string | undefined) => void;
  placeholder?: string;
  value?: string;
}) {
  const [draft, setDraft] = useState(value ?? "");

  useEffect(() => {
    setDraft(value ?? "");
  }, [value]);

  const commit = () => {
    const trimmed = draft.trim();
    onCommit(trimmed === "" ? undefined : trimmed);
  };

  return (
    <div className="flex min-w-0 flex-col gap-2">
      <Label htmlFor={id} className="text-xs font-semibold text-ink-2">
        {label}
      </Label>
      <Input
        id={id}
        className="w-full border-line-strong bg-card text-ink shadow-none"
        placeholder={placeholder ?? "输入后回车筛选"}
        value={draft}
        onChange={(event) => setDraft(event.target.value)}
        onBlur={commit}
        onKeyDown={(event) => {
          if (event.key === "Enter") {
            commit();
          }
        }}
      />
    </div>
  );
}

export function LogPagination({
  isFetching,
  itemCount,
  offset,
  onOffsetChange,
  pageSize
}: {
  isFetching: boolean;
  itemCount: number;
  offset: number;
  onOffsetChange: (offset: number) => void;
  pageSize: number;
}) {
  const currentPage = Math.floor(offset / pageSize) + 1;
  const canPrev = offset > 0;
  const canNext = itemCount >= pageSize;

  return (
    <div className="flex items-center justify-between gap-3 border-t border-line px-5 py-3">
      <p className="text-xs text-ink-2 tabular-nums">
        第 {currentPage} 页 · 本页 {itemCount} 条{isFetching ? " · 加载中…" : ""}
      </p>
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          disabled={!canPrev || isFetching}
          onClick={() => onOffsetChange(Math.max(0, offset - pageSize))}
        >
          上一页
        </Button>
        <Button
          variant="outline"
          size="sm"
          disabled={!canNext || isFetching}
          onClick={() => onOffsetChange(offset + pageSize)}
        >
          下一页
        </Button>
      </div>
    </div>
  );
}

export function LogChips({
  onValueChange,
  options,
  value
}: {
  onValueChange: (value: string | undefined) => void;
  options: Array<{ label: string; value: string }>;
  value: string | undefined;
}) {
  const all = { label: "全部", value: "__all__" };
  const current = value ?? "__all__";

  return (
    <div className="flex flex-wrap gap-1.5 px-5 pt-4">
      {[all, ...options].map((opt) => (
        <button
          key={opt.value}
          type="button"
          onClick={() => onValueChange(opt.value === "__all__" ? undefined : opt.value)}
          className={[
            "rounded-full border px-3 py-1 text-xs font-medium transition-colors",
            current === opt.value
              ? "border-brand bg-brand-soft text-brand-deep"
              : "border-line bg-card text-ink-2 hover:border-line-strong hover:text-ink",
          ].join(" ")}
        >
          {opt.label}
        </button>
      ))}
    </div>
  );
}

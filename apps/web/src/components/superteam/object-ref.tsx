import { useEffect, useRef, useState } from "react";
import { Check, Copy } from "lucide-react";
import {
  missingObjectLabel,
  type MissingObjectKind,
} from "@/lib/status-labels";
import { cn } from "@/lib/utils";

/**
 * 业务对象指称（DESIGN.md「对象指称用名称，标识符只作补充」的组件落点）：
 * - 有名称时名称为主文本，UUID 降级为 mono 短 id chip，点击复制完整 id；
 * - 名称缺失时主文本为「未命名{类型} (短id)」（D3），chip 仍可复制全 id；
 * - 未传 kind 时类型回退为「对象」，业务列表应传 kind 或直接用 missingObjectLabel。
 */
export function ObjectRef({
  className,
  id,
  kind,
  name,
}: {
  className?: string;
  id?: string | null;
  /** 名缺失时用于 missingObjectLabel；业务列表建议必传 */
  kind?: MissingObjectKind;
  name?: string | null;
}) {
  const trimmedName = name?.trim();
  const trimmedId = id?.trim();

  if (!trimmedName && !trimmedId) {
    return null;
  }
  if (!trimmedId) {
    return <span className={cn("min-w-0 break-words", className)}>{trimmedName}</span>;
  }
  if (!trimmedName) {
    return (
      <span className={cn("inline-flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5", className)}>
        <span className="min-w-0 break-words">
          {missingObjectLabel(kind ?? "object", trimmedId)}
        </span>
        <ObjectIdChip id={trimmedId} />
      </span>
    );
  }
  return (
    <span className={cn("inline-flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5", className)}>
      <span className="min-w-0 break-words">{trimmedName}</span>
      <ObjectIdChip id={trimmedId} />
    </span>
  );
}

/** UUID / 复合 id 短显：取首段 + 省略号（无连字符时原样）。全站唯一 shortId 实现。 */
export function shortId(id: string): string {
  const trimmed = id.trim();
  if (!trimmed) {
    return trimmed;
  }
  const head = trimmed.split("-")[0] ?? trimmed;
  return head.length < trimmed.length ? `${head}…` : head;
}

/** mono 标识符 chip：默认短显，点击复制完整 id；full 时全量显示（仅技术详情区）。 */
export function ObjectIdChip({
  className,
  full = false,
  id,
}: {
  className?: string;
  full?: boolean;
  id: string;
}) {
  const [copied, setCopied] = useState(false);
  const resetTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(
    () => () => {
      if (resetTimer.current) {
        clearTimeout(resetTimer.current);
      }
    },
    [],
  );

  const copy = async (event: React.MouseEvent) => {
    event.stopPropagation();
    try {
      await navigator.clipboard.writeText(id);
      setCopied(true);
      if (resetTimer.current) {
        clearTimeout(resetTimer.current);
      }
      resetTimer.current = setTimeout(() => setCopied(false), 1500);
    } catch {
      // 剪贴板不可用（非安全上下文等）时静默降级，title 仍可读全量 id
    }
  };

  return (
    <button
      type="button"
      title={copied ? "已复制" : `点击复制：${id}`}
      aria-label={`复制标识符 ${id}`}
      onClick={copy}
      className={cn(
        "inline-flex min-w-0 shrink-0 items-center gap-1 rounded-md border border-line bg-card-soft px-1.5 py-0.5 font-mono text-[11px] leading-4 text-ink-3 transition-colors hover:border-line-strong hover:text-ink-2",
        full && "max-w-full shrink",
        className,
      )}
    >
      <span className={cn("min-w-0", full ? "break-all text-left" : "whitespace-nowrap")}>
        {full ? id : shortId(id)}
      </span>
      {copied ? (
        <Check aria-hidden className="size-3 shrink-0 text-ok" />
      ) : (
        <Copy aria-hidden className="size-3 shrink-0 opacity-70" />
      )}
    </button>
  );
}

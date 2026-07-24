import { useEffect, useRef, useState } from "react";
import { Check, Copy } from "lucide-react";
import { cn } from "@/lib/utils";

/**
 * 业务对象指称（DESIGN.md「对象指称用名称，标识符只作补充」的组件落点）：
 * - 有名称时名称为主文本，UUID 降级为 mono 短 id chip，点击复制完整 id；
 * - 名称缺失（来源已删除）时回退显示完整 id，同样可复制。
 */
export function ObjectRef({
  className,
  id,
  name
}: {
  className?: string;
  id?: string | null;
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
    return <ObjectIdChip className={className} id={trimmedId} full />;
  }
  return (
    <span className={cn("inline-flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5", className)}>
      <span className="min-w-0 break-words">{trimmedName}</span>
      <ObjectIdChip id={trimmedId} />
    </span>
  );
}

function shortId(id: string) {
  const head = id.split("-")[0] ?? id;
  return head.length < id.length ? `${head}…` : head;
}

/** mono 标识符 chip：默认短显，点击复制完整 id；full 时全量显示（无名称回退场景）。 */
export function ObjectIdChip({
  className,
  full = false,
  id
}: {
  className?: string;
  full?: boolean;
  id: string;
}) {
  const [copied, setCopied] = useState(false);
  const resetTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => () => {
    if (resetTimer.current) {
      clearTimeout(resetTimer.current);
    }
  }, []);

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

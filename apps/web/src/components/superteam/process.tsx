import {
  useEffect,
  useId,
  useRef,
  useState,
  type ChangeEvent,
  type ComponentProps,
  type DragEvent,
  type ReactNode,
} from "react";
import { Check, Copy, Upload } from "lucide-react";
import { cn } from "@/lib/utils";
import { type Tone } from "@/components/superteam/primitives";

/**
 * Soft-Flat Batch C · 流程与审计基础件
 *
 * Stepper / Timeline / Progress / ProgressRing / FileDropzone / CopyableMono
 * 规范：page-archetypes（向导）、data-display、surfaces（Signature 进度）、forms（上传）
 */

const toneSolidBg: Record<Tone, string> = {
  brand: "bg-brand",
  info: "bg-info",
  ok: "bg-ok",
  warn: "bg-warn",
  danger: "bg-danger",
  mute: "bg-mute",
  artifact: "bg-artifact",
};

const toneText: Record<Tone, string> = {
  brand: "text-brand",
  info: "text-info",
  ok: "text-ok",
  warn: "text-warn",
  danger: "text-danger",
  mute: "text-mute",
  artifact: "text-artifact",
};

const toneSoftBg: Record<Tone, string> = {
  brand: "bg-brand-soft",
  info: "bg-info-soft",
  ok: "bg-ok-soft",
  warn: "bg-warn-soft",
  danger: "bg-danger-soft",
  mute: "bg-mute-soft",
  artifact: "bg-artifact-soft",
};

// ─── Stepper ─────────────────────────────────────────────────────────────────

export type StepperStep = {
  id: string;
  label: ReactNode;
  description?: ReactNode;
  /** 显式禁用该步（不可点击回退） */
  disabled?: boolean;
};

export type StepperProps = {
  className?: string;
  steps: StepperStep[];
  /** 当前步骤 index（0-based） */
  current: number;
  /**
   * 点击已完成步骤时回调（index）。
   * 不传则步骤不可点（仅展示）；业务自行决定是否允许回退。
   */
  onStepChange?: (index: number) => void;
  orientation?: "horizontal" | "vertical";
};

/**
 * 向导步骤链。当前=品牌实心；已完成=品牌软底+勾；未到=中性。
 * 不改变步骤圆尺寸（稳定布局）。
 */
function Stepper({
  className,
  steps,
  current,
  onStepChange,
  orientation = "horizontal",
}: StepperProps) {
  const vertical = orientation === "vertical";
  return (
    <nav
      data-slot="stepper"
      data-orientation={orientation}
      aria-label="步骤"
      className={cn(
        vertical
          ? "flex flex-col gap-0"
          : "flex flex-wrap items-center gap-2 rounded-full border border-line bg-card px-3 py-2 shadow-sm sm:gap-3 sm:px-4",
        className,
      )}
    >
      <ol
        className={cn(
          vertical ? "flex flex-col gap-0" : "flex flex-wrap items-center gap-2 sm:gap-3",
        )}
      >
        {steps.map((step, index) => {
          const completed = index < current;
          const active = index === current;
          const upcoming = index > current;
          // 仅已完成步骤可点回退；当前步保持非 button，避免与页脚主 CTA 同名冲突
          const clickable =
            Boolean(onStepChange) && !step.disabled && completed;

          const marker = (
            <span
              data-slot="stepper-marker"
              className={cn(
                "inline-grid size-6 shrink-0 place-items-center rounded-full text-[11px] font-bold tabular-nums",
                active && "bg-brand text-white",
                completed && !active && "bg-brand-soft text-brand-deep",
                upcoming && "bg-card-soft text-ink-3",
              )}
            >
              {completed && !active ? (
                <Check aria-hidden className="size-3.5 stroke-[2.5]" />
              ) : (
                index + 1
              )}
            </span>
          );

          const label = (
            <span className="min-w-0">
              <span
                className={cn(
                  "block truncate text-[13px] font-semibold",
                  active || completed ? "text-ink" : "text-ink-3",
                )}
              >
                {step.label}
              </span>
              {step.description ? (
                <span className="mt-0.5 block truncate text-[11px] text-ink-3">
                  {step.description}
                </span>
              ) : null}
            </span>
          );

          const body = (
            <>
              {marker}
              {label}
            </>
          );

          return (
            <li
              key={step.id}
              data-slot="stepper-step"
              data-state={active ? "current" : completed ? "completed" : "upcoming"}
              className={cn(
                "flex items-center gap-2",
                vertical && "relative pb-6 last:pb-0",
              )}
              aria-current={active ? "step" : undefined}
            >
              {vertical && index < steps.length - 1 ? (
                <span
                  aria-hidden
                  className={cn(
                    "absolute start-[11px] top-7 bottom-1 w-px",
                    completed ? "bg-brand/40" : "bg-line",
                  )}
                />
              ) : null}
              {clickable ? (
                <button
                  type="button"
                  disabled={step.disabled}
                  aria-label={
                    typeof step.label === "string"
                      ? `回到步骤 ${index + 1}：${step.label}`
                      : `回到步骤 ${index + 1}`
                  }
                  onClick={() => onStepChange?.(index)}
                  className={cn(
                    "relative z-[1] flex min-w-0 items-center gap-2 rounded-lg text-start",
                    "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2",
                    step.disabled && "cursor-not-allowed opacity-50",
                  )}
                >
                  {body}
                </button>
              ) : (
                <div className="relative z-[1] flex min-w-0 items-center gap-2">{body}</div>
              )}
              {!vertical && index < steps.length - 1 ? (
                <span
                  aria-hidden
                  className={cn(
                    "mx-0.5 hidden h-px w-6 sm:block sm:w-8",
                    completed ? "bg-brand/45" : "bg-line",
                  )}
                />
              ) : null}
            </li>
          );
        })}
      </ol>
    </nav>
  );
}

// ─── Timeline ────────────────────────────────────────────────────────────────

export type TimelineItemData = {
  id: string;
  title: ReactNode;
  description?: ReactNode;
  meta?: ReactNode;
  time?: ReactNode;
  tone?: Tone;
  /** 自定义点内图标；默认实心圆点 */
  icon?: ReactNode;
  children?: ReactNode;
};

function Timeline({
  className,
  items,
  ...props
}: ComponentProps<"ol"> & { items?: TimelineItemData[] }) {
  const list = items;
  return (
    <ol
      data-slot="timeline"
      className={cn("relative flex flex-col gap-0", className)}
      {...props}
    >
      {list
        ? list.map((item, index) => (
            <TimelineItem
              key={item.id}
              {...item}
              isLast={index === list.length - 1}
            />
          ))
        : props.children}
    </ol>
  );
}

function TimelineItem({
  className,
  id: _id,
  title,
  description,
  meta,
  time,
  tone = "mute",
  icon,
  children,
  isLast,
  ...props
}: Omit<ComponentProps<"li">, "title" | "id"> &
  TimelineItemData & {
    isLast?: boolean;
  }) {
  return (
    <li
      data-slot="timeline-item"
      data-tone={tone}
      className={cn("relative flex gap-3 pb-5 last:pb-0", className)}
      {...props}
    >
      <div className="relative flex w-4 shrink-0 flex-col items-center">
        <span
          data-slot="timeline-marker"
          className={cn(
            "relative z-[1] mt-1 inline-grid size-3.5 place-items-center rounded-full border-2 border-card",
            icon ? cn("size-6 rounded-md", toneSoftBg[tone], toneText[tone]) : toneSolidBg[tone],
          )}
        >
          {icon ? <span className="[&_svg]:size-3.5">{icon}</span> : null}
        </span>
        {!isLast ? (
          <span
            aria-hidden
            className="absolute top-5 bottom-0 w-px bg-line"
          />
        ) : null}
      </div>
      <div className="min-w-0 flex-1 pt-0.5">
        <div className="flex min-w-0 flex-wrap items-baseline justify-between gap-x-3 gap-y-0.5">
          <p className="text-[13px] font-semibold text-ink">{title}</p>
          {time ? (
            <time className="shrink-0 text-[11px] tabular-nums text-ink-3">{time}</time>
          ) : null}
        </div>
        {description ? (
          <p className="mt-0.5 text-[12px] leading-5 text-ink-2">{description}</p>
        ) : null}
        {meta ? (
          <div className="mt-1 text-[11px] text-ink-3">{meta}</div>
        ) : null}
        {children ? <div className="mt-2 min-w-0">{children}</div> : null}
      </div>
    </li>
  );
}

// ─── Progress ────────────────────────────────────────────────────────────────

export type ProgressProps = {
  className?: string;
  /** 0–100；不传则不确定进度 */
  value?: number;
  tone?: Tone;
  label?: ReactNode;
  /** 在条右侧显示百分比 */
  showValue?: boolean;
};

function Progress({
  className,
  value,
  tone = "brand",
  label,
  showValue,
}: ProgressProps) {
  const indeterminate = value == null || Number.isNaN(value);
  const clamped = indeterminate ? 0 : Math.max(0, Math.min(100, value));
  return (
    <div data-slot="progress" className={cn("grid gap-1.5", className)}>
      {label || showValue ? (
        <div className="flex items-center justify-between gap-2 text-[12px]">
          {label ? <span className="text-ink-2">{label}</span> : <span />}
          {showValue && !indeterminate ? (
            <span className="tabular-nums text-ink-3">{Math.round(clamped)}%</span>
          ) : null}
        </div>
      ) : null}
      <div
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={indeterminate ? undefined : Math.round(clamped)}
        aria-label={typeof label === "string" ? label : "进度"}
        className="relative h-2 w-full overflow-hidden rounded-full bg-card-soft"
      >
        <div
          data-slot="progress-indicator"
          className={cn(
            "h-full rounded-full transition-[width] duration-300 ease-out",
            toneSolidBg[tone],
            indeterminate && "w-1/3 animate-pulse",
          )}
          style={indeterminate ? undefined : { width: `${clamped}%` }}
        />
      </div>
    </div>
  );
}

export type ProgressRingProps = {
  className?: string;
  /** 0–100 */
  value: number;
  size?: number;
  strokeWidth?: number;
  tone?: Tone;
  /** 环心内容；默认百分比 */
  children?: ReactNode;
  label?: string;
};

/** Signature / 概览用环形进度；大面积装饰勿滥用。 */
function ProgressRing({
  className,
  value,
  size = 72,
  strokeWidth = 6,
  tone = "brand",
  children,
  label = "进度",
}: ProgressRingProps) {
  const clamped = Math.max(0, Math.min(100, value));
  const r = (size - strokeWidth) / 2;
  const c = 2 * Math.PI * r;
  const offset = c - (clamped / 100) * c;
  const toneStroke: Record<Tone, string> = {
    brand: "stroke-brand",
    info: "stroke-info",
    ok: "stroke-ok",
    warn: "stroke-warn",
    danger: "stroke-danger",
    mute: "stroke-mute",
    artifact: "stroke-artifact",
  };
  return (
    <div
      data-slot="progress-ring"
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={Math.round(clamped)}
      className={cn("relative inline-grid place-items-center", className)}
      style={{ width: size, height: size }}
    >
      <svg width={size} height={size} className="-rotate-90" aria-hidden>
        <circle
          cx={size / 2}
          cy={size / 2}
          r={r}
          fill="none"
          strokeWidth={strokeWidth}
          className="stroke-card-soft"
        />
        <circle
          cx={size / 2}
          cy={size / 2}
          r={r}
          fill="none"
          strokeWidth={strokeWidth}
          strokeLinecap="round"
          strokeDasharray={c}
          strokeDashoffset={offset}
          className={cn("transition-[stroke-dashoffset] duration-500 ease-out", toneStroke[tone])}
        />
      </svg>
      <div className="absolute inset-0 grid place-items-center text-[13px] font-extrabold tabular-nums text-ink">
        {children ?? `${Math.round(clamped)}%`}
      </div>
    </div>
  );
}

// ─── FileDropzone ────────────────────────────────────────────────────────────

export type FileDropzoneProps = {
  className?: string;
  accept?: string;
  multiple?: boolean;
  disabled?: boolean;
  /** 当前已选文件（受控展示） */
  files?: File[];
  /** 选择/拖入后回调；multiple=false 时最多 1 个 */
  onFilesChange?: (files: File[]) => void;
  /** 清空 */
  onClear?: () => void;
  label?: ReactNode;
  description?: ReactNode;
  error?: ReactNode;
  /** 隐藏 file input 的 id；不传则自动生成 */
  inputId?: string;
};

function FileDropzone({
  className,
  accept,
  multiple = false,
  disabled,
  files,
  onFilesChange,
  onClear,
  label = "上传文件",
  description = "拖拽到此处，或点击选择",
  error,
  inputId,
}: FileDropzoneProps) {
  const autoId = useId();
  const id = inputId ?? autoId;
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragActive, setDragActive] = useState(false);
  const selected = files ?? [];

  const commit = (list: FileList | File[] | null) => {
    if (!list) return;
    const next = Array.from(list);
    const limited = multiple ? next : next.slice(0, 1);
    onFilesChange?.(limited);
  };

  const onInputChange = (event: ChangeEvent<HTMLInputElement>) => {
    commit(event.target.files);
    // 允许重复选择同一文件
    event.target.value = "";
  };

  const onDrop = (event: DragEvent<HTMLDivElement>) => {
    event.preventDefault();
    setDragActive(false);
    if (disabled) return;
    commit(event.dataTransfer.files);
  };

  return (
    <div
      data-slot="file-dropzone"
      data-drag-active={dragActive ? "true" : undefined}
      className={cn("grid gap-2", className)}
    >
      <div
        role="button"
        tabIndex={disabled ? -1 : 0}
        aria-disabled={disabled || undefined}
        aria-describedby={error ? `${id}-error` : undefined}
        onKeyDown={(event) => {
          if (disabled) return;
          if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            inputRef.current?.click();
          }
        }}
        onClick={() => {
          if (!disabled) inputRef.current?.click();
        }}
        onDragEnter={(event) => {
          event.preventDefault();
          if (!disabled) setDragActive(true);
        }}
        onDragOver={(event) => {
          event.preventDefault();
          if (!disabled) setDragActive(true);
        }}
        onDragLeave={() => setDragActive(false)}
        onDrop={onDrop}
        className={cn(
          "flex min-w-0 cursor-pointer flex-col items-center justify-center gap-2 rounded-inner border border-dashed px-4 py-8 text-center transition-colors",
          "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 focus-visible:ring-offset-card",
          dragActive
            ? "border-brand bg-brand-soft/50"
            : "border-line-strong/70 bg-card hover:border-brand/60 hover:bg-brand-soft/15",
          disabled && "cursor-not-allowed opacity-50 hover:border-line-strong/70 hover:bg-card",
          error && "border-danger/50 bg-danger-soft/30",
        )}
      >
        <input
          ref={inputRef}
          id={id}
          type="file"
          accept={accept}
          multiple={multiple}
          disabled={disabled}
          className="sr-only"
          tabIndex={-1}
          onChange={onInputChange}
        />
        <span
          className={cn(
            "inline-grid size-11 place-items-center rounded-[13px] border border-black/5 bg-brand-soft text-brand shadow-sm",
            error && "bg-danger-soft text-danger",
          )}
        >
          <Upload aria-hidden className="size-5" />
        </span>
        <div className="min-w-0">
          <p className="text-[14px] font-bold text-ink">{label}</p>
          <p className="mt-0.5 text-[12px] text-ink-2">{description}</p>
        </div>
        {selected.length > 0 ? (
          <ul className="mt-1 w-full max-w-md space-y-1 text-start">
            {selected.map((file) => (
              <li
                key={`${file.name}-${file.size}-${file.lastModified}`}
                className="truncate rounded-lg bg-card-soft px-2.5 py-1.5 text-[12px] font-medium text-ink"
              >
                {file.name}
                <span className="ms-2 text-ink-3">
                  {formatBytes(file.size)}
                </span>
              </li>
            ))}
          </ul>
        ) : null}
      </div>
      {selected.length > 0 && onClear ? (
        <div className="flex justify-end">
          <button
            type="button"
            disabled={disabled}
            onClick={onClear}
            className="text-[12px] font-semibold text-ink-3 hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
          >
            清除
          </button>
        </div>
      ) : null}
      {error ? (
        <p id={`${id}-error`} role="alert" className="text-xs text-danger">
          {error}
        </p>
      ) : null}
    </div>
  );
}

function formatBytes(size: number): string {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}

// ─── CopyableMono ────────────────────────────────────────────────────────────

export type CopyableMonoProps = {
  className?: string;
  value: string;
  /** 展示文本；默认 value */
  display?: string;
  /** 截断展示（中间/尾部由 CSS truncate）；点按复制完整 value */
  truncate?: boolean;
  /** 可访问名；默认「复制 …」 */
  label?: string;
};

/**
 * 等宽可复制文本（路径、ID、hash）。
 * 业务对象名称+短 id 优先用 ObjectRef / ObjectIdChip。
 */
function CopyableMono({
  className,
  value,
  display,
  truncate = true,
  label,
}: CopyableMonoProps) {
  const [copied, setCopied] = useState(false);
  const resetTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const shown = display ?? value;

  useEffect(
    () => () => {
      if (resetTimer.current) clearTimeout(resetTimer.current);
    },
    [],
  );

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      if (resetTimer.current) clearTimeout(resetTimer.current);
      resetTimer.current = setTimeout(() => setCopied(false), 1500);
    } catch {
      // 非安全上下文等：title 仍可读全文
    }
  };

  return (
    <button
      type="button"
      data-slot="copyable-mono"
      title={copied ? "已复制" : `点击复制：${value}`}
      aria-label={label ?? `复制 ${value}`}
      onClick={(event) => {
        event.stopPropagation();
        void copy();
      }}
      className={cn(
        "inline-flex max-w-full min-w-0 items-center gap-1 rounded-md border border-line bg-card-soft px-1.5 py-0.5 font-mono text-[11px] leading-4 text-ink-3 transition-colors hover:border-line-strong hover:text-ink-2",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-1",
        className,
      )}
    >
      <span
        className={cn(
          "min-w-0 text-start",
          truncate ? "truncate" : "break-all whitespace-normal",
        )}
      >
        {shown}
      </span>
      {copied ? (
        <Check aria-hidden className="size-3 shrink-0 text-ok" />
      ) : (
        <Copy aria-hidden className="size-3 shrink-0 opacity-70" />
      )}
    </button>
  );
}

export {
  Stepper,
  Timeline,
  TimelineItem,
  Progress,
  ProgressRing,
  FileDropzone,
  CopyableMono,
};

/*
 * ── 迁移边界（Batch C）──────────────────────────────────────────────────────
 *
 * Stepper
 *   - 创建向导（团队/员工/技能）触达时替换手写 step 圆点链。
 *   - 步骤状态与校验仍由页面持有；Stepper 只负责展示与可选回退点击。
 *
 * Timeline vs RunEventTimeline
 *   - RunEventTimeline 继续承载 run 事件归并业务；外壳可逐步改为 Timeline/TimelineItem。
 *   - 配置修订史、审计流水等新面优先用 Timeline。
 *
 * Progress / ProgressRing
 *   - 线形：列表行、上传、任务占比；环形：SignatureCard / 概览锚点，每屏克制。
 *
 * FileDropzone
 *   - 技能 zip / 附件上传新面优先；skills/upload 触达再迁，可保留 SignatureCard 外壳包 FileDropzone。
 *
 * CopyableMono vs ObjectIdChip
 *   - 对象名称+id：ObjectRef / ObjectIdChip。
 *   - 纯路径、hash、request id、日志标识：CopyableMono。
 */

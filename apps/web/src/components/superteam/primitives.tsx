import { forwardRef, type ComponentProps, type ReactNode } from "react";
import { Slot } from "@radix-ui/react-slot";
import { buttonVariants } from "@/components/superteam/button-variants";
import { Search } from "lucide-react";
import { cn } from "@/lib/utils";

/**
 * SuperTeam v3 · Soft-Flat 组件族
 *
 * 设计基线见 DESIGN.md（容器选择规则）与 docs/design-system/{tokens,surfaces,data-display}.md。
 * 全部基于 theme.css 的 Soft-Flat token（Tailwind：`text-ink` / `bg-brand` / `rounded-card` / `shadow-card` 等）。
 * 一套语言两种容器：SoftCard 系列承载概览与外壳；WorkSurface + DataTable 承载密集数据本体。
 */

export type Tone = "brand" | "info" | "ok" | "warn" | "danger" | "mute" | "artifact";

const toneText: Record<Tone, string> = {
  brand: "text-brand",
  info: "text-info",
  ok: "text-ok",
  warn: "text-warn",
  danger: "text-danger",
  mute: "text-mute",
  artifact: "text-artifact"
};

const toneSoftBg: Record<Tone, string> = {
  brand: "bg-brand-soft",
  info: "bg-info-soft",
  ok: "bg-ok-soft",
  warn: "bg-warn-soft",
  danger: "bg-danger-soft",
  mute: "bg-mute-soft",
  artifact: "bg-artifact-soft"
};

/** soft 底上的文字层（--*-text，比 solid 深两档，≥4.5:1）。 */
const toneTextStrong: Record<Tone, string> = {
  brand: "text-brand-deep",
  info: "text-info-text",
  ok: "text-ok-text",
  warn: "text-warn-text",
  danger: "text-danger-text",
  mute: "text-mute-text",
  artifact: "text-artifact-text"
};

const toneSolidBg: Record<Tone, string> = {
  brand: "bg-brand",
  info: "bg-info",
  ok: "bg-ok",
  warn: "bg-warn",
  danger: "bg-danger",
  mute: "bg-mute",
  artifact: "bg-artifact"
};

/** 柔和白卡：页面外壳、概览卡、面板、表格容器外壳。 */
function SoftCard({
  className,
  interactive,
  ...props
}: ComponentProps<"div"> & { interactive?: boolean }) {
  return (
    <div
      data-slot="soft-card"
      className={cn(
        "rounded-card bg-card text-ink shadow-sm border border-line",
        interactive &&
          "cursor-pointer transition-all duration-300 ease-out hover:-translate-y-1 hover:shadow-md hover:border-line-strong active:scale-[0.99]",
        className,
      )}
      {...props}
    />
  );
}

/**
 * Tier A 玻璃卡（Glass Card）：沉浸极光玻璃外壳，仅用于 DESIGN.md 定义的 Tier A
 * 入口/创建画布（任务发起、技能上传、员工创建、登录/onboarding）。样式单一来源为
 * index.css 的 `.glass`（取自 --aurora-* token），禁止在 Tier B/C 数据/审计面使用。
 * 玻璃壳装实底内核：内部表单输入、逐行数据仍用实底 SoftCard/WorkSurface，不给每行套玻璃。
 */
function GlassCard({ className, ...props }: ComponentProps<"div">) {
  return <div data-slot="glass-card" className={cn("glass", className)} {...props} />;
}

/** squircle 语义图标芯片：实底柔色背景 + 同色图标。 */
function IconTile({
  className,
  tone = "brand",
  size = "default",
  ...props
}: ComponentProps<"span"> & { tone?: Tone; size?: "sm" | "default" | "lg" }) {
  const sizeClass =
    size === "sm"
      ? "size-9 rounded-[11px] [&_svg]:size-4"
      : size === "lg"
        ? "size-12 rounded-[15px] [&_svg]:size-6"
        : "size-11 rounded-[13px] [&_svg]:size-5";
  return (
    <span
      data-slot="icon-tile"
      className={cn(
        "inline-grid shrink-0 place-items-center border border-black/5 dark:border-white/5 shadow-sm shadow-black/5 dark:shadow-black/10",
        sizeClass,
        toneText[tone],
        toneSoftBg[tone],
        className,
      )}
      {...props}
    />
  );
}

/** 状态胶囊：文字用 text 层（过 AA），圆点用 solid。tone 表达紧迫度/状态，类别请用图标+文字。 */
function StatusPill({
  className,
  tone = "mute",
  showDot = true,
  children,
  ...props
}: ComponentProps<"span"> & { tone?: Tone; showDot?: boolean }) {
  return (
    <span
      data-slot="status-pill"
      data-tone={tone}
      className={cn(
        "inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1 text-xs font-semibold whitespace-nowrap",
        toneTextStrong[tone],
        toneSoftBg[tone],
        className,
      )}
      {...props}
    >
      {showDot ? <span aria-hidden className={cn("size-1.5 rounded-full", toneSolidBg[tone])} /> : null}
      {children}
    </span>
  );
}

/**
 * tone → 顶部色条渐变（对齐 TrendStatCard 的 accentGradient 语言）。
 * loud 时强制用 warn 渐变；mute 保持低调双灰。
 */
const toneAccentGradient: Record<Tone, string> = {
  brand:    "from-brand    to-brand/40",
  info:     "from-info     to-info/40",
  ok:       "from-ok       to-ok/40",
  warn:     "from-warn     to-warn/40",
  danger:   "from-danger   to-danger/40",
  mute:     "from-mute/50  to-mute/10",
  artifact: "from-artifact to-artifact/40"
};

/**
 * 概览指标卡。动效对齐数字员工 TrendStatCard：
 * shadow-card 基础阴影 → hover shadow-pop（可感知的深度变化）+ -translate-y-0.5 上浮 + 200ms。
 * 顶部 3px 色条作为视觉锚，与员工卡片保持同一设计语言。
 * loud=需人介入时，色条和数值切换为 warn 色。
 */
function MetricCard({
  className,
  icon,
  iconTone = "brand",
  label,
  value,
  meta,
  loud,
  action
}: {
  className?: string;
  icon?: ReactNode;
  iconTone?: Tone;
  label: string;
  value: ReactNode;
  meta?: ReactNode;
  loud?: boolean;
  action?: ReactNode;
}) {
  const accentTone = loud ? "warn" : iconTone;
  return (
    // 裸 div：不走 SoftCard，避免其 shadow-sm 与 shadow-card 并存导致 tailwind-merge
    // 无法解决冲突（自定义 shadow token 不在 merge 内置组）。
    // 完整基础类手动对齐 SoftCard：rounded-card bg-card border border-line。
    <div
      data-slot="metric-card"
      className={cn(
        "group relative cursor-pointer overflow-hidden",
        "rounded-card border border-line bg-card text-ink",
        // shadow-card 起步，hover → shadow-pop，与 TrendStatCard 完全一致
        "shadow-card",
        "px-4 py-4",
        "transition-all duration-200 hover:-translate-y-0.5 hover:shadow-pop active:scale-[0.98]",
        loud && "bg-gradient-to-b from-warn-soft to-card",
        className,
      )}
    >
      {/* 顶部色条：与员工卡片 TrendStatCard 同语言，3px 渐变线 */}
      <span
        aria-hidden
        className={cn(
          "pointer-events-none absolute inset-x-0 top-0 h-[3px] bg-gradient-to-r",
          toneAccentGradient[accentTone],
        )}
      />

      {/* 顶行：图标 + 标签 + 可选操作 */}
      <div className="mb-2 flex items-center gap-1.5">
        {icon ? (
          <IconTile tone={accentTone} size="sm">
            {icon}
          </IconTile>
        ) : null}
        <p className="min-w-0 flex-1 truncate text-[12px] font-medium text-ink-2">{label}</p>
        {action ? <div className="shrink-0">{action}</div> : null}
      </div>

      {/* 主数值：truncate 防止长字符串（如"5天10小时"）折行 */}
      <p
        className={cn(
          "truncate text-[1.875rem] font-bold leading-none tracking-tight tabular-nums",
          loud && "text-warn",
        )}
      >
        {value}
      </p>

      {/* 补注行 */}
      {meta ? <p className="mt-1.5 text-[11px] text-ink-3">{meta}</p> : null}
    </div>
  );
}

/**
 * 矩枢 signature 卡：实底 + 品牌顶线 + 低对比网格 + 节点/短路径。
 * 每个概览屏最多一块，靠控制平面母题建立识别，不使用大面积渐变或装饰光斑。
 * 每个概览屏最多一块，承载该页最有故事性的信息。子元素文字默认继承深色 ink。
 */
function SignatureCard({ className, children, ...props }: ComponentProps<"div">) {
  return (
    <div
      data-slot="signature-card"
      className={cn(
        "relative isolate overflow-hidden rounded-card p-6 text-[color:var(--signature-ink)] shadow-pop",
        "border border-[color:var(--signature-border)]",
        className,
      )}
      style={{ backgroundColor: "var(--signature-surface)" }}
      {...props}
    >
      {/* 顶部品牌色 accent 条：让 signature 一眼区别于普通白卡 */}
      <span
        aria-hidden
        className="pointer-events-none absolute inset-x-0 top-0 h-[3px]"
        style={{ background: "var(--brand-grad)" }}
      />
      {/* 细网格纹理 */}
      <span
        aria-hidden
        className="pointer-events-none absolute inset-0 -z-20"
        style={{
          backgroundImage:
            "linear-gradient(to right, var(--signature-grid) 1px, transparent 1px), linear-gradient(to bottom, var(--signature-grid) 1px, transparent 1px)",
          backgroundSize: "22px 22px",
          maskImage: "linear-gradient(135deg, #000 0%, rgba(0,0,0,0.68) 42%, transparent 86%)",
          WebkitMaskImage:
            "linear-gradient(135deg, #000 0%, rgba(0,0,0,0.68) 42%, transparent 86%)"
}}
      />
      {/* 轻量 surface wash，降低纯白卡与网格之间的生硬感 */}
      <span
        aria-hidden
        className="pointer-events-none absolute inset-0 -z-10"
        style={{ background: "var(--signature-wash)" }}
      />
      <span
        aria-hidden
        className="pointer-events-none absolute right-8 top-8 -z-10 h-px w-28"
        style={{ background: "var(--signature-route)" }}
      />
      <span
        aria-hidden
        className="pointer-events-none absolute right-[7.5rem] top-8 -z-10 size-2 rounded-full"
        style={{ background: "var(--signature-node)" }}
      />
      <span
        aria-hidden
        className="pointer-events-none absolute right-8 top-8 -z-10 size-2 rounded-full"
        style={{ background: "var(--signature-node)" }}
      />
      <span
        aria-hidden
        className="pointer-events-none absolute right-8 top-8 -z-10 h-16 w-px"
        style={{ background: "var(--signature-route)" }}
      />
      <span
        aria-hidden
        className="pointer-events-none absolute right-8 top-[5.75rem] -z-10 size-2 rounded-full"
        style={{ background: "var(--signature-node)" }}
      />
      {children}
    </div>
  );
}

/** 脆数据面容器：把密集表格装进柔和白卡（软壳装脆数据）。 */
function WorkSurface({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      data-slot="work-surface"
      className={cn("overflow-hidden rounded-card bg-card border border-line shadow-card", className)}
      {...props}
    />
  );
}

/** 密集表格：实底高对比，sticky 表头，tabular 数字，危险/预警行左 accent bar。 */
function DataTable({ className, ...props }: ComponentProps<"table">) {
  return (
    <div className="overflow-auto">
      <table
        data-slot="data-table"
        className={cn("w-full border-separate border-spacing-0 text-[13px]", className)}
        {...props}
      />
    </div>
  );
}

function Th({ className, ...props }: ComponentProps<"th">) {
  return (
    <th
      className={cn(
        "sticky top-0 z-[1] border-b border-line-strong bg-card-soft px-4 py-2.5 text-left text-[11px] font-bold tracking-wide text-ink-3 uppercase whitespace-nowrap",
        className,
      )}
      {...props}
    />
  );
}

function Td({ className, ...props }: ComponentProps<"td">) {
  return (
    <td
      className={cn("border-b border-line px-4 py-2.5 align-middle", className)}
      {...props}
    />
  );
}

/** 数据行。tone='danger'|'warn' 给整行实底软色 + 左侧实色 accent bar。 */
function Tr({
  className,
  tone,
  ...props
}: ComponentProps<"tr"> & { tone?: "danger" | "warn" }) {
  return (
    <tr
      data-tone={tone}
      className={cn(
        "transition-colors duration-200 [&:hover>td]:bg-card-inner [&:last-child>td]:border-b-0",
        tone === "danger" &&
          "[&>td]:bg-danger-soft [&>td:first-child]:shadow-[inset_3px_0_0_var(--danger)]",
        tone === "warn" &&
          "[&>td]:bg-warn-soft [&>td:first-child]:shadow-[inset_3px_0_0_var(--warn)]",
        className,
      )}
      {...props}
    />
  );
}

export type ButtonVariant = "primary" | "outline" | "ghost" | "danger" | "glass" | "default" | "secondary" | "destructive" | "link";
export type ButtonSize = "default" | "sm" | "icon" | "lg";
export type Density = "comfortable" | "compact";

const Button = forwardRef<
  HTMLButtonElement,
  ComponentProps<"button"> & {
    variant?: ButtonVariant;
    size?: ButtonSize;
    asChild?: boolean;
  }
>(({ className, variant = "primary", size = "default", asChild, ...props }, ref) => {
  const Comp = asChild ? Slot : "button";
  return (
    <Comp
      ref={ref}
      data-slot="app-button"
      data-variant={variant}
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
    />
  );
});
Button.displayName = "Button";

/** 灰底圆角图标按钮（顶栏/行内操作）。 */
function IconButton({ className, ...props }: ComponentProps<"button">) {
  return (
    <button
      data-slot="icon-button"
      className={cn(
        "inline-grid size-9 place-items-center rounded-xl bg-card-soft text-ink-2 shadow-card transition-all duration-200 ease-out hover:text-ink hover:-translate-y-0.5 hover:shadow-pop active:scale-[0.95]",
        className,
      )}
      {...props}
    />
  );
}

/** 筛选 chip：未选中=白卡描边，选中=蓝柔底蓝字。 */
function Chip({
  className,
  active,
  count,
  asChild,
  ...props
}: ComponentProps<"button"> & { active?: boolean; count?: number; asChild?: boolean }) {
  // 标签装饰默认用 span，避免在可移除 tag 内再嵌套 button；筛选触发器用 button。
  const interactive = Boolean(props.onClick) || props.type === "button" || asChild;
  const Comp: "button" | "span" = interactive ? "button" : "span";
  return (
    <Comp
      data-slot="chip"
      data-active={active || undefined}
      className={cn(
        "inline-flex items-center gap-1.5 rounded-xl px-3.5 py-2 text-[13px] font-semibold transition-all duration-200 ease-out",
        interactive && "active:scale-[0.97]",
        active
          ? "bg-brand-soft text-brand-deep"
          : "bg-card text-ink-2 border border-line shadow-sm",
        interactive && !active && "hover:text-ink hover:-translate-y-0.5 hover:shadow-md",
        className,
      )}
      {...(props as ComponentProps<"button">)}
    >
      {props.children}
      {count !== undefined ? (
        <span className={cn("text-xs", active ? "text-brand" : "text-ink-3")}>{count}</span>
      ) : null}
    </Comp>
  );
}/** 扁平 Tabs 容器（药丸式白卡内的一组 Tab）。配合 PageTabList + PageTab。 */
function PageTabs({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      data-slot="page-tabs"
      className={cn(
        "inline-flex w-fit flex-wrap gap-1 rounded-[14px] bg-card p-1.5 shadow-card",
        className,
      )}
      {...props}
    />
  );
}

function PageTabList({ className, ...props }: ComponentProps<"div">) {
  return <div data-slot="page-tab-list" className={cn("flex flex-wrap gap-1", className)} {...props} />;
}

function PageTab({
  className,
  active,
  ...props
}: ComponentProps<"button"> & { active?: boolean }) {
  return (
    <button
      data-slot="page-tab"
      aria-pressed={active}
      className={cn(
        "rounded-[10px] px-4 py-2 text-[13px] font-semibold transition-colors",
        active
          ? "bg-brand-soft text-brand-deep"
          : "text-ink-2 hover:bg-card-soft hover:text-ink",
        className,
      )}
      {...props}
    />
  );
}

/** 分段控件：用于“舒适 / 紧凑”密度切换等二选一场景。 */
function Segmented<T extends string>({
  className,
  options,
  value,
  onChange,
  ...props
}: Omit<ComponentProps<"div">, "onChange"> & {
  options: Array<{ label: ReactNode; value: T }>;
  value: T;
  onChange: (value: T) => void;
}) {
  return (
    <div
      data-slot="segmented"
      className={cn("inline-flex gap-1 rounded-lg bg-card-soft p-0.5", className)}
      {...props}
    >
      {options.map((opt) => (
        <button
          key={opt.value}
          type="button"
          onClick={() => onChange(opt.value)}
          aria-pressed={value === opt.value}
          className={cn(
            "rounded-[7px] px-2.5 py-1.5 text-xs font-semibold transition-colors",
            value === opt.value
              ? "bg-card text-ink shadow-card"
              : "text-ink-2 hover:text-ink",
          )}
        >
          {opt.label}
        </button>
      ))}
    </div>
  );
}

/** 页面标题区：标题 + 副标题 + 右侧主操作槽。 */
function PageHeader({
  className,
  variant = "page",
  title,
  subtitle,
  action,
  actions,
  icon,
  iconTone = "brand",
  back
}: {
  className?: string;
  variant?: "page" | "shell";
  title: ReactNode;
  subtitle?: ReactNode;
  action?: ReactNode;
  actions?: ReactNode;
  icon?: ReactNode;
  iconTone?: Tone;
  back?: ReactNode;
}) {
  const isShell = variant === "shell";
  const leading =
    back ?? (icon ? <IconTile tone={iconTone} size={isShell ? "default" : "lg"}>{icon}</IconTile> : null);
  const trailing = actions ?? action;

  return (
    <header
      data-slot="page-header"
      data-variant={variant}
      className={cn(
        isShell
          ? "flex w-full min-w-0 items-center justify-between gap-3"
          : "flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between",
        className,
      )}
    >
      <div className={cn("flex min-w-0 items-center", isShell ? "gap-2.5" : "gap-3")}>
        {leading}
        <div className="min-w-0">
          <h1
            className={cn(
              "leading-tight font-extrabold tracking-tight text-ink",
              isShell ? "truncate text-[18px]" : "text-[28px]",
            )}
          >
            {title}
          </h1>
          {subtitle ? (
            <p className={cn("truncate text-ink-2", isShell ? "mt-0.5 text-xs" : "mt-1 text-[13px]")}>
              {subtitle}
            </p>
          ) : null}
        </div>
      </div>
      {trailing ? <div className="flex shrink-0 gap-2">{trailing}</div> : null}
    </header>
  );
}

/** 工具栏搜索框（无边框，嵌在白卡工具栏里）。 */
function ToolbarSearch({
  className,
  ...props
}: ComponentProps<"input"> & { className?: string }) {
  return (
    <label
      data-slot="toolbar-search"
      className={cn(
        "flex min-w-[200px] flex-1 items-center gap-2 rounded-[10px] bg-card-soft px-3 py-2 text-ink-3",
        className,
      )}
    >
      <Search aria-hidden className="size-4" />
      <input
        className="w-full border-0 bg-transparent text-[13px] text-ink outline-none placeholder:text-ink-3"
        {...props}
      />
    </label>
  );
}

/** 受控分页：上一页/页码/下一页 + 每页条数。 */
function Pagination({
  className,
  total,
  page,
  pageSize,
  pageCount,
  onPageChange,
  onPageSizeChange,
  pageSizeOptions = [10, 20, 50]
}: {
  className?: string;
  total: number;
  page: number;
  pageSize: number;
  pageCount: number;
  onPageChange: (page: number) => void;
  onPageSizeChange?: (size: number) => void;
  pageSizeOptions?: number[];
}) {
  const windowPages = Array.from({ length: Math.min(pageCount, 5) }, (_, i) => i + 1);
  return (
    <div
      data-slot="pagination"
      className={cn(
        "flex flex-col gap-3 border-t border-line px-4 py-3 text-[13px] text-ink-2 md:flex-row md:items-center md:justify-between",
        className,
      )}
    >
      <span className="tabular-nums">
        共 <b className="font-bold text-ink">{total}</b> 条
      </span>
      <div className="flex flex-wrap items-center gap-3">
        <div className="flex items-center gap-1">
          <button
            type="button"
            aria-label="上一页"
            disabled={page <= 1}
            onClick={() => onPageChange(Math.max(1, page - 1))}
            className="grid size-8 place-items-center rounded-md border border-line bg-card text-ink-2 disabled:opacity-40"
          >
            ‹
          </button>
          {windowPages.map((p) => (
            <button
              key={p}
              type="button"
              aria-current={page === p ? "page" : undefined}
              onClick={() => onPageChange(p)}
              className={cn(
                "grid size-8 place-items-center rounded-md border text-[13px]",
                page === p
                  ? "border-brand bg-brand text-white"
                  : "border-line bg-card text-ink-2",
              )}
            >
              {p}
            </button>
          ))}
          {pageCount > 5 ? <span className="px-1">…</span> : null}
          <button
            type="button"
            aria-label="下一页"
            disabled={page >= pageCount}
            onClick={() => onPageChange(Math.min(pageCount, page + 1))}
            className="grid size-8 place-items-center rounded-md border border-line bg-card text-ink-2 disabled:opacity-40"
          >
            ›
          </button>
        </div>
        {onPageSizeChange ? (
          <select
            aria-label="每页条数"
            value={pageSize}
            onChange={(e) => onPageSizeChange(Number(e.target.value))}
            className="rounded-md border border-line bg-card px-2 py-1.5 text-[13px] text-ink-2 outline-none"
          >
            {pageSizeOptions.map((n) => (
              <option key={n} value={n}>
                {n} 条/页
              </option>
            ))}
          </select>
        ) : null}
      </div>
    </div>
  );
}

/** 空状态。 */
function EmptyState({
  className,
  icon,
  title,
  description,
  action,
  ...props
}: Omit<ComponentProps<"div">, "title"> & {
  icon?: ReactNode;
  title: ReactNode;
  description?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div
      data-slot="empty-state"
      role="status"
      className={cn("flex flex-col items-center gap-3 px-6 py-16 text-center", className)}
      {...props}
    >
      {icon ? <span className="text-ink-3 [&_svg]:size-10">{icon}</span> : null}
      <p className="text-[15px] font-semibold text-ink">{title}</p>
      {description ? <p className="max-w-sm text-[13px] text-ink-2">{description}</p> : null}
      {action ? <div className="mt-1">{action}</div> : null}
    </div>
  );
}

/** 加载中。 */
function LoadingState({
  className,
  label = "加载中…",
  ...props
}: ComponentProps<"div"> & { label?: ReactNode }) {
  return (
    <div
      data-slot="loading-state"
      role="status"
      aria-busy="true"
      className={cn("flex items-center justify-center gap-3 px-6 py-16 text-ink-2", className)}
      {...props}
    >
      <span className="size-5 animate-spin rounded-full border-2 border-line-strong border-t-brand" />
      <span className="text-[13px]">{label}</span>
    </div>
  );
}

/** 错误态。 */
function ErrorState({
  className,
  title = "加载失败",
  description,
  onRetry,
  ...props
}: ComponentProps<"div"> & {
  title?: ReactNode;
  description?: ReactNode;
  onRetry?: () => void;
}) {
  return (
    <div
      data-slot="error-state"
      role="alert"
      className={cn("flex flex-col gap-3 rounded-inner bg-danger-soft p-5 text-danger", className)}
      {...props}
    >
      <p className="text-[15px] font-bold">{title}</p>
      {description ? <p className="text-[13px] text-ink-2">{description}</p> : null}
      {onRetry ? (
        <div>
          <Button variant="danger" size="sm" onClick={onRetry}>
            重试
          </Button>
        </div>
      ) : null}
    </div>
  );
}

/** 权限不足。 */
function PermissionDenied({
  className,
  title = "无访问权限",
  description = "你当前的身份没有访问该资源的权限，请联系管理员或切换身份。",
  action,
  ...props
}: ComponentProps<"div"> & {
  title?: ReactNode;
  description?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div
      data-slot="permission-denied"
      role="alert"
      className={cn(
        "flex flex-col items-center gap-3 rounded-card bg-card px-6 py-16 text-center shadow-card",
        className,
      )}
      {...props}
    >
      <span className="grid size-12 place-items-center rounded-[15px] bg-mute-soft text-mute [&_svg]:size-6">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none">
          <path d="M12 2 4 6v6c0 5 3.4 8 8 10 4.6-2 8-5 8-10V6z" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" />
        </svg>
      </span>
      <p className="text-[15px] font-semibold text-ink">{title}</p>
      <p className="max-w-sm text-[13px] text-ink-2">{description}</p>
      {action ? <div className="mt-1">{action}</div> : null}
    </div>
  );
}

/** 统一状态面：根据 isLoading/isError/error/denied 渲染对应态，否则渲染 children。 */
function StateSurface({
  isLoading,
  isError,
  error,
  denied,
  empty,
  children,
  onRetry,
  emptyState
}: {
  isLoading?: boolean;
  isError?: boolean;
  error?: unknown;
  denied?: boolean;
  empty?: boolean;
  emptyState?: ReactNode;
  children?: ReactNode;
  onRetry?: () => void;
}) {
  if (denied) return <PermissionDenied />;
  if (isError) {
    const msg = error instanceof Error ? error.message : undefined;
    return <ErrorState description={msg} onRetry={onRetry} />;
  }
  if (isLoading) return <LoadingState />;
  if (empty) return <>{emptyState ?? <EmptyState title="暂无数据" />}</>;
  return <>{children}</>;
}

export {
  SoftCard,
  GlassCard,
  IconTile,
  StatusPill,
  MetricCard,
  SignatureCard,
  WorkSurface,
  DataTable,
  Th,
  Td,
  Tr,
  Button,
  IconButton,
  Chip,
  PageTabs,
  PageTabList,
  PageTab,
  Segmented,
  PageHeader,
  ToolbarSearch,
  Pagination,
  EmptyState,
  LoadingState,
  ErrorState,
  PermissionDenied,
  StateSurface
};


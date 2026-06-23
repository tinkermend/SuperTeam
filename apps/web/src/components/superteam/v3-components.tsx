import { type ComponentProps, type ReactNode } from "react";
import { Slot } from "@radix-ui/react-slot";
import { Search } from "lucide-react";
import { cn } from "@/lib/utils";

/**
 * SuperTeam v3 · Soft-Flat 组件族
 *
 * 设计基线见 DESIGN.md（容器选择规则）与 docs/design-system/{tokens,surfaces,data-display}.md。
 * 全部基于 theme.css 的 `--v3-*` token（Tailwind 暴露为 v3-* 颜色 / rounded-v3-* / shadow-v3）。
 * 一套语言两种容器：SoftCard 系列承载概览与外壳；WorkSurface + V3Table 承载密集数据本体。
 */

export type V3Tone = "brand" | "info" | "ok" | "warn" | "danger" | "mute" | "artifact";

const toneText: Record<V3Tone, string> = {
  brand: "text-v3-brand",
  info: "text-v3-info",
  ok: "text-v3-ok",
  warn: "text-v3-warn",
  danger: "text-v3-danger",
  mute: "text-v3-mute",
  artifact: "text-v3-artifact",
};

const toneSoftBg: Record<V3Tone, string> = {
  brand: "bg-v3-brand-soft",
  info: "bg-v3-info-soft",
  ok: "bg-v3-ok-soft",
  warn: "bg-v3-warn-soft",
  danger: "bg-v3-danger-soft",
  mute: "bg-v3-mute-soft",
  artifact: "bg-v3-artifact-soft",
};

/** 柔和白卡：页面外壳、概览卡、面板、表格容器外壳。 */
function SoftCard({
  className,
  interactive,
  ...props
}: ComponentProps<"div"> & { interactive?: boolean }) {
  return (
    <div
      data-slot="v3-soft-card"
      className={cn(
        "rounded-v3-card bg-v3-card text-v3-ink shadow-v3",
        interactive &&
          "cursor-pointer transition-[transform,box-shadow] duration-150 hover:-translate-y-0.5 hover:shadow-v3-pop",
        className,
      )}
      {...props}
    />
  );
}

/** squircle 语义图标芯片：实底柔色背景 + 同色图标。 */
function IconTile({
  className,
  tone = "brand",
  size = "default",
  ...props
}: ComponentProps<"span"> & { tone?: V3Tone; size?: "sm" | "default" | "lg" }) {
  const sizeClass =
    size === "sm"
      ? "size-9 rounded-[11px] [&_svg]:size-4"
      : size === "lg"
        ? "size-12 rounded-[15px] [&_svg]:size-6"
        : "size-11 rounded-[13px] [&_svg]:size-5";
  return (
    <span
      data-slot="v3-icon-tile"
      className={cn(
        "inline-grid shrink-0 place-items-center",
        sizeClass,
        toneText[tone],
        toneSoftBg[tone],
        className,
      )}
      {...props}
    />
  );
}

/** 状态胶囊：实底柔色，过 AA。tone 表达紧迫度/状态，类别请用图标+文字。 */
function StatusPill({
  className,
  tone = "mute",
  showDot = true,
  children,
  ...props
}: ComponentProps<"span"> & { tone?: V3Tone; showDot?: boolean }) {
  return (
    <span
      data-slot="v3-status-pill"
      className={cn(
        "inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1 text-xs font-bold whitespace-nowrap",
        toneText[tone],
        toneSoftBg[tone],
        className,
      )}
      {...props}
    >
      {showDot ? <span aria-hidden className="size-1.5 rounded-full bg-current" /> : null}
      {children}
    </span>
  );
}

/** 概览指标卡：图标芯片 + 标签 + 黑色粗体大数字 + 辅助行。loud=需人介入时点亮。 */
function V3MetricCard({
  className,
  icon,
  iconTone = "brand",
  label,
  value,
  meta,
  loud,
  action,
}: {
  className?: string;
  icon?: ReactNode;
  iconTone?: V3Tone;
  label: string;
  value: ReactNode;
  meta?: ReactNode;
  loud?: boolean;
  action?: ReactNode;
}) {
  return (
    <SoftCard
      className={cn(
        "p-5",
        loud && "bg-gradient-to-b from-v3-warn-soft to-v3-card",
        className,
      )}
    >
      <div className="mb-2.5 flex items-center gap-2">
        {icon ? (
          <IconTile tone={loud ? "warn" : iconTone} size="default">
            {icon}
          </IconTile>
        ) : null}
        {action ? <div className="ml-auto">{action}</div> : null}
      </div>
      <p className="text-[13px] text-v3-ink-2">{label}</p>
      <p
        className={cn(
          "mt-1 text-[2.5rem] leading-none font-extrabold tracking-tight tabular-nums",
          loud && "text-v3-warn",
        )}
      >
        {value}
      </p>
      {meta ? <p className="mt-1.5 text-xs text-v3-ink-3">{meta}</p> : null}
    </SoftCard>
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
      data-slot="v3-signature-card"
      className={cn(
        "relative isolate overflow-hidden rounded-v3-card p-6 text-[color:var(--v3-signature-ink)] shadow-v3-pop",
        "border border-[color:var(--v3-signature-border)]",
        className,
      )}
      style={{ backgroundColor: "var(--v3-signature-surface)" }}
      {...props}
    >
      {/* 顶部品牌色 accent 条：让 signature 一眼区别于普通白卡 */}
      <span
        aria-hidden
        className="pointer-events-none absolute inset-x-0 top-0 h-[3px]"
        style={{ background: "var(--v3-brand-grad)" }}
      />
      {/* 细网格纹理 */}
      <span
        aria-hidden
        className="pointer-events-none absolute inset-0 -z-20"
        style={{
          backgroundImage:
            "linear-gradient(to right, var(--v3-signature-grid) 1px, transparent 1px), linear-gradient(to bottom, var(--v3-signature-grid) 1px, transparent 1px)",
          backgroundSize: "22px 22px",
          maskImage: "linear-gradient(135deg, #000 0%, rgba(0,0,0,0.68) 42%, transparent 86%)",
          WebkitMaskImage:
            "linear-gradient(135deg, #000 0%, rgba(0,0,0,0.68) 42%, transparent 86%)",
        }}
      />
      {/* 轻量 surface wash，降低纯白卡与网格之间的生硬感 */}
      <span
        aria-hidden
        className="pointer-events-none absolute inset-0 -z-10"
        style={{ background: "var(--v3-signature-wash)" }}
      />
      <span
        aria-hidden
        className="pointer-events-none absolute right-8 top-8 -z-10 h-px w-28"
        style={{ background: "var(--v3-signature-route)" }}
      />
      <span
        aria-hidden
        className="pointer-events-none absolute right-[7.5rem] top-8 -z-10 size-2 rounded-full"
        style={{ background: "var(--v3-signature-node)" }}
      />
      <span
        aria-hidden
        className="pointer-events-none absolute right-8 top-8 -z-10 size-2 rounded-full"
        style={{ background: "var(--v3-signature-node)" }}
      />
      <span
        aria-hidden
        className="pointer-events-none absolute right-8 top-8 -z-10 h-16 w-px"
        style={{ background: "var(--v3-signature-route)" }}
      />
      <span
        aria-hidden
        className="pointer-events-none absolute right-8 top-[5.75rem] -z-10 size-2 rounded-full"
        style={{ background: "var(--v3-signature-node)" }}
      />
      {children}
    </div>
  );
}

/** 脆数据面容器：把密集表格装进柔和白卡（软壳装脆数据）。 */
function WorkSurface({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      data-slot="v3-work-surface"
      className={cn("overflow-hidden rounded-v3-card bg-v3-card shadow-v3", className)}
      {...props}
    />
  );
}

/** 密集表格：实底高对比，sticky 表头，tabular 数字，危险/预警行左 accent bar。 */
function V3Table({ className, ...props }: ComponentProps<"table">) {
  return (
    <div className="overflow-auto">
      <table
        data-slot="v3-table"
        className={cn("w-full border-separate border-spacing-0 text-[13px]", className)}
        {...props}
      />
    </div>
  );
}

function V3Th({ className, ...props }: ComponentProps<"th">) {
  return (
    <th
      className={cn(
        "sticky top-0 z-[1] border-b border-v3-line-strong bg-v3-card-soft px-4 py-2.5 text-left text-[11px] font-bold tracking-wide text-v3-ink-3 uppercase whitespace-nowrap",
        className,
      )}
      {...props}
    />
  );
}

function V3Td({ className, ...props }: ComponentProps<"td">) {
  return (
    <td
      className={cn("border-b border-v3-line px-4 py-2.5 align-middle", className)}
      {...props}
    />
  );
}

/** 数据行。tone='danger'|'warn' 给整行实底软色 + 左侧实色 accent bar。 */
function V3Tr({
  className,
  tone,
  ...props
}: ComponentProps<"tr"> & { tone?: "danger" | "warn" }) {
  return (
    <tr
      data-tone={tone}
      className={cn(
        "transition-colors [&:hover>td]:bg-v3-card-inner [&:last-child>td]:border-b-0",
        tone === "danger" &&
          "[&>td]:bg-v3-danger-soft [&>td:first-child]:shadow-[inset_3px_0_0_var(--v3-danger)]",
        tone === "warn" &&
          "[&>td]:bg-v3-warn-soft [&>td:first-child]:shadow-[inset_3px_0_0_var(--v3-warn)]",
        className,
      )}
      {...props}
    />
  );
}

export type V3ButtonVariant = "primary" | "outline" | "ghost" | "danger";
export type V3ButtonSize = "default" | "sm" | "icon";
export type V3Density = "comfortable" | "compact";

const buttonBase =
  "inline-flex items-center justify-center gap-2 rounded-xl font-semibold transition-colors disabled:pointer-events-none disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-v3-brand/60 focus-visible:ring-offset-1 focus-visible:ring-offset-v3-bg";

const buttonVariant: Record<V3ButtonVariant, string> = {
  primary: "bg-v3-brand text-white shadow-v3 hover:bg-v3-brand-deep",
  outline: "bg-v3-card text-v3-ink border border-v3-line-strong hover:bg-v3-card-soft",
  ghost: "bg-transparent text-v3-ink-2 hover:bg-v3-card-soft hover:text-v3-ink",
  danger: "bg-v3-danger-soft text-v3-danger hover:brightness-95",
};

const buttonSize: Record<V3ButtonSize, string> = {
  default: "h-10 px-4 text-[13px]",
  sm: "h-8 px-3 text-xs",
  icon: "size-9 p-0",
};

/** 主/次/危险/幽灵按钮。asChild 时把样式合并到子元素（用于 Link 按钮化）。 */
function V3Button({
  className,
  variant = "primary",
  size = "default",
  asChild,
  ...props
}: ComponentProps<"button"> & {
  variant?: V3ButtonVariant;
  size?: V3ButtonSize;
  asChild?: boolean;
}) {
  const Comp = asChild ? Slot : "button";
  return (
    <Comp
      data-slot="v3-button"
      data-variant={variant}
      className={cn(buttonBase, buttonVariant[variant], buttonSize[size], className)}
      {...props}
    />
  );
}

/** 灰底圆角图标按钮（顶栏/行内操作）。 */
function V3IconButton({ className, ...props }: ComponentProps<"button">) {
  return (
    <button
      data-slot="v3-icon-button"
      className={cn(
        "inline-grid size-9 place-items-center rounded-xl bg-v3-card-soft text-v3-ink-2 shadow-v3 transition-colors hover:text-v3-ink",
        className,
      )}
      {...props}
    />
  );
}

/** 筛选 chip：未选中=白卡描边，选中=蓝柔底蓝字。 */
function V3Chip({
  className,
  active,
  count,
  ...props
}: ComponentProps<"button"> & { active?: boolean; count?: number }) {
  return (
    <button
      data-slot="v3-chip"
      data-active={active || undefined}
      className={cn(
        "inline-flex items-center gap-1.5 rounded-xl px-3.5 py-2 text-[13px] font-semibold transition-colors",
        active
          ? "bg-v3-brand-soft text-v3-brand-deep"
          : "bg-v3-card text-v3-ink-2 shadow-v3 hover:text-v3-ink",
        className,
      )}
      {...props}
    >
      {props.children}
      {count !== undefined ? (
        <span className={cn("text-xs", active ? "text-v3-brand" : "text-v3-ink-3")}>{count}</span>
      ) : null}
    </button>
  );
}

/** 扁平 Tabs 容器（药丸式白卡内的一组 Tab）。配合 V3TabList + V3Tab。 */
function V3Tabs({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      data-slot="v3-tabs"
      className={cn(
        "inline-flex w-fit flex-wrap gap-1 rounded-[14px] bg-v3-card p-1.5 shadow-v3",
        className,
      )}
      {...props}
    />
  );
}

function V3TabList({ className, ...props }: ComponentProps<"div">) {
  return <div data-slot="v3-tab-list" className={cn("flex flex-wrap gap-1", className)} {...props} />;
}

function V3Tab({
  className,
  active,
  ...props
}: ComponentProps<"button"> & { active?: boolean }) {
  return (
    <button
      data-slot="v3-tab"
      aria-pressed={active}
      className={cn(
        "rounded-[10px] px-4 py-2 text-[13px] font-semibold transition-colors",
        active
          ? "bg-v3-brand-soft text-v3-brand-deep"
          : "text-v3-ink-2 hover:bg-v3-card-soft hover:text-v3-ink",
        className,
      )}
      {...props}
    />
  );
}

/** 分段控件：用于“舒适 / 紧凑”密度切换等二选一场景。 */
function V3Segmented<T extends string>({
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
      data-slot="v3-segmented"
      className={cn("inline-flex gap-1 rounded-lg bg-v3-card-soft p-0.5", className)}
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
              ? "bg-v3-card text-v3-ink shadow-v3"
              : "text-v3-ink-2 hover:text-v3-ink",
          )}
        >
          {opt.label}
        </button>
      ))}
    </div>
  );
}

/** 页面标题区：标题 + 副标题 + 右侧主操作槽。 */
function V3PageHeader({
  className,
  title,
  subtitle,
  action,
  actions,
  icon,
  iconTone = "brand",
  back,
}: {
  className?: string;
  title: ReactNode;
  subtitle?: ReactNode;
  action?: ReactNode;
  actions?: ReactNode;
  icon?: ReactNode;
  iconTone?: V3Tone;
  back?: ReactNode;
}) {
  const leading = back ?? (icon ? <IconTile tone={iconTone} size="lg">{icon}</IconTile> : null);
  const trailing = actions ?? action;

  return (
    <header
      data-slot="v3-page-header"
      className={cn("flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between", className)}
    >
      <div className="flex min-w-0 items-center gap-3">
        {leading}
        <div className="min-w-0">
          <h1 className="text-[28px] leading-tight font-extrabold tracking-tight text-v3-ink">
            {title}
          </h1>
          {subtitle ? <p className="mt-1 text-[13px] text-v3-ink-2">{subtitle}</p> : null}
        </div>
      </div>
      {trailing ? <div className="flex shrink-0 gap-2">{trailing}</div> : null}
    </header>
  );
}

/** 工具栏搜索框（无边框，嵌在白卡工具栏里）。 */
function V3ToolbarSearch({
  className,
  ...props
}: ComponentProps<"input"> & { className?: string }) {
  return (
    <label
      data-slot="v3-toolbar-search"
      className={cn(
        "flex min-w-[200px] flex-1 items-center gap-2 rounded-[10px] bg-v3-card-soft px-3 py-2 text-v3-ink-3",
        className,
      )}
    >
      <Search aria-hidden className="size-4" />
      <input
        className="w-full border-0 bg-transparent text-[13px] text-v3-ink outline-none placeholder:text-v3-ink-3"
        {...props}
      />
    </label>
  );
}

/** 受控分页：上一页/页码/下一页 + 每页条数。 */
function V3Pagination({
  className,
  total,
  page,
  pageSize,
  pageCount,
  onPageChange,
  onPageSizeChange,
  pageSizeOptions = [10, 20, 50],
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
      data-slot="v3-pagination"
      className={cn(
        "flex flex-col gap-3 border-t border-v3-line px-4 py-3 text-[13px] text-v3-ink-2 md:flex-row md:items-center md:justify-between",
        className,
      )}
    >
      <span className="tabular-nums">
        共 <b className="font-bold text-v3-ink">{total}</b> 条
      </span>
      <div className="flex flex-wrap items-center gap-3">
        <div className="flex items-center gap-1">
          <button
            type="button"
            aria-label="上一页"
            disabled={page <= 1}
            onClick={() => onPageChange(Math.max(1, page - 1))}
            className="grid size-8 place-items-center rounded-md border border-v3-line bg-v3-card text-v3-ink-2 disabled:opacity-40"
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
                  ? "border-v3-brand bg-v3-brand text-white"
                  : "border-v3-line bg-v3-card text-v3-ink-2",
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
            className="grid size-8 place-items-center rounded-md border border-v3-line bg-v3-card text-v3-ink-2 disabled:opacity-40"
          >
            ›
          </button>
        </div>
        {onPageSizeChange ? (
          <select
            aria-label="每页条数"
            value={pageSize}
            onChange={(e) => onPageSizeChange(Number(e.target.value))}
            className="rounded-md border border-v3-line bg-v3-card px-2 py-1.5 text-[13px] text-v3-ink-2 outline-none"
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
function V3EmptyState({
  className,
  icon,
  title,
  description,
  action,
  ...props
}: ComponentProps<"div"> & {
  icon?: ReactNode;
  title: ReactNode;
  description?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div
      data-slot="v3-empty-state"
      role="status"
      className={cn("flex flex-col items-center gap-3 px-6 py-16 text-center", className)}
      {...props}
    >
      {icon ? <span className="text-v3-ink-3 [&_svg]:size-10">{icon}</span> : null}
      <p className="text-[15px] font-semibold text-v3-ink">{title}</p>
      {description ? <p className="max-w-sm text-[13px] text-v3-ink-2">{description}</p> : null}
      {action ? <div className="mt-1">{action}</div> : null}
    </div>
  );
}

/** 加载中。 */
function V3LoadingState({
  className,
  label = "加载中…",
  ...props
}: ComponentProps<"div"> & { label?: ReactNode }) {
  return (
    <div
      data-slot="v3-loading-state"
      role="status"
      aria-busy="true"
      className={cn("flex items-center justify-center gap-3 px-6 py-16 text-v3-ink-2", className)}
      {...props}
    >
      <span className="size-5 animate-spin rounded-full border-2 border-v3-line-strong border-t-v3-brand" />
      <span className="text-[13px]">{label}</span>
    </div>
  );
}

/** 错误态。 */
function V3ErrorState({
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
      data-slot="v3-error-state"
      role="alert"
      className={cn("flex flex-col gap-3 rounded-v3-inner bg-v3-danger-soft p-5 text-v3-danger", className)}
      {...props}
    >
      <p className="text-[15px] font-bold">{title}</p>
      {description ? <p className="text-[13px] text-v3-ink-2">{description}</p> : null}
      {onRetry ? (
        <div>
          <V3Button variant="danger" size="sm" onClick={onRetry}>
            重试
          </V3Button>
        </div>
      ) : null}
    </div>
  );
}

/** 权限不足。 */
function V3PermissionDenied({
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
      data-slot="v3-permission-denied"
      role="alert"
      className={cn(
        "flex flex-col items-center gap-3 rounded-v3-card bg-v3-card px-6 py-16 text-center shadow-v3",
        className,
      )}
      {...props}
    >
      <span className="grid size-12 place-items-center rounded-[15px] bg-v3-mute-soft text-v3-mute [&_svg]:size-6">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none">
          <path d="M12 2 4 6v6c0 5 3.4 8 8 10 4.6-2 8-5 8-10V6z" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" />
        </svg>
      </span>
      <p className="text-[15px] font-semibold text-v3-ink">{title}</p>
      <p className="max-w-sm text-[13px] text-v3-ink-2">{description}</p>
      {action ? <div className="mt-1">{action}</div> : null}
    </div>
  );
}

/** 统一状态面：根据 isLoading/isError/error/denied 渲染对应态，否则渲染 children。 */
function V3StateSurface({
  isLoading,
  isError,
  error,
  denied,
  empty,
  children,
  onRetry,
  emptyState,
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
  if (denied) return <V3PermissionDenied />;
  if (isError) {
    const msg = error instanceof Error ? error.message : undefined;
    return <V3ErrorState description={msg} onRetry={onRetry} />;
  }
  if (isLoading) return <V3LoadingState />;
  if (empty) return <>{emptyState ?? <V3EmptyState title="暂无数据" />}</>;
  return <>{children}</>;
}

export {
  SoftCard,
  IconTile,
  StatusPill,
  V3MetricCard,
  SignatureCard,
  WorkSurface,
  V3Table,
  V3Th,
  V3Td,
  V3Tr,
  V3Button,
  V3IconButton,
  V3Chip,
  V3Tabs,
  V3TabList,
  V3Tab,
  V3Segmented,
  V3PageHeader,
  V3ToolbarSearch,
  V3Pagination,
  V3EmptyState,
  V3LoadingState,
  V3ErrorState,
  V3PermissionDenied,
  V3StateSurface,
};

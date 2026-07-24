import {
  type ComponentProps,
  type ReactNode,
} from "react";
import { ChevronRight, FilterX, Inbox, Settings2 } from "lucide-react";
import { toast, type ExternalToast } from "sonner";
import * as TabsPrimitive from "@radix-ui/react-tabs";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { cn } from "@/lib/utils";
import { formatDateTime, formatRelativeTime } from "@/lib/format-time";
import { EmptyState, IconTile, type Tone } from "@/components/superteam/primitives";

/**
 * Soft-Flat Batch D · 收敛与减噪
 *
 * Breadcrumb / SectionHeader / Empty* 预设 / SoftTabs / RelativeTime /
 * ButtonGroup / AvatarStack / notify* / LogLine / CodeBlock
 *
 * Badge / Tabs 边界见文件底注释与 design-system 文档（状态→StatusPill，筛选→Chip，
 * 页面级 Tab→PageTabs，面板/路由受控 Tab→SoftTabs；ui/badge 不进业务新代码）。
 */

// ─── Breadcrumb ──────────────────────────────────────────────────────────────

export type BreadcrumbItemData = {
  id?: string;
  label: ReactNode;
  /** 非末项可导航；末项忽略 href */
  href?: string;
  onClick?: () => void;
};

function Breadcrumb({
  className,
  items,
  ...props
}: ComponentProps<"nav"> & { items: BreadcrumbItemData[] }) {
  if (items.length === 0) return null;
  return (
    <nav
      data-slot="breadcrumb"
      aria-label="面包屑"
      className={cn("min-w-0", className)}
      {...props}
    >
      <ol className="flex min-w-0 flex-wrap items-center gap-1 text-[13px]">
        {items.map((item, index) => {
          const last = index === items.length - 1;
          const key = item.id ?? String(index);
          return (
            <li key={key} className="flex min-w-0 items-center gap-1">
              {index > 0 ? (
                <ChevronRight
                  aria-hidden
                  className="size-3.5 shrink-0 text-ink-3"
                />
              ) : null}
              {last || (!item.href && !item.onClick) ? (
                <span
                  className={cn(
                    "min-w-0 truncate font-medium",
                    last ? "text-ink" : "text-ink-3",
                  )}
                  aria-current={last ? "page" : undefined}
                >
                  {item.label}
                </span>
              ) : item.href ? (
                <a
                  href={item.href}
                  onClick={item.onClick}
                  className="min-w-0 truncate font-medium text-ink-3 transition-colors hover:text-brand"
                >
                  {item.label}
                </a>
              ) : (
                <button
                  type="button"
                  onClick={item.onClick}
                  className="min-w-0 truncate font-medium text-ink-3 transition-colors hover:text-brand"
                >
                  {item.label}
                </button>
              )}
            </li>
          );
        })}
      </ol>
    </nav>
  );
}

// ─── SectionHeader ───────────────────────────────────────────────────────────

function SectionHeader({
  className,
  icon,
  iconTone = "brand",
  title,
  description,
  meta,
  actions,
  ...props
}: ComponentProps<"div"> & {
  icon?: ReactNode;
  iconTone?: Tone;
  title: ReactNode;
  description?: ReactNode;
  meta?: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <div
      data-slot="section-header"
      className={cn(
        "flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between",
        className,
      )}
      {...props}
    >
      <div className="flex min-w-0 items-start gap-3">
        {icon ? (
          <IconTile tone={iconTone} size="sm" className="mt-0.5">
            {icon}
          </IconTile>
        ) : null}
        <div className="min-w-0">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <h2 className="text-[15px] font-bold tracking-tight text-ink">
              {title}
            </h2>
            {meta ? (
              <div className="text-[12px] text-ink-3">{meta}</div>
            ) : null}
          </div>
          {description ? (
            <p className="mt-0.5 text-[13px] text-ink-2">{description}</p>
          ) : null}
        </div>
      </div>
      {actions ? (
        <div
          data-slot="section-header-actions"
          className="flex shrink-0 flex-wrap items-center gap-2"
        >
          {actions}
        </div>
      ) : null}
    </div>
  );
}

// ─── EmptyState 预设 ─────────────────────────────────────────────────────────

type EmptyPresetProps = {
  className?: string;
  title?: ReactNode;
  description?: ReactNode;
  icon?: ReactNode;
  action?: ReactNode;
};

function EmptyNoData({
  title = "暂无数据",
  description = "还没有任何条目。",
  icon = <Inbox />,
  className,
  action,
}: EmptyPresetProps) {
  return (
    <EmptyState
      data-empty-kind="no-data"
      className={className}
      icon={icon}
      title={title}
      description={description}
      action={action}
    />
  );
}

function EmptyNoMatch({
  title = "无匹配结果",
  description = "当前筛选条件下没有条目，可调整条件或清除筛选。",
  icon = <FilterX />,
  className,
  action,
}: EmptyPresetProps) {
  return (
    <EmptyState
      data-empty-kind="no-match"
      className={className}
      icon={icon}
      title={title}
      description={description}
      action={action}
    />
  );
}

function EmptyUnconfigured({
  title = "尚未配置",
  description = "完成必要配置后即可使用此功能。",
  icon = <Settings2 />,
  className,
  action,
}: EmptyPresetProps) {
  return (
    <EmptyState
      data-empty-kind="unconfigured"
      className={className}
      icon={icon}
      title={title}
      description={description}
      action={action}
    />
  );
}

// ─── SoftTabs（面板级 Radix Tabs Soft-Flat 皮肤）────────────────────────────

function SoftTabs({
  className,
  ...props
}: ComponentProps<typeof TabsPrimitive.Root>) {
  return (
    <TabsPrimitive.Root
      data-slot="soft-tabs"
      className={cn("flex flex-col gap-3", className)}
      {...props}
    />
  );
}

function SoftTabsList({
  className,
  ...props
}: ComponentProps<typeof TabsPrimitive.List>) {
  return (
    <TabsPrimitive.List
      data-slot="soft-tabs-list"
      className={cn(
        "inline-flex h-auto w-fit max-w-full flex-wrap items-center justify-start gap-1 rounded-[14px] bg-card p-1.5 text-ink-2 shadow-card",
        className,
      )}
      {...props}
    />
  );
}

function SoftTabsTrigger({
  className,
  ...props
}: ComponentProps<typeof TabsPrimitive.Trigger>) {
  return (
    <TabsPrimitive.Trigger
      data-slot="soft-tabs-trigger"
      className={cn(
        "inline-flex items-center justify-center gap-1.5 rounded-[10px] px-4 py-2 text-[13px] font-semibold whitespace-nowrap transition-colors",
        "text-ink-2 hover:bg-card-soft hover:text-ink",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-1",
        "disabled:pointer-events-none disabled:opacity-50",
        "data-[state=active]:bg-brand-soft data-[state=active]:text-brand-deep data-[state=active]:shadow-none",
        className,
      )}
      {...props}
    />
  );
}

function SoftTabsContent({
  className,
  ...props
}: ComponentProps<typeof TabsPrimitive.Content>) {
  return (
    <TabsPrimitive.Content
      data-slot="soft-tabs-content"
      className={cn("min-w-0 flex-1 outline-none", className)}
      {...props}
    />
  );
}

// ─── RelativeTime ────────────────────────────────────────────────────────────

function RelativeTime({
  className,
  value,
  ...props
}: Omit<ComponentProps<"time">, "dateTime" | "children"> & {
  value: string;
}) {
  const absolute = (() => {
    try {
      return formatDateTime(value);
    } catch {
      return value;
    }
  })();
  return (
    <time
      data-slot="relative-time"
      dateTime={value}
      title={absolute}
      className={cn("tabular-nums text-ink-3", className)}
      {...props}
    >
      {formatRelativeTime(value)}
    </time>
  );
}

// ─── ButtonGroup ─────────────────────────────────────────────────────────────

function ButtonGroup({
  className,
  ...props
}: ComponentProps<"div">) {
  return (
    <div
      data-slot="button-group"
      role="group"
      className={cn(
        "inline-flex items-center",
        // 相邻按钮贴合、共享边框
        "[&_[data-slot=app-button]]:rounded-none [&_[data-slot=app-button]]:shadow-none",
        "[&_[data-slot=app-button]:first-child]:rounded-s-xl",
        "[&_[data-slot=app-button]:last-child]:rounded-e-xl",
        "[&_[data-slot=app-button]:not(:first-child)]:-ms-px",
        "[&_[data-slot=icon-button]]:rounded-none",
        "[&_[data-slot=icon-button]:first-child]:rounded-s-xl",
        "[&_[data-slot=icon-button]:last-child]:rounded-e-xl",
        "[&_[data-slot=icon-button]:not(:first-child)]:-ms-px",
        className,
      )}
      {...props}
    />
  );
}

// ─── AvatarStack ─────────────────────────────────────────────────────────────

export type AvatarStackItem = {
  id: string;
  name: string;
  src?: string | null;
};

function AvatarStack({
  className,
  items,
  max = 4,
  size = "sm",
  ...props
}: ComponentProps<"div"> & {
  items: AvatarStackItem[];
  max?: number;
  size?: "sm" | "md";
}) {
  const shown = items.slice(0, max);
  const rest = Math.max(0, items.length - shown.length);
  const dim = size === "md" ? "size-9 text-[12px]" : "size-7 text-[11px]";
  return (
    <div
      data-slot="avatar-stack"
      className={cn("flex items-center", className)}
      {...props}
    >
      {shown.map((item, index) => (
        <Avatar
          key={item.id}
          title={item.name}
          className={cn(
            dim,
            "ring-2 ring-card",
            index > 0 && "-ms-2",
          )}
        >
          {item.src ? <AvatarImage src={item.src} alt="" /> : null}
          <AvatarFallback className="bg-brand-soft font-semibold text-brand-deep">
            {item.name.trim().slice(0, 1).toUpperCase() || "?"}
          </AvatarFallback>
        </Avatar>
      ))}
      {rest > 0 ? (
        <span
          className={cn(
            dim,
            "-ms-2 inline-grid place-items-center rounded-full bg-card-soft font-semibold text-ink-2 ring-2 ring-card",
          )}
          title={`还有 ${rest} 人`}
        >
          +{rest}
        </span>
      ) : null}
    </div>
  );
}

// ─── Toast helpers ───────────────────────────────────────────────────────────

const toastBase: ExternalToast = {
  duration: 3200,
};

/** 成功轻反馈；同一逻辑点勿连弹多条。 */
function notifySuccess(message: string, options?: ExternalToast) {
  return toast.success(message, { ...toastBase, ...options });
}

function notifyError(message: string, options?: ExternalToast) {
  return toast.error(message, { ...toastBase, duration: 4800, ...options });
}

function notifyWarning(message: string, options?: ExternalToast) {
  return toast.warning(message, { ...toastBase, ...options });
}

function notifyInfo(message: string, options?: ExternalToast) {
  return toast.message(message, { ...toastBase, ...options });
}

// ─── LogLine / CodeBlock ─────────────────────────────────────────────────────

export type LogLineTone = "mute" | "info" | "ok" | "warn" | "danger";

function LogLine({
  className,
  time,
  level,
  tone = "mute",
  children,
  ...props
}: ComponentProps<"div"> & {
  time?: ReactNode;
  level?: ReactNode;
  tone?: LogLineTone;
}) {
  const levelColor: Record<LogLineTone, string> = {
    mute: "text-ink-3",
    info: "text-info-text",
    ok: "text-ok-text",
    warn: "text-warn-text",
    danger: "text-danger-text",
  };
  return (
    <div
      data-slot="log-line"
      data-tone={tone}
      className={cn(
        "grid grid-cols-[auto_auto_minmax(0,1fr)] items-start gap-x-3 gap-y-0.5 border-b border-line/70 px-3 py-1.5 font-mono text-[12px] leading-5 last:border-b-0",
        className,
      )}
      {...props}
    >
      <span className="shrink-0 tabular-nums text-ink-3">{time ?? "—"}</span>
      <span className={cn("shrink-0 font-semibold uppercase", levelColor[tone])}>
        {level ?? tone}
      </span>
      <span className="min-w-0 break-all text-ink-2">{children}</span>
    </div>
  );
}

function CodeBlock({
  className,
  children,
  ...props
}: ComponentProps<"pre">) {
  return (
    <pre
      data-slot="code-block"
      className={cn(
        "overflow-x-auto rounded-inner border border-line bg-card-soft p-3 font-mono text-[12px] leading-5 text-ink-2",
        className,
      )}
      {...props}
    >
      {children}
    </pre>
  );
}

export {
  Breadcrumb,
  SectionHeader,
  EmptyNoData,
  EmptyNoMatch,
  EmptyUnconfigured,
  SoftTabs,
  SoftTabsList,
  SoftTabsTrigger,
  SoftTabsContent,
  RelativeTime,
  ButtonGroup,
  AvatarStack,
  notifySuccess,
  notifyError,
  notifyWarning,
  notifyInfo,
  LogLine,
  CodeBlock,
};

/*
 * ── 迁移边界（Batch D）──────────────────────────────────────────────────────
 *
 * Breadcrumb vs ShellPageHeaderBack
 *   - 单级返回：ShellPageHeaderBack。
 *   - ≥2 级路径：Breadcrumb（可与页头并存于标题上方）。
 *   - 站内路由：优先传入 onClick + TanStack navigate，或外层用 Link 包自定义项；
 *     href 适合真 URL / 需可新开的链接。
 *
 * SectionHeader vs PageHeader / ObjectHeader
 *   - 页面级：PageHeader / ShellPageHeader。
 *   - 对象详情头：ObjectHeader。
 *   - SoftCard / 面板内分区：SectionHeader。
 *
 * Empty*
 *   - 通用壳仍可用 EmptyState。
 *   - 无数据 / 筛选无匹配 / 未配置：优先 EmptyNoData / EmptyNoMatch / EmptyUnconfigured，
 *     避免三处同一句「暂无数据」。
 *
 * PageTabs vs SoftTabs vs ui/tabs
 *   - 页面级、非 Radix 受控/路由按钮条：PageTabs + PageTab。
 *   - 需要 Radix Tabs 状态/键盘/content 面板：SoftTabs*（Soft-Flat 皮肤）。
 *   - 禁止业务新代码直接用未换肤的 ui/tabs 默认 muted 样式；触达时换 SoftTabs。
 *
 * Badge / StatusPill / Chip
 *   - 状态：StatusPill。筛选/标签选择：Chip。
 *   - ui/badge：仅内部/第三方复合；业务 features 禁止新增引用。
 *
 * RelativeTime
 *   - 展示层组件；格式化逻辑仍在 lib/format-time（可单测）。
 *
 * notify*
 *   - 包一层 sonner，统一时长；勿在同一提交路径 success+error 叠两条。
 *   - 字段校验仍用 Field error，不用 toast 替代。
 *
 * AvatarStack vs UserIdentity
 *   - 单人：UserIdentity。多人堆叠概览：AvatarStack。
 *
 * LogLine / CodeBlock
 *   - 装在 WorkSurface 内；不替代密表 DataTable。
 */

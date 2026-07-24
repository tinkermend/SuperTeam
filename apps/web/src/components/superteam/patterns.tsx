import {
  forwardRef,
  type ComponentProps,
  type ReactNode,
} from "react";
import { Slot } from "@radix-ui/react-slot";
import { MoreHorizontal } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import { IconButton } from "@/components/superteam/primitives";

/**
 * Soft-Flat Batch B · 目录 / 详情同构
 *
 * EntityCard / FactRow / ObjectHeader / DescriptionList / ActionMenu / Skeleton*
 * 规范：DESIGN.md「身份型实体目录」、page-archetypes、data-display、a11y-and-dark
 */

// ─── FactRow ─────────────────────────────────────────────────────────────────

/** 紧凑事实行：左标签右值；用于 EntityCard 与详情摘要。 */
function FactRow({
  className,
  label,
  value,
  mono,
  ...props
}: ComponentProps<"div"> & {
  label: ReactNode;
  value?: ReactNode;
  mono?: boolean;
}) {
  return (
    <div
      data-slot="fact-row"
      className={cn(
        "flex min-w-0 items-baseline justify-between gap-3 text-[12px] leading-4",
        className,
      )}
      {...props}
    >
      <span className="shrink-0 text-ink-3">{label}</span>
      <span
        className={cn(
          "min-w-0 truncate text-end text-ink-2",
          mono && "font-mono text-[11px] tracking-tight",
        )}
        title={typeof value === "string" ? value : undefined}
      >
        {value ?? "—"}
      </span>
    </div>
  );
}

// ─── EntityCard ──────────────────────────────────────────────────────────────

export type EntityCardProps = Omit<ComponentProps<"div">, "title"> & {
  /** 选中态：品牌边 + 浅底 + 左侧 accent bar；不改变尺寸 */
  selected?: boolean;
  /** 可扫读目录卡的 hover 微抬 */
  interactive?: boolean;
  /** 合并到 Link/a 根节点（整卡可点）；卡内 actions 仍 stopPropagation */
  asChild?: boolean;
  leading?: ReactNode;
  title: ReactNode;
  subtitle?: ReactNode;
  status?: ReactNode;
  /** 2–4 条高频事实 */
  facts?: Array<{ label: ReactNode; value?: ReactNode; mono?: boolean; key?: string }>;
  footer?: ReactNode;
  /** 卡内次要动作；点击 stopPropagation */
  actions?: ReactNode;
};

/**
 * 身份型目录卡：识别优先（leading + 名 + 状态），2–4 事实行，克制选中态。
 */
const EntityCard = forwardRef<HTMLDivElement, EntityCardProps>(function EntityCard(
  {
    className,
    selected,
    interactive,
    asChild = false,
    leading,
    title,
    subtitle,
    status,
    facts,
    footer,
    actions,
    children,
    ...props
  },
  ref,
) {
  const Comp = asChild ? Slot : "div";
  return (
    <Comp
      ref={ref}
      data-slot="entity-card"
      data-selected={selected ? "true" : undefined}
      className={cn(
        "relative flex flex-col gap-3 rounded-card border border-line bg-card p-4 text-ink shadow-sm",
        selected &&
          "border-brand bg-brand-soft/40 shadow-[inset_3px_0_0_0_var(--brand)]",
        interactive &&
          !selected &&
          "cursor-pointer transition-all duration-300 ease-out hover:-translate-y-1 hover:border-line-strong hover:shadow-md active:scale-[0.99]",
        interactive && selected && "cursor-pointer",
        className,
      )}
      {...props}
    >
      <div className="flex min-w-0 items-start gap-3">
        {leading ? (
          <div data-slot="entity-card-leading" className="shrink-0">
            {leading}
          </div>
        ) : null}
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-start gap-2">
            <div className="min-w-0 flex-1">
              <h3
                className="line-clamp-2 text-[15px] font-bold leading-5 tracking-tight text-ink"
                title={typeof title === "string" ? title : undefined}
              >
                {title}
              </h3>
              {subtitle ? (
                <div className="mt-0.5 truncate text-[12px] text-ink-3">{subtitle}</div>
              ) : null}
            </div>
            {status ? (
              <div data-slot="entity-card-status" className="shrink-0">
                {status}
              </div>
            ) : null}
            {actions ? (
              <div
                data-slot="entity-card-actions"
                className="shrink-0"
                onClick={(event) => event.stopPropagation()}
                onKeyDown={(event) => event.stopPropagation()}
              >
                {actions}
              </div>
            ) : null}
          </div>
        </div>
      </div>
      {facts && facts.length > 0 ? (
        <div data-slot="entity-card-facts" className="grid gap-1.5">
          {facts.slice(0, 4).map((fact, index) => (
            <FactRow
              key={fact.key ?? String(index)}
              label={fact.label}
              value={fact.value}
              mono={fact.mono}
            />
          ))}
        </div>
      ) : null}
      {children}
      {footer ? (
        <div data-slot="entity-card-footer" className="border-t border-line pt-3">
          {footer}
        </div>
      ) : null}
    </Comp>
  );
});

export type ObjectHeaderProps = {
  className?: string;
  leading?: ReactNode;
  title: ReactNode;
  subtitle?: ReactNode;
  status?: ReactNode;
  /** 次要元信息行（提供商、归属、时间等） */
  meta?: ReactNode;
  /** 主/次 CTA */
  actions?: ReactNode;
  /** 溢出菜单等 */
  overflow?: ReactNode;
};

/**
 * 运营/对象详情头：名 + 状态 + 主操作；UUID 勿作主标题（用 ObjectRef）。
 */
function ObjectHeader({
  className,
  leading,
  title,
  subtitle,
  status,
  meta,
  actions,
  overflow,
}: ObjectHeaderProps) {
  return (
    <header
      data-slot="object-header"
      className={cn(
        "flex flex-col gap-4 rounded-card border border-line bg-card p-5 shadow-sm sm:flex-row sm:items-start sm:justify-between",
        className,
      )}
    >
      <div className="flex min-w-0 flex-1 items-start gap-3.5">
        {leading ? (
          <div data-slot="object-header-leading" className="shrink-0">
            {leading}
          </div>
        ) : null}
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <h1 className="min-w-0 text-[22px] font-extrabold leading-tight tracking-tight text-ink">
              {title}
            </h1>
            {status ? (
              <div data-slot="object-header-status" className="shrink-0">
                {status}
              </div>
            ) : null}
          </div>
          {subtitle ? (
            <p className="mt-1 text-[13px] text-ink-2">{subtitle}</p>
          ) : null}
          {meta ? (
            <div
              data-slot="object-header-meta"
              className="mt-2 flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-[12px] text-ink-3"
            >
              {meta}
            </div>
          ) : null}
        </div>
      </div>
      {actions || overflow ? (
        <div
          data-slot="object-header-actions"
          className="flex shrink-0 flex-wrap items-center gap-2"
        >
          {actions}
          {overflow}
        </div>
      ) : null}
    </header>
  );
}

// ─── DescriptionList ─────────────────────────────────────────────────────────

export type DescriptionItem = {
  key?: string;
  label: ReactNode;
  value?: ReactNode;
  mono?: boolean;
  /** 双列时占满整行 */
  fullWidth?: boolean;
};

function DescriptionList({
  className,
  items,
  columns = 1,
  ...props
}: ComponentProps<"dl"> & {
  items: DescriptionItem[];
  columns?: 1 | 2;
}) {
  return (
    <dl
      data-slot="description-list"
      data-columns={columns}
      className={cn(
        "grid gap-x-6 gap-y-3",
        columns === 2 ? "sm:grid-cols-2" : "grid-cols-1",
        className,
      )}
      {...props}
    >
      {items.map((item, index) => (
        <div
          key={item.key ?? String(index)}
          data-slot="description-item"
          className={cn(
            "grid min-w-0 gap-1",
            item.fullWidth && columns === 2 && "sm:col-span-2",
          )}
        >
          <dt className="text-[12px] font-medium text-ink-3">{item.label}</dt>
          <dd
            className={cn(
              "min-w-0 break-words text-[13px] text-ink",
              item.mono && "font-mono text-[12px] text-ink-2",
            )}
          >
            {item.value ?? "—"}
          </dd>
        </div>
      ))}
    </dl>
  );
}

// ─── ActionMenu ──────────────────────────────────────────────────────────────

export type ActionMenuItem = {
  key: string;
  label: ReactNode;
  icon?: ReactNode;
  onSelect?: () => void;
  disabled?: boolean;
  destructive?: boolean;
  separatorBefore?: boolean;
};

function ActionMenu({
  className,
  label = "更多操作",
  items,
  align = "end",
  trigger,
  ...props
}: {
  className?: string;
  /** 触发器可访问名（中文） */
  label?: string;
  items: ActionMenuItem[];
  align?: "start" | "center" | "end";
  /** 自定义触发器；默认 IconButton + MoreHorizontal */
  trigger?: ReactNode;
} & Omit<ComponentProps<typeof DropdownMenu>, "children">) {
  return (
    <DropdownMenu {...props}>
      <DropdownMenuTrigger asChild>
        {trigger ?? (
          <IconButton type="button" aria-label={label} className={className}>
            <MoreHorizontal className="size-4" />
          </IconButton>
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align={align}
        className="min-w-40 rounded-xl border-line bg-card p-1 text-ink shadow-pop"
      >
        {items.map((item) => (
          <div key={item.key}>
            {item.separatorBefore ? (
              <DropdownMenuSeparator className="my-1 bg-line" />
            ) : null}
            <DropdownMenuItem
              disabled={item.disabled}
              variant={item.destructive ? "destructive" : "default"}
              className={cn(
                "cursor-pointer rounded-lg px-2.5 py-2 text-[13px]",
                item.destructive
                  ? "text-danger focus:bg-danger-soft focus:text-danger"
                  : "focus:bg-card-soft focus:text-ink",
              )}
              onSelect={() => item.onSelect?.()}
            >
              {item.icon ? (
                <span className="[&_svg]:size-4">{item.icon}</span>
              ) : null}
              {item.label}
            </DropdownMenuItem>
          </div>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

// ─── Skeletons ───────────────────────────────────────────────────────────────

function SkeletonBlock({
  className,
  ...props
}: ComponentProps<"div">) {
  return (
    <div
      data-slot="skeleton-block"
      className={cn(
        "animate-pulse rounded-md bg-card-soft dark:bg-card-inner",
        className,
      )}
      {...props}
    />
  );
}

function TableSkeleton({
  className,
  rows = 5,
  cols = 4,
}: {
  className?: string;
  rows?: number;
  cols?: number;
}) {
  return (
    <div
      data-slot="table-skeleton"
      role="status"
      aria-busy="true"
      aria-label="表格加载中"
      className={cn("w-full overflow-hidden rounded-inner border border-line", className)}
    >
      <div className="grid gap-0 border-b border-line bg-card-soft px-4 py-3"
        style={{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))` }}
      >
        {Array.from({ length: cols }, (_, i) => (
          <SkeletonBlock key={`h-${i}`} className="h-3 w-16" />
        ))}
      </div>
      {Array.from({ length: rows }, (_, r) => (
        <div
          key={`r-${r}`}
          className="grid items-center gap-3 border-b border-line px-4 py-3 last:border-b-0"
          style={{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))` }}
        >
          {Array.from({ length: cols }, (_, c) => (
            <SkeletonBlock
              key={`c-${r}-${c}`}
              className={cn("h-3", c === 0 ? "w-[75%]" : "w-1/2")}
            />
          ))}
        </div>
      ))}
    </div>
  );
}

function CardGridSkeleton({
  className,
  count = 6,
}: {
  className?: string;
  count?: number;
}) {
  return (
    <div
      data-slot="card-grid-skeleton"
      role="status"
      aria-busy="true"
      aria-label="卡片列表加载中"
      className={cn(
        "grid gap-3 sm:grid-cols-2 xl:grid-cols-3",
        className,
      )}
    >
      {Array.from({ length: count }, (_, i) => (
        <div
          key={i}
          className="flex flex-col gap-3 rounded-card border border-line bg-card p-4 shadow-sm"
        >
          <div className="flex items-center gap-3">
            <SkeletonBlock className="size-11 rounded-[13px]" />
            <div className="min-w-0 flex-1 space-y-2">
              <SkeletonBlock className="h-4 w-[66%]" />
              <SkeletonBlock className="h-3 w-[33%]" />
            </div>
          </div>
          <SkeletonBlock className="h-3 w-full" />
          <SkeletonBlock className="h-3 w-[80%]" />
        </div>
      ))}
    </div>
  );
}

function DetailSkeleton({ className }: { className?: string }) {
  return (
    <div
      data-slot="detail-skeleton"
      role="status"
      aria-busy="true"
      aria-label="详情加载中"
      className={cn("grid gap-4", className)}
    >
      <div className="flex items-start gap-3.5 rounded-card border border-line bg-card p-5 shadow-sm">
        <SkeletonBlock className="size-12 rounded-[15px]" />
        <div className="min-w-0 flex-1 space-y-2">
          <SkeletonBlock className="h-6 w-48" />
          <SkeletonBlock className="h-3 w-72 max-w-full" />
          <SkeletonBlock className="h-3 w-40" />
        </div>
        <SkeletonBlock className="h-9 w-24 rounded-xl" />
      </div>
      <div className="rounded-card border border-line bg-card p-5 shadow-sm">
        <div className="grid gap-4 sm:grid-cols-2">
          {Array.from({ length: 6 }, (_, i) => (
            <div key={i} className="space-y-2">
              <SkeletonBlock className="h-3 w-16" />
              <SkeletonBlock className="h-4 w-[75%]" />
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

export {
  FactRow,
  EntityCard,
  ObjectHeader,
  DescriptionList,
  ActionMenu,
  SkeletonBlock,
  TableSkeleton,
  CardGridSkeleton,
  DetailSkeleton,
};

/*
 * ── 迁移边界（Batch B）──────────────────────────────────────────────────────
 *
 * EntityCard
 *   - 新目录网格优先用 EntityCard，不要复制 selected 边框/inset bar。
 *   - 整卡导航：外层 <Link> 包 EntityCard 时用 onClick 导航或把 card 放进 Link 的
 *     子树时避免卡内再放主按钮；次要动作用 actions 槽（已 stopPropagation）。
 *   - asChild：见下方 EntityCard 实现修正——应用 Slot 包住整卡根节点。
 *
 * ObjectHeader vs EmployeeDetailHeader
 *   - 新详情头用 ObjectHeader 组 leading/status/actions；业务特有预览/统计仍可本地扩展。
 *   - 触达 EmployeeDetailHeader 时可逐步把外壳换成 ObjectHeader，不一次强迁。
 *
 * DescriptionList vs 手写 dl/grid
 *   - 详情摘要、侧栏键值优先 DescriptionList；审计 mono 列用 mono。
 *
 * ActionMenu vs 手写 DropdownMenu+IconButton
 *   - 行/卡溢出操作优先 ActionMenu；危险项 destructive: true。
 *
 * Skeleton* vs LoadingState
 *   - 首屏尚无骨架：LoadingState / StateSurface isLoading。
 *   - 列表/表/详情已知布局的局部刷新：TableSkeleton / CardGridSkeleton / DetailSkeleton。
 *   - ui/skeleton 仅底层；业务面用本文件 Soft-Flat 骨架。
 */

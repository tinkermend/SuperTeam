import {
  forwardRef,
  type ComponentProps,
  type ReactNode,
} from "react";
import { XIcon } from "lucide-react";
import {
  Dialog,
  DialogClose,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import {
  Sheet,
  SheetClose,
  SheetDescription,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import * as DialogPrimitive from "@radix-ui/react-dialog";
import { cn } from "@/lib/utils";
import { IconTile, type Tone } from "@/components/superteam/primitives";

/**
 * Soft-Flat Batch A · 浮层 / 表单字段 / 行内提示 / 命令 keycap / 列表工具栏
 *
 * 规范：docs/design-system/{overlays,forms,form-flows,feedback,navigation,actions}.md
 * 与 ConfirmDialog / ShellPageHeader 的迁移边界见本文件底部注释与 DESIGN.md 概念表。
 */

// ─── tone helpers (keep local; avoid exporting internal maps from primitives) ─

const toneTextStrong: Record<Tone, string> = {
  brand: "text-brand-deep",
  info: "text-info-text",
  ok: "text-ok-text",
  warn: "text-warn-text",
  danger: "text-danger-text",
  mute: "text-mute-text",
  artifact: "text-artifact-text",
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

const toneBorder: Record<Tone, string> = {
  brand: "border-brand/25",
  info: "border-info/25",
  ok: "border-ok/25",
  warn: "border-warn/30",
  danger: "border-danger/30",
  mute: "border-line",
  artifact: "border-artifact/25",
};

// ─── Kbd (Keycap) ────────────────────────────────────────────────────────────

/**
 * 命令中心 / 快捷键 keycap。固定尺寸，hover/focus 不得改变布局尺寸。
 * 用法：`按 <Kbd>⌘</Kbd><Kbd>K</Kbd> 打开命令菜单`
 */
function Kbd({ className, ...props }: ComponentProps<"kbd">) {
  return (
    <kbd
      data-slot="kbd"
      className={cn(
        "inline-flex h-5 min-w-5 shrink-0 items-center justify-center rounded-md border border-line bg-card-soft px-1.5 font-mono text-[11px] font-medium leading-none text-ink-2 shadow-[0_1px_0_var(--line)]",
        className,
      )}
      {...props}
    />
  );
}

// ─── Callout ─────────────────────────────────────────────────────────────────

export type CalloutTone = Tone;

/**
 * 行内提示 / 次级 Banner。非阻断信息用此组件，整页失败仍用 ErrorState。
 */
function Callout({
  className,
  tone = "info",
  title,
  description,
  icon,
  action,
  children,
  ...props
}: ComponentProps<"div"> & {
  tone?: CalloutTone;
  title?: ReactNode;
  description?: ReactNode;
  icon?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div
      data-slot="callout"
      data-tone={tone}
      role="note"
      className={cn(
        "flex gap-3 rounded-inner border px-3.5 py-3",
        toneSoftBg[tone],
        toneBorder[tone],
        className,
      )}
      {...props}
    >
      {icon ? (
        <IconTile tone={tone} size="sm" className="mt-0.5 shadow-none">
          {icon}
        </IconTile>
      ) : null}
      <div className="min-w-0 flex-1">
        {title ? (
          <p className={cn("text-[13px] font-semibold leading-5", toneTextStrong[tone])}>
            {title}
          </p>
        ) : null}
        {description ? (
          <p
            className={cn(
              "text-[13px] leading-5 text-ink-2",
              title ? "mt-0.5" : undefined,
            )}
          >
            {description}
          </p>
        ) : null}
        {children ? <div className={cn("text-[13px] text-ink-2", title || description ? "mt-1.5" : undefined)}>{children}</div> : null}
        {action ? <div className="mt-2 flex flex-wrap gap-2">{action}</div> : null}
      </div>
    </div>
  );
}

// ─── Field / FormSection ─────────────────────────────────────────────────────

/**
 * 表单字段壳（与 react-hook-form 的 `FormField` 无关）。
 * 控件本体仍用 `components/ui` 的 Input/Select/Textarea；本组件只统一 label/hint/error 节奏。
 */
function Field({
  className,
  label,
  htmlFor,
  required,
  hint,
  error,
  children,
  ...props
}: ComponentProps<"div"> & {
  label?: ReactNode;
  htmlFor?: string;
  required?: boolean;
  hint?: ReactNode;
  error?: ReactNode;
}) {
  const describedBy = [
    hint && htmlFor ? `${htmlFor}-hint` : null,
    error && htmlFor ? `${htmlFor}-error` : null,
  ]
    .filter(Boolean)
    .join(" ") || undefined;

  return (
    <div data-slot="field" className={cn("grid gap-1.5", className)} {...props}>
      {label ? (
        <label
          htmlFor={htmlFor}
          className="text-[13px] font-medium text-ink"
        >
          {label}
          {required ? (
            <span className="ms-0.5 text-danger" aria-hidden>
              *
            </span>
          ) : null}
        </label>
      ) : null}
      <div
        data-slot="field-control"
        className="min-w-0"
        // Consumers should forward aria-describedby onto the control when needed.
        data-describedby={describedBy}
      >
        {children}
      </div>
      {hint && !error ? (
        <p
          id={htmlFor ? `${htmlFor}-hint` : undefined}
          data-slot="field-hint"
          className="text-xs text-ink-3"
        >
          {hint}
        </p>
      ) : null}
      {error ? (
        <p
          id={htmlFor ? `${htmlFor}-error` : undefined}
          data-slot="field-error"
          role="alert"
          className="text-xs text-danger"
        >
          {error}
        </p>
      ) : null}
    </div>
  );
}

/** 表单分组：标题 + 可选说明 + 字段堆叠。 */
function FormSection({
  className,
  title,
  description,
  children,
  ...props
}: ComponentProps<"section"> & {
  title?: ReactNode;
  description?: ReactNode;
}) {
  return (
    <section
      data-slot="form-section"
      className={cn("grid gap-4", className)}
      {...props}
    >
      {title || description ? (
        <header className="grid gap-1">
          {title ? (
            <h3 className="text-[15px] font-bold tracking-tight text-ink">{title}</h3>
          ) : null}
          {description ? (
            <p className="text-[13px] text-ink-2">{description}</p>
          ) : null}
        </header>
      ) : null}
      <div data-slot="form-section-body" className="grid gap-4">
        {children}
      </div>
    </section>
  );
}

// ─── ListToolbar ─────────────────────────────────────────────────────────────

/**
 * 目录 / 密表上方筛条骨架。
 * 槽位：search（通常放 ToolbarSearch）/ filters（Chip 组）/ segments / actions / children。
 */
function ListToolbar({
  className,
  search,
  filters,
  segments,
  actions,
  children,
  ...props
}: ComponentProps<"div"> & {
  search?: ReactNode;
  filters?: ReactNode;
  segments?: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <div
      data-slot="list-toolbar"
      className={cn(
        "flex flex-col gap-3 border-b border-line px-4 py-3 sm:flex-row sm:flex-wrap sm:items-center",
        className,
      )}
      {...props}
    >
      {search ? (
        <div data-slot="list-toolbar-search" className="min-w-0 flex-1 sm:min-w-[200px]">
          {search}
        </div>
      ) : null}
      {filters ? (
        <div
          data-slot="list-toolbar-filters"
          className="flex min-w-0 flex-wrap items-center gap-2"
        >
          {filters}
        </div>
      ) : null}
      {segments ? (
        <div data-slot="list-toolbar-segments" className="shrink-0">
          {segments}
        </div>
      ) : null}
      {children}
      {actions ? (
        <div
          data-slot="list-toolbar-actions"
          className="flex shrink-0 flex-wrap items-center gap-2 sm:ms-auto"
        >
          {actions}
        </div>
      ) : null}
    </div>
  );
}

// ─── Soft Dialog ─────────────────────────────────────────────────────────────

const SoftDialog = Dialog;
const SoftDialogTrigger = DialogTrigger;
const SoftDialogClose = DialogClose;

export type SoftDialogSize = "sm" | "md" | "lg";

const softDialogSizeClass: Record<SoftDialogSize, string> = {
  sm: "sm:max-w-md",
  md: "sm:max-w-lg",
  lg: "sm:max-w-2xl",
};

const SoftDialogContent = forwardRef<
  HTMLDivElement,
  ComponentProps<typeof DialogPrimitive.Content> & {
    size?: SoftDialogSize;
    showCloseButton?: boolean;
  }
>(function SoftDialogContent(
  { className, children, size = "md", showCloseButton = true, ...props },
  ref,
) {
  return (
    <DialogPortal>
      <DialogOverlay className="bg-ink/40 backdrop-blur-[2px]" />
      <DialogPrimitive.Content
        ref={ref}
        data-slot="soft-dialog-content"
        data-size={size}
        className={cn(
          "fixed top-[50%] left-[50%] z-50 grid w-full max-w-[calc(100%-2rem)] translate-x-[-50%] translate-y-[-50%] gap-0 overflow-hidden rounded-card border border-line bg-card p-0 text-ink shadow-pop duration-200",
          "data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95",
          softDialogSizeClass[size],
          className,
        )}
        {...props}
      >
        {children}
        {showCloseButton ? (
          <DialogPrimitive.Close
            data-slot="soft-dialog-close"
            className={cn(
              "absolute end-3 top-3 inline-grid size-8 place-items-center rounded-xl bg-card-soft text-ink-2 transition-colors",
              "hover:bg-card-inner hover:text-ink",
              "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 focus-visible:ring-offset-card",
              "disabled:pointer-events-none disabled:opacity-40",
            )}
          >
            <XIcon className="size-4" />
            <span className="sr-only">关闭</span>
          </DialogPrimitive.Close>
        ) : null}
      </DialogPrimitive.Content>
    </DialogPortal>
  );
});

function SoftDialogHeader({
  className,
  icon,
  iconTone = "brand",
  children,
  ...props
}: ComponentProps<"div"> & {
  icon?: ReactNode;
  iconTone?: Tone;
}) {
  return (
    <div
      data-slot="soft-dialog-header"
      className={cn(
        "flex items-start gap-3 border-b border-line px-6 py-5 pe-12",
        className,
      )}
      {...props}
    >
      {icon ? <IconTile tone={iconTone}>{icon}</IconTile> : null}
      <div className="grid min-w-0 flex-1 gap-1.5">{children}</div>
    </div>
  );
}

function SoftDialogTitle({
  className,
  ...props
}: ComponentProps<typeof DialogTitle>) {
  return (
    <DialogTitle
      data-slot="soft-dialog-title"
      className={cn(
        "text-xl font-extrabold tracking-tight text-ink",
        className,
      )}
      {...props}
    />
  );
}

function SoftDialogDescription({
  className,
  ...props
}: ComponentProps<typeof DialogDescription>) {
  return (
    <DialogDescription
      data-slot="soft-dialog-description"
      className={cn("text-[15px] leading-snug text-ink-2", className)}
      {...props}
    />
  );
}

function SoftDialogBody({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      data-slot="soft-dialog-body"
      className={cn("max-h-[min(60vh,28rem)] overflow-y-auto px-6 py-5", className)}
      {...props}
    />
  );
}

function SoftDialogFooter({
  className,
  left,
  children,
  ...props
}: ComponentProps<"div"> & { left?: ReactNode }) {
  return (
    <div
      data-slot="soft-dialog-footer"
      className={cn(
        "flex flex-col-reverse gap-3 border-t border-line bg-card-soft/60 px-6 py-4 sm:flex-row sm:items-center sm:justify-end sm:gap-2",
        className,
      )}
      {...props}
    >
      {left ? (
        <div className="min-w-0 flex-1 text-[13px] text-ink-3 sm:me-auto sm:pe-3">
          {left}
        </div>
      ) : null}
      {children}
    </div>
  );
}

// ─── Soft Sheet ──────────────────────────────────────────────────────────────

const SoftSheet = Sheet;
const SoftSheetTrigger = SheetTrigger;
const SoftSheetClose = SheetClose;

export type SoftSheetSide = "top" | "right" | "bottom" | "left";
export type SoftSheetSize = "md" | "lg";

const softSheetWidth: Record<SoftSheetSize, string> = {
  md: "sm:max-w-md",
  lg: "sm:max-w-lg",
};

const SoftSheetContent = forwardRef<
  HTMLDivElement,
  ComponentProps<typeof DialogPrimitive.Content> & {
    side?: SoftSheetSide;
    size?: SoftSheetSize;
    showCloseButton?: boolean;
  }
>(function SoftSheetContent(
  {
    className,
    children,
    side = "right",
    size = "md",
    showCloseButton = true,
    ...props
  },
  ref,
) {
  const horizontal = side === "right" || side === "left";
  return (
    <DialogPrimitive.Portal>
      <DialogPrimitive.Overlay
        data-slot="soft-sheet-overlay"
        className="fixed inset-0 z-50 bg-ink/40 backdrop-blur-[2px] data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:animate-in data-[state=open]:fade-in-0"
      />
      <DialogPrimitive.Content
        ref={ref}
        data-slot="soft-sheet-content"
        data-side={side}
        data-size={size}
        className={cn(
          "fixed z-50 flex flex-col gap-0 bg-card text-ink shadow-pop transition ease-in-out",
          "data-[state=closed]:animate-out data-[state=closed]:duration-300 data-[state=open]:animate-in data-[state=open]:duration-400",
          side === "right" &&
            "inset-y-0 end-0 h-full w-full border-s border-line data-[state=closed]:slide-out-to-end data-[state=open]:slide-in-from-end",
          side === "left" &&
            "inset-y-0 start-0 h-full w-full border-e border-line data-[state=closed]:slide-out-to-start data-[state=open]:slide-in-from-start",
          side === "top" &&
            "inset-x-0 top-0 h-auto max-h-[85vh] border-b border-line data-[state=closed]:slide-out-to-top data-[state=open]:slide-in-from-top",
          side === "bottom" &&
            "inset-x-0 bottom-0 h-auto max-h-[85vh] border-t border-line data-[state=closed]:slide-out-to-bottom data-[state=open]:slide-in-from-bottom",
          horizontal && cn("w-3/4", softSheetWidth[size]),
          className,
        )}
        {...props}
      >
        {children}
        {showCloseButton ? (
          <DialogPrimitive.Close
            data-slot="soft-sheet-close"
            className={cn(
              "absolute end-3 top-3 inline-grid size-8 place-items-center rounded-xl bg-card-soft text-ink-2 transition-colors",
              "hover:bg-card-inner hover:text-ink",
              "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 focus-visible:ring-offset-card",
              "disabled:pointer-events-none disabled:opacity-40",
            )}
          >
            <XIcon className="size-4" />
            <span className="sr-only">关闭</span>
          </DialogPrimitive.Close>
        ) : null}
      </DialogPrimitive.Content>
    </DialogPrimitive.Portal>
  );
});

function SoftSheetHeader({
  className,
  icon,
  iconTone = "brand",
  children,
  ...props
}: ComponentProps<"div"> & {
  icon?: ReactNode;
  iconTone?: Tone;
}) {
  return (
    <div
      data-slot="soft-sheet-header"
      className={cn(
        "flex shrink-0 items-start gap-3 border-b border-line px-5 py-4 pe-12",
        className,
      )}
      {...props}
    >
      {icon ? <IconTile tone={iconTone} size="sm">{icon}</IconTile> : null}
      <div className="grid min-w-0 flex-1 gap-1">{children}</div>
    </div>
  );
}

function SoftSheetTitle({
  className,
  ...props
}: ComponentProps<typeof SheetTitle>) {
  return (
    <SheetTitle
      data-slot="soft-sheet-title"
      className={cn("text-lg font-extrabold tracking-tight text-ink", className)}
      {...props}
    />
  );
}

function SoftSheetDescription({
  className,
  ...props
}: ComponentProps<typeof SheetDescription>) {
  return (
    <SheetDescription
      data-slot="soft-sheet-description"
      className={cn("text-[13px] text-ink-2", className)}
      {...props}
    />
  );
}

function SoftSheetBody({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      data-slot="soft-sheet-body"
      className={cn("min-h-0 flex-1 overflow-y-auto px-5 py-4", className)}
      {...props}
    />
  );
}

function SoftSheetFooter({
  className,
  left,
  children,
  ...props
}: ComponentProps<"div"> & { left?: ReactNode }) {
  return (
    <div
      data-slot="soft-sheet-footer"
      className={cn(
        "mt-auto flex shrink-0 flex-col-reverse gap-3 border-t border-line bg-card-soft/60 px-5 py-4 sm:flex-row sm:items-center sm:justify-end sm:gap-2",
        className,
      )}
      {...props}
    >
      {left ? (
        <div className="min-w-0 flex-1 text-[13px] text-ink-3 sm:me-auto sm:pe-3">
          {left}
        </div>
      ) : null}
      {children}
    </div>
  );
}

export {
  Kbd,
  Callout,
  Field,
  FormSection,
  ListToolbar,
  SoftDialog,
  SoftDialogTrigger,
  SoftDialogClose,
  SoftDialogContent,
  SoftDialogHeader,
  SoftDialogTitle,
  SoftDialogDescription,
  SoftDialogBody,
  SoftDialogFooter,
  SoftSheet,
  SoftSheetTrigger,
  SoftSheetClose,
  SoftSheetContent,
  SoftSheetHeader,
  SoftSheetTitle,
  SoftSheetDescription,
  SoftSheetBody,
  SoftSheetFooter,
};

/*
 * ── 迁移边界 ────────────────────────────────────────────────────────────────
 *
 * ConfirmDialog（apps/web/src/components/confirm-dialog.tsx）
 *   - 保留：简单确认 / 危险确认 / form submit 确认的一站式 API（AlertDialog 语义）。
 *   - 视觉与 SoftDialog 对齐同一套 token/class；新确认框优先 ConfirmDialog，
 *     不要用 SoftDialog 重写等价确认流，除非需要自定义 body 结构。
 *
 * 业务 Dialog / Sheet
 *   - 新代码：from '@/components/superteam' 的 SoftDialog* / SoftSheet*。
 *   - 触达旧代码时从 ui/dialog、ui/sheet 迁移；勿在 feature 内复制 rounded-card/shadow-pop。
 *   - ui/dialog、ui/sheet 仍可作为 Radix 行为 primitive，供 Soft* 与白名单复合组件使用。
 *
 * ShellPageHeader / PageHeader
 *   - 页面级标题仍用 ShellPageHeader（layout）或 PageHeader（superteam）。
 *   - SoftDialogHeader / SoftSheetHeader 仅用于浮层，不替代页头。
 *
 * Field vs ui/form FormField
 *   - Field = 视觉壳；RHF 绑定继续用 ui/form 的 FormField/FormItem，或自行把 Input 放进 Field。
 *   - 不要把业务校验逻辑写进 Field。
 *
 * ListToolbar vs data-table/toolbar
 *   - ListToolbar 是 Soft-Flat 槽位骨架，目录卡网格与非 TanStack 表优先用它。
 *   - components/data-table/* 可逐步把外壳 class 对齐 ListToolbar，但不强制一次替换。
 */

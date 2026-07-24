import { cva, type VariantProps } from "class-variance-authority";

/**
 * Soft-Flat 按钮样式唯一事实源。
 * superteam Button 与 ui/button 共用；业务请从 @/components/superteam 导入 Button。
 */
export const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 rounded-xl font-semibold transition-all duration-200 ease-out active:scale-[0.97] disabled:pointer-events-none disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/60 focus-visible:ring-offset-1 focus-visible:ring-offset-background whitespace-nowrap [&_svg]:pointer-events-none [&_svg:not([class*='size-'])]:size-4 shrink-0 [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        primary: "bg-brand text-white shadow-card hover:bg-brand-deep",
        default: "bg-brand text-white shadow-card hover:bg-brand-deep",
        outline:
          "bg-card text-ink border border-line-strong hover:bg-card-soft",
        secondary:
          "bg-card-soft text-ink border border-line hover:bg-card",
        ghost: "bg-transparent text-ink-2 hover:bg-card-soft hover:text-ink",
        danger: "bg-danger-soft text-danger hover:brightness-95",
        destructive: "bg-danger-soft text-danger hover:brightness-95",
        glass:
          "border border-[color:var(--aurora-accent-line)] bg-[color:var(--aurora-panel)] text-brand-deep hover:bg-[color:var(--aurora-accent-soft)]",
        link: "text-brand underline-offset-4 hover:underline bg-transparent shadow-none",
      },
      size: {
        default: "h-10 px-4 text-[13px] has-[>svg]:px-3",
        sm: "h-8 px-3 text-xs gap-1.5 has-[>svg]:px-2.5",
        lg: "h-10 px-6 text-[13px] has-[>svg]:px-4",
        icon: "size-9 p-0",
      },
    },
    defaultVariants: {
      variant: "primary",
      size: "default",
    },
  },
);

export type ButtonVariantProps = VariantProps<typeof buttonVariants>;

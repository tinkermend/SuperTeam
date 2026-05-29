import type { ReactNode } from "react";

import { cn } from "./utils";

type IconBadgeTone = "info" | "success" | "warning" | "danger" | "neutral" | "accent";

type IconBadgeProps = {
  children: ReactNode;
  label: string;
  tone?: IconBadgeTone;
};

const toneClassNames: Record<IconBadgeTone, string> = {
  accent: "border-status-accent/20 bg-status-accent/10 text-status-accent",
  danger: "border-status-danger/20 bg-status-danger/10 text-status-danger",
  info: "border-status-info/20 bg-status-info/10 text-status-info",
  neutral: "border-border bg-muted text-muted-foreground",
  success: "border-status-success/20 bg-status-success/10 text-status-success",
  warning: "border-status-warning/20 bg-status-warning/10 text-status-warning",
};

export function IconBadge({ children, label, tone = "neutral" }: IconBadgeProps) {
  return (
    <span
      aria-label={label}
      className={cn("inline-flex size-9 shrink-0 items-center justify-center rounded-md border", toneClassNames[tone])}
      data-tone={tone}
      role="img"
    >
      {children}
    </span>
  );
}

import type { ReactNode } from "react";

import { cn } from "./utils";

type StatusBadgeProps = {
  children: ReactNode;
  tone: "ok" | "warning" | "danger" | "neutral";
};

const toneClassNames: Record<StatusBadgeProps["tone"], string> = {
  danger: "border-status-danger/20 bg-status-danger/10 text-status-danger",
  neutral: "border-border bg-muted text-muted-foreground",
  ok: "border-status-success/20 bg-status-success/10 text-status-success",
  warning: "border-status-warning/20 bg-status-warning/10 text-status-warning",
};

export function StatusBadge({ children, tone }: StatusBadgeProps) {
  return (
    <span
      className={cn("inline-flex h-6 items-center rounded border px-2 text-xs font-medium", toneClassNames[tone])}
      data-tone={tone}
    >
      {children}
    </span>
  );
}

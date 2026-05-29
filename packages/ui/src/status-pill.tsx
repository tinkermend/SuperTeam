import type { ReactNode } from "react";

import { cn } from "./utils";

type StatusPillTone = "info" | "success" | "warning" | "danger" | "neutral" | "accent";

type StatusPillProps = {
  children: ReactNode;
  description?: ReactNode;
  tone?: StatusPillTone;
};

const toneClassNames: Record<StatusPillTone, string> = {
  accent: "border-status-accent/20 bg-status-accent/10 text-status-accent",
  danger: "border-status-danger/20 bg-status-danger/10 text-status-danger",
  info: "border-status-info/20 bg-status-info/10 text-status-info",
  neutral: "border-border bg-muted text-muted-foreground",
  success: "border-status-success/20 bg-status-success/10 text-status-success",
  warning: "border-status-warning/20 bg-status-warning/10 text-status-warning",
};

export function StatusPill({ children, description, tone = "neutral" }: StatusPillProps) {
  return (
    <span className="inline-flex min-w-0 items-center gap-2">
      <span
        className={cn(
          "inline-flex min-h-6 min-w-0 items-center gap-1.5 break-words rounded-full border px-2 text-xs font-medium",
          toneClassNames[tone],
        )}
        data-tone={tone}
      >
        <span className="size-1.5 rounded-full bg-current" aria-hidden="true" />
        {children}
      </span>
      {description ? <span className="min-w-0 break-words text-xs text-muted-foreground">{description}</span> : null}
    </span>
  );
}

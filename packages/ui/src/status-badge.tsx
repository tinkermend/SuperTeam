import type { ReactNode } from "react";

type StatusBadgeProps = {
  children: ReactNode;
  tone: "ok" | "warning" | "danger" | "neutral";
};

const toneClassNames: Record<StatusBadgeProps["tone"], string> = {
  danger: "border-red-200 bg-red-50 text-red-700",
  neutral: "border-slate-200 bg-slate-50 text-slate-700",
  ok: "border-emerald-200 bg-emerald-50 text-emerald-700",
  warning: "border-amber-200 bg-amber-50 text-amber-700",
};

export function StatusBadge({ children, tone }: StatusBadgeProps) {
  return (
    <span
      className={`inline-flex h-6 items-center rounded border px-2 text-xs font-medium ${toneClassNames[tone]}`}
      data-tone={tone}
    >
      {children}
    </span>
  );
}

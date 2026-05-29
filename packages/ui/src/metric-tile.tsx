import type { ReactNode } from "react";

type MetricTileProps = {
  icon?: ReactNode;
  label: string;
  meta?: ReactNode;
  status?: ReactNode;
  value: ReactNode;
};

export function MetricTile({ icon, label, meta, status, value }: MetricTileProps) {
  return (
    <section
      aria-label={label}
      className="flex min-h-28 min-w-0 flex-col justify-between rounded-md border bg-card p-4 text-card-foreground shadow-sm"
      role="group"
    >
      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="truncate text-sm text-muted-foreground">{label}</p>
          <p className="mt-2 text-2xl font-semibold leading-none">{value}</p>
        </div>
        {icon}
      </div>
      <div className="mt-4 flex min-w-0 items-center justify-between gap-3 text-xs text-muted-foreground">
        <span className="min-w-0 break-words">{meta}</span>
        {status ? <span className="shrink-0 font-medium text-foreground">{status}</span> : null}
      </div>
    </section>
  );
}

import type { ReactNode } from "react";

type TimelineItemProps = {
  description?: ReactNode;
  status?: ReactNode;
  time: ReactNode;
  title: ReactNode;
};

export function TimelineItem({ description, status, time, title }: TimelineItemProps) {
  return (
    <li className="grid grid-cols-[auto_minmax(0,1fr)] gap-3">
      <span className="mt-1.5 size-2.5 rounded-full bg-primary" aria-hidden="true" />
      <div className="min-w-0 border-b pb-3 last:border-b-0">
        <div className="flex min-w-0 items-start justify-between gap-3">
          <p className="min-w-0 break-words text-sm font-medium">{title}</p>
          <time className="shrink-0 text-xs text-muted-foreground">{time}</time>
        </div>
        {description ? <p className="mt-1 break-words text-sm text-muted-foreground">{description}</p> : null}
        {status ? <p className="mt-2 break-words text-xs font-medium text-foreground">{status}</p> : null}
      </div>
    </li>
  );
}

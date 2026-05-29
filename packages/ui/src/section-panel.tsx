import type { ReactNode } from "react";

type SectionPanelProps = {
  action?: ReactNode;
  children: ReactNode;
  description?: ReactNode;
  title: ReactNode;
};

export function SectionPanel({ action, children, description, title }: SectionPanelProps) {
  return (
    <section className="min-w-0 rounded-md border bg-card text-card-foreground shadow-sm">
      <header className="flex min-w-0 flex-col items-start justify-between gap-3 border-b px-4 py-3 sm:flex-row sm:gap-4">
        <div className="min-w-0">
          <h2 className="break-words text-sm font-semibold">{title}</h2>
          {description ? <p className="mt-1 break-all text-sm text-muted-foreground sm:break-normal">{description}</p> : null}
        </div>
        {action ? <div className="shrink-0">{action}</div> : null}
      </header>
      <div className="min-w-0 break-all p-4 sm:break-normal">{children}</div>
    </section>
  );
}

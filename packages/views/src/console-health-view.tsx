import { StatusBadge } from "@superteam/ui";

type ConsoleHealthViewProps = {
  summary: string;
};

export function ConsoleHealthView({ summary }: ConsoleHealthViewProps) {
  return (
    <section className="grid gap-3 rounded-md border bg-card p-4 text-card-foreground shadow-sm">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-base font-semibold">Control Plane</h2>
        <StatusBadge tone="ok">Live</StatusBadge>
      </div>
      <p className="text-sm text-muted-foreground">{summary}</p>
    </section>
  );
}

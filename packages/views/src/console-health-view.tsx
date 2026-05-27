import { StatusBadge } from "@superteam/ui";

type ConsoleHealthViewProps = {
  summary: string;
};

export function ConsoleHealthView({ summary }: ConsoleHealthViewProps) {
  return (
    <section className="grid gap-3 rounded-md border border-slate-200 bg-white p-4 shadow-sm">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-base font-semibold text-slate-950">Control Plane</h2>
        <StatusBadge tone="ok">Live</StatusBadge>
      </div>
      <p className="text-sm text-slate-600">{summary}</p>
    </section>
  );
}

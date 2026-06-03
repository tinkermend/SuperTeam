import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import type { TeamOverview } from "@/lib/api/teams";

type TeamOverviewTabProps = {
  overview: TeamOverview;
};

export function TeamOverviewTab({ overview }: TeamOverviewTabProps) {
  return (
    <div className="grid gap-4 md:grid-cols-4">
      <OverviewMetric label="成员" value={overview.member_count} />
      <OverviewMetric label="数字员工" value={overview.digital_employee_count} />
      <OverviewMetric label="能力" value={overview.capability_count} />
      <OverviewMetric label="待批准" value={overview.pending_item_count} />
      <div className="md:col-span-4">
        <Separator />
      </div>
      <div className="flex flex-col gap-2 md:col-span-4">
        <p className="text-sm font-medium">治理版本</p>
        {overview.current_revision ? (
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <Badge variant="secondary">v{overview.current_revision.revision_number}</Badge>
            <span className="text-muted-foreground">{overview.current_revision.status}</span>
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">当前团队尚未启用治理版本。</p>
        )}
      </div>
    </div>
  );
}

function OverviewMetric({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-md border p-3">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="mt-2 text-2xl font-semibold">{value}</p>
    </div>
  );
}

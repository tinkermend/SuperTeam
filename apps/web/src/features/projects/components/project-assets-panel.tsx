import {
  SoftTabs,
  SoftTabsContent,
  SoftTabsList,
  SoftTabsTrigger,
} from "@/components/superteam";
import type {
  ProjectAcceptanceRecord,
  ProjectArtifactRef,
  ProjectBudgetLedgerEntry,
  ProjectBudgetSummary,
  ProjectReportRef
} from "@/lib/api/projects";
import { ProjectAcceptancePanel } from "./project-acceptance-panel";
import { ProjectArtifactReportPanel } from "./project-artifact-report-panel";
import { ProjectBudgetPanel } from "./project-budget-panel";

const assetsTabTriggerClass =
  "h-8 flex-none rounded-[8px] border-0 px-3 py-1.5 text-[12.5px] font-semibold text-ink-2 shadow-none transition-colors data-[state=active]:bg-brand-soft data-[state=active]:text-brand-deep data-[state=active]:shadow-none data-[state=inactive]:hover:bg-card-soft data-[state=inactive]:hover:text-ink";

type ProjectAssetsPanelProps = {
  acceptance?: ProjectAcceptanceRecord;
  artifacts?: ProjectArtifactRef[];
  budgetLedger?: ProjectBudgetLedgerEntry[];
  budgetSummary?: ProjectBudgetSummary;
  initialTab?: "artifacts" | "budget" | "acceptance";
  reports?: ProjectReportRef[];
};

/** 资产区：工件 + 预算 + 结项结论（只读；§6.3 下线自由验收写入口）。 */
export function ProjectAssetsPanel({
  acceptance,
  artifacts = [],
  budgetLedger = [],
  budgetSummary,
  initialTab = "artifacts",
  reports = []
}: ProjectAssetsPanelProps) {
  return (
    <SoftTabs className="flex w-full min-w-0 flex-col gap-3" defaultValue={initialTab}>
      <div className="min-w-0 overflow-x-auto pb-1">
        <SoftTabsList
          aria-label="项目资产"
          className="h-auto w-max min-w-full max-w-none justify-start gap-1 overflow-visible rounded-[12px] bg-card p-1 text-ink shadow-card sm:min-w-0"
        >
          <SoftTabsTrigger className={assetsTabTriggerClass} value="artifacts">
            工件
          </SoftTabsTrigger>
          <SoftTabsTrigger className={assetsTabTriggerClass} value="budget">
            预算
          </SoftTabsTrigger>
          <SoftTabsTrigger className={assetsTabTriggerClass} value="acceptance">
            结项
          </SoftTabsTrigger>
        </SoftTabsList>
      </div>

      <SoftTabsContent className="m-0" value="artifacts">
        <ProjectArtifactReportPanel artifacts={artifacts} reports={reports} />
      </SoftTabsContent>
      <SoftTabsContent className="m-0" value="budget">
        <ProjectBudgetPanel budgetLedger={budgetLedger} budgetSummary={budgetSummary} />
      </SoftTabsContent>
      <SoftTabsContent className="m-0" value="acceptance">
        <ProjectAcceptancePanel acceptance={acceptance} />
      </SoftTabsContent>
    </SoftTabs>
  );
}

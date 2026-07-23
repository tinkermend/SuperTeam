import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type {
  CreateProjectAcceptanceInput,
  ProjectAcceptanceRecord,
  ProjectArtifactRef,
  ProjectBudgetLedgerEntry,
  ProjectBudgetSummary,
  ProjectReportRef,
} from "@/lib/api/projects";
import { ProjectAcceptancePanel } from "./project-acceptance-panel";
import { ProjectArtifactReportPanel } from "./project-artifact-report-panel";
import { ProjectBudgetPanel } from "./project-budget-panel";

const assetsTabTriggerClass =
  "h-8 flex-none rounded-[8px] border-0 px-3 py-1.5 text-[12.5px] font-semibold text-v3-ink-2 shadow-none transition-colors data-[state=active]:bg-v3-brand-soft data-[state=active]:text-v3-brand-deep data-[state=active]:shadow-none data-[state=inactive]:hover:bg-v3-card-soft data-[state=inactive]:hover:text-v3-ink";

type ProjectAssetsPanelProps = {
  acceptance?: ProjectAcceptanceRecord;
  artifacts?: ProjectArtifactRef[];
  budgetLedger?: ProjectBudgetLedgerEntry[];
  budgetSummary?: ProjectBudgetSummary;
  initialTab?: "artifacts" | "budget" | "acceptance";
  onCreateAcceptance: (input: CreateProjectAcceptanceInput) => void;
  reports?: ProjectReportRef[];
};

/** 资产区：工件 + 预算 + 验收（合并原三个对等顶栏 Tab）。 */
export function ProjectAssetsPanel({
  acceptance,
  artifacts = [],
  budgetLedger = [],
  budgetSummary,
  initialTab = "artifacts",
  onCreateAcceptance,
  reports = [],
}: ProjectAssetsPanelProps) {
  return (
    <Tabs className="flex w-full min-w-0 flex-col gap-3" defaultValue={initialTab}>
      <div className="min-w-0 overflow-x-auto pb-1">
        <TabsList
          aria-label="项目资产"
          className="h-auto w-max min-w-full max-w-none justify-start gap-1 overflow-visible rounded-[12px] bg-v3-card p-1 text-v3-ink shadow-v3 sm:min-w-0"
        >
          <TabsTrigger className={assetsTabTriggerClass} value="artifacts">
            工件
          </TabsTrigger>
          <TabsTrigger className={assetsTabTriggerClass} value="budget">
            预算
          </TabsTrigger>
          <TabsTrigger className={assetsTabTriggerClass} value="acceptance">
            验收
          </TabsTrigger>
        </TabsList>
      </div>

      <TabsContent className="m-0" value="artifacts">
        <ProjectArtifactReportPanel artifacts={artifacts} reports={reports} />
      </TabsContent>
      <TabsContent className="m-0" value="budget">
        <ProjectBudgetPanel budgetLedger={budgetLedger} budgetSummary={budgetSummary} />
      </TabsContent>
      <TabsContent className="m-0" value="acceptance">
        <ProjectAcceptancePanel acceptance={acceptance} onCreateAcceptance={onCreateAcceptance} />
      </TabsContent>
    </Tabs>
  );
}

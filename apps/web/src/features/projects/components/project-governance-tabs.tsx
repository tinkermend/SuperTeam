import { Tabs, TabsContent } from "@/components/ui/tabs";
import { LiquidTabsList, LiquidTabsTrigger } from "@/components/superteam";
import type {
  ProjectAcceptanceRecord,
  ProjectArchivePreview,
  ProjectArchiveSnapshot,
  ProjectArtifactRef,
  ProjectBudgetLedgerEntry,
  ProjectBudgetSummary,
  ProjectEvidenceRef,
  ProjectReportRef,
} from "@/lib/api/projects";
import { ProjectAcceptancePanel } from "./project-acceptance-panel";
import { ProjectArchivePanel } from "./project-archive-panel";
import { ProjectArtifactReportPanel } from "./project-artifact-report-panel";
import { ProjectBudgetPanel } from "./project-budget-panel";
import { ProjectEvidencePanel } from "./project-evidence-panel";

type ProjectGovernanceTabsProps = {
  acceptance?: ProjectAcceptanceRecord;
  archivePreview?: ProjectArchivePreview;
  archiveSnapshots?: ProjectArchiveSnapshot[];
  artifacts?: ProjectArtifactRef[];
  budgetLedger?: ProjectBudgetLedgerEntry[];
  budgetSummary?: ProjectBudgetSummary;
  decisionRequestCount: number;
  demandCount: number;
  evidence?: ProjectEvidenceRef[];
  executionSummaryCount: number;
  reports?: ProjectReportRef[];
  routeDecisionCount: number;
  taskCount: number;
};

export function ProjectGovernanceTabs({
  acceptance,
  archivePreview,
  archiveSnapshots = [],
  artifacts = [],
  budgetLedger = [],
  budgetSummary,
  decisionRequestCount,
  demandCount,
  evidence = [],
  executionSummaryCount,
  reports = [],
  routeDecisionCount,
  taskCount,
}: ProjectGovernanceTabsProps) {
  const unresolvedRiskCount = acceptance?.unresolved_risks.length ?? 0;

  return (
    <Tabs className="flex w-full min-w-0 flex-col gap-3" defaultValue="evidence">
      <div className="w-full min-w-0 max-w-full overflow-x-auto overflow-y-hidden pb-1 [-webkit-overflow-scrolling:touch]">
        <LiquidTabsList
          aria-label="项目详情治理视图"
          className="w-max min-w-full max-w-none flex-nowrap"
        >
          <LiquidTabsTrigger className="flex-none shrink-0" value="evidence">
            证据链
          </LiquidTabsTrigger>
          <LiquidTabsTrigger className="flex-none shrink-0" value="artifacts">
            工件报告
          </LiquidTabsTrigger>
          <LiquidTabsTrigger className="flex-none shrink-0" value="budget">
            预算流水
          </LiquidTabsTrigger>
          <LiquidTabsTrigger className="flex-none shrink-0" value="acceptance">
            验收结论
          </LiquidTabsTrigger>
          <LiquidTabsTrigger className="flex-none shrink-0" value="archive">
            归档预览
          </LiquidTabsTrigger>
        </LiquidTabsList>
      </div>

      <TabsContent className="m-0" value="evidence">
        <ProjectEvidencePanel evidence={evidence} />
      </TabsContent>
      <TabsContent className="m-0" value="artifacts">
        <ProjectArtifactReportPanel artifacts={artifacts} reports={reports} />
      </TabsContent>
      <TabsContent className="m-0" value="budget">
        <ProjectBudgetPanel
          budgetLedger={budgetLedger}
          budgetSummary={budgetSummary}
        />
      </TabsContent>
      <TabsContent className="m-0" value="acceptance">
        <ProjectAcceptancePanel acceptance={acceptance} />
      </TabsContent>
      <TabsContent className="m-0" value="archive">
        <ProjectArchivePanel
          archivePreview={archivePreview}
          archiveSnapshots={archiveSnapshots}
          artifactCount={artifacts.length}
          budgetLedgerCount={budgetLedger.length}
          decisionRequestCount={decisionRequestCount}
          demandCount={demandCount}
          evidenceCount={evidence.length}
          executionSummaryCount={executionSummaryCount}
          reportCount={reports.length}
          routeDecisionCount={routeDecisionCount}
          taskCount={taskCount}
          unresolvedRiskCount={unresolvedRiskCount}
        />
      </TabsContent>
    </Tabs>
  );
}

import { V3TabList, V3Tabs } from "@/components/superteam";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type {
  CreateProjectArchiveSnapshotInput,
  CreateProjectEvidenceInput,
  ProjectAcceptanceRecord,
  ProjectArchivePreview,
  ProjectArchiveSnapshot,
  ProjectArtifactRef,
  ProjectBudgetLedgerEntry,
  ProjectBudgetSummary,
  ProjectEvidenceRef,
  ProjectEvidenceVerificationStatus,
  ProjectReportRef,
} from "@/lib/api/projects";
import { ProjectAcceptancePanel } from "./project-acceptance-panel";
import { ProjectArchivePanel } from "./project-archive-panel";
import { ProjectArtifactReportPanel } from "./project-artifact-report-panel";
import { ProjectBudgetPanel } from "./project-budget-panel";
import { ProjectEvidencePanel } from "./project-evidence-panel";

const governanceTabTriggerClass =
  "h-auto flex-none shrink-0 rounded-[10px] border-0 bg-transparent px-4 py-2 text-[13px] font-semibold text-v3-ink-2 shadow-none transition-colors hover:bg-v3-card-soft hover:text-v3-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-v3-brand/60 disabled:pointer-events-none disabled:opacity-50 data-[state=active]:bg-v3-brand-soft data-[state=active]:text-v3-brand-deep data-[state=active]:shadow-none";

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
  initialTab?: "evidence" | "artifacts" | "budget" | "acceptance" | "archive";
  onCreateArchiveSnapshot: (input: CreateProjectArchiveSnapshotInput) => void;
  onCreateEvidence: (input: CreateProjectEvidenceInput) => void;
  onPatchEvidence: (
    evidenceId: string,
    verificationStatus: ProjectEvidenceVerificationStatus,
  ) => void;
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
  initialTab = "evidence",
  onCreateArchiveSnapshot,
  onCreateEvidence,
  onPatchEvidence,
  reports = [],
  routeDecisionCount,
  taskCount,
}: ProjectGovernanceTabsProps) {
  const unresolvedRiskCount = acceptance?.unresolved_risks.length ?? 0;

  return (
    <Tabs className="flex w-full min-w-0 flex-col gap-3" defaultValue={initialTab}>
      <div className="w-full min-w-0 max-w-full overflow-x-auto overflow-y-hidden pb-1 [-webkit-overflow-scrolling:touch]">
        <TabsList
          aria-label="项目详情治理视图"
          className="h-auto w-max min-w-full max-w-none justify-start overflow-visible rounded-[14px] bg-v3-card p-1.5 text-v3-ink shadow-v3"
        >
          <V3Tabs
            className="w-full min-w-0 bg-transparent p-0 shadow-none"
          >
            <V3TabList className="flex-nowrap">
              <TabsTrigger className={governanceTabTriggerClass} value="evidence">
                证据链
              </TabsTrigger>
              <TabsTrigger className={governanceTabTriggerClass} value="artifacts">
                工件报告
              </TabsTrigger>
              <TabsTrigger className={governanceTabTriggerClass} value="budget">
                预算流水
              </TabsTrigger>
              <TabsTrigger className={governanceTabTriggerClass} value="acceptance">
                结项结论
              </TabsTrigger>
              <TabsTrigger className={governanceTabTriggerClass} value="archive">
                归档预览
              </TabsTrigger>
            </V3TabList>
          </V3Tabs>
        </TabsList>
      </div>

      <TabsContent className="m-0" value="evidence">
        <ProjectEvidencePanel
          evidence={evidence}
          onCreateEvidence={onCreateEvidence}
          onPatchEvidence={onPatchEvidence}
        />
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
          onCreateArchiveSnapshot={onCreateArchiveSnapshot}
          reportCount={reports.length}
          routeDecisionCount={routeDecisionCount}
          taskCount={taskCount}
          unresolvedRiskCount={unresolvedRiskCount}
        />
      </TabsContent>
    </Tabs>
  );
}

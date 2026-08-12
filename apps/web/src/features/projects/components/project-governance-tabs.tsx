import {
  PageTabList,
  PageTabs,
  SoftTabs,
  SoftTabsContent,
  SoftTabsList,
  SoftTabsTrigger,
} from "@/components/superteam";
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
  ProjectReportRef
} from "@/lib/api/projects";
import { ProjectAcceptancePanel } from "./project-acceptance-panel";
import { ProjectArchivePanel } from "./project-archive-panel";
import { ProjectArtifactReportPanel } from "./project-artifact-report-panel";
import { ProjectBudgetPanel } from "./project-budget-panel";
import { ProjectEvidencePanel } from "./project-evidence-panel";

const governanceTabTriggerClass =
  "h-auto flex-none shrink-0 rounded-[10px] border-0 bg-transparent px-4 py-2 text-[13px] font-semibold text-ink-2 shadow-none transition-colors hover:bg-card-soft hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/60 disabled:pointer-events-none disabled:opacity-50 data-[state=active]:bg-brand-soft data-[state=active]:text-brand-deep data-[state=active]:shadow-none";

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
  focusEvidenceId?: string;
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
  focusEvidenceId,
  initialTab = "evidence",
  onCreateArchiveSnapshot,
  onCreateEvidence,
  onPatchEvidence,
  reports = [],
  routeDecisionCount,
  taskCount
}: ProjectGovernanceTabsProps) {
  const unresolvedRiskCount = acceptance?.unresolved_risks.length ?? 0;

  return (
    <SoftTabs
      className="flex w-full min-w-0 flex-col gap-3"
      defaultValue={initialTab}
      key={initialTab}
    >
      <div className="w-full min-w-0 max-w-full overflow-x-auto overflow-y-hidden pb-1 [-webkit-overflow-scrolling:touch]">
        <SoftTabsList
          aria-label="项目详情治理视图"
          className="h-auto w-max min-w-full max-w-none justify-start overflow-visible rounded-[14px] bg-card p-1.5 text-ink shadow-card"
        >
          <PageTabs
            className="w-full min-w-0 bg-transparent p-0 shadow-none"
          >
            <PageTabList className="flex-nowrap">
              <SoftTabsTrigger className={governanceTabTriggerClass} value="evidence">
                证据链
              </SoftTabsTrigger>
              <SoftTabsTrigger className={governanceTabTriggerClass} value="artifacts">
                工件报告
              </SoftTabsTrigger>
              <SoftTabsTrigger className={governanceTabTriggerClass} value="budget">
                预算流水
              </SoftTabsTrigger>
              <SoftTabsTrigger className={governanceTabTriggerClass} value="acceptance">
                结项结论
              </SoftTabsTrigger>
              <SoftTabsTrigger className={governanceTabTriggerClass} value="archive">
                归档预览
              </SoftTabsTrigger>
            </PageTabList>
          </PageTabs>
        </SoftTabsList>
      </div>

      <SoftTabsContent className="m-0" value="evidence">
        <ProjectEvidencePanel
          evidence={evidence}
          focusEvidenceId={focusEvidenceId}
          onCreateEvidence={onCreateEvidence}
          onPatchEvidence={onPatchEvidence}
        />
      </SoftTabsContent>
      <SoftTabsContent className="m-0" value="artifacts">
        <ProjectArtifactReportPanel artifacts={artifacts} reports={reports} />
      </SoftTabsContent>
      <SoftTabsContent className="m-0" value="budget">
        <ProjectBudgetPanel
          budgetLedger={budgetLedger}
          budgetSummary={budgetSummary}
        />
      </SoftTabsContent>
      <SoftTabsContent className="m-0" value="acceptance">
        <ProjectAcceptancePanel acceptance={acceptance} />
      </SoftTabsContent>
      <SoftTabsContent className="m-0" value="archive">
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
      </SoftTabsContent>
    </SoftTabs>
  );
}

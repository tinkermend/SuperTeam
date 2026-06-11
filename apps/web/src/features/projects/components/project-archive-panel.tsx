import { Archive, Database } from "lucide-react";
import {
  LiquidCard,
  SemanticIconTile,
  StatusBadge,
} from "@/components/superteam";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type {
  ProjectArchivePreview,
  ProjectArchiveSnapshot,
} from "@/lib/api/projects";

type ProjectArchivePanelProps = {
  archivePreview?: ProjectArchivePreview;
  archiveSnapshots?: ProjectArchiveSnapshot[];
  artifactCount: number;
  budgetLedgerCount: number;
  decisionRequestCount: number;
  demandCount: number;
  evidenceCount: number;
  executionSummaryCount: number;
  reportCount: number;
  routeDecisionCount: number;
  taskCount: number;
  unresolvedRiskCount: number;
};

export function ProjectArchivePanel({
  archivePreview,
  archiveSnapshots = [],
  artifactCount,
  budgetLedgerCount,
  decisionRequestCount,
  demandCount,
  evidenceCount,
  executionSummaryCount,
  reportCount,
  routeDecisionCount,
  taskCount,
  unresolvedRiskCount,
}: ProjectArchivePanelProps) {
  const retainedArtifactCount = new Set(
    archiveSnapshots.flatMap((snapshot) => snapshot.retained_artifact_ids),
  ).size;
  const blockedReasons = archivePreview?.blocked_reasons ?? [];
  const estimatedObjectRefs = archivePreview?.estimated_object_refs ?? [];
  const effectiveEvidenceCount = archivePreview?.evidence_count ?? evidenceCount;
  const effectiveArtifactCount = archivePreview?.artifact_count ?? artifactCount;
  const effectiveReportCount = archivePreview?.report_count ?? reportCount;
  const effectiveRiskCount = blockedReasons.length || unresolvedRiskCount;
  const previewStatus: {
    label: string;
    tone: "neutral" | "success" | "warning";
  } = archivePreview
    ? {
        label: archivePreview.retention_pending ? "保留待处理" : "可归档",
        tone: archivePreview.retention_pending ? "warning" : "success",
      }
    : { label: "待预览", tone: "neutral" };

  return (
    <LiquidCard className="rounded-xl">
      <div className="flex items-center justify-between gap-3 border-b p-4">
        <div className="flex min-w-0 items-center gap-3">
          <SemanticIconTile tone="neutral" size="sm">
            <Archive />
          </SemanticIconTile>
          <div className="min-w-0">
            <h3 className="font-semibold">归档预览</h3>
            <p className="truncate text-xs text-muted-foreground">
              当前项目归档对象、保留状态与快照估算
            </p>
          </div>
        </div>
        <StatusBadge tone={previewStatus.tone}>{previewStatus.label}</StatusBadge>
      </div>

      <div className="grid gap-4 p-4">
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <MetricBlock label="需求数" value={demandCount} />
          <MetricBlock label="任务数" value={taskCount} />
          <MetricBlock label="RouteDecision 数" value={routeDecisionCount} />
          <MetricBlock label="ExecutionSummary 数" value={executionSummaryCount} />
          <MetricBlock label="DecisionRequest 数" value={decisionRequestCount} />
          <MetricBlock label="EvidenceRef 数" value={effectiveEvidenceCount} />
          <MetricBlock label="ArtifactRef 数" value={effectiveArtifactCount} />
          <MetricBlock label="ReportRef 数" value={effectiveReportCount} />
          <MetricBlock label="预算流水数" value={budgetLedgerCount} />
          <MetricBlock label="未关闭风险" value={effectiveRiskCount} />
          <MetricBlock label="保留工件" value={retainedArtifactCount} />
          <MetricBlock label="ObjectRef 估算" value={estimatedObjectRefs.length} />
        </div>

        <section className="grid gap-2 rounded-lg border bg-white/55 p-3">
          <div className="flex items-center gap-2 text-sm font-semibold">
            <Database className="size-4 text-[color:var(--superteam-neutral)]" />
            当前项目
          </div>
          <div className="grid gap-2 text-xs text-muted-foreground">
            <p>
              Preview ID:{" "}
              <span className="font-mono text-foreground">
                {archivePreview?.project_id ?? "-"}
              </span>
            </p>
            <p>
              estimated_object_refs:{" "}
              <span className="font-mono text-foreground">
                {estimatedObjectRefs.length}
              </span>
              ，blocked_reasons:{" "}
              <span className="font-mono text-foreground">
                {blockedReasons.length}
              </span>
            </p>
          </div>
          {blockedReasons.length > 0 ? (
            <div className="flex flex-wrap gap-2">
              {blockedReasons.slice(0, 4).map((reason, index) => (
                <StatusBadge key={`${index}-${stringifyValue(reason)}`} tone="warning">
                  {stringifyValue(reason)}
                </StatusBadge>
              ))}
            </div>
          ) : null}
        </section>

        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>快照类型</TableHead>
              <TableHead>状态</TableHead>
              <TableHead className="min-w-[220px]">Object Ref</TableHead>
              <TableHead>保留工件</TableHead>
              <TableHead className="min-w-[180px]">摘要</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {archiveSnapshots.length === 0 ? (
              <TableRow>
                <TableCell
                  className="h-24 text-center text-sm text-muted-foreground"
                  colSpan={5}
                >
                  暂无归档快照
                </TableCell>
              </TableRow>
            ) : (
              archiveSnapshots.map((snapshot) => (
                <TableRow key={snapshot.id}>
                  <TableCell>{snapshot.snapshot_type}</TableCell>
                  <TableCell>
                    <StatusBadge tone={snapshot.status === "completed" ? "success" : "warning"}>
                      {snapshot.status}
                    </StatusBadge>
                  </TableCell>
                  <TableCell className="max-w-[280px]">
                    <span className="block truncate font-mono text-xs">
                      {snapshot.object_ref ?? "-"}
                    </span>
                  </TableCell>
                  <TableCell>{snapshot.retained_artifact_ids.length}</TableCell>
                  <TableCell className="max-w-[260px] whitespace-normal">
                    <span className="line-clamp-2 text-sm">
                      {snapshot.summary || "快照已记录"}
                    </span>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
    </LiquidCard>
  );
}

function MetricBlock({ label, value }: { label: string; value: number }) {
  return (
    <div className="min-w-0 rounded-lg border bg-white/55 p-3">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="mt-2 font-mono text-sm font-semibold">
        {new Intl.NumberFormat("zh-CN").format(value)}
      </p>
    </div>
  );
}

function stringifyValue(value: unknown) {
  if (typeof value === "string") {
    return value;
  }
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

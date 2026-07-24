import { useState, type ReactNode } from "react";
import { Archive, Database } from "lucide-react";
import {
  IconTile,
  SoftCard,
  StatusPill,
  Button,
  EmptyState,
  DataTable,
  Td,
  Th,
  Tr,
  WorkSurface,
  type Tone
} from "@/components/superteam";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type {
  CreateProjectArchiveSnapshotInput,
  ProjectArchivePreview,
  ProjectArchiveSnapshot
} from "@/lib/api/projects";
import { statusLabel } from "@/lib/status-labels";

type ProjectArchivePanelProps = {
  archivePreview?: ProjectArchivePreview;
  archiveSnapshots?: ProjectArchiveSnapshot[];
  artifactCount: number;
  budgetLedgerCount: number;
  decisionRequestCount: number;
  demandCount: number;
  evidenceCount: number;
  executionSummaryCount: number;
  onCreateArchiveSnapshot: (input: CreateProjectArchiveSnapshotInput) => void;
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
  onCreateArchiveSnapshot,
  reportCount,
  routeDecisionCount,
  taskCount,
  unresolvedRiskCount
}: ProjectArchivePanelProps) {
  const [objectRef, setObjectRef] = useState("");
  const [summary, setSummary] = useState("");
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
    tone: Tone;
  } = archivePreview
    ? {
        label: archivePreview.retention_pending ? "保留待处理" : "可归档",
        tone: archivePreview.retention_pending ? "warn" : "ok"
}
    : { label: "待预览", tone: "mute" };

  function submitArchiveSnapshot() {
    const nextObjectRef = objectRef.trim();
    if (!nextObjectRef) {
      return;
    }

    onCreateArchiveSnapshot({
      object_ref: nextObjectRef,
      snapshot_type: "project_archive",
      summary: summary.trim() || undefined
});
    setObjectRef("");
    setSummary("");
  }

  return (
    <div className="grid gap-4">
      <SoftCard className="overflow-hidden">
        <div className="flex items-center justify-between gap-3 border-b border-line p-4">
          <div className="flex min-w-0 items-center gap-3">
            <IconTile tone="mute" size="sm">
              <Archive />
            </IconTile>
            <div className="min-w-0">
              <h3 className="font-semibold text-ink">归档预览</h3>
              <p className="truncate text-xs text-ink-2">
                当前项目归档对象、保留状态与快照估算
              </p>
            </div>
          </div>
          <StatusPill tone={previewStatus.tone}>{previewStatus.label}</StatusPill>
        </div>

        <div className="grid gap-4 p-4">
          <div className="grid gap-3">
            <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
              <Field label="快照 Object Ref">
                <Input
                  className="border-line bg-card"
                  value={objectRef}
                  onChange={(event) => setObjectRef(event.target.value)}
                  placeholder="s3://superteam/project/archive.json"
                />
              </Field>
              <Field label="快照摘要">
                <Input
                  className="border-line bg-card"
                  value={summary}
                  onChange={(event) => setSummary(event.target.value)}
                  placeholder="当前项目归档快照"
                />
              </Field>
            </div>
            <div className="flex justify-end">
              <Button
                disabled={!objectRef.trim()}
                type="button"
                onClick={submitArchiveSnapshot}
              >
                生成归档快照
              </Button>
            </div>
          </div>

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

          <section className="grid gap-2 rounded-inner bg-card-soft p-3">
            <div className="flex items-center gap-2 text-sm font-semibold text-ink">
              <Database className="size-4 text-mute" />
              当前项目
            </div>
            <div className="grid gap-2 text-xs text-ink-2">
              <p>
                Preview ID:{" "}
                <span className="font-mono text-ink">
                  {archivePreview?.project_id ?? "-"}
                </span>
              </p>
              <p>
                estimated_object_refs:{" "}
                <span className="font-mono text-ink">
                  {estimatedObjectRefs.length}
                </span>
                ，blocked_reasons:{" "}
                <span className="font-mono text-ink">
                  {blockedReasons.length}
                </span>
              </p>
            </div>
            {blockedReasons.length > 0 ? (
              <div className="flex flex-wrap gap-2">
                {blockedReasons.slice(0, 4).map((reason, index) => (
                  <StatusPill key={`${index}-${stringifyValue(reason)}`} tone="warn">
                    {stringifyValue(reason)}
                  </StatusPill>
                ))}
              </div>
            ) : null}
          </section>
        </div>
      </SoftCard>

      <WorkSurface>
        <DataTable>
          <thead>
            <tr>
              <Th>快照类型</Th>
              <Th>状态</Th>
              <Th className="min-w-[220px]">Object Ref</Th>
              <Th>保留工件</Th>
              <Th className="min-w-[180px]">摘要</Th>
            </tr>
          </thead>
          <tbody>
            {archiveSnapshots.length === 0 ? (
              <Tr>
                <Td colSpan={5}>
                  <EmptyState title="暂无归档快照" />
                </Td>
              </Tr>
            ) : (
              archiveSnapshots.map((snapshot) => (
                <Tr key={snapshot.id}>
                  <Td className="text-ink-2">{snapshot.snapshot_type}</Td>
                  <Td>
                    <StatusPill tone={snapshot.status === "completed" ? "ok" : "warn"}>
                      {statusLabel(snapshot.status)}
                    </StatusPill>
                  </Td>
                  <Td className="max-w-[280px]">
                    <span className="block truncate font-mono text-xs text-ink">
                      {snapshot.object_ref ?? "-"}
                    </span>
                  </Td>
                  <Td className="text-ink tabular-nums">
                    {snapshot.retained_artifact_ids.length}
                  </Td>
                  <Td className="max-w-[260px] whitespace-normal">
                    <span className="line-clamp-2 text-sm text-ink-2">
                      {snapshot.summary || "快照已记录"}
                    </span>
                  </Td>
                </Tr>
              ))
            )}
          </tbody>
        </DataTable>
      </WorkSurface>
    </div>
  );
}

function Field({ children, label }: { children: ReactNode; label: string }) {
  return (
    <Label className="grid gap-2 text-[13px] font-semibold text-ink-2">
      <span>{label}</span>
      {children}
    </Label>
  );
}

function MetricBlock({ label, value }: { label: string; value: number }) {
  return (
    <div className="min-w-0 rounded-inner bg-card-soft p-3">
      <p className="text-xs text-ink-2">{label}</p>
      <p className="mt-2 font-mono text-sm font-semibold text-ink tabular-nums">
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

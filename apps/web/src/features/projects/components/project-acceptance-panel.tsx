import { CheckCircle2 } from "lucide-react";
import {
  IconTile,
  SoftCard,
  StatusPill,
  V3EmptyState,
  type V3Tone,
} from "@/components/superteam";
import type { ProjectAcceptanceRecord } from "@/lib/api/projects";
import { acceptanceStatusLabel } from "@/lib/status-labels";

type ProjectAcceptancePanelProps = {
  acceptance?: ProjectAcceptanceRecord;
  /** §6.3：结项结论只读；自由验收表单已下线，写入口走收件箱/需求页签署。 */
  evidenceRefIds?: string[];
  reportRefIds?: string[];
};

/** 结项结论只读面板——不再提供自由验收提交表单（§6.3 / F7）。 */
export function ProjectAcceptancePanel({
  acceptance,
}: ProjectAcceptancePanelProps) {
  return (
    <div className="grid gap-4" data-testid="project-closure-readonly">
      <SoftCard className="overflow-hidden">
        <div className="flex items-center justify-between gap-3 border-b border-v3-line p-4">
          <div className="flex min-w-0 items-center gap-3">
            <IconTile tone="ok" size="sm">
              <CheckCircle2 />
            </IconTile>
            <div className="min-w-0">
              <h3 className="font-semibold text-v3-ink">结项结论</h3>
              <p className="truncate text-xs text-v3-ink-2">
                只读归档结论；签署与结项请在收件箱或需求页完成
              </p>
            </div>
          </div>
          {acceptance ? (
            <StatusPill tone={acceptanceTone(acceptance.status)}>
              {acceptanceStatusLabel(acceptance.status)}
            </StatusPill>
          ) : (
            <StatusPill tone="mute">未结项</StatusPill>
          )}
        </div>

        {!acceptance ? (
          <div className="p-4">
            <V3EmptyState
              description="项目全部需求终态后，将通过「结项确认」或「通过并结项」写入结论。"
              title="尚未产生结项结论"
            />
          </div>
        ) : (
          <div className="grid gap-4 p-4">
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
              <MetricBlock label="验收人" value={acceptance.accepted_by_user_id} />
              <MetricBlock
                label="EvidenceRef"
                value={`${acceptance.evidence_ref_ids.length} 条`}
              />
              <MetricBlock
                label="ReportRef"
                value={`${acceptance.report_ref_ids.length} 条`}
              />
              <MetricBlock
                label="未关闭风险"
                value={`${acceptance.unresolved_risks.length} 项`}
              />
            </div>

            <section className="grid gap-2">
              <h4 className="text-sm font-semibold text-v3-ink">结论</h4>
              <p className="rounded-v3-inner bg-v3-card-soft p-3 text-sm leading-6 text-v3-ink">
                {acceptance.conclusion}
              </p>
              {acceptance.summary ? (
                <p className="text-xs text-v3-ink-2">{acceptance.summary}</p>
              ) : null}
            </section>

            <div className="grid gap-4 lg:grid-cols-2">
              <ReferenceList label="EvidenceRef" refs={acceptance.evidence_ref_ids} />
              <ReferenceList label="ReportRef" refs={acceptance.report_ref_ids} />
            </div>

            <section className="grid gap-2">
              <div className="flex items-center justify-between gap-3">
                <h4 className="text-sm font-semibold text-v3-ink">未关闭风险</h4>
                <span className="text-xs text-v3-ink-2">
                  {acceptance.unresolved_risks.length} 项
                </span>
              </div>
              {acceptance.unresolved_risks.length === 0 ? (
                <p className="rounded-v3-inner bg-v3-card-soft p-3 text-sm text-v3-ink-2">
                  暂无未关闭风险
                </p>
              ) : (
                <ul className="grid gap-2">
                  {acceptance.unresolved_risks.slice(0, 4).map((risk, index) => (
                    <li
                      className="rounded-v3-inner bg-v3-card-soft p-3 font-mono text-xs text-v3-ink"
                      key={`${index}-${stringifyValue(risk)}`}
                    >
                      {stringifyValue(risk)}
                    </li>
                  ))}
                </ul>
              )}
            </section>
          </div>
        )}
      </SoftCard>
    </div>
  );
}

function MetricBlock({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-v3-inner bg-v3-card-soft p-3">
      <p className="text-xs text-v3-ink-2">{label}</p>
      <p className="mt-2 truncate text-sm font-semibold text-v3-ink">{value}</p>
    </div>
  );
}

function ReferenceList({ label, refs }: { label: string; refs: string[] }) {
  return (
    <section className="grid gap-2">
      <div className="flex items-center justify-between gap-3">
        <h4 className="text-sm font-semibold text-v3-ink">{label}</h4>
        <span className="text-xs text-v3-ink-2">{refs.length} 条</span>
      </div>
      <div className="min-h-16 rounded-v3-inner bg-v3-card-soft p-3">
        {refs.length === 0 ? (
          <p className="text-sm text-v3-ink-2">暂无引用</p>
        ) : (
          <div className="flex flex-wrap gap-2">
            {refs.map((ref) => (
              <StatusPill key={ref} tone="mute">
                {ref}
              </StatusPill>
            ))}
          </div>
        )}
      </div>
    </section>
  );
}

function acceptanceTone(status: ProjectAcceptanceRecord["status"]): V3Tone {
  if (status === "accepted") {
    return "ok";
  }
  if (status === "rejected") {
    return "danger";
  }
  if (status === "needs_more_evidence") {
    return "warn";
  }
  return "info";
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

import { useState, type ReactNode } from "react";
import { CheckCircle2 } from "lucide-react";
import {
  IconTile,
  SoftCard,
  StatusPill,
  V3Button,
  V3EmptyState,
  type V3Tone,
} from "@/components/superteam";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import type {
  CreateProjectAcceptanceInput,
  ProjectAcceptanceRecord,
} from "@/lib/api/projects";

type ProjectAcceptancePanelProps = {
  acceptance?: ProjectAcceptanceRecord;
  evidenceRefIds?: string[];
  onCreateAcceptance: (input: CreateProjectAcceptanceInput) => void;
  reportRefIds?: string[];
};

export function ProjectAcceptancePanel({
  acceptance,
  evidenceRefIds = [],
  onCreateAcceptance,
  reportRefIds = [],
}: ProjectAcceptancePanelProps) {
  const [conclusion, setConclusion] = useState("");
  const [summary, setSummary] = useState("");

  function submitAcceptance() {
    const nextConclusion = conclusion.trim();
    if (!nextConclusion) {
      return;
    }

    onCreateAcceptance({
      conclusion: nextConclusion,
      evidence_ref_ids: evidenceRefIds,
      report_ref_ids: reportRefIds,
      status: "accepted",
      summary: summary.trim() || undefined,
      unresolved_risks: [],
    });
    setConclusion("");
    setSummary("");
  }

  return (
    <div className="grid gap-4">
      <SoftCard className="overflow-hidden">
        <div className="flex items-center justify-between gap-3 border-b border-v3-line p-4">
          <div className="flex min-w-0 items-center gap-3">
            <IconTile tone="ok" size="sm">
              <CheckCircle2 />
            </IconTile>
            <div className="min-w-0">
              <h3 className="font-semibold text-v3-ink">验收结论</h3>
              <p className="truncate text-xs text-v3-ink-2">
                人类验收、证据引用与未关闭风险
              </p>
            </div>
          </div>
          {acceptance ? (
            <StatusPill tone={acceptanceTone(acceptance.status)}>
              {acceptance.status}
            </StatusPill>
          ) : (
            <StatusPill tone="mute">未提交</StatusPill>
          )}
        </div>

        <div className="grid gap-3 p-4">
          <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
            <Field label="验收结论">
              <Textarea
                aria-label="验收结论"
                className="border-v3-line bg-v3-card"
                value={conclusion}
                onChange={(event) => setConclusion(event.target.value)}
                placeholder="证据完整，同意归档"
              />
            </Field>
            <Field label="验收摘要">
              <Textarea
                className="border-v3-line bg-v3-card"
                value={summary}
                onChange={(event) => setSummary(event.target.value)}
                placeholder="补充验收范围、例外项或后续动作"
              />
            </Field>
          </div>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-xs text-v3-ink-2">
              EvidenceRef {evidenceRefIds.length} 条，ReportRef {reportRefIds.length} 条
            </p>
            <V3Button
              disabled={!conclusion.trim()}
              type="button"
              onClick={submitAcceptance}
            >
              提交验收
            </V3Button>
          </div>
        </div>
      </SoftCard>

      {!acceptance ? (
        <SoftCard>
          <V3EmptyState title="尚未提交验收结论" />
        </SoftCard>
      ) : (
        <SoftCard className="grid gap-4 p-4">
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
              <h4 className="text-sm font-semibold text-v3-ink">Unresolved Risks</h4>
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
        </SoftCard>
      )}
    </div>
  );
}

function Field({ children, label }: { children: ReactNode; label: string }) {
  return (
    <Label className="grid gap-2 text-[13px] font-semibold text-v3-ink-2">
      <span>{label}</span>
      {children}
    </Label>
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

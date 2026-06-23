import { useState, type ReactNode } from "react";
import { FileSearch } from "lucide-react";
import {
  IconTile,
  SoftCard,
  StatusPill,
  V3Button,
  V3EmptyState,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  WorkSurface,
  type V3Tone,
} from "@/components/superteam";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import type {
  CreateProjectEvidenceInput,
  ProjectEvidenceRef,
  ProjectEvidenceVerificationStatus,
} from "@/lib/api/projects";

type ProjectEvidencePanelProps = {
  evidence?: ProjectEvidenceRef[];
  onCreateEvidence: (input: CreateProjectEvidenceInput) => void;
  onPatchEvidence: (
    evidenceId: string,
    verificationStatus: ProjectEvidenceVerificationStatus,
  ) => void;
};

export function ProjectEvidencePanel({
  evidence = [],
  onCreateEvidence,
  onPatchEvidence,
}: ProjectEvidencePanelProps) {
  const [title, setTitle] = useState("");
  const [sourceRef, setSourceRef] = useState("");
  const [summary, setSummary] = useState("");

  function submitEvidence() {
    const nextTitle = title.trim();
    const nextSourceRef = sourceRef.trim();
    if (!nextTitle || !nextSourceRef) {
      return;
    }

    onCreateEvidence({
      evidence_type: "manual",
      source_ref: nextSourceRef,
      source_type: "manual",
      summary: summary.trim() || undefined,
      title: nextTitle,
    });
    setTitle("");
    setSourceRef("");
    setSummary("");
  }

  return (
    <div className="grid gap-4">
      <SoftCard className="overflow-hidden">
        <div className="flex items-center justify-between gap-3 border-b border-v3-line p-4">
          <div className="flex min-w-0 items-center gap-3">
            <IconTile tone="info" size="sm">
              <FileSearch />
            </IconTile>
            <div className="min-w-0">
              <h3 className="font-semibold text-v3-ink">证据链</h3>
              <p className="truncate text-xs text-v3-ink-2">
                当前项目可追踪证据引用
              </p>
            </div>
          </div>
          <StatusPill tone="mute">{evidence.length} 条</StatusPill>
        </div>

        <div className="grid gap-4 p-4">
          <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
            <Field label="证据标题">
              <Input
                className="border-v3-line bg-v3-card"
                value={title}
                onChange={(event) => setTitle(event.target.value)}
                placeholder="补充验收附件"
              />
            </Field>
            <Field label="来源引用">
              <Input
                className="border-v3-line bg-v3-card"
                value={sourceRef}
                onChange={(event) => setSourceRef(event.target.value)}
                placeholder="s3://superteam/project/archive.md"
              />
            </Field>
          </div>
          <Field label="证据摘要">
            <Textarea
              className="border-v3-line bg-v3-card"
              value={summary}
              onChange={(event) => setSummary(event.target.value)}
              placeholder="补充说明证据覆盖范围、来源和验证口径"
            />
          </Field>
          <div className="flex justify-end">
            <V3Button
              disabled={!title.trim() || !sourceRef.trim()}
              type="button"
              onClick={submitEvidence}
            >
              新增证据
            </V3Button>
          </div>
        </div>
      </SoftCard>

      <WorkSurface>
        <V3Table>
          <thead>
            <tr>
              <V3Th className="min-w-[180px]">标题</V3Th>
              <V3Th>类型</V3Th>
              <V3Th className="min-w-[200px]">来源</V3Th>
              <V3Th>状态</V3Th>
              <V3Th className="w-[132px] text-right">操作</V3Th>
            </tr>
          </thead>
          <tbody>
            {evidence.length === 0 ? (
              <V3Tr>
                <V3Td colSpan={5}>
                  <V3EmptyState title="暂无证据引用，治理区域保持可见。" />
                </V3Td>
              </V3Tr>
            ) : (
              evidence.map((item) => (
                <V3Tr key={item.id}>
                  <V3Td className="max-w-[280px] whitespace-normal">
                    <div className="grid gap-1">
                      <span className="line-clamp-2 font-medium text-v3-ink">
                        {item.title}
                      </span>
                      {item.summary ? (
                        <span className="line-clamp-1 text-xs text-v3-ink-2">
                          {item.summary}
                        </span>
                      ) : null}
                    </div>
                  </V3Td>
                  <V3Td>
                    <StatusPill tone="info">{item.evidence_type}</StatusPill>
                  </V3Td>
                  <V3Td className="max-w-[260px] whitespace-normal">
                    <div className="grid gap-1">
                      <span className="text-xs text-v3-ink-2">
                        {item.source_type}
                      </span>
                      <span className="truncate font-mono text-xs text-v3-ink">
                        {item.source_ref}
                      </span>
                    </div>
                  </V3Td>
                  <V3Td>
                    <StatusPill tone={evidenceStatusTone(item.verification_status)}>
                      {item.verification_status}
                    </StatusPill>
                  </V3Td>
                  <V3Td className="text-right">
                    <V3Button
                      aria-label={`标记已验证：${item.title}`}
                      size="sm"
                      type="button"
                      variant="outline"
                      onClick={() => onPatchEvidence(item.id, "verified")}
                    >
                      标记已验证
                    </V3Button>
                  </V3Td>
                </V3Tr>
              ))
            )}
          </tbody>
        </V3Table>
      </WorkSurface>
    </div>
  );
}

function Field({
  children,
  label,
}: {
  children: ReactNode;
  label: string;
}) {
  return (
    <Label className="grid gap-2 text-[13px] font-semibold text-v3-ink-2">
      <span>{label}</span>
      {children}
    </Label>
  );
}

function evidenceStatusTone(status: ProjectEvidenceRef["verification_status"]): V3Tone {
  if (status === "verified") {
    return "ok";
  }
  if (status === "rejected") {
    return "danger";
  }
  if (status === "submitted" || status === "linked") {
    return "warn";
  }
  return "mute";
}

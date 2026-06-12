import { useState, type ReactNode } from "react";
import { FileSearch } from "lucide-react";
import {
  LiquidCard,
  SemanticIconTile,
  StatusBadge,
  type Tone,
} from "@/components/superteam";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Button } from "@/components/ui/button";
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
    <LiquidCard className="rounded-xl">
      <div className="flex items-center justify-between gap-3 border-b p-4">
        <div className="flex min-w-0 items-center gap-3">
          <SemanticIconTile tone="decision" size="sm">
            <FileSearch />
          </SemanticIconTile>
          <div className="min-w-0">
            <h3 className="font-semibold">证据链</h3>
            <p className="truncate text-xs text-muted-foreground">
              当前项目可追踪证据引用
            </p>
          </div>
        </div>
        <StatusBadge tone="neutral">{evidence.length} 条</StatusBadge>
      </div>

      <div className="grid gap-4 p-4">
        <div className="grid gap-3 rounded-lg border bg-white/55 p-3">
          <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
            <Field label="证据标题">
              <Input
                value={title}
                onChange={(event) => setTitle(event.target.value)}
                placeholder="补充验收附件"
              />
            </Field>
            <Field label="来源引用">
              <Input
                value={sourceRef}
                onChange={(event) => setSourceRef(event.target.value)}
                placeholder="s3://superteam/project/archive.md"
              />
            </Field>
          </div>
          <Field label="证据摘要">
            <Textarea
              value={summary}
              onChange={(event) => setSummary(event.target.value)}
              placeholder="补充说明证据覆盖范围、来源和验证口径"
            />
          </Field>
          <div className="flex justify-end">
            <Button
              disabled={!title.trim() || !sourceRef.trim()}
              type="button"
              onClick={submitEvidence}
            >
              新增证据
            </Button>
          </div>
        </div>

        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="min-w-[180px]">标题</TableHead>
              <TableHead>类型</TableHead>
              <TableHead className="min-w-[200px]">来源</TableHead>
              <TableHead>状态</TableHead>
              <TableHead className="w-[132px] text-right">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {evidence.length === 0 ? (
              <TableRow>
                <TableCell
                  className="h-24 text-center text-sm text-muted-foreground"
                  colSpan={5}
                >
                  暂无证据引用，治理区域保持可见。
                </TableCell>
              </TableRow>
            ) : (
              evidence.map((item) => (
                <TableRow key={item.id}>
                  <TableCell className="max-w-[280px] whitespace-normal">
                    <div className="grid gap-1">
                      <span className="line-clamp-2 font-medium">{item.title}</span>
                      {item.summary ? (
                        <span className="line-clamp-1 text-xs text-muted-foreground">
                          {item.summary}
                        </span>
                      ) : null}
                    </div>
                  </TableCell>
                  <TableCell>
                    <StatusBadge tone="info">{item.evidence_type}</StatusBadge>
                  </TableCell>
                  <TableCell className="max-w-[260px] whitespace-normal">
                    <div className="grid gap-1">
                      <span className="text-xs text-muted-foreground">
                        {item.source_type}
                      </span>
                      <span className="truncate font-mono text-xs">
                        {item.source_ref}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell>
                    <StatusBadge tone={evidenceStatusTone(item.verification_status)}>
                      {item.verification_status}
                    </StatusBadge>
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      aria-label={`标记已验证：${item.title}`}
                      size="sm"
                      type="button"
                      variant="outline"
                      onClick={() => onPatchEvidence(item.id, "verified")}
                    >
                      标记已验证
                    </Button>
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

function Field({
  children,
  label,
}: {
  children: ReactNode;
  label: string;
}) {
  return (
    <Label className="grid gap-2">
      <span>{label}</span>
      {children}
    </Label>
  );
}

function evidenceStatusTone(status: ProjectEvidenceRef["verification_status"]): Tone {
  if (status === "verified") {
    return "success";
  }
  if (status === "rejected") {
    return "danger";
  }
  if (status === "submitted" || status === "linked") {
    return "warning";
  }
  return "neutral";
}

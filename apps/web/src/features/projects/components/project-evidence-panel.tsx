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
import type { ProjectEvidenceRef } from "@/lib/api/projects";

type ProjectEvidencePanelProps = {
  evidence?: ProjectEvidenceRef[];
};

export function ProjectEvidencePanel({
  evidence = [],
}: ProjectEvidencePanelProps) {
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

      <div className="p-4">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="min-w-[180px]">标题</TableHead>
              <TableHead>类型</TableHead>
              <TableHead className="min-w-[200px]">来源</TableHead>
              <TableHead>状态</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {evidence.length === 0 ? (
              <TableRow>
                <TableCell
                  className="h-24 text-center text-sm text-muted-foreground"
                  colSpan={4}
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
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
    </LiquidCard>
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

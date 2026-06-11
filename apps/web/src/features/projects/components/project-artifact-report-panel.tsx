import { FileArchive, FileText } from "lucide-react";
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
import type { ProjectArtifactRef, ProjectReportRef } from "@/lib/api/projects";

type ProjectArtifactReportPanelProps = {
  artifacts?: ProjectArtifactRef[];
  reports?: ProjectReportRef[];
};

export function ProjectArtifactReportPanel({
  artifacts = [],
  reports = [],
}: ProjectArtifactReportPanelProps) {
  return (
    <LiquidCard className="rounded-xl">
      <div className="flex items-center justify-between gap-3 border-b p-4">
        <div className="flex min-w-0 items-center gap-3">
          <SemanticIconTile tone="artifact" size="sm">
            <FileArchive />
          </SemanticIconTile>
          <div className="min-w-0">
            <h3 className="font-semibold">工件报告</h3>
            <p className="truncate text-xs text-muted-foreground">
              工件保留状态与报告对象引用
            </p>
          </div>
        </div>
        <StatusBadge tone="artifact">
          {artifacts.length + reports.length} 项
        </StatusBadge>
      </div>

      <div className="grid gap-5 p-4">
        <section className="grid gap-2">
          <div className="flex items-center justify-between gap-3">
            <h4 className="flex items-center gap-2 text-sm font-semibold">
              <FileArchive className="size-4 text-[color:var(--superteam-artifact)]" />
              工件
            </h4>
            <span className="text-xs text-muted-foreground">
              {artifacts.length} 条
            </span>
          </div>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="min-w-[180px]">标题</TableHead>
                <TableHead>类型</TableHead>
                <TableHead>Retention Status</TableHead>
                <TableHead className="min-w-[220px]">Object Ref</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {artifacts.length === 0 ? (
                <TableRow>
                  <TableCell
                    className="h-20 text-center text-sm text-muted-foreground"
                    colSpan={4}
                  >
                    暂无工件引用
                  </TableCell>
                </TableRow>
              ) : (
                artifacts.map((artifact) => (
                  <TableRow key={artifact.id}>
                    <TableCell className="max-w-[260px] whitespace-normal">
                      <span className="line-clamp-2 font-medium">
                        {artifact.title}
                      </span>
                    </TableCell>
                    <TableCell>{artifact.artifact_type}</TableCell>
                    <TableCell>
                      <StatusBadge tone={retentionTone(artifact.retention_status)}>
                        {artifact.retention_status}
                      </StatusBadge>
                    </TableCell>
                    <TableCell className="max-w-[280px]">
                      <span className="block truncate font-mono text-xs">
                        {artifact.object_ref}
                      </span>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </section>

        <section className="grid gap-2 border-t pt-4">
          <div className="flex items-center justify-between gap-3">
            <h4 className="flex items-center gap-2 text-sm font-semibold">
              <FileText className="size-4 text-[color:var(--superteam-info)]" />
              报告
            </h4>
            <span className="text-xs text-muted-foreground">{reports.length} 条</span>
          </div>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="min-w-[180px]">标题</TableHead>
                <TableHead>类型</TableHead>
                <TableHead>Report Format</TableHead>
                <TableHead className="min-w-[220px]">Object Ref</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {reports.length === 0 ? (
                <TableRow>
                  <TableCell
                    className="h-20 text-center text-sm text-muted-foreground"
                    colSpan={4}
                  >
                    暂无报告引用
                  </TableCell>
                </TableRow>
              ) : (
                reports.map((report) => (
                  <TableRow key={report.id}>
                    <TableCell className="max-w-[260px] whitespace-normal">
                      <div className="grid gap-1">
                        <span className="line-clamp-2 font-medium">
                          {report.title}
                        </span>
                        {report.summary ? (
                          <span className="line-clamp-1 text-xs text-muted-foreground">
                            {report.summary}
                          </span>
                        ) : null}
                      </div>
                    </TableCell>
                    <TableCell>{report.report_type}</TableCell>
                    <TableCell>
                      <StatusBadge tone="info">{report.format}</StatusBadge>
                    </TableCell>
                    <TableCell className="max-w-[280px]">
                      <span className="block truncate font-mono text-xs">
                        {report.object_ref}
                      </span>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </section>
      </div>
    </LiquidCard>
  );
}

function retentionTone(status: string): Tone {
  if (["retained", "locked", "hold"].includes(status)) {
    return "success";
  }
  if (["pending", "retention_pending"].includes(status)) {
    return "warning";
  }
  if (["failed", "expired"].includes(status)) {
    return "danger";
  }
  return "neutral";
}

import { FileArchive, FileText } from "lucide-react";
import {
  IconTile,
  SoftCard,
  StatusPill,
  V3EmptyState,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  WorkSurface,
  type V3Tone,
} from "@/components/superteam";
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
    <div className="grid gap-4">
      <SoftCard className="flex items-center justify-between gap-3 p-4">
        <div className="flex min-w-0 items-center gap-3">
          <IconTile tone="artifact" size="sm">
            <FileArchive />
          </IconTile>
          <div className="min-w-0">
            <h3 className="font-semibold text-v3-ink">工件报告</h3>
            <p className="truncate text-xs text-v3-ink-2">
              工件保留状态与报告对象引用
            </p>
          </div>
        </div>
        <StatusPill tone="artifact">
          {artifacts.length + reports.length} 项
        </StatusPill>
      </SoftCard>

      <WorkSurface>
        <div className="flex items-center justify-between gap-3 border-b border-v3-line p-4">
          <h4 className="flex items-center gap-2 text-sm font-semibold text-v3-ink">
            <FileArchive className="size-4 text-v3-artifact" />
            工件
          </h4>
          <span className="text-xs text-v3-ink-2">{artifacts.length} 条</span>
        </div>
        <V3Table>
          <thead>
            <tr>
              <V3Th className="min-w-[180px]">标题</V3Th>
              <V3Th>类型</V3Th>
              <V3Th>Retention Status</V3Th>
              <V3Th className="min-w-[220px]">Object Ref</V3Th>
            </tr>
          </thead>
          <tbody>
            {artifacts.length === 0 ? (
              <V3Tr>
                <V3Td colSpan={4}>
                  <V3EmptyState title="暂无工件引用" />
                </V3Td>
              </V3Tr>
            ) : (
              artifacts.map((artifact) => (
                <V3Tr key={artifact.id}>
                  <V3Td className="max-w-[260px] whitespace-normal">
                    <span className="line-clamp-2 font-medium text-v3-ink">
                      {artifact.title}
                    </span>
                  </V3Td>
                  <V3Td className="text-v3-ink-2">{artifact.artifact_type}</V3Td>
                  <V3Td>
                    <StatusPill tone={retentionTone(artifact.retention_status)}>
                      {artifact.retention_status}
                    </StatusPill>
                  </V3Td>
                  <V3Td className="max-w-[280px]">
                    <span className="block truncate font-mono text-xs text-v3-ink">
                      {artifact.object_ref}
                    </span>
                  </V3Td>
                </V3Tr>
              ))
            )}
          </tbody>
        </V3Table>
      </WorkSurface>

      <WorkSurface>
        <div className="flex items-center justify-between gap-3 border-b border-v3-line p-4">
          <h4 className="flex items-center gap-2 text-sm font-semibold text-v3-ink">
            <FileText className="size-4 text-v3-info" />
            报告
          </h4>
          <span className="text-xs text-v3-ink-2">{reports.length} 条</span>
        </div>
        <V3Table>
          <thead>
            <tr>
              <V3Th className="min-w-[180px]">标题</V3Th>
              <V3Th>类型</V3Th>
              <V3Th>Report Format</V3Th>
              <V3Th className="min-w-[220px]">Object Ref</V3Th>
            </tr>
          </thead>
          <tbody>
            {reports.length === 0 ? (
              <V3Tr>
                <V3Td colSpan={4}>
                  <V3EmptyState title="暂无报告引用" />
                </V3Td>
              </V3Tr>
            ) : (
              reports.map((report) => (
                <V3Tr key={report.id}>
                  <V3Td className="max-w-[260px] whitespace-normal">
                    <div className="grid gap-1">
                      <span className="line-clamp-2 font-medium text-v3-ink">
                        {report.title}
                      </span>
                      {report.summary ? (
                        <span className="line-clamp-1 text-xs text-v3-ink-2">
                          {report.summary}
                        </span>
                      ) : null}
                    </div>
                  </V3Td>
                  <V3Td className="text-v3-ink-2">{report.report_type}</V3Td>
                  <V3Td>
                    <StatusPill tone="info">{report.format}</StatusPill>
                  </V3Td>
                  <V3Td className="max-w-[280px]">
                    <span className="block truncate font-mono text-xs text-v3-ink">
                      {report.object_ref}
                    </span>
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

function retentionTone(status: string): V3Tone {
  if (["retained", "locked", "hold"].includes(status)) {
    return "ok";
  }
  if (["pending", "retention_pending"].includes(status)) {
    return "warn";
  }
  if (["failed", "expired"].includes(status)) {
    return "danger";
  }
  return "mute";
}

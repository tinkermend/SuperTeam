import {
  Download,
  Eye,
  FileArchive,
  FileOutput,
  FileText,
  PackageCheck
} from "lucide-react";
import { useState } from "react";
import {
  IconTile,
  SoftCard,
  StatusPill,
  EmptyState,
  DataTable,
  Td,
  Th,
  Tr,
  WorkSurface,
  type Tone
} from "@/components/superteam";
import type { ProjectArtifactRef, ProjectReportRef } from "@/lib/api/projects";
import { statusLabel } from "@/lib/status-labels";
import {
  ArtifactPreviewSheet,
  artifactContentHref,
  artifactPreviewKind
} from "./artifact-preview-sheet";

type ProjectArtifactReportPanelProps = {
  artifacts?: ProjectArtifactRef[];
  reports?: ProjectReportRef[];
};

/** 执行输出附件(自动兜底捕获)与证据/其他工件分区展示(输出附件 spec §3)。 */
const ATTACHMENT_TYPES = new Set([
  "execution_output",
  "execution_output_skipped",
]);

/** 正式交付物(deliverables/ 契约声明,平台已核对对象存在;v2 spec §4)。 */
const DECLARED_TYPES = new Set(["declared", "declared_skipped"]);

export function ProjectArtifactReportPanel({
  artifacts = [],
  reports = []
}: ProjectArtifactReportPanelProps) {
  const [previewArtifact, setPreviewArtifact] =
    useState<ProjectArtifactRef | null>(null);
  const declared = artifacts.filter((artifact) =>
    DECLARED_TYPES.has(artifact.artifact_type),
  );
  const attachments = artifacts.filter((artifact) =>
    ATTACHMENT_TYPES.has(artifact.artifact_type),
  );
  const evidenceArtifacts = artifacts.filter(
    (artifact) =>
      !ATTACHMENT_TYPES.has(artifact.artifact_type) &&
      !DECLARED_TYPES.has(artifact.artifact_type),
  );

  return (
    <div className="grid gap-4">
      <SoftCard className="flex items-center justify-between gap-3 p-4">
        <div className="flex min-w-0 items-center gap-3">
          <IconTile tone="artifact" size="sm">
            <FileArchive />
          </IconTile>
          <div className="min-w-0">
            <h3 className="font-semibold text-ink">工件报告</h3>
            <p className="truncate text-xs text-ink-2">
              执行输出、工件保留状态与报告对象引用
            </p>
          </div>
        </div>
        <StatusPill tone="artifact">
          {artifacts.length + reports.length} 项
        </StatusPill>
      </SoftCard>

      <OutputFilesSection
        artifacts={declared}
        icon={<PackageCheck className="size-4 text-artifact" />}
        onPreview={setPreviewArtifact}
        subtitle="契约声明的交付物(deliverables/),平台已核对对象存在;内容为原样产出"
        title="正式交付物"
      />
      <OutputFilesSection
        artifacts={attachments}
        icon={<FileOutput className="size-4 text-artifact" />}
        onPreview={setPreviewArtifact}
        subtitle="数字员工执行时新产生的文件,原样采集,未经平台核实与脱敏"
        title="执行输出附件"
      />

      <WorkSurface>
        <div className="flex items-center justify-between gap-3 border-b border-line p-4">
          <h4 className="flex items-center gap-2 text-sm font-semibold text-ink">
            <FileArchive className="size-4 text-artifact" />
            证据与其他工件
          </h4>
          <span className="text-xs text-ink-2">
            {evidenceArtifacts.length} 条
          </span>
        </div>
        <DataTable>
          <thead>
            <tr>
              <Th className="min-w-[180px]">标题</Th>
              <Th>类型</Th>
              <Th>保留状态</Th>
              <Th className="min-w-[220px]">Object Ref</Th>
              <Th>内容</Th>
            </tr>
          </thead>
          <tbody>
            {evidenceArtifacts.length === 0 ? (
              <Tr>
                <Td colSpan={5}>
                  <EmptyState title="暂无工件引用" />
                </Td>
              </Tr>
            ) : (
              evidenceArtifacts.map((artifact) => (
                <Tr key={artifact.id}>
                  <Td className="max-w-[260px] whitespace-normal">
                    <span className="line-clamp-2 font-medium text-ink">
                      {artifact.title}
                    </span>
                  </Td>
                  <Td className="text-ink-2">{artifact.artifact_type}</Td>
                  <Td>
                    <StatusPill tone={retentionTone(artifact.retention_status)}>
                      {statusLabel(artifact.retention_status)}
                    </StatusPill>
                  </Td>
                  <Td className="max-w-[280px]">
                    <span className="block truncate font-mono text-xs text-ink">
                      {artifact.object_ref}
                    </span>
                  </Td>
                  <Td>
                    <ArtifactContentActions
                      artifact={artifact}
                      onPreview={setPreviewArtifact}
                    />
                  </Td>
                </Tr>
              ))
            )}
          </tbody>
        </DataTable>
      </WorkSurface>

      <WorkSurface>
        <div className="flex items-center justify-between gap-3 border-b border-line p-4">
          <h4 className="flex items-center gap-2 text-sm font-semibold text-ink">
            <FileText className="size-4 text-info" />
            报告
          </h4>
          <span className="text-xs text-ink-2">{reports.length} 条</span>
        </div>
        <DataTable>
          <thead>
            <tr>
              <Th className="min-w-[180px]">标题</Th>
              <Th>类型</Th>
              <Th>Report Format</Th>
              <Th className="min-w-[220px]">Object Ref</Th>
            </tr>
          </thead>
          <tbody>
            {reports.length === 0 ? (
              <Tr>
                <Td colSpan={4}>
                  <EmptyState title="暂无报告引用" />
                </Td>
              </Tr>
            ) : (
              reports.map((report) => (
                <Tr key={report.id}>
                  <Td className="max-w-[260px] whitespace-normal">
                    <div className="grid gap-1">
                      <span className="line-clamp-2 font-medium text-ink">
                        {report.title}
                      </span>
                      {report.summary ? (
                        <span className="line-clamp-1 text-xs text-ink-2">
                          {report.summary}
                        </span>
                      ) : null}
                    </div>
                  </Td>
                  <Td className="text-ink-2">{report.report_type}</Td>
                  <Td>
                    <StatusPill tone="info">{report.format}</StatusPill>
                  </Td>
                  <Td className="max-w-[280px]">
                    <span className="block truncate font-mono text-xs text-ink">
                      {report.object_ref}
                    </span>
                  </Td>
                </Tr>
              ))
            )}
          </tbody>
        </DataTable>
      </WorkSurface>

      <ArtifactPreviewSheet
        artifact={previewArtifact}
        onClose={() => setPreviewArtifact(null)}
      />
    </div>
  );
}

/** 输出文件类分区(正式交付物/执行输出附件共用):空集合时整区不渲染。 */
function OutputFilesSection({
  artifacts,
  icon,
  onPreview,
  subtitle,
  title
}: {
  artifacts: ProjectArtifactRef[];
  icon: React.ReactNode;
  onPreview: (artifact: ProjectArtifactRef) => void;
  subtitle: string;
  title: string;
}) {
  if (artifacts.length === 0) {
    return null;
  }
  return (
    <WorkSurface>
      <div className="flex items-center justify-between gap-3 border-b border-line p-4">
        <div className="min-w-0">
          <h4 className="flex items-center gap-2 text-sm font-semibold text-ink">
            {icon}
            {title}
          </h4>
          <p className="mt-0.5 text-xs text-ink-2">{subtitle}</p>
        </div>
        <span className="shrink-0 text-xs text-ink-2">
          {artifacts.length} 条
        </span>
      </div>
      <DataTable>
        <thead>
          <tr>
            <Th className="min-w-[200px]">文件</Th>
            <Th>格式</Th>
            <Th>大小</Th>
            <Th>保留状态</Th>
            <Th>内容</Th>
          </tr>
        </thead>
        <tbody>
          {artifacts.map((artifact) => (
            <Tr key={artifact.id}>
              <Td className="max-w-[300px] whitespace-normal">
                <div className="grid gap-0.5">
                  <span className="line-clamp-2 font-medium text-ink">
                    {artifact.title}
                  </span>
                  {attachmentRelativePath(artifact) ? (
                    <span className="truncate font-mono text-xs text-ink-2">
                      {attachmentRelativePath(artifact)}
                    </span>
                  ) : null}
                </div>
              </Td>
              <Td className="text-ink-2">
                {formatLabel(artifact.content_type)}
              </Td>
              <Td className="text-ink-2 tabular-nums">
                {formatSize(artifact.size_bytes)}
              </Td>
              <Td>
                <StatusPill tone={retentionTone(artifact.retention_status)}>
                  {statusLabel(artifact.retention_status)}
                </StatusPill>
              </Td>
              <Td>
                <ArtifactContentActions
                  artifact={artifact}
                  onPreview={onPreview}
                />
              </Td>
            </Tr>
          ))}
        </tbody>
      </DataTable>
    </WorkSurface>
  );
}

function ArtifactContentActions({
  artifact,
  onPreview
}: {
  artifact: ProjectArtifactRef;
  onPreview: (artifact: ProjectArtifactRef) => void;
}) {
  if (artifact.artifact_type.endsWith("_skipped")) {
    return <span className="text-xs text-ink-2">未采集(超限)</span>;
  }
  if (!isRetrievableArtifact(artifact)) {
    return <span className="text-xs text-ink-2">不可取回</span>;
  }
  return (
    <div className="flex items-center gap-3">
      {artifactPreviewKind(artifact) != null ? (
        <button
          className="inline-flex items-center gap-1 text-xs font-medium text-brand hover:underline"
          onClick={() => onPreview(artifact)}
          type="button"
        >
          <Eye aria-hidden className="size-3.5" />
          预览
        </button>
      ) : null}
      {/* 下载走 302 → presigned GET,属外部下载场景,原生 <a> 合规。 */}
      <a
        className="inline-flex items-center gap-1 text-xs font-medium text-brand hover:underline"
        href={artifactContentHref(artifact.id)}
        rel="noreferrer"
        target="_blank"
      >
        <Download aria-hidden className="size-3.5" />
        下载
      </a>
    </div>
  );
}

/** 只有 runtime 采集上传的内容寻址对象可经平台取回;自报/外部引用无内容。 */
function isRetrievableArtifact(artifact: ProjectArtifactRef): boolean {
  return artifact.object_ref.startsWith("artifacts/");
}

function attachmentRelativePath(artifact: ProjectArtifactRef): string | null {
  const path = artifact.metadata?.["relative_path"];
  return typeof path === "string" && path !== "" && path !== artifact.title
    ? path
    : null;
}

function formatLabel(contentType?: string): string {
  if (!contentType) {
    return "—";
  }
  const subtype = contentType.split(";")[0]?.split("/")[1] ?? contentType;
  const known: Record<string, string> = {
    html: "HTML",
    markdown: "Markdown",
    plain: "文本",
    csv: "CSV",
    json: "JSON",
    msword: "Word",
    "vnd.openxmlformats-officedocument.wordprocessingml.document": "Word",
    "vnd.openxmlformats-officedocument.spreadsheetml.sheet": "Excel"
};
  return known[subtype] ?? subtype;
}

function formatSize(sizeBytes?: number): string {
  if (sizeBytes == null || Number.isNaN(sizeBytes)) {
    return "—";
  }
  if (sizeBytes < 1024) {
    return `${sizeBytes} B`;
  }
  if (sizeBytes < 1024 * 1024) {
    return `${(sizeBytes / 1024).toFixed(1)} KB`;
  }
  return `${(sizeBytes / (1024 * 1024)).toFixed(1)} MB`;
}

function retentionTone(status: string): Tone {
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

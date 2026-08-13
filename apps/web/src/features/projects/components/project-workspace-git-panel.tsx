import { Button, StatusPill } from "@/components/superteam";
import { formatRelativeTime } from "@/lib/format-time";
import type { ProjectWorkspaceGitStatus } from "@/lib/api/projects";
import {
  workspaceGitCleanLabel,
  workspaceGitFileCategoryLabel,
  workspaceGitRepoStateLabel,
  workspaceGitSampleErrorLabel,
} from "@/lib/status-labels";
import { useState } from "react";

function shortHash(head: string | undefined): string {
  const value = (head ?? "").trim();
  if (!value) return "";
  return value.slice(0, 7);
}

function toneForStatus(status: ProjectWorkspaceGitStatus | undefined): "ok" | "warn" | "mute" | "danger" {
  if (!status) return "mute";
  if (status.sample_error) return "warn";
  if (!status.applicable) return "mute";
  if (status.repo_state === "rebase" || status.repo_state === "merge") return "danger";
  if (status.is_clean === false) return "warn";
  if (status.is_clean === true) return "ok";
  return "mute";
}

function primaryLabel(status: ProjectWorkspaceGitStatus | undefined): string {
  if (!status) return "尚未采样";
  if (status.refresh_pending) return "正在刷新现场";
  if (status.sample_error) return workspaceGitSampleErrorLabel(status.sample_error);
  if (!status.applicable) return "非 git";
  if (status.repo_state === "rebase" || status.repo_state === "merge") {
    return workspaceGitRepoStateLabel(status.repo_state);
  }
  if (status.is_clean === true) return workspaceGitCleanLabel("clean");
  if (status.is_clean === false) return workspaceGitCleanLabel("dirty");
  return "尚未采样";
}

export function ProjectWorkspaceGitBadge({
  className,
  status,
}: {
  className?: string;
  status?: ProjectWorkspaceGitStatus | null;
}) {
  return (
    <StatusPill className={className} tone={toneForStatus(status ?? undefined)}>
      {primaryLabel(status ?? undefined)}
    </StatusPill>
  );
}

export function ProjectWorkspaceGitPanel({
  onRefresh,
  pending,
  status,
}: {
  onRefresh?: () => void;
  pending?: boolean;
  status?: ProjectWorkspaceGitStatus | null;
}) {
  const [expanded, setExpanded] = useState(false);
  const head = shortHash(status?.head_commit);
  const sampled =
    status?.sampled_at != null ? formatRelativeTime(status.sampled_at) : null;
  const dirty = status?.applicable && status.is_clean === false;
  const entries = status?.uncommitted_entries ?? [];
  const pillLabel = primaryLabel(status ?? undefined);
  const errorDetail =
    status?.sample_error && status.sample_error.trim() !== pillLabel
      ? status.sample_error
      : null;

  return (
    <div
      className="mt-3 min-w-0 overflow-hidden rounded-[14px] border border-line/80 bg-card-soft/60 px-3 py-2.5"
      data-testid="project-workspace-git-panel"
    >
      <div className="flex min-w-0 items-start gap-2">
        <div className="min-w-0 flex-1 space-y-1.5">
          <ProjectWorkspaceGitBadge
            className="max-w-full min-w-0 whitespace-normal break-words leading-snug"
            status={status}
          />
          <div className="flex min-w-0 flex-col gap-0.5">
            {head ? (
              <span className="font-mono text-[11px] tabular-nums text-ink-2" title={status?.head_commit}>
                HEAD {head}
              </span>
            ) : null}
            {status?.current_branch ? (
              <span
                className="min-w-0 max-w-full break-all font-mono text-[11px] text-ink-3"
                title={status.current_branch}
              >
                {status.current_branch}
              </span>
            ) : null}
            {sampled ? (
              <span className="text-[11px] text-ink-3">采样 {sampled}</span>
            ) : null}
          </div>
        </div>
        {onRefresh ? (
          <Button
            className="h-7 shrink-0 px-2.5 text-[12px]"
            disabled={pending || status?.refresh_pending}
            size="sm"
            type="button"
            variant="outline"
            onClick={onRefresh}
          >
            {pending || status?.refresh_pending ? "刷新中…" : "刷新现场"}
          </Button>
        ) : null}
      </div>
      {errorDetail ? (
        <p className="mt-1.5 min-w-0 break-words text-[11px] text-warn-text">{errorDetail}</p>
      ) : null}
      {dirty ? (
        <div className="mt-2">
          <button
            className="text-[11px] font-medium text-ink-2 underline-offset-2 hover:text-ink hover:underline"
            type="button"
            onClick={() => setExpanded((value) => !value)}
          >
            {expanded ? "收起未提交清单" : "展开未提交清单"}
            {status?.uncommitted_count
              ? `（${status.uncommitted_count}${status.uncommitted_truncated ? "+" : ""}）`
              : ""}
          </button>
          {expanded ? (
            <ul className="mt-2 max-h-48 space-y-1 overflow-auto rounded-[10px] border border-line/70 bg-card px-2.5 py-2 text-[11px]">
              {entries.map((entry) => (
                <li className="flex min-w-0 gap-2" key={`${entry.category}:${entry.path}`}>
                  <span className="shrink-0 text-ink-3">
                    {workspaceGitFileCategoryLabel(entry.category)}
                  </span>
                  <span className="min-w-0 break-all font-mono text-ink">{entry.path}</span>
                </li>
              ))}
              {status?.uncommitted_truncated ? (
                <li className="text-ink-3">
                  另有 {status.uncommitted_omitted ?? 0} 个未列出
                </li>
              ) : null}
              {entries.length === 0 ? (
                <li className="text-ink-3">清单未返回，仅知未提交数 {status?.uncommitted_count ?? 0}</li>
              ) : null}
            </ul>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

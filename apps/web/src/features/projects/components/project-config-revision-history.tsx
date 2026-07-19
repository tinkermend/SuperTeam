import type { ReactNode } from "react";
import { FileArchive, GitBranch, History, ShieldCheck } from "lucide-react";
import {
  IconTile,
  ObjectRef,
  SoftCard,
  StatusPill,
  V3Button,
  V3EmptyState,
  V3LoadingState,
} from "@/components/superteam";
import type { ProjectConfigRevision } from "@/lib/api/projects";

type ProjectConfigRevisionHistoryProps = {
  error?: string;
  isDetailLoading?: boolean;
  isLoading?: boolean;
  isRefreshing?: boolean;
  revisions: ProjectConfigRevision[];
  selectedRevision?: ProjectConfigRevision;
  selectedRevisionId?: string;
  onSelectRevision: (revisionId: string) => void;
  resolveUserName?: (id: string | null | undefined) => string | undefined;
};

const policySections = [
  {
    icon: GitBranch,
    keys: ["coordination_policy", "coordinationPolicy"],
    title: "协调策略",
  },
  {
    icon: ShieldCheck,
    keys: ["approval_policy", "approvalPolicy"],
    title: "审批策略",
  },
  {
    icon: FileArchive,
    keys: ["evidence_policy", "evidencePolicy"],
    title: "证据归档规则",
  },
] as const;

export function ProjectConfigRevisionHistory({
  error,
  isDetailLoading,
  isLoading,
  isRefreshing,
  revisions,
  selectedRevision,
  selectedRevisionId,
  onSelectRevision,
  resolveUserName,
}: ProjectConfigRevisionHistoryProps) {
  const sortedRevisions = [...revisions].sort(
    (left, right) => (right.revision_number ?? 0) - (left.revision_number ?? 0),
  );

  return (
    <SoftCard className="overflow-hidden">
      <div className="flex flex-col gap-3 border-b border-v3-line p-4 lg:flex-row lg:items-center lg:justify-between">
        <div className="flex min-w-0 items-start gap-3">
          <IconTile tone="mute">
            <History />
          </IconTile>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="font-semibold text-v3-ink">配置修订历史</h3>
              <StatusPill tone="mute">{revisions.length} 个 revision</StatusPill>
              {isRefreshing ? <StatusPill tone="info">刷新中</StatusPill> : null}
            </div>
            <p className="mt-1 text-xs text-v3-ink-2">
              查看项目配置快照，核对协调、审批和证据归档策略的版本变化。
            </p>
          </div>
        </div>
        {selectedRevision ? (
          <StatusPill tone="info">
            revision #{selectedRevision.revision_number}
          </StatusPill>
        ) : null}
      </div>

      {error ? (
        <div className="border-b border-v3-line bg-v3-danger-soft px-4 py-3 text-sm text-v3-danger">
          {error}
        </div>
      ) : null}

      {isLoading && revisions.length === 0 ? (
        <V3LoadingState label="正在加载配置修订历史" />
      ) : null}

      {!isLoading && revisions.length === 0 ? (
        <V3EmptyState title="暂无配置修订历史" />
      ) : null}

      {revisions.length > 0 ? (
        <div className="grid gap-0 lg:grid-cols-[280px_minmax(0,1fr)]">
          <div className="border-b border-v3-line p-3 lg:border-r lg:border-b-0">
            <div className="grid gap-2">
              {sortedRevisions.map((revision) => {
                const isSelected = revision.id === selectedRevisionId;
                return (
                  <V3Button
                    aria-label={`查看 revision #${revision.revision_number}`}
                    className="h-auto justify-start rounded-lg px-3 py-2 text-left"
                    key={revision.id}
                    type="button"
                    variant={isSelected ? "primary" : "ghost"}
                    onClick={() => onSelectRevision(revision.id)}
                  >
                    <span className="grid min-w-0 gap-1">
                      <span className="flex min-w-0 items-center gap-2">
                        <span className="truncate font-medium">
                          revision #{revision.revision_number}
                        </span>
                        {isSelected ? (
                          <StatusPill className="shrink-0" tone="info">
                            当前
                          </StatusPill>
                        ) : null}
                      </span>
                      <span className="truncate text-xs font-normal text-v3-ink-2">
                        {revision.change_summary || "未记录变更摘要"}
                      </span>
                    </span>
                  </V3Button>
                );
              })}
            </div>
          </div>

          <div className="min-w-0 p-4">
            {selectedRevision ? (
              <RevisionDetail
                isDetailLoading={isDetailLoading}
                resolveUserName={resolveUserName}
                revision={selectedRevision}
              />
            ) : (
              <div className="text-sm text-v3-ink-2">请选择一个 revision</div>
            )}
          </div>
        </div>
      ) : null}
    </SoftCard>
  );
}

function RevisionDetail({
  isDetailLoading,
  resolveUserName,
  revision,
}: {
  isDetailLoading?: boolean;
  resolveUserName?: (id: string | null | undefined) => string | undefined;
  revision: ProjectConfigRevision;
}) {
  const changedSections = revisionChangedSections(revision);
  const configSnapshot = revisionConfigSnapshot(revision);

  return (
    <div className="grid gap-4">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h4 className="text-sm font-semibold text-v3-ink">
              revision #{revision.revision_number}
            </h4>
            {isDetailLoading ? <StatusPill tone="info">详情加载中</StatusPill> : null}
            {revision.previous_revision_id ? (
              <StatusPill tone="mute">可对比上一版</StatusPill>
            ) : (
              <StatusPill tone="mute">初始快照</StatusPill>
            )}
          </div>
          <p className="mt-1 text-xs text-v3-ink-2">
            {revision.change_summary || "未记录变更摘要"}
          </p>
        </div>
        <dl className="grid gap-1 text-xs text-v3-ink-2 sm:grid-cols-2 lg:min-w-80">
          <RevisionMeta label="创建时间" value={formatDateTime(revision.created_at)} />
          <RevisionMeta
            label="创建人"
            value={
              revision.created_by_user_id ? (
                <ObjectRef
                  id={revision.created_by_user_id}
                  name={resolveUserName?.(revision.created_by_user_id)}
                />
              ) : (
                "未记录"
              )
            }
          />
          <RevisionMeta label="策略指纹" value={revision.policy_fingerprint || "未记录"} />
          <RevisionMeta label="事件 ID" value={revision.created_event_id || "未记录"} />
        </dl>
      </div>

      <div className="grid gap-2">
        <div className="flex items-center justify-between gap-3">
          <h5 className="text-sm font-semibold text-v3-ink">策略对比</h5>
          <StatusPill tone="mute">
            {changedSections.length} 个变更区块
          </StatusPill>
        </div>
        <div className="grid gap-3 xl:grid-cols-3">
          {policySections.map((section) => {
            const Icon = section.icon;
            const value = getPolicyValue(configSnapshot, section.keys);
            const changed = isPolicyChanged(revision, section.keys);
            return (
              <section
                className="min-w-0 rounded-v3-inner border border-v3-line bg-v3-card-soft p-3"
                key={section.title}
              >
                <div className="mb-2 flex items-center justify-between gap-2">
                  <div className="flex min-w-0 items-center gap-2">
                    <Icon className="size-4 shrink-0 text-v3-brand" />
                    <h6 className="truncate text-sm font-medium text-v3-ink">
                      {section.title}
                    </h6>
                  </div>
                  <StatusPill tone={changed ? "warn" : "mute"}>
                    {changed ? "本次变更" : "快照"}
                  </StatusPill>
                </div>
                <pre className="max-h-64 overflow-auto rounded-md border border-v3-line bg-v3-card p-3 font-mono text-xs leading-5 text-v3-ink">
                  {stringifyJson(value)}
                </pre>
              </section>
            );
          })}
        </div>
      </div>
    </div>
  );
}

function RevisionMeta({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="min-w-0 rounded-md border border-v3-line bg-v3-card-soft px-2.5 py-2">
      <dt>{label}</dt>
      <dd className="truncate font-medium text-v3-ink">{value}</dd>
    </div>
  );
}

function asRecord(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) return {};
  return value as Record<string, unknown>;
}

function asArray(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function revisionChangedSections(revision: ProjectConfigRevision): unknown[] {
  return asArray(
    (revision as ProjectConfigRevision & { changed_sections?: unknown }).changed_sections,
  );
}

function revisionDiffSummary(revision: ProjectConfigRevision): Record<string, unknown> {
  return asRecord(
    (revision as ProjectConfigRevision & { diff_summary?: unknown }).diff_summary,
  );
}

function revisionConfigSnapshot(revision: ProjectConfigRevision): Record<string, unknown> {
  return asRecord(
    (revision as ProjectConfigRevision & { config_snapshot?: unknown }).config_snapshot,
  );
}

function getPolicyValue(
  snapshot: Record<string, unknown>,
  keys: readonly string[],
): unknown {
  const containers = getSnapshotContainers(snapshot);
  for (const container of containers) {
    for (const key of keys) {
      if (Object.prototype.hasOwnProperty.call(container, key)) {
        return container[key];
      }
    }
  }
  return null;
}

function getSnapshotContainers(snapshot: Record<string, unknown>) {
  const containers = [snapshot];
  const nestedKeys = [
    "project",
    "project_snapshot",
    "projectSnapshot",
    "config",
    "config_snapshot",
    "configSnapshot",
  ];
  for (const key of nestedKeys) {
    const value = asRecord(snapshot[key]);
    if (Object.keys(value).length > 0) containers.push(value);
  }
  return containers;
}

function isPolicyChanged(
  revision: ProjectConfigRevision,
  keys: readonly string[],
): boolean {
  const changedSections = revisionChangedSections(revision).map((section) =>
    String(section),
  );
  const diffSummary = revisionDiffSummary(revision);
  if (keys.some((key) => changedSections.includes(key))) return true;
  return keys.some((key) =>
    Object.prototype.hasOwnProperty.call(diffSummary, key),
  );
}

function stringifyJson(value: unknown) {
  try {
    return JSON.stringify(value ?? null, null, 2) ?? "null";
  } catch {
    return String(value);
  }
}

function formatDateTime(value?: string) {
  if (!value) return "未记录";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

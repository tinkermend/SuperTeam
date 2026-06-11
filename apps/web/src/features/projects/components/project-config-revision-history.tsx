import { FileArchive, GitBranch, History, ShieldCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  LiquidCard,
  SemanticIconTile,
  StatusBadge,
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
}: ProjectConfigRevisionHistoryProps) {
  const sortedRevisions = [...revisions].sort(
    (left, right) => right.revision_number - left.revision_number,
  );

  return (
    <LiquidCard className="rounded-xl">
      <div className="flex flex-col gap-3 border-b p-4 lg:flex-row lg:items-center lg:justify-between">
        <div className="flex min-w-0 items-start gap-3">
          <SemanticIconTile tone="neutral">
            <History />
          </SemanticIconTile>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="font-semibold">配置修订历史</h3>
              <StatusBadge tone="neutral">{revisions.length} 个 revision</StatusBadge>
              {isRefreshing ? <StatusBadge tone="info">刷新中</StatusBadge> : null}
            </div>
            <p className="mt-1 text-xs text-muted-foreground">
              查看项目配置快照，核对协调、审批和证据归档策略的版本变化。
            </p>
          </div>
        </div>
        {selectedRevision ? (
          <StatusBadge tone="info">
            revision #{selectedRevision.revision_number}
          </StatusBadge>
        ) : null}
      </div>

      {error ? (
        <div className="border-b px-4 py-3 text-sm text-destructive">{error}</div>
      ) : null}

      {isLoading && revisions.length === 0 ? (
        <div className="p-5 text-sm text-muted-foreground">正在加载配置修订历史</div>
      ) : null}

      {!isLoading && revisions.length === 0 ? (
        <div className="p-5 text-sm text-muted-foreground">暂无配置修订历史</div>
      ) : null}

      {revisions.length > 0 ? (
        <div className="grid gap-0 lg:grid-cols-[280px_minmax(0,1fr)]">
          <div className="border-b p-3 lg:border-b-0 lg:border-r">
            <div className="grid gap-2">
              {sortedRevisions.map((revision) => {
                const isSelected = revision.id === selectedRevisionId;
                return (
                  <Button
                    aria-label={`查看 revision #${revision.revision_number}`}
                    className="h-auto justify-start rounded-lg px-3 py-2 text-left"
                    key={revision.id}
                    type="button"
                    variant={isSelected ? "secondary" : "ghost"}
                    onClick={() => onSelectRevision(revision.id)}
                  >
                    <span className="grid min-w-0 gap-1">
                      <span className="flex min-w-0 items-center gap-2">
                        <span className="truncate font-medium">
                          revision #{revision.revision_number}
                        </span>
                        {isSelected ? (
                          <StatusBadge className="shrink-0" tone="info">
                            当前
                          </StatusBadge>
                        ) : null}
                      </span>
                      <span className="truncate text-xs font-normal text-muted-foreground">
                        {revision.change_summary || "未记录变更摘要"}
                      </span>
                    </span>
                  </Button>
                );
              })}
            </div>
          </div>

          <div className="min-w-0 p-4">
            {selectedRevision ? (
              <RevisionDetail
                isDetailLoading={isDetailLoading}
                revision={selectedRevision}
              />
            ) : (
              <div className="text-sm text-muted-foreground">请选择一个 revision</div>
            )}
          </div>
        </div>
      ) : null}
    </LiquidCard>
  );
}

function RevisionDetail({
  isDetailLoading,
  revision,
}: {
  isDetailLoading?: boolean;
  revision: ProjectConfigRevision;
}) {
  return (
    <div className="grid gap-4">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h4 className="text-sm font-semibold">
              revision #{revision.revision_number}
            </h4>
            {isDetailLoading ? <StatusBadge tone="info">详情加载中</StatusBadge> : null}
            {revision.previous_revision_id ? (
              <StatusBadge tone="neutral">可对比上一版</StatusBadge>
            ) : (
              <StatusBadge tone="neutral">初始快照</StatusBadge>
            )}
          </div>
          <p className="mt-1 text-xs text-muted-foreground">
            {revision.change_summary || "未记录变更摘要"}
          </p>
        </div>
        <dl className="grid gap-1 text-xs text-muted-foreground sm:grid-cols-2 lg:min-w-80">
          <RevisionMeta label="创建时间" value={formatDateTime(revision.created_at)} />
          <RevisionMeta label="创建人" value={revision.created_by_user_id} />
          <RevisionMeta label="策略指纹" value={revision.policy_fingerprint || "未记录"} />
          <RevisionMeta label="事件 ID" value={revision.created_event_id || "未记录"} />
        </dl>
      </div>

      <div className="grid gap-2">
        <div className="flex items-center justify-between gap-3">
          <h5 className="text-sm font-semibold">策略对比</h5>
          <StatusBadge tone="neutral">
            {revision.changed_sections.length} 个变更区块
          </StatusBadge>
        </div>
        <div className="grid gap-3 xl:grid-cols-3">
          {policySections.map((section) => {
            const Icon = section.icon;
            const value = getPolicyValue(revision.config_snapshot, section.keys);
            const changed = isPolicyChanged(revision, section.keys);
            return (
              <section
                className="min-w-0 rounded-lg border bg-white/65 p-3"
                key={section.title}
              >
                <div className="mb-2 flex items-center justify-between gap-2">
                  <div className="flex min-w-0 items-center gap-2">
                    <Icon className="size-4 shrink-0 text-primary" />
                    <h6 className="truncate text-sm font-medium">{section.title}</h6>
                  </div>
                  <StatusBadge tone={changed ? "warning" : "neutral"}>
                    {changed ? "本次变更" : "快照"}
                  </StatusBadge>
                </div>
                <pre className="max-h-64 overflow-auto rounded-md border bg-white/80 p-3 text-xs leading-5 text-foreground">
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

function RevisionMeta({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-md border bg-white/60 px-2.5 py-2">
      <dt>{label}</dt>
      <dd className="truncate font-medium text-foreground">{value}</dd>
    </div>
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
    const value = snapshot[key];
    if (value && typeof value === "object" && !Array.isArray(value)) {
      containers.push(value as Record<string, unknown>);
    }
  }
  return containers;
}

function isPolicyChanged(
  revision: ProjectConfigRevision,
  keys: readonly string[],
): boolean {
  const changedSections = revision.changed_sections.map((section) => String(section));
  if (keys.some((key) => changedSections.includes(key))) return true;
  return keys.some((key) =>
    Object.prototype.hasOwnProperty.call(revision.diff_summary, key),
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

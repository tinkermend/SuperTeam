import { useMemo, useState } from "react";
import { StatusPill } from "@/components/superteam";
import type { CapabilityProjectionSnapshot, ProjectedSkillConflict } from "@/lib/api/projects";
import { statusLabel } from "@/lib/status-labels";

type AttemptCapabilityProjectionProps = {
  projection?: CapabilityProjectionSnapshot;
};

function sourcePill(scope: string | undefined) {
  const raw = (scope ?? "").trim();
  if (!raw) return null;
  return (
    <StatusPill showDot={false} tone={sourceTone(raw)}>
      {statusLabel(raw)}
    </StatusPill>
  );
}

function sourceTone(scope: string): "info" | "ok" | "warn" | "mute" {
  switch (scope) {
    case "project":
    case "project_binding":
      return "info";
    case "dependency_closure":
      return "warn";
    case "team":
      return "mute";
    case "employee":
      return "ok";
    case "workspace_native":
      return "warn";
    default:
      return "mute";
  }
}

function conflictSentence(conflict: ProjectedSkillConflict): string {
  const slug = conflict.slug || "未知技能";
  const source = (conflict.source ?? "").trim();
  if (source === "project_binding") {
    const win = statusLabel(conflict.winning_source || "project");
    const drop = statusLabel(conflict.dropped_source || "employee");
    return `「${slug}」保留${win}，覆盖${drop}`;
  }
  if (source === "workspace_native") {
    return `「${slug}」工作区已有同名技能，跳过员工侧投影`;
  }
  const sourceLabel = source ? statusLabel(source) : "未知来源";
  return `「${slug}」发生技能冲突（${sourceLabel}）`;
}

export function AttemptCapabilityProjection({ projection }: AttemptCapabilityProjectionProps) {
  const snap = projection;
  const summary = snap?.summary;
  const totalItems = (summary?.skill_count ?? 0) + (summary?.mcp_count ?? 0);
  const conflictCount = summary?.conflict_count ?? 0;
  const defaultExpanded = totalItems <= 6 && conflictCount === 0;
  const [expanded, setExpanded] = useState(defaultExpanded);

  const titleMeta = useMemo(() => {
    if (!snap?.available) return null;
    return `技能 ${summary?.skill_count ?? 0} · MCP ${summary?.mcp_count ?? 0} · 冲突 ${conflictCount}`;
  }, [snap, summary, conflictCount]);

  if (!snap) {
    return null;
  }

  return (
    <div className="grid gap-2 rounded-inner bg-card p-3" data-testid="attempt-capability-projection">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-xs font-semibold text-ink">能力投影</p>
        {snap.available && titleMeta ? (
          <span className="text-xs text-ink-2">{titleMeta}</span>
        ) : null}
      </div>

      {!snap.available ? (
        <p className="text-xs text-ink-3">本次尝试无能力投影快照</p>
      ) : totalItems === 0 && conflictCount === 0 ? (
        <p className="text-xs text-ink-3">未投影任何技能或 MCP</p>
      ) : (
        <>
          {!expanded ? (
            <button
              className="justify-self-start text-xs font-semibold text-brand hover:opacity-80"
              data-testid="attempt-capability-projection-expand"
              type="button"
              onClick={() => setExpanded(true)}
            >
              展开详情
            </button>
          ) : (
            <div className="grid gap-3">
              {snap.skills.length > 0 ? (
                <div className="grid gap-1.5">
                  <p className="text-[11px] font-semibold uppercase tracking-wide text-ink-3">技能</p>
                  <ul className="grid gap-1.5">
                    {snap.skills.map((skill) => {
                      const name = skill.skill_name?.trim() || skill.skill_key;
                      return (
                        <li
                          className="flex flex-wrap items-center justify-between gap-2 text-sm text-ink"
                          key={`${skill.skill_id}-${skill.skill_key}`}
                        >
                          <span className="min-w-0">
                            <span className="font-medium">{name}</span>
                            {skill.skill_name ? (
                              <span className="ml-1 font-mono text-xs text-ink-2">({skill.skill_key})</span>
                            ) : null}
                          </span>
                          {sourcePill(skill.source_scope)}
                        </li>
                      );
                    })}
                  </ul>
                </div>
              ) : null}

              {snap.mcp_servers.length > 0 ? (
                <div className="grid gap-1.5">
                  <p className="text-[11px] font-semibold uppercase tracking-wide text-ink-3">MCP</p>
                  <ul className="grid gap-1.5">
                    {snap.mcp_servers.map((server) => {
                      const name = server.server_name?.trim() || server.server_key;
                      return (
                        <li
                          className="flex flex-wrap items-center justify-between gap-2 text-sm text-ink"
                          key={`${server.server_id}-${server.server_key}`}
                        >
                          <span className="min-w-0">
                            <span className="font-medium">{name}</span>
                            {server.server_name ? (
                              <span className="ml-1 font-mono text-xs text-ink-2">({server.server_key})</span>
                            ) : null}
                          </span>
                          {sourcePill(server.source_scope)}
                        </li>
                      );
                    })}
                  </ul>
                </div>
              ) : null}

              {snap.skill_conflicts.length > 0 ? (
                <div className="grid gap-1.5">
                  <p className="text-[11px] font-semibold uppercase tracking-wide text-warn-text">冲突</p>
                  <ul className="grid gap-1.5">
                    {snap.skill_conflicts.map((conflict, index) => (
                      <li
                        className="rounded-inner bg-warn-soft px-2 py-1.5 text-xs text-ink"
                        key={`${conflict.slug}-${conflict.source}-${index}`}
                      >
                        {conflictSentence(conflict)}
                      </li>
                    ))}
                  </ul>
                </div>
              ) : null}

              {totalItems > 6 || conflictCount > 0 ? (
                <button
                  className="justify-self-start text-xs font-semibold text-ink-2 hover:text-ink"
                  type="button"
                  onClick={() => setExpanded(false)}
                >
                  收起
                </button>
              ) : null}
            </div>
          )}
        </>
      )}
    </div>
  );
}

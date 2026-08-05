import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/superteam";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  getPlaybookReadiness,
  listProjectCastings,
  listRoleCandidates,
  putProjectCastings,
  type CastingAssignment,
} from "@/lib/api/casting";
import { listScenarioTemplates, type ScenarioTemplate } from "@/lib/api/scenario-templates";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";

type PlaybookCastingPanelProps = {
  projectId: string;
  /** When set, only this template is editable (e.g. demand submit). */
  lockedTemplateKey?: string;
  onCastingSaved?: () => void;
};

function rolesFromTemplate(template: ScenarioTemplate | undefined): { key: string; title: string; caps: string[] }[] {
  const roles = template?.spec?.roles;
  if (!Array.isArray(roles)) return [];
  return roles
    .map((raw) => {
      const r = raw as { key?: string; title?: string; required_capabilities?: string[] };
      const key = (r.key ?? "").trim();
      if (!key) return null;
      return {
        key,
        title: (r.title ?? key).trim() || key,
        caps: Array.isArray(r.required_capabilities) ? r.required_capabilities : [],
      };
    })
    .filter((x): x is { key: string; title: string; caps: string[] } => x != null);
}

function readinessSummary(item: {
  runnable: boolean;
  deepest_exit: { label: string } | null;
  next_exit_needs_roles: string[];
  missing_roles_for_any: string[];
}): string {
  if (!item.runnable) {
    const miss = item.missing_roles_for_any?.length
      ? item.missing_roles_for_any.join("、")
      : "角色不足";
    return `暂不可跑 · 缺：${miss}`;
  }
  const deepest = item.deepest_exit?.label ?? "已可达";
  if (item.next_exit_needs_roles?.length) {
    return `可走到「${deepest}」 · 再往深需要：${item.next_exit_needs_roles.join("、")}`;
  }
  return `可走到「${deepest}」 · 角色齐备`;
}

export function PlaybookCastingPanel({
  projectId,
  lockedTemplateKey,
  onCastingSaved,
}: PlaybookCastingPanelProps) {
  const apiBaseUrl = resolveControlPlaneUrl();
  const queryClient = useQueryClient();
  const [templateKey, setTemplateKey] = useState(lockedTemplateKey ?? "");
  const [picks, setPicks] = useState<Record<string, string>>({});
  const [error, setError] = useState("");

  const templates = useQuery({
    queryKey: ["scenario-templates"],
    queryFn: () => listScenarioTemplates({ baseUrl: apiBaseUrl }),
  });
  const activeTemplates = (templates.data ?? []).filter((t) => t.status === "active");
  const selectedTemplate = activeTemplates.find((t) => t.template_key === templateKey);
  const roleRows = useMemo(() => rolesFromTemplate(selectedTemplate), [selectedTemplate]);

  const readiness = useQuery({
    queryKey: ["playbook-readiness", projectId],
    queryFn: () => getPlaybookReadiness({ baseUrl: apiBaseUrl }, projectId),
    enabled: Boolean(projectId),
  });

  const castings = useQuery({
    queryKey: ["project-castings", projectId, templateKey],
    queryFn: () => listProjectCastings({ baseUrl: apiBaseUrl }, projectId, templateKey),
    enabled: Boolean(projectId && templateKey),
  });

  // Seed picks from saved casting when template/castings load.
  useEffect(() => {
    if (!castings.data) return;
    const next: Record<string, string> = {};
    for (const row of castings.data) {
      next[row.role_key] = row.digital_employee_id;
    }
    if (Object.keys(next).length > 0) {
      setPicks((prev) => ({ ...next, ...prev }));
    }
  }, [castings.data]);

  const saveMutation = useMutation({
    mutationFn: async () => {
      const assignments: CastingAssignment[] = roleRows
        .map((r) => ({
          role_key: r.key,
          digital_employee_id: picks[r.key] ?? "",
        }))
        .filter((a) => a.digital_employee_id);
      return putProjectCastings(
        { baseUrl: apiBaseUrl },
        projectId,
        { scenario_template_key: templateKey, assignments },
      );
    },
    onSuccess: async () => {
      setError("");
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["project-castings", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["playbook-readiness", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["project-members", projectId] }),
      ]);
      onCastingSaved?.();
    },
    onError: (err: Error) => setError(err.message || "保存编制失败"),
  });

  return (
    <div className="grid gap-4 rounded-lg border border-border/60 p-4">
      <div>
        <h3 className="text-sm font-semibold text-ink">剧本编制</h3>
        <p className="mt-1 text-xs text-muted-foreground">
          为每个角色指定一人；选人自动加入项目成员池。能力仅作提示，⚠ 仍可选。
        </p>
      </div>

      {(readiness.data ?? []).length > 0 ? (
        <div className="grid gap-1.5 text-sm">
          {(readiness.data ?? []).map((item) => (
            <div
              key={item.scenario_template_key}
              className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5 rounded bg-muted/40 px-2 py-1.5"
            >
              <span className="font-medium">{item.template_name}</span>
              <span className="text-muted-foreground">
                {readinessSummary(item)}
              </span>
            </div>
          ))}
        </div>
      ) : null}

      {!lockedTemplateKey ? (
        <div className="grid gap-2">
          <Label>场景模板</Label>
          <Select value={templateKey || undefined} onValueChange={setTemplateKey}>
            <SelectTrigger className="w-full">
              <SelectValue placeholder="选择要编制的剧本" />
            </SelectTrigger>
            <SelectContent>
              {activeTemplates.map((t) => (
                <SelectItem key={t.template_key} value={t.template_key}>
                  {t.name}（{t.template_key}）
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      ) : null}

      {templateKey && roleRows.length > 0 ? (
        <div className="grid gap-3">
          {roleRows.map((role) => (
            <RolePickRow
              key={role.key}
              projectId={projectId}
              roleKey={role.key}
              roleTitle={role.title}
              requiredCaps={role.caps}
              value={picks[role.key] ?? ""}
              onChange={(employeeId) =>
                setPicks((prev) => ({ ...prev, [role.key]: employeeId }))
              }
            />
          ))}
          <div className="flex items-center gap-3">
            <Button
              type="button"
              disabled={saveMutation.isPending}
              onClick={() => saveMutation.mutate()}
            >
              {saveMutation.isPending ? "保存中…" : "保存编制"}
            </Button>
            {error ? <p className="text-sm text-destructive">{error}</p> : null}
          </div>
        </div>
      ) : templateKey ? (
        <p className="text-sm text-muted-foreground">该模板未声明 roles。</p>
      ) : null}
    </div>
  );
}

function RolePickRow({
  projectId,
  roleKey,
  roleTitle,
  requiredCaps,
  value,
  onChange,
}: {
  projectId: string;
  roleKey: string;
  roleTitle: string;
  requiredCaps: string[];
  value: string;
  onChange: (employeeId: string) => void;
}) {
  const apiBaseUrl = resolveControlPlaneUrl();
  const candidates = useQuery({
    queryKey: ["role-candidates", projectId, roleKey, requiredCaps.join(",")],
    queryFn: () =>
      listRoleCandidates({ baseUrl: apiBaseUrl }, projectId, roleKey, requiredCaps),
  });

  // Group by team name for display
  const groups = useMemo(() => {
    const map = new Map<string, typeof candidates.data>();
    for (const c of candidates.data ?? []) {
      const team = c.team_name || "未分组";
      const list = map.get(team) ?? [];
      list.push(c);
      map.set(team, list);
    }
    return Array.from(map.entries());
  }, [candidates.data]);

  return (
    <div className="grid gap-1.5">
      <Label>
        {roleTitle}
        <span className="ml-1 font-normal text-muted-foreground">({roleKey})</span>
      </Label>
      <Select value={value || undefined} onValueChange={onChange}>
        <SelectTrigger className="w-full">
          <SelectValue placeholder={candidates.isLoading ? "加载候选人…" : "选择员工"} />
        </SelectTrigger>
        <SelectContent>
          {(candidates.data ?? []).length === 0 ? (
            <SelectItem value="__none__" disabled>
              暂无具备该角色的员工
            </SelectItem>
          ) : (
            groups.flatMap(([team, list]) =>
              (list ?? []).map((c) => {
                const mark =
                  c.capability_fit === "matched"
                    ? "✓"
                    : c.capability_fit === "partial"
                      ? "⚠"
                      : "⚠";
                const capHint =
                  c.capability_fit === "matched"
                    ? c.matched_capabilities.length
                      ? `具备 ${c.matched_capabilities.join("、")}`
                      : "能力匹配"
                    : c.missing_capabilities.length
                      ? `缺 ${c.missing_capabilities.join("、")}`
                      : "能力不足";
                return (
                  <SelectItem key={c.digital_employee_id} value={c.digital_employee_id}>
                    {mark} {c.name}（{team}） · {capHint}
                  </SelectItem>
                );
              }),
            )
          )}
        </SelectContent>
      </Select>
    </div>
  );
}

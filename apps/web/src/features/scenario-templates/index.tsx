import { Fragment, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, ChevronRight, LayoutTemplate, Plus } from "lucide-react";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import {
  StatusPill,
  Button,
  EmptyState,
  ErrorState,
  LoadingState,
  MetricCard,
  DataTable,
  Td,
  Th,
  Tr,
  WorkSurface
} from "@/components/superteam";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { ApiRequestError } from "@/lib/api/client";
import {
  listScenarioTemplateVersions,
  listScenarioTemplates,
  patchScenarioTemplate,
  scenarioTemplateAcceptanceCriteria,
  scenarioTemplateExits,
  scenarioTemplateRoles,
  scenarioTemplateSkeleton,
  type ScenarioTemplate
} from "@/lib/api/scenario-templates";
import { getScenarioTemplateRoleView } from "@/lib/api/casting";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { formatRelativeTime } from "@/lib/format-time";
import { CreateScenarioTemplateDialog } from "./create-dialog";
import { CreateScenarioTemplateVersionDialog } from "./version-dialog";

export function ScenarioTemplatesPage() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const queryClient = useQueryClient();
  const [expandedKey, setExpandedKey] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [versionEditRow, setVersionEditRow] = useState<ScenarioTemplate | null>(null);
  const [statusToggleRow, setStatusToggleRow] = useState<ScenarioTemplate | null>(null);
  const [statusToggleError, setStatusToggleError] = useState<string | null>(null);

  const templates = useQuery({
    queryKey: ["scenario-templates"],
    queryFn: () => listScenarioTemplates({ baseUrl: apiBaseUrl })
});

  const patchMutation = useMutation({
    mutationFn: (input: { key: string; status: "active" | "disabled" }) =>
      patchScenarioTemplate({ baseUrl: apiBaseUrl }, input.key, { status: input.status }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["scenario-templates"] });
      setStatusToggleError(null);
      setStatusToggleRow(null);
    },
    onError: (error: unknown) => {
      setStatusToggleError(
        error instanceof ApiRequestError && error.detail
          ? error.detail
          : error instanceof Error
            ? error.message
            : "更新状态失败",
      );
    }
});

  const rows = templates.data ?? [];
  const isInitialLoading = templates.isPending && rows.length === 0;
  const isBlockingError = templates.isError && rows.length === 0;
  const activeCount = rows.filter((row) => row.status === "active").length;
  const nextStatus = statusToggleRow?.status === "active" ? "disabled" : "active";

  return (
    <>
      <ShellPageHeader
        icon={<LayoutTemplate />}
        iconTone="brand"
        title="场景模板"
        subtitle="沉淀各类场景的分解骨架与交接契约，驱动规划实例化"
      />
      <Main width="wide" className="min-w-0 overflow-x-hidden">
        <div className="flex min-w-0 flex-col gap-6">
          <div className="flex flex-wrap items-center justify-start gap-2 sm:justify-end">
            <Button className="h-11 self-start px-5" onClick={() => setShowCreate(true)}>
              <Plus data-icon="inline-start" />
              新建模板
            </Button>
          </div>

          <section
            className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3"
            aria-label="场景模板指标"
          >
            <MetricCard label="模板总数" value={`${rows.length}`} iconTone="brand" />
            <MetricCard label="启用中" value={`${activeCount}`} iconTone="ok" />
          </section>

          <WorkSurface className="min-w-0">
            {isInitialLoading ? (
              <LoadingState label="加载场景模板…" />
            ) : isBlockingError ? (
              <ErrorState title="加载失败" description="无法加载场景模板" />
            ) : rows.length === 0 ? (
              <EmptyState
                icon={<LayoutTemplate />}
                title="还没有场景模板"
                description="新建一个场景模板，或依赖种子数据；项目创建时可绑定其一驱动规划。"
              />
            ) : (
              <DataTable>
                <thead>
                  <tr>
                    <Th aria-label="展开" />
                    <Th>模板</Th>
                    <Th>key</Th>
                    <Th>角色</Th>
                    <Th>骨架步骤</Th>
                    <Th>状态</Th>
                    <Th>更新</Th>
                    <Th aria-label="操作" />
                  </tr>
                </thead>
                <tbody>
                  {rows.map((row) => (
                    <ScenarioTemplateRow
                      key={row.id}
                      apiBaseUrl={apiBaseUrl}
                      row={row}
                      expanded={expandedKey === row.template_key}
                      onToggle={() =>
                        setExpandedKey((current) =>
                          current === row.template_key ? null : row.template_key,
                        )
                      }
                      onRequestVersion={() => setVersionEditRow(row)}
                      onRequestStatusToggle={() => {
                        setStatusToggleError(null);
                        setStatusToggleRow(row);
                      }}
                    />
                  ))}
                </tbody>
              </DataTable>
            )}
          </WorkSurface>
        </div>
      </Main>

      <CreateScenarioTemplateDialog
        apiBaseUrl={apiBaseUrl}
        open={showCreate}
        onOpenChange={setShowCreate}
      />

      <CreateScenarioTemplateVersionDialog
        apiBaseUrl={apiBaseUrl}
        template={versionEditRow}
        onOpenChange={(open) => {
          if (!open) setVersionEditRow(null);
        }}
      />

      <ConfirmDialog
        open={statusToggleRow !== null}
        onOpenChange={(open) => {
          if (!open) {
            setStatusToggleRow(null);
            setStatusToggleError(null);
          }
        }}
        title={
          nextStatus === "disabled"
            ? `停用 ${statusToggleRow?.template_key ?? ""}`
            : `启用 ${statusToggleRow?.template_key ?? ""}`
        }
        desc={
          nextStatus === "disabled" ? (
            <div className="flex flex-col gap-2">
              <p>
                停用后新规划请求将回落到通用（generic）行为，不再匹配该场景模板的分解骨架与验收判据。已实例化的项目不受影响。
              </p>
              {statusToggleError ? (
                <p className="text-danger">{statusToggleError}</p>
              ) : null}
            </div>
          ) : (
            <div className="flex flex-col gap-2">
              <p>启用后新规划请求将重新匹配该场景模板的分解骨架与验收判据。</p>
              {statusToggleError ? (
                <p className="text-danger">{statusToggleError}</p>
              ) : null}
            </div>
          )
        }
        confirmText={nextStatus === "disabled" ? "确认停用" : "确认启用"}
        destructive={nextStatus === "disabled"}
        isLoading={patchMutation.isPending}
        handleConfirm={() => {
          if (statusToggleRow) {
            patchMutation.mutate({ key: statusToggleRow.template_key, status: nextStatus });
          }
        }}
      />
    </>
  );
}

function ScenarioTemplateRow({
  apiBaseUrl,
  row,
  expanded,
  onToggle,
  onRequestVersion,
  onRequestStatusToggle
}: {
  apiBaseUrl: string;
  row: ScenarioTemplate;
  expanded: boolean;
  onToggle: () => void;
  onRequestVersion: () => void;
  onRequestStatusToggle: () => void;
}) {
  const roles = scenarioTemplateRoles(row);
  const skeleton = scenarioTemplateSkeleton(row);
  const criteria = scenarioTemplateAcceptanceCriteria(row);
  const exits = scenarioTemplateExits(row);
  const skeletonChain = skeleton
    .map((step) => step.step ?? "")
    .filter(Boolean)
    .join(" → ");

  const renderCriterion = (criterion: typeof criteria[0]) => {
    if (typeof criterion === "string") {
      return criterion;
    }
    if (
      typeof criterion === "object" &&
      criterion !== null &&
      "statement" in criterion
    ) {
      const statement = criterion.statement;
      if (criterion.applies_from_exit) {
        const exit = exits.find((e) => e.deliverable === criterion.applies_from_exit);
        const exitLabel = exit?.label;
        if (exitLabel) {
          return `${statement}（出口 ≥ ${exitLabel}）`;
        }
        return `${statement}（出口 ≥ ${criterion.applies_from_exit}）`;
      }
      return statement;
    }
    return null;
  };

  const versions = useQuery({
    queryKey: ["scenario-template-versions", row.template_key],
    queryFn: () =>
      listScenarioTemplateVersions({ baseUrl: apiBaseUrl }, row.template_key),
    enabled: expanded
});

  const roleView = useQuery({
    queryKey: ["scenario-template-role-view", row.template_key],
    queryFn: () =>
      getScenarioTemplateRoleView({ baseUrl: apiBaseUrl }, row.template_key),
    enabled: expanded
});

  return (
    <Fragment>
      <Tr className="cursor-pointer" onClick={onToggle}>
        <Td className="w-8 text-ink-2">
          {expanded ? (
            <ChevronDown className="size-4" />
          ) : (
            <ChevronRight className="size-4" />
          )}
        </Td>
        <Td>
          <div className="min-w-0">
            <p className="truncate font-medium text-ink">{row.name}</p>
            <p className="truncate text-xs text-ink-2">{row.description}</p>
          </div>
        </Td>
        <Td className="font-mono text-xs">{row.template_key}</Td>
        <Td>{roles.length} 个</Td>
        <Td>{skeleton.length ? `${skeleton.length} 步` : "无骨架"}</Td>
        <Td>
          <StatusPill tone={row.status === "active" ? "ok" : "mute"}>
            {row.status === "active" ? "启用" : "停用"}
          </StatusPill>
        </Td>
        <Td className="text-xs text-ink-2 tabular-nums">
          {formatRelativeTime(row.updated_at)}
        </Td>
        <Td>
          <div className="flex justify-end gap-1" onClick={(event) => event.stopPropagation()}>
            <Button
              variant="outline"
              size="sm"
              onClick={onRequestVersion}
            >
              升版
            </Button>
            <Button
              variant={row.status === "active" ? "outline" : "primary"}
              size="sm"
              onClick={onRequestStatusToggle}
            >
              {row.status === "active" ? "停用" : "启用"}
            </Button>
          </div>
        </Td>
      </Tr>
      {expanded ? (
        <tr>
          <td colSpan={8} className="bg-card-soft px-4 py-3">
            <div className="grid gap-3 text-sm md:grid-cols-3">
              <div>
                <p className="text-xs font-semibold text-ink-2">角色</p>
                <ul className="mt-1 grid gap-1">
                  {roles.length === 0 ? (
                    <li className="text-ink-2">无角色约束</li>
                  ) : (
                    roles.map((role) => (
                      <li key={role.key ?? role.title}>
                        <span className="font-medium">{role.title ?? role.key}</span>
                        {role.independent_from?.length ? (
                          <span className="ml-1 text-xs text-danger">
                            须独立于 {role.independent_from.join("、")}
                          </span>
                        ) : null}
                        {role.collapsible_with?.length ? (
                          <span className="ml-1 text-xs text-ink-2">
                            可与 {role.collapsible_with.join("、")} 同人
                          </span>
                        ) : null}
                      </li>
                    ))
                  )}
                </ul>
              </div>
              <div>
                <p className="text-xs font-semibold text-ink-2">分解骨架</p>
                <p className="mt-1 font-mono text-xs">
                  {skeletonChain || "无（generic 行为）"}
                </p>
              </div>
              <div>
                <p className="text-xs font-semibold text-ink-2">默认验收判据</p>
                <ul className="mt-1 grid gap-1">
                  {criteria.length === 0 ? (
                    <li className="text-ink-2">无</li>
                  ) : (
                    criteria
                      .map((criterion, idx) => ({
                        key: typeof criterion === "string" ? criterion : `criterion-${idx}`,
                        rendered: renderCriterion(criterion)
}))
                      .filter((item) => item.rendered !== null)
                      .map((item) => (
                        <li key={item.key}>{item.rendered}</li>
                      ))
                  )}
                </ul>
              </div>
            </div>
            <div className="mt-4 border-t border-line pt-3">
              <p className="text-xs font-semibold text-ink-2">角色与收口</p>
              <p className="mt-0.5 text-[11px] text-ink-3">
                收口档位与所需角色由服务端计算（与规划期同一套规则），前端只渲染。
              </p>
              {roleView.isPending ? (
                <p className="mt-2 text-xs text-ink-2">加载角色视图…</p>
              ) : roleView.isError ? (
                <p className="mt-2 text-xs text-danger">无法加载角色视图</p>
              ) : (
                <div className="mt-2 grid gap-3 md:grid-cols-2">
                  <div>
                    <p className="text-[11px] font-semibold uppercase tracking-wide text-ink-3">
                      角色
                    </p>
                    {(roleView.data?.roles ?? []).length === 0 ? (
                      <p className="mt-1 text-xs text-ink-2">无角色约束</p>
                    ) : (
                      <ul className="mt-1 grid gap-1.5">
                        {(roleView.data?.roles ?? []).map((role) => (
                          <li key={role.role_key} className="text-xs">
                            <span className="font-medium text-ink">
                              {role.title || role.role_key}
                            </span>{" "}
                            <span className="font-mono text-ink-3">{role.role_key}</span>
                            {role.required_capabilities.length ? (
                              <span className="ml-1 text-ink-2">
                                建议能力 {role.required_capabilities.join("、")}
                              </span>
                            ) : null}
                            <span className="ml-1 text-ink-2">
                              · 本租户持有者 {role.holder_count} 人
                            </span>
                          </li>
                        ))}
                      </ul>
                    )}
                  </div>
                  <div>
                    <p className="text-[11px] font-semibold uppercase tracking-wide text-ink-3">
                      收口档位（浅 → 深）
                    </p>
                    {(roleView.data?.exits ?? []).length === 0 ? (
                      <p className="mt-1 text-xs text-ink-2">无收口档位</p>
                    ) : (
                      <ul className="mt-1 grid gap-1.5">
                        {(roleView.data?.exits ?? []).map((exit) => {
                          const titleOf = (key: string) => {
                            const hit = (roleView.data?.roles ?? []).find(
                              (r) => r.role_key === key,
                            );
                            return hit?.title || key;
                          };
                          const requiredLabels = exit.required_roles.map(titleOf);
                          const independenceNote = exit.role_independence_pairs
                            .map((pair) => pair.roles.map(titleOf).join(" + "))
                            .filter(Boolean);
                          return (
                            <li key={exit.deliverable} className="text-xs">
                              <span className="font-medium text-ink">
                                {exit.label || exit.deliverable}
                              </span>{" "}
                              <span className="font-mono text-ink-3">{exit.deliverable}</span>
                              <span className="block text-ink-2">
                                需要：
                                {requiredLabels.length
                                  ? requiredLabels.join(" + ")
                                  : "无角色"}
                                {independenceNote.length ? (
                                  <span className="ml-1 text-danger">
                                    （须不同人：{independenceNote.join("；")}）
                                  </span>
                                ) : null}
                              </span>
                            </li>
                          );
                        })}
                      </ul>
                    )}
                  </div>
                </div>
              )}
            </div>

            <div className="mt-4 border-t border-line pt-3">
              <p className="text-xs font-semibold text-ink-2">版本历史</p>
              {versions.isPending ? (
                <p className="mt-1 text-xs text-ink-2">加载版本历史…</p>
              ) : versions.isError ? (
                <p className="mt-1 text-xs text-danger">无法加载版本历史</p>
              ) : (versions.data ?? []).length === 0 ? (
                <p className="mt-1 text-xs text-ink-2">暂无版本记录</p>
              ) : (
                <ul className="mt-1 flex flex-col gap-1">
                  {(versions.data ?? []).map((version) => (
                    <li
                      key={version.id}
                      className="flex items-center gap-2 text-xs text-ink-2"
                    >
                      <span className="font-mono font-medium text-ink">
                        v{version.version}
                      </span>
                      <span className="tabular-nums">
                        {formatRelativeTime(version.created_at)}
                      </span>
                      {version.version === row.active_version ? (
                        <StatusPill tone="ok">当前版本</StatusPill>
                      ) : null}
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </td>
        </tr>
      ) : null}
    </Fragment>
  );
}

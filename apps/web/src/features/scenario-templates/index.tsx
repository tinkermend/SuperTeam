import { Fragment, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, ChevronRight, LayoutTemplate, Plus } from "lucide-react";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import {
  StatusPill,
  V3Button,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  V3MetricCard,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  WorkSurface,
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
  type ScenarioTemplate,
} from "@/lib/api/scenario-templates";
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
    queryFn: () => listScenarioTemplates({ baseUrl: apiBaseUrl }),
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
    },
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
            <V3Button className="h-11 self-start px-5" onClick={() => setShowCreate(true)}>
              <Plus data-icon="inline-start" />
              新建模板
            </V3Button>
          </div>

          <section
            className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3"
            aria-label="场景模板指标"
          >
            <V3MetricCard label="模板总数" value={`${rows.length}`} iconTone="brand" />
            <V3MetricCard label="启用中" value={`${activeCount}`} iconTone="ok" />
          </section>

          <WorkSurface className="min-w-0">
            {isInitialLoading ? (
              <V3LoadingState label="加载场景模板…" />
            ) : isBlockingError ? (
              <V3ErrorState title="加载失败" description="无法加载场景模板" />
            ) : rows.length === 0 ? (
              <V3EmptyState
                icon={<LayoutTemplate />}
                title="还没有场景模板"
                description="新建一个场景模板，或依赖种子数据；项目创建时可绑定其一驱动规划。"
              />
            ) : (
              <V3Table>
                <thead>
                  <tr>
                    <V3Th aria-label="展开" />
                    <V3Th>模板</V3Th>
                    <V3Th>key</V3Th>
                    <V3Th>角色</V3Th>
                    <V3Th>骨架步骤</V3Th>
                    <V3Th>状态</V3Th>
                    <V3Th>更新</V3Th>
                    <V3Th aria-label="操作" />
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
              </V3Table>
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
                <p className="text-v3-danger">{statusToggleError}</p>
              ) : null}
            </div>
          ) : (
            <div className="flex flex-col gap-2">
              <p>启用后新规划请求将重新匹配该场景模板的分解骨架与验收判据。</p>
              {statusToggleError ? (
                <p className="text-v3-danger">{statusToggleError}</p>
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
  onRequestStatusToggle,
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
    enabled: expanded,
  });

  return (
    <Fragment>
      <V3Tr className="cursor-pointer" onClick={onToggle}>
        <V3Td className="w-8 text-v3-ink-2">
          {expanded ? (
            <ChevronDown className="size-4" />
          ) : (
            <ChevronRight className="size-4" />
          )}
        </V3Td>
        <V3Td>
          <div className="min-w-0">
            <p className="truncate font-medium text-v3-ink">{row.name}</p>
            <p className="truncate text-xs text-v3-ink-2">{row.description}</p>
          </div>
        </V3Td>
        <V3Td className="font-mono text-xs">{row.template_key}</V3Td>
        <V3Td>{roles.length} 个</V3Td>
        <V3Td>{skeleton.length ? `${skeleton.length} 步` : "无骨架"}</V3Td>
        <V3Td>
          <StatusPill tone={row.status === "active" ? "ok" : "mute"}>
            {row.status === "active" ? "启用" : "停用"}
          </StatusPill>
        </V3Td>
        <V3Td className="text-xs text-v3-ink-2 tabular-nums">
          {formatRelativeTime(row.updated_at)}
        </V3Td>
        <V3Td>
          <div className="flex justify-end gap-1" onClick={(event) => event.stopPropagation()}>
            <V3Button
              variant="outline"
              size="sm"
              onClick={onRequestVersion}
            >
              升版
            </V3Button>
            <V3Button
              variant={row.status === "active" ? "outline" : "primary"}
              size="sm"
              onClick={onRequestStatusToggle}
            >
              {row.status === "active" ? "停用" : "启用"}
            </V3Button>
          </div>
        </V3Td>
      </V3Tr>
      {expanded ? (
        <tr>
          <td colSpan={8} className="bg-v3-card-soft px-4 py-3">
            <div className="grid gap-3 text-sm md:grid-cols-3">
              <div>
                <p className="text-xs font-semibold text-v3-ink-2">角色</p>
                <ul className="mt-1 grid gap-1">
                  {roles.length === 0 ? (
                    <li className="text-v3-ink-2">无角色约束</li>
                  ) : (
                    roles.map((role) => (
                      <li key={role.key ?? role.title}>
                        <span className="font-medium">{role.title ?? role.key}</span>
                        {role.independent_from?.length ? (
                          <span className="ml-1 text-xs text-v3-danger">
                            须独立于 {role.independent_from.join("、")}
                          </span>
                        ) : null}
                        {role.collapsible_with?.length ? (
                          <span className="ml-1 text-xs text-v3-ink-2">
                            可与 {role.collapsible_with.join("、")} 同人
                          </span>
                        ) : null}
                      </li>
                    ))
                  )}
                </ul>
              </div>
              <div>
                <p className="text-xs font-semibold text-v3-ink-2">分解骨架</p>
                <p className="mt-1 font-mono text-xs">
                  {skeletonChain || "无（generic 行为）"}
                </p>
              </div>
              <div>
                <p className="text-xs font-semibold text-v3-ink-2">默认验收判据</p>
                <ul className="mt-1 grid gap-1">
                  {criteria.length === 0 ? (
                    <li className="text-v3-ink-2">无</li>
                  ) : (
                    criteria
                      .map((criterion, idx) => ({
                        key: typeof criterion === "string" ? criterion : `criterion-${idx}`,
                        rendered: renderCriterion(criterion),
                      }))
                      .filter((item) => item.rendered !== null)
                      .map((item) => (
                        <li key={item.key}>{item.rendered}</li>
                      ))
                  )}
                </ul>
              </div>
            </div>
            <div className="mt-4 border-t border-v3-line pt-3">
              <p className="text-xs font-semibold text-v3-ink-2">版本历史</p>
              {versions.isPending ? (
                <p className="mt-1 text-xs text-v3-ink-2">加载版本历史…</p>
              ) : versions.isError ? (
                <p className="mt-1 text-xs text-v3-danger">无法加载版本历史</p>
              ) : (versions.data ?? []).length === 0 ? (
                <p className="mt-1 text-xs text-v3-ink-2">暂无版本记录</p>
              ) : (
                <ul className="mt-1 flex flex-col gap-1">
                  {(versions.data ?? []).map((version) => (
                    <li
                      key={version.id}
                      className="flex items-center gap-2 text-xs text-v3-ink-2"
                    >
                      <span className="font-mono font-medium text-v3-ink">
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

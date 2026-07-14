import { Fragment, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ChevronDown, ChevronRight, LayoutTemplate } from "lucide-react";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import {
  StatusPill,
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
import {
  listScenarioTemplates,
  scenarioTemplateAcceptanceCriteria,
  scenarioTemplateRoles,
  scenarioTemplateSkeleton,
  type ScenarioTemplate,
} from "@/lib/api/scenario-templates";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { formatRelativeTime } from "@/lib/format-time";

export function ScenarioTemplatesPage() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const [expandedKey, setExpandedKey] = useState<string | null>(null);

  const templates = useQuery({
    queryKey: ["scenario-templates"],
    queryFn: () => listScenarioTemplates({ baseUrl: apiBaseUrl }),
  });

  const rows = templates.data ?? [];
  const isInitialLoading = templates.isPending && rows.length === 0;
  const isBlockingError = templates.isError && rows.length === 0;
  const activeCount = rows.filter((row) => row.status === "active").length;

  return (
    <>
      <ShellPageHeader
        icon={<LayoutTemplate />}
        iconTone="brand"
        title="场景模板"
        subtitle="沉淀各类场景的分解骨架与交接契约，驱动规划实例化（P1 只读）"
      />
      <Main width="wide" className="min-w-0 overflow-x-hidden">
        <div className="flex min-w-0 flex-col gap-6">
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
                description="场景模板由种子数据或后续管理入口注册；项目创建时可绑定其一驱动规划。"
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
                  </tr>
                </thead>
                <tbody>
                  {rows.map((row) => (
                    <ScenarioTemplateRow
                      key={row.id}
                      row={row}
                      expanded={expandedKey === row.template_key}
                      onToggle={() =>
                        setExpandedKey((current) =>
                          current === row.template_key ? null : row.template_key,
                        )
                      }
                    />
                  ))}
                </tbody>
              </V3Table>
            )}
          </WorkSurface>
        </div>
      </Main>
    </>
  );
}

function ScenarioTemplateRow({
  row,
  expanded,
  onToggle,
}: {
  row: ScenarioTemplate;
  expanded: boolean;
  onToggle: () => void;
}) {
  const roles = scenarioTemplateRoles(row);
  const skeleton = scenarioTemplateSkeleton(row);
  const criteria = scenarioTemplateAcceptanceCriteria(row);
  const skeletonChain = skeleton
    .map((step) => step.step ?? "")
    .filter(Boolean)
    .join(" → ");

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
      </V3Tr>
      {expanded ? (
        <tr>
          <td colSpan={7} className="bg-v3-card-soft px-4 py-3">
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
                    criteria.map((criterion) => <li key={criterion}>{criterion}</li>)
                  )}
                </ul>
              </div>
            </div>
          </td>
        </tr>
      ) : null}
    </Fragment>
  );
}

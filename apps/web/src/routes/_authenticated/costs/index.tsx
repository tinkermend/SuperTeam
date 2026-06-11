import { createFileRoute, useSearch } from "@tanstack/react-router";
import { CircleDollarSign } from "lucide-react";
import { LiquidCard, SemanticIconTile } from "@/components/superteam";
import { CostsProjectView } from "@/features/projects/components/project-budget-panel";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";

export const Route = createFileRoute("/_authenticated/costs/")({
  component: CostsRoute,
});

function CostsRoute() {
  const search = useSearch({ strict: false });
  const projectId = projectIDFromSearch(search);

  return (
    <div className="mx-auto flex w-full max-w-6xl flex-col gap-4 p-4 md:p-6">
      <div className="flex items-center gap-3">
        <SemanticIconTile tone="warning" size="sm">
          <CircleDollarSign />
        </SemanticIconTile>
        <div className="min-w-0">
          <h1 className="text-lg font-semibold">成本中心</h1>
          <p className="truncate text-sm text-muted-foreground">
            {projectId ? `项目 ${projectId}` : "等待项目上下文"}
          </p>
        </div>
      </div>

      {projectId ? (
        <CostsProjectView
          apiBaseUrl={resolveControlPlaneUrl()}
          projectId={projectId}
        />
      ) : (
        <LiquidCard className="rounded-xl p-5">
          <div className="flex items-center gap-3">
            <SemanticIconTile tone="warning" size="sm">
              <CircleDollarSign />
            </SemanticIconTile>
            <div>
              <h2 className="font-semibold">请选择项目后查看成本</h2>
              <p className="mt-1 text-sm text-muted-foreground">
                从项目详情进入成本中心，或在地址中提供 project_id。
              </p>
            </div>
          </div>
        </LiquidCard>
      )}
    </div>
  );
}

function projectIDFromSearch(search: unknown) {
  if (!search || typeof search !== "object" || !("project_id" in search)) {
    return undefined;
  }
  const value = search.project_id;
  return typeof value === "string" ? value.trim() : undefined;
}

import { createFileRoute, useSearch } from "@tanstack/react-router";
import { CircleDollarSign } from "lucide-react";
import {
  IconTile,
  V3EmptyState,
  V3PageHeader,
  WorkSurface,
} from "@/components/superteam";
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
      <V3PageHeader
        back={
          <IconTile tone="warn" size="lg">
            <CircleDollarSign />
          </IconTile>
        }
        title="成本中心"
        subtitle={projectId ? `项目 ${projectId}` : "等待项目上下文"}
      />

      {projectId ? (
        <CostsProjectView
          apiBaseUrl={resolveControlPlaneUrl()}
          projectId={projectId}
        />
      ) : (
        <WorkSurface>
          <V3EmptyState
            icon={<CircleDollarSign />}
            title="请选择项目后查看成本"
            description="从项目详情进入成本中心，或在地址中提供 project_id。"
          />
        </WorkSurface>
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

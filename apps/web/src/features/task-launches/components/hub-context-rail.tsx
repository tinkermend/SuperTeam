import { useMutation, useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import type { ApiClientOptions } from "@/lib/api/client";
import { getProject, refreshProjectWorkspaceGitStatus } from "@/lib/api/projects";
import { ProjectWorkspaceGitPanel } from "@/features/projects/components/project-workspace-git-panel";

type HubContextRailProps = {
  apiOptions: ApiClientOptions;
  footnote: string;
  projectId: string;
  children?: ReactNode;
};

export function HubContextRail({
  apiOptions,
  children,
  footnote,
  projectId,
}: HubContextRailProps) {
  const projectDetailQuery = useQuery({
    enabled: Boolean(projectId),
    queryFn: () => getProject(apiOptions, projectId),
    queryKey: ["project", projectId],
  });
  const refreshGitMutation = useMutation({
    mutationFn: () => refreshProjectWorkspaceGitStatus(apiOptions, projectId),
    onSuccess: () => {
      void projectDetailQuery.refetch();
    },
  });

  return (
    <aside aria-label="项目现场" className="hub-rail hub-rail-right" data-testid="hub-context-rail">
      <p className="hub-rail-label">项目现场</p>
      <div className="hub-rail-section">
        {projectId ? (
          <ProjectWorkspaceGitPanel
            onRefresh={() => refreshGitMutation.mutate()}
            pending={
              refreshGitMutation.isPending ||
              Boolean(projectDetailQuery.data?.workspace_git?.refresh_pending)
            }
            status={projectDetailQuery.data?.workspace_git}
          />
        ) : (
          <p className="hub-rail-line">选择项目后显示 git 状态</p>
        )}
      </div>
      {children}
      <p className="hub-rail-foot">{footnote}</p>
    </aside>
  );
}

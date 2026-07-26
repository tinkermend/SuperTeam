import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { EmptyState, LoadingState, SoftCard } from "@/components/superteam";
import { ApiRequestError } from "@/lib/api/client";
import { getProjectDemandLaunchDetail } from "@/lib/api/projects";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";

export const Route = createFileRoute("/_authenticated/workflows/$demandId")({
  component: WorkflowDemandRedirect
});

/**
 * 流程编排详情页已退役（IA Phase 2 P2c），需求流程的权威落点是项目详情
 * `?tab=demands&demand=`。本壳只做 demand→project 解析后重定向，保住飞书卡片
 * `deep_link=/workflows/{demand}` 等历史深链；404 给明确空态，不劫持到别的需求
 * （沿用 Phase 1 直链保护语义）。
 */
function WorkflowDemandRedirect() {
  const { demandId } = Route.useParams();
  const navigate = useNavigate();
  const apiBaseUrl = resolveControlPlaneUrl();

  const detailQuery = useQuery({
    queryFn: () => getProjectDemandLaunchDetail({ baseUrl: apiBaseUrl }, demandId),
    // 与项目详情需求区同 key 族，重定向后落地页直接命中缓存。
    queryKey: ["workflow-detail", apiBaseUrl, demandId],
    retry: false
});

  const projectId = detailQuery.data?.project.id;

  useEffect(() => {
    if (!projectId) return;
    void navigate({
      params: { projectId },
      replace: true,
      search: { demand: demandId, tab: "demands" },
      to: "/projects/$projectId"
});
  }, [demandId, navigate, projectId]);

  if (
    detailQuery.isError &&
    detailQuery.error instanceof ApiRequestError &&
    detailQuery.error.status === 404
  ) {
    return (
      <SoftCard className="m-6 p-8">
        <EmptyState
          action={
            <Link className="text-sm font-semibold text-brand" search={{ view: "instances" }} to="/">
              前往任务中枢查看流程实例
            </Link>
          }
          description="该需求不存在或不可见，可能已被删除。"
          title="找不到该流程"
        />
      </SoftCard>
    );
  }

  if (detailQuery.isError) {
    return (
      <SoftCard className="m-6 p-8">
        <EmptyState description="加载需求信息失败，请稍后重试。" title="跳转失败" />
      </SoftCard>
    );
  }

  return <LoadingState label="正在打开需求流程…" />;
}

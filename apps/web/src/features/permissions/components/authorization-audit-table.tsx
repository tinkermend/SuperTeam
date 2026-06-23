import { useQuery } from "@tanstack/react-query";
import { FileClock } from "lucide-react";
import type { ApiClientOptions, AuthzDecisionRecord } from "@/lib/api";
import { listAuthzDecisions } from "@/lib/api";
import {
  IconTile,
  StatusPill,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  WorkSurface,
} from "@/components/superteam";

type AuthorizationAuditTableProps = {
  apiOptions: ApiClientOptions;
};

export function AuthorizationAuditTable({ apiOptions }: AuthorizationAuditTableProps) {
  const decisionsQuery = useQuery({
    queryKey: ["authz-decisions", apiOptions.baseUrl, 50, 0],
    queryFn: () => listAuthzDecisions({ ...apiOptions, limit: 50, offset: 0 }),
  });

  return (
    <WorkSurface>
      <div className="flex items-start gap-3 border-b border-v3-line px-5 py-4">
        <IconTile tone="artifact" size="sm">
          <FileClock />
        </IconTile>
        <div>
          <h2 className="text-base font-bold text-v3-ink">授权审计</h2>
          <p className="mt-1 text-sm text-v3-ink-2">最近 50 条授权决策。</p>
        </div>
      </div>
      {decisionsQuery.isLoading ? (
        <V3LoadingState label="加载授权审计…" />
      ) : decisionsQuery.isError ? (
        <V3ErrorState
          className="m-5"
          title="授权审计加载失败"
          description="请稍后刷新或检查 Control Plane 连接。"
        />
      ) : (decisionsQuery.data?.items.length ?? 0) === 0 ? (
        <V3EmptyState title="暂无授权审计记录" description="授权决策产生后会显示在这里。" />
      ) : (
        <V3Table>
          <thead>
            <V3Tr>
              <V3Th>时间</V3Th>
              <V3Th>结果</V3Th>
              <V3Th>动作</V3Th>
              <V3Th>Actor</V3Th>
              <V3Th>资源</V3Th>
              <V3Th>原因</V3Th>
            </V3Tr>
          </thead>
          <tbody>
            {decisionsQuery.data?.items.map((decision) => (
              <V3Tr key={decision.id} tone={decision.result === "succeeded" ? undefined : "danger"}>
                <V3Td className="whitespace-nowrap tabular-nums">{formatTime(decision.created_at)}</V3Td>
                <V3Td>
                  <DecisionBadge result={decision.result} />
                </V3Td>
                <V3Td>{decision.action}</V3Td>
                <V3Td>{formatActor(decision)}</V3Td>
                <V3Td>{formatResource(decision.resource_type, decision.resource_id)}</V3Td>
                <V3Td className="max-w-72 truncate">{decision.reason ?? "-"}</V3Td>
              </V3Tr>
            ))}
          </tbody>
        </V3Table>
      )}
    </WorkSurface>
  );
}

function DecisionBadge({ result }: { result: AuthzDecisionRecord["result"] }) {
  return <StatusPill tone={result === "succeeded" ? "ok" : "danger"}>{result === "succeeded" ? "允许" : "拒绝"}</StatusPill>;
}

function formatActor(decision: AuthzDecisionRecord) {
  const actorType = decision.actor_type ?? (decision.user_id ? "user" : "");
  const actorId = decision.actor_id ?? decision.user_id ?? decision.username ?? "";

  if (!actorType && !actorId) {
    return "-";
  }

  return [actorType, actorId].filter(Boolean).join(":");
}

function formatResource(type?: string | null, id?: string | null) {
  if (!type && !id) {
    return "-";
  }

  return [type, id].filter(Boolean).join(":");
}

function formatTime(value: string) {
  return new Intl.DateTimeFormat("zh-CN", {
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    month: "2-digit",
  }).format(new Date(value));
}

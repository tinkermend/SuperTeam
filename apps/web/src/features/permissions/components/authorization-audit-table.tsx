import { useQuery } from "@tanstack/react-query";
import { FileClock, ShieldAlert } from "lucide-react";
import type { ApiClientOptions, AuthzDecisionRecord } from "@/lib/api";
import { listAuthzDecisions } from "@/lib/api";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

type AuthorizationAuditTableProps = {
  apiOptions: ApiClientOptions;
};

export function AuthorizationAuditTable({ apiOptions }: AuthorizationAuditTableProps) {
  const decisionsQuery = useQuery({
    queryKey: ["authz-decisions", apiOptions.baseUrl, 50, 0],
    queryFn: () => listAuthzDecisions({ ...apiOptions, limit: 50, offset: 0 }),
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <FileClock />
          授权审计
        </CardTitle>
        <CardDescription>最近 50 条授权决策。</CardDescription>
      </CardHeader>
      <CardContent>
        {decisionsQuery.isLoading ? (
          <Skeleton className="h-40" />
        ) : decisionsQuery.isError ? (
          <Alert variant="destructive">
            <ShieldAlert />
            <AlertTitle>授权审计加载失败</AlertTitle>
            <AlertDescription>请稍后刷新或检查 Control Plane 连接。</AlertDescription>
          </Alert>
        ) : (decisionsQuery.data?.items.length ?? 0) === 0 ? (
          <p className="text-sm text-muted-foreground">暂无授权审计记录。</p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>时间</TableHead>
                <TableHead>结果</TableHead>
                <TableHead>动作</TableHead>
                <TableHead>Actor</TableHead>
                <TableHead>资源</TableHead>
                <TableHead>原因</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {decisionsQuery.data?.items.map((decision) => (
                <TableRow key={decision.id}>
                  <TableCell>{formatTime(decision.created_at)}</TableCell>
                  <TableCell>
                    <DecisionBadge result={decision.result} />
                  </TableCell>
                  <TableCell>{decision.action}</TableCell>
                  <TableCell>{formatActor(decision)}</TableCell>
                  <TableCell>{formatResource(decision.resource_type, decision.resource_id)}</TableCell>
                  <TableCell className="max-w-72 truncate">{decision.reason ?? "-"}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  );
}

function DecisionBadge({ result }: { result: AuthzDecisionRecord["result"] }) {
  return <Badge variant={result === "succeeded" ? "default" : "destructive"}>{result === "succeeded" ? "允许" : "拒绝"}</Badge>;
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

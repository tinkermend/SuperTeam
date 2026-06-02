import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, RefreshCw, Server } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import { approveRuntimeEnrollment, listRuntimeEnrollments, listRuntimeNodes } from "@/lib/api/runtime";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";

export function RuntimeNodesPage() {
  const apiBaseUrl = resolveControlPlaneUrl();

  return <RuntimeNodesView apiBaseUrl={apiBaseUrl} />;
}

type RuntimeNodesViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
};

export function RuntimeNodesView({ apiBaseUrl, fetcher }: RuntimeNodesViewProps) {
  const queryClient = useQueryClient();
  const enrollments = useQuery({
    queryKey: ["runtime-enrollments"],
    queryFn: () => listRuntimeEnrollments({ baseUrl: apiBaseUrl, fetcher }),
  });
  const nodes = useQuery({
    queryKey: ["runtime-nodes"],
    queryFn: () => listRuntimeNodes({ baseUrl: apiBaseUrl, fetcher }),
  });
  const approve = useMutation({
    mutationFn: (id: string) => approveRuntimeEnrollment({ baseUrl: apiBaseUrl, fetcher }, id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["runtime-enrollments"] });
      void queryClient.invalidateQueries({ queryKey: ["runtime-nodes"] });
    },
  });
  const pendingEnrollments = (enrollments.data ?? []).filter((item) => item.status === "pending");

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main>
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <div className="flex size-10 items-center justify-center rounded-md border bg-muted">
              <Server />
            </div>
            <div>
              <h1 className="text-2xl font-bold tracking-tight">Runtime 节点</h1>
              <p className="text-sm text-muted-foreground">接入审批、在线状态和 Provider 能力。</p>
            </div>
          </div>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              void queryClient.invalidateQueries({ queryKey: ["runtime-enrollments"] });
              void queryClient.invalidateQueries({ queryKey: ["runtime-nodes"] });
            }}
          >
            <RefreshCw className="size-4" />
            刷新
          </Button>
        </div>

        <div className="grid gap-4 xl:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
          <Card>
            <CardHeader>
              <CardTitle>待接入节点</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              {enrollments.isLoading ? <p className="text-sm text-muted-foreground">加载中</p> : null}
              {enrollments.isError ? <p className="text-sm text-destructive">接入列表加载失败</p> : null}
              {pendingEnrollments.map((item) => (
                <div key={item.id} className="flex items-center justify-between gap-3 rounded-md border p-3">
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="truncate font-medium">{item.node_id}</span>
                      <Badge variant="secondary">待接入</Badge>
                    </div>
                    <p className="mt-1 truncate text-xs text-muted-foreground">{item.id}</p>
                  </div>
                  <Button
                    type="button"
                    size="sm"
                    disabled={approve.isPending}
                    onClick={() => approve.mutate(item.id)}
                  >
                    <Check className="size-4" />
                    接入
                  </Button>
                </div>
              ))}
              {!enrollments.isLoading && pendingEnrollments.length === 0 ? (
                <p className="text-sm text-muted-foreground">暂无待接入节点</p>
              ) : null}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>已接入节点</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              {nodes.isLoading ? <p className="text-sm text-muted-foreground">加载中</p> : null}
              {nodes.isError ? <p className="text-sm text-destructive">节点列表加载失败</p> : null}
              {(nodes.data ?? []).map((node) => (
                <div key={node.node_id} className="flex items-center justify-between gap-3 rounded-md border p-3">
                  <div className="min-w-0">
                    <p className="truncate font-medium">{node.name || node.node_id}</p>
                    <p className="mt-1 truncate text-sm text-muted-foreground">
                      {node.supported_providers.length > 0 ? node.supported_providers.join(", ") : "无 Provider"}
                    </p>
                  </div>
                  <div className="flex shrink-0 items-center gap-2">
                    <span className="text-sm text-muted-foreground">
                      {node.current_load}/{node.max_slots}
                    </span>
                    <Badge variant={node.status === "online" ? "default" : "secondary"}>{node.status}</Badge>
                  </div>
                </div>
              ))}
              {!nodes.isLoading && (nodes.data ?? []).length === 0 ? (
                <p className="text-sm text-muted-foreground">暂无已接入节点</p>
              ) : null}
            </CardContent>
          </Card>
        </div>
      </Main>
    </>
  );
}

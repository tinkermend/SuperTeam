import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Server, ShieldAlert } from "lucide-react";
import type { ApiClientOptions, CreateRuntimeScopeRequest, RuntimeScope, RuntimeScopeNode, RuntimeScopeType } from "@/lib/api";
import { createRuntimeScope, listRuntimeScopes, updateRuntimeScope } from "@/lib/api";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

type RuntimeScopesProps = {
  apiOptions: ApiClientOptions;
};

const defaultScopeType: RuntimeScopeType = "tenant";

export function RuntimeScopes({ apiOptions }: RuntimeScopesProps) {
  const queryClient = useQueryClient();
  const scopesQuery = useQuery({
    queryKey: ["runtime-scopes", apiOptions.baseUrl],
    queryFn: () => listRuntimeScopes(apiOptions),
  });
  const createScopeMutation = useMutation({
    mutationFn: (input: CreateRuntimeScopeRequest) => createRuntimeScope(apiOptions, input),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["runtime-scopes"] }),
  });
  const updateScopeMutation = useMutation({
    mutationFn: ({ scopeId, status }: { scopeId: string; status: RuntimeScope["status"] }) => updateRuntimeScope(apiOptions, scopeId, status),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["runtime-scopes"] }),
  });
  const [runtimeNodeId, setRuntimeNodeId] = useState("");
  const [tenantId, setTenantId] = useState("");
  const [teamId, setTeamId] = useState("");
  const [scopeType, setScopeType] = useState<RuntimeScopeType>(defaultScopeType);
  const [scopeValue, setScopeValue] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  const nodes = scopesQuery.data?.nodes ?? [];

  function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const input = {
      runtime_node_id: runtimeNodeId.trim(),
      tenant_id: tenantId.trim(),
      team_id: teamId.trim() || undefined,
      scope_type: scopeType,
      scope_value: scopeValue.trim(),
    };

    if (!input.runtime_node_id || !input.tenant_id || !input.scope_value) {
      setFormError("请填写 Runtime Node ID、租户 ID 和范围值。");
      return;
    }

    setFormError(null);
    createScopeMutation.mutate(input, {
      onSuccess: () => {
        setRuntimeNodeId("");
        setTenantId("");
        setTeamId("");
        setScopeType(defaultScopeType);
        setScopeValue("");
      },
    });
  }

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Server />
            新增 Runtime 范围
          </CardTitle>
          <CardDescription>为 Runtime 节点绑定可领取任务的租户或团队范围。</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="grid gap-3 lg:grid-cols-[1fr_1fr_1fr_160px_1fr_auto]" onSubmit={handleSubmit}>
            <div className="flex flex-col gap-2">
              <Label htmlFor="runtime-node-id">Runtime Node ID</Label>
              <Input id="runtime-node-id" value={runtimeNodeId} onChange={(event) => setRuntimeNodeId(event.target.value)} />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="runtime-scope-tenant-id">租户 ID</Label>
              <Input id="runtime-scope-tenant-id" value={tenantId} onChange={(event) => setTenantId(event.target.value)} />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="runtime-scope-team-id">团队 ID</Label>
              <Input id="runtime-scope-team-id" value={teamId} onChange={(event) => setTeamId(event.target.value)} />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="runtime-scope-type">范围类型</Label>
              <Select value={scopeType} onValueChange={(value) => setScopeType(value as RuntimeScopeType)}>
                <SelectTrigger id="runtime-scope-type" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem value="tenant">租户</SelectItem>
                    <SelectItem value="team">团队</SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="runtime-scope-value">范围值</Label>
              <Input id="runtime-scope-value" value={scopeValue} onChange={(event) => setScopeValue(event.target.value)} />
            </div>
            <div className="flex items-end">
              <Button type="submit" disabled={createScopeMutation.isPending}>
                新增
              </Button>
            </div>
          </form>
          {formError ? <p className="mt-3 text-sm text-destructive">{formError}</p> : null}
          {createScopeMutation.isError ? <p className="mt-3 text-sm text-destructive">Runtime 范围创建失败。</p> : null}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Runtime 范围</CardTitle>
          <CardDescription>节点心跳状态、Provider 支持能力和已绑定授权范围。</CardDescription>
        </CardHeader>
        <CardContent>
          {scopesQuery.isLoading ? (
            <Skeleton className="h-48" />
          ) : scopesQuery.isError ? (
            <Alert variant="destructive">
              <ShieldAlert />
              <AlertTitle>Runtime 范围加载失败</AlertTitle>
              <AlertDescription>请稍后刷新或检查 Control Plane 连接。</AlertDescription>
            </Alert>
          ) : nodes.length === 0 ? (
            <p className="text-sm text-muted-foreground">暂无 Runtime 授权范围。</p>
          ) : (
            <RuntimeScopeTable nodes={nodes} onToggleScope={(scope) => updateScopeMutation.mutate({ scopeId: scope.id, status: scope.status === "active" ? "disabled" : "active" })} toggling={updateScopeMutation.isPending} />
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function RuntimeScopeTable({
  nodes,
  onToggleScope,
  toggling,
}: {
  nodes: RuntimeScopeNode[];
  onToggleScope: (scope: RuntimeScope) => void;
  toggling: boolean;
}) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Runtime 节点</TableHead>
          <TableHead>状态</TableHead>
          <TableHead>租户</TableHead>
          <TableHead>团队</TableHead>
          <TableHead>范围</TableHead>
          <TableHead>操作</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {nodes.flatMap((node) => renderNodeRows(node, onToggleScope, toggling))}
      </TableBody>
    </Table>
  );
}

function renderNodeRows(node: RuntimeScopeNode, onToggleScope: (scope: RuntimeScope) => void, toggling: boolean) {
  if (node.scopes.length === 0) {
    return [
      <TableRow key={`${node.runtime_node_id}-empty`}>
        <TableCell>{formatNode(node)}</TableCell>
        <TableCell>
          <Badge variant="secondary">{node.status}</Badge>
        </TableCell>
        <TableCell colSpan={4} className="text-muted-foreground">
          暂无范围
        </TableCell>
      </TableRow>,
    ];
  }

  return node.scopes.map((scope) => (
    <TableRow key={scope.id}>
      <TableCell>{formatNode(node)}</TableCell>
      <TableCell>
        <Badge variant={scope.status === "active" ? "default" : "secondary"}>{scope.status === "active" ? "启用" : "禁用"}</Badge>
      </TableCell>
      <TableCell>{scope.tenant_id}</TableCell>
      <TableCell>{scope.team_id ?? "-"}</TableCell>
      <TableCell>
        {scope.scope_type}:{scope.scope_value}
      </TableCell>
      <TableCell>
        <Button type="button" variant="outline" size="sm" disabled={toggling} onClick={() => onToggleScope(scope)}>
          {scope.status === "active" ? "禁用" : "启用"}
        </Button>
      </TableCell>
    </TableRow>
  ));
}

function formatNode(node: RuntimeScopeNode) {
  const providers = node.supported_providers.length > 0 ? node.supported_providers.join(", ") : "无 Provider";
  return `${node.name || node.node_id} (${providers}, ${node.current_load}/${node.max_slots})`;
}

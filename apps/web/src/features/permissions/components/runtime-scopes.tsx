import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Server, ShieldAlert } from "lucide-react";
import type { ApiClientOptions, CreateRuntimeScopeRequest, RuntimeScope, RuntimeScopeNode, RuntimeScopeType } from "@/lib/api";
import { createRuntimeScope, listRuntimeScopes, updateRuntimeScope } from "@/lib/api";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
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
  const [formError, setFormError] = useState<string | null>(null);
  const [scopePendingConfirmation, setScopePendingConfirmation] = useState<RuntimeScope | null>(null);

  const nodes = scopesQuery.data?.nodes ?? [];
  const trimmedRuntimeNodeId = runtimeNodeId.trim();
  const trimmedTenantId = tenantId.trim();
  const trimmedTeamId = teamId.trim();
  const derivedScopeValue = scopeType === "tenant" ? trimmedTenantId : trimmedTeamId;
  const pendingStatus = scopePendingConfirmation?.status === "active" ? "disabled" : "active";

  function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();

    if (!trimmedRuntimeNodeId || !trimmedTenantId) {
      setFormError("请填写 Runtime Node ID 和租户 ID。");
      return;
    }

    if (scopeType === "team" && !trimmedTeamId) {
      setFormError("团队范围需要填写团队 ID。");
      return;
    }

    const input: CreateRuntimeScopeRequest = {
      runtime_node_id: trimmedRuntimeNodeId,
      tenant_id: trimmedTenantId,
      scope_type: scopeType,
      scope_value: derivedScopeValue,
      ...(scopeType === "team" ? { team_id: trimmedTeamId } : {}),
    };

    setFormError(null);
    createScopeMutation.mutate(input, {
      onSuccess: () => {
        setRuntimeNodeId("");
        setTenantId("");
        setTeamId("");
        setScopeType(defaultScopeType);
      },
    });
  }

  function handleConfirmToggleScope() {
    if (!scopePendingConfirmation) {
      return;
    }

    updateScopeMutation.mutate(
      {
        scopeId: scopePendingConfirmation.id,
        status: pendingStatus,
      },
      {
        onSettled: () => setScopePendingConfirmation(null),
      },
    );
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
          <form className="grid gap-3 lg:grid-cols-[1fr_1fr_1fr_160px_auto]" noValidate onSubmit={handleSubmit}>
            <div className="flex flex-col gap-2">
              <Label htmlFor="runtime-node-id">Runtime Node ID</Label>
              <Input id="runtime-node-id" required aria-invalid={Boolean(formError && !trimmedRuntimeNodeId)} value={runtimeNodeId} onChange={(event) => setRuntimeNodeId(event.target.value)} />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="runtime-scope-tenant-id">租户 ID</Label>
              <Input id="runtime-scope-tenant-id" required aria-invalid={Boolean(formError && !trimmedTenantId)} value={tenantId} onChange={(event) => setTenantId(event.target.value)} />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="runtime-scope-team-id">团队 ID{scopeType === "team" ? "" : "（可选）"}</Label>
              <Input id="runtime-scope-team-id" required={scopeType === "team"} aria-invalid={Boolean(formError && scopeType === "team" && !trimmedTeamId)} value={teamId} onChange={(event) => setTeamId(event.target.value)} />
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
            <div className="flex items-end">
              <Button type="submit" disabled={createScopeMutation.isPending}>
                新增
              </Button>
            </div>
          </form>
          <p className="mt-3 text-sm text-muted-foreground">
            范围值将由{scopeType === "tenant" ? "租户 ID" : "团队 ID"}自动生成：{derivedScopeValue || "待填写"}
          </p>
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
            <RuntimeScopeTable nodes={nodes} onToggleScope={setScopePendingConfirmation} toggling={updateScopeMutation.isPending} />
          )}
        </CardContent>
      </Card>
      <AlertDialog open={Boolean(scopePendingConfirmation)} onOpenChange={(open) => !open && setScopePendingConfirmation(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {pendingStatus === "disabled" ? "确认禁用 Runtime 范围" : "确认启用 Runtime 范围"}
            </AlertDialogTitle>
            <AlertDialogDescription>
              此操作会影响该 Runtime 节点的后续任务领取行为。禁用后，节点不能再领取不在有效范围内的任务；启用后，节点会重新按该范围参与任务 claim。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={updateScopeMutation.isPending}>取消</AlertDialogCancel>
            <AlertDialogAction onClick={handleConfirmToggleScope} disabled={updateScopeMutation.isPending}>
              {pendingStatus === "disabled" ? "确认禁用" : "确认启用"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
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

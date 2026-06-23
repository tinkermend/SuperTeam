import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Server, ShieldAlert } from "lucide-react";
import type { ApiClientOptions, CreateRuntimeScopeRequest, RuntimeScope, RuntimeScopeNode, RuntimeScopeType } from "@/lib/api";
import { createRuntimeScope, listRuntimeScopes, updateRuntimeScope } from "@/lib/api";
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  IconTile,
  SoftCard,
  StatusPill,
  V3Button,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  WorkSurface,
} from "@/components/superteam";

type RuntimeScopesProps = {
  apiOptions: ApiClientOptions;
};

const defaultScopeType: RuntimeScopeType = "tenant";
const fieldClassName =
  "rounded-xl border-v3-line-strong bg-v3-card text-v3-ink shadow-none focus-visible:ring-v3-brand/60 focus-visible:ring-offset-v3-bg";
const labelClassName = "text-[13px] font-semibold text-v3-ink-2";

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
      <SoftCard className="p-5">
        <div className="mb-5 flex items-start gap-3">
          <IconTile tone="artifact" size="sm">
            <Server />
          </IconTile>
          <div>
            <h2 className="text-base font-bold text-v3-ink">新增 Runtime 范围</h2>
            <p className="mt-1 text-sm text-v3-ink-2">为 Runtime 节点绑定可领取任务的租户或团队范围。</p>
          </div>
        </div>
        <form className="grid gap-3 lg:grid-cols-[1fr_1fr_1fr_160px_1fr_auto]" noValidate onSubmit={handleSubmit}>
          <div className="flex flex-col gap-2">
            <Label className={labelClassName} htmlFor="runtime-node-id">Runtime Node ID</Label>
            <Input className={fieldClassName} id="runtime-node-id" required aria-invalid={Boolean(formError && !trimmedRuntimeNodeId)} value={runtimeNodeId} onChange={(event) => setRuntimeNodeId(event.target.value)} />
          </div>
          <div className="flex flex-col gap-2">
            <Label className={labelClassName} htmlFor="runtime-scope-tenant-id">租户 ID</Label>
            <Input className={fieldClassName} id="runtime-scope-tenant-id" required aria-invalid={Boolean(formError && !trimmedTenantId)} value={tenantId} onChange={(event) => setTenantId(event.target.value)} />
          </div>
          <div className="flex flex-col gap-2">
            <Label className={labelClassName} htmlFor="runtime-scope-team-id">团队 ID{scopeType === "team" ? "" : "（可选）"}</Label>
            <Input className={fieldClassName} id="runtime-scope-team-id" required={scopeType === "team"} aria-invalid={Boolean(formError && scopeType === "team" && !trimmedTeamId)} value={teamId} onChange={(event) => setTeamId(event.target.value)} />
          </div>
          <div className="flex flex-col gap-2">
            <Label className={labelClassName} htmlFor="runtime-scope-type">范围类型</Label>
            <Select value={scopeType} onValueChange={(value) => setScopeType(value as RuntimeScopeType)}>
              <SelectTrigger id="runtime-scope-type" className={`${fieldClassName} w-full`}>
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
            <Label className={labelClassName} htmlFor="runtime-scope-value">范围值</Label>
            <Input className={fieldClassName} id="runtime-scope-value" readOnly value={derivedScopeValue || "待填写"} />
          </div>
          <div className="flex items-end">
            <V3Button type="submit" disabled={createScopeMutation.isPending}>
              新增
            </V3Button>
          </div>
        </form>
        <p className="mt-3 text-sm text-v3-ink-2">
          范围值将由{scopeType === "tenant" ? "租户 ID" : "团队 ID"}自动生成：{derivedScopeValue || "待填写"}
        </p>
        {formError ? <p className="mt-3 text-sm text-v3-danger">{formError}</p> : null}
        {createScopeMutation.isError ? <p className="mt-3 text-sm text-v3-danger">Runtime 范围创建失败。</p> : null}
      </SoftCard>
      <WorkSurface>
        <div className="flex items-start gap-3 border-b border-v3-line px-5 py-4">
          <IconTile tone="info" size="sm">
            <Server />
          </IconTile>
          <div>
            <h2 className="text-base font-bold text-v3-ink">Runtime 范围</h2>
            <p className="mt-1 text-sm text-v3-ink-2">节点心跳状态、Provider 支持能力和已绑定授权范围。</p>
          </div>
        </div>
        {scopesQuery.isLoading ? (
          <V3LoadingState label="加载 Runtime 范围…" />
        ) : scopesQuery.isError ? (
          <V3ErrorState
            className="m-5"
            title="Runtime 范围加载失败"
            description="请稍后刷新或检查 Control Plane 连接。"
          />
        ) : nodes.length === 0 ? (
          <V3EmptyState icon={<ShieldAlert />} title="暂无 Runtime 授权范围" description="创建范围后，节点领取任务的边界会显示在这里。" />
        ) : (
          <RuntimeScopeTable nodes={nodes} onToggleScope={setScopePendingConfirmation} toggling={updateScopeMutation.isPending} />
        )}
      </WorkSurface>
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
    <V3Table>
      <thead>
        <V3Tr>
          <V3Th>Runtime 节点</V3Th>
          <V3Th>状态</V3Th>
          <V3Th>租户</V3Th>
          <V3Th>团队</V3Th>
          <V3Th>范围</V3Th>
          <V3Th>操作</V3Th>
        </V3Tr>
      </thead>
      <tbody>
        {nodes.flatMap((node) => renderNodeRows(node, onToggleScope, toggling))}
      </tbody>
    </V3Table>
  );
}

function renderNodeRows(node: RuntimeScopeNode, onToggleScope: (scope: RuntimeScope) => void, toggling: boolean) {
  if (node.scopes.length === 0) {
    return [
      <V3Tr key={`${node.runtime_node_id}-empty`}>
        <V3Td>{formatNode(node)}</V3Td>
        <V3Td>
          <StatusPill tone={node.status === "online" ? "ok" : "mute"}>{node.status}</StatusPill>
        </V3Td>
        <V3Td colSpan={4} className="text-v3-ink-3">
          暂无范围
        </V3Td>
      </V3Tr>,
    ];
  }

  return node.scopes.map((scope) => (
    <V3Tr key={scope.id}>
      <V3Td>{formatNode(node)}</V3Td>
      <V3Td>
        <StatusPill tone={scope.status === "active" ? "ok" : "mute"}>{scope.status === "active" ? "启用" : "禁用"}</StatusPill>
      </V3Td>
      <V3Td>{scope.tenant_id}</V3Td>
      <V3Td>{scope.team_id ?? "-"}</V3Td>
      <V3Td>
        {scope.scope_type}:{scope.scope_value}
      </V3Td>
      <V3Td>
        <V3Button type="button" variant="outline" size="sm" disabled={toggling} onClick={() => onToggleScope(scope)}>
          {scope.status === "active" ? "禁用" : "启用"}
        </V3Button>
      </V3Td>
    </V3Tr>
  ));
}

function formatNode(node: RuntimeScopeNode) {
  const providers = node.supported_providers.length > 0 ? node.supported_providers.join(", ") : "无 Provider";
  return `${node.name || node.node_id} (${providers}, ${node.current_load}/${node.max_slots})`;
}

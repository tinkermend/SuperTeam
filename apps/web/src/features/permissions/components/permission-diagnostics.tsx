import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { ShieldQuestion } from "lucide-react";
import type { ApiClientOptions, CheckPermissionRequest, CheckPermissionResponse } from "@/lib/api";
import { checkPermission } from "@/lib/api";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

type PermissionDiagnosticsProps = {
  apiOptions: ApiClientOptions;
};

const actionOptions: CheckPermissionRequest["action"][] = [
  "console.access",
  "tenant.access",
  "team.access",
  "task.claim",
  "authz_center.read",
  "runtime_scope.manage",
];

export function PermissionDiagnostics({ apiOptions }: PermissionDiagnosticsProps) {
  const [actorType, setActorType] = useState("user");
  const [actorId, setActorId] = useState("");
  const [action, setAction] = useState<CheckPermissionRequest["action"]>("console.access");
  const [resourceType, setResourceType] = useState("console");
  const [resourceId, setResourceId] = useState("web");
  const [tenantId, setTenantId] = useState("");
  const [teamId, setTeamId] = useState("");
  const [formError, setFormError] = useState<string | null>(null);
  const checkMutation = useMutation({
    mutationFn: (input: CheckPermissionRequest) => checkPermission(apiOptions, input),
  });
  const trimmedActorId = actorId.trim();
  const trimmedTenantId = tenantId.trim();
  const trimmedTeamId = teamId.trim();
  const trimmedResourceId = resourceId.trim();

  function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const validationError = validateDiagnosticForm({
      action,
      actorId: trimmedActorId,
      resourceId: trimmedResourceId,
      resourceType,
      teamId: trimmedTeamId,
      tenantId: trimmedTenantId,
    });

    if (validationError) {
      setFormError(validationError);
      return;
    }

    if (!trimmedActorId || !trimmedTenantId) {
      setFormError("请填写 Actor ID 和租户 ID。");
      return;
    }

    setFormError(null);
    checkMutation.mutate({
      actor: {
        id: trimmedActorId,
        type: actorType.trim(),
      },
      action,
      resource: {
        id: trimmedResourceId,
        type: resourceType.trim(),
      },
      tenant_id: trimmedTenantId,
      team_id: trimmedTeamId || undefined,
    });
  }

  function handleActionChange(nextAction: CheckPermissionRequest["action"]) {
    setAction(nextAction);
    setFormError(null);

    const defaults = getResourceDefaults(nextAction, {
      teamId: trimmedTeamId,
      tenantId: trimmedTenantId,
    });
    setResourceType(defaults.resourceType);
    setResourceId(defaults.resourceId);
  }

  function handleTenantIdChange(value: string) {
    setTenantId(value);
    const nextTenantId = value.trim();

    if (usesTenantResource(action) && (!resourceId.trim() || resourceId.trim() === tenantId.trim())) {
      setResourceId(nextTenantId);
    }
  }

  function handleTeamIdChange(value: string) {
    setTeamId(value);
    const nextTeamId = value.trim();

    if (action === "team.access" && (!resourceId.trim() || resourceId.trim() === teamId.trim())) {
      setResourceId(nextTeamId);
    }
  }

  return (
    <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_420px]">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <ShieldQuestion />
            权限诊断
          </CardTitle>
          <CardDescription>用当前授权引擎检查 Actor 对资源动作的访问结果。</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="grid gap-3 md:grid-cols-2" noValidate onSubmit={handleSubmit}>
            <div className="flex flex-col gap-2">
              <Label htmlFor="diagnostic-actor-type">Actor 类型</Label>
              <Select value={actorType} onValueChange={setActorType}>
                <SelectTrigger id="diagnostic-actor-type" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem value="user">user</SelectItem>
                    <SelectItem value="runtime_node">runtime_node</SelectItem>
                    <SelectItem value="service_account">service_account</SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="diagnostic-actor-id">Actor ID</Label>
              <Input id="diagnostic-actor-id" required aria-invalid={Boolean(formError && !trimmedActorId)} value={actorId} onChange={(event) => setActorId(event.target.value)} />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="diagnostic-action">动作</Label>
              <Select value={action} onValueChange={(value) => handleActionChange(value as CheckPermissionRequest["action"])}>
                <SelectTrigger id="diagnostic-action" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {actionOptions.map((option) => (
                      <SelectItem key={option} value={option}>
                        {option}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="diagnostic-resource-type">资源类型</Label>
              <Input id="diagnostic-resource-type" required aria-invalid={Boolean(formError && resourceType.trim() !== expectedResourceType(action))} value={resourceType} onChange={(event) => setResourceType(event.target.value)} />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="diagnostic-resource-id">资源 ID</Label>
              <Input id="diagnostic-resource-id" required aria-invalid={Boolean(formError && !trimmedResourceId)} value={resourceId} onChange={(event) => setResourceId(event.target.value)} />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="diagnostic-tenant-id">租户 ID</Label>
              <Input id="diagnostic-tenant-id" required aria-invalid={Boolean(formError && !trimmedTenantId)} value={tenantId} onChange={(event) => handleTenantIdChange(event.target.value)} />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="diagnostic-team-id">团队 ID</Label>
              <Input id="diagnostic-team-id" required={action === "team.access"} aria-invalid={Boolean(formError && action === "team.access" && !trimmedTeamId)} value={teamId} onChange={(event) => handleTeamIdChange(event.target.value)} />
            </div>
            <div className="flex items-end">
              <Button type="submit" disabled={checkMutation.isPending}>
                开始诊断
              </Button>
            </div>
          </form>
          {formError ? <p className="mt-3 text-sm text-destructive">{formError}</p> : null}
          {checkMutation.isError ? <p className="mt-3 text-sm text-destructive">权限诊断失败。</p> : null}
        </CardContent>
      </Card>
      <DiagnosticsResult result={checkMutation.data} />
    </div>
  );
}

function getResourceDefaults(
  action: CheckPermissionRequest["action"],
  scopeIds: {
    teamId: string;
    tenantId: string;
  },
) {
  switch (action) {
    case "console.access":
      return {
        resourceId: "web",
        resourceType: "console",
      };
    case "tenant.access":
    case "authz_center.read":
    case "runtime_scope.manage":
      return {
        resourceId: scopeIds.tenantId,
        resourceType: "tenant",
      };
    case "team.access":
      return {
        resourceId: scopeIds.teamId,
        resourceType: "team",
      };
    case "task.claim":
      return {
        resourceId: "",
        resourceType: "task",
      };
  }
}

function expectedResourceType(action: CheckPermissionRequest["action"]) {
  return getResourceDefaults(action, { teamId: "", tenantId: "" }).resourceType;
}

function usesTenantResource(action: CheckPermissionRequest["action"]) {
  return action === "tenant.access" || action === "authz_center.read" || action === "runtime_scope.manage";
}

function validateDiagnosticForm({
  action,
  actorId,
  resourceId,
  resourceType,
  teamId,
  tenantId,
}: {
  action: CheckPermissionRequest["action"];
  actorId: string;
  resourceId: string;
  resourceType: string;
  teamId: string;
  tenantId: string;
}) {
  if (!actorId || !tenantId) {
    return "请填写 Actor ID 和租户 ID。";
  }

  const expectedType = expectedResourceType(action);
  if (resourceType.trim() !== expectedType) {
    return `动作 ${action} 需要资源类型 ${expectedType}。`;
  }

  if (action === "console.access" && resourceId !== "web") {
    return "console.access 的资源 ID 应为 web。";
  }

  if (usesTenantResource(action) && !resourceId) {
    return `动作 ${action} 需要租户资源 ID。`;
  }

  if (action === "team.access" && (!teamId || !resourceId)) {
    return "team.access 需要团队 ID 和团队资源 ID。";
  }

  if (action === "task.claim" && !resourceId) {
    return "task.claim 需要任务资源 ID。";
  }

  return null;
}

function DiagnosticsResult({ result }: { result?: CheckPermissionResponse }) {
  if (!result) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>诊断结果</CardTitle>
          <CardDescription>提交后展示授权引擎、命中规则和快照。</CardDescription>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">暂无诊断结果。</p>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center justify-between gap-3">
          <span>诊断结果</span>
          <Badge variant={result.allowed ? "default" : "destructive"}>{result.allowed ? "允许" : "拒绝"}</Badge>
        </CardTitle>
        <CardDescription>{result.engine}</CardDescription>
      </CardHeader>
      <CardContent>
        <Alert>
          <AlertTitle>{result.reason}</AlertTitle>
          <AlertDescription>
            命中规则：{result.matched_rule || "-"}
            <br />
            快照字段：{result.snapshot ? Object.keys(result.snapshot).length : 0}
          </AlertDescription>
        </Alert>
      </CardContent>
    </Card>
  );
}

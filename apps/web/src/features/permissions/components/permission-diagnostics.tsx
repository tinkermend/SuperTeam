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

  function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();

    if (!actorId.trim() || !tenantId.trim()) {
      setFormError("请填写 Actor ID 和租户 ID。");
      return;
    }

    setFormError(null);
    checkMutation.mutate({
      actor: {
        id: actorId.trim(),
        type: actorType.trim(),
      },
      action,
      resource: {
        id: resourceId.trim(),
        type: resourceType.trim(),
      },
      tenant_id: tenantId.trim(),
      team_id: teamId.trim() || undefined,
    });
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
          <form className="grid gap-3 md:grid-cols-2" onSubmit={handleSubmit}>
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
              <Input id="diagnostic-actor-id" value={actorId} onChange={(event) => setActorId(event.target.value)} />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="diagnostic-action">动作</Label>
              <Select value={action} onValueChange={(value) => setAction(value as CheckPermissionRequest["action"])}>
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
              <Input id="diagnostic-resource-type" value={resourceType} onChange={(event) => setResourceType(event.target.value)} />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="diagnostic-resource-id">资源 ID</Label>
              <Input id="diagnostic-resource-id" value={resourceId} onChange={(event) => setResourceId(event.target.value)} />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="diagnostic-tenant-id">租户 ID</Label>
              <Input id="diagnostic-tenant-id" value={tenantId} onChange={(event) => setTenantId(event.target.value)} />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="diagnostic-team-id">团队 ID</Label>
              <Input id="diagnostic-team-id" value={teamId} onChange={(event) => setTeamId(event.target.value)} />
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

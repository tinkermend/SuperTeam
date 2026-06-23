import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { ShieldQuestion } from "lucide-react";
import type { ApiClientOptions, CheckPermissionRequest, CheckPermissionResponse } from "@/lib/api";
import { checkPermission } from "@/lib/api";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { IconTile, SoftCard, StatusPill, V3Button } from "@/components/superteam";

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
  "team.create",
  "team.read",
  "team.update",
  "team.disable",
  "team.archive",
  "team.restore",
  "team.member.add",
  "team.member.remove",
  "team.member.change_role",
  "team.member.request_privileged_role",
  "team.member.approve_privileged_role",
  "team.governance.read",
  "team.governance.edit",
  "team.governance.approve",
  "team.capability.bind",
  "team.capability.unbind",
  "team.audit.read",
];
const fieldClassName =
  "rounded-xl border-v3-line-strong bg-v3-card text-v3-ink shadow-none focus-visible:ring-v3-brand/60 focus-visible:ring-offset-v3-bg";
const labelClassName = "text-[13px] font-semibold text-v3-ink-2";

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

    if (usesTenantResource(action, resourceType) && (!resourceId.trim() || resourceId.trim() === tenantId.trim())) {
      setResourceId(nextTenantId);
    }
  }

  function handleTeamIdChange(value: string) {
    setTeamId(value);
    const nextTeamId = value.trim();

    if (usesTeamResource(action, resourceType) && (!resourceId.trim() || resourceId.trim() === teamId.trim())) {
      setResourceId(nextTeamId);
    }
  }

  return (
    <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_420px]">
      <SoftCard className="p-5">
        <div className="mb-5 flex items-start gap-3">
          <IconTile tone="warn" size="sm">
            <ShieldQuestion />
          </IconTile>
          <div>
            <h2 className="text-base font-bold text-v3-ink">权限诊断</h2>
            <p className="mt-1 text-sm text-v3-ink-2">用当前授权引擎检查 Actor 对资源动作的访问结果。</p>
          </div>
        </div>
        <form className="grid gap-3 md:grid-cols-2" noValidate onSubmit={handleSubmit}>
            <div className="flex flex-col gap-2">
              <Label className={labelClassName} htmlFor="diagnostic-actor-type">Actor 类型</Label>
              <Select value={actorType} onValueChange={setActorType}>
                <SelectTrigger id="diagnostic-actor-type" className={`${fieldClassName} w-full`}>
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
              <Label className={labelClassName} htmlFor="diagnostic-actor-id">Actor ID</Label>
              <Input className={fieldClassName} id="diagnostic-actor-id" required aria-invalid={Boolean(formError && !trimmedActorId)} value={actorId} onChange={(event) => setActorId(event.target.value)} />
            </div>
            <div className="flex flex-col gap-2">
              <Label className={labelClassName} htmlFor="diagnostic-action">动作</Label>
              <Select value={action} onValueChange={(value) => handleActionChange(value as CheckPermissionRequest["action"])}>
                <SelectTrigger id="diagnostic-action" className={`${fieldClassName} w-full`}>
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
              <Label className={labelClassName} htmlFor="diagnostic-resource-type">资源类型</Label>
              <Input className={fieldClassName} id="diagnostic-resource-type" required aria-invalid={Boolean(formError && !expectedResourceTypes(action).includes(resourceType.trim()))} value={resourceType} onChange={(event) => setResourceType(event.target.value)} />
            </div>
            <div className="flex flex-col gap-2">
              <Label className={labelClassName} htmlFor="diagnostic-resource-id">资源 ID</Label>
              <Input className={fieldClassName} id="diagnostic-resource-id" required aria-invalid={Boolean(formError && !trimmedResourceId)} value={resourceId} onChange={(event) => setResourceId(event.target.value)} />
            </div>
            <div className="flex flex-col gap-2">
              <Label className={labelClassName} htmlFor="diagnostic-tenant-id">租户 ID</Label>
              <Input className={fieldClassName} id="diagnostic-tenant-id" required aria-invalid={Boolean(formError && !trimmedTenantId)} value={tenantId} onChange={(event) => handleTenantIdChange(event.target.value)} />
            </div>
            <div className="flex flex-col gap-2">
              <Label className={labelClassName} htmlFor="diagnostic-team-id">团队 ID</Label>
              <Input className={fieldClassName} id="diagnostic-team-id" required={usesTeamResource(action, resourceType)} aria-invalid={Boolean(formError && usesTeamResource(action, resourceType) && !trimmedTeamId)} value={teamId} onChange={(event) => handleTeamIdChange(event.target.value)} />
            </div>
            <div className="flex items-end">
              <V3Button type="submit" disabled={checkMutation.isPending}>
                开始诊断
              </V3Button>
            </div>
        </form>
        {formError ? <p className="mt-3 text-sm text-v3-danger">{formError}</p> : null}
        {checkMutation.isError ? <p className="mt-3 text-sm text-v3-danger">权限诊断失败。</p> : null}
      </SoftCard>
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
    case "team.create":
      return {
        resourceId: scopeIds.tenantId,
        resourceType: "tenant",
      };
    case "team.read":
      if (scopeIds.teamId) {
        return {
          resourceId: scopeIds.teamId,
          resourceType: "team",
        };
      }
      return {
        resourceId: scopeIds.tenantId,
        resourceType: "tenant",
      };
    case "team.access":
    case "team.update":
    case "team.disable":
    case "team.archive":
    case "team.restore":
    case "team.member.add":
    case "team.member.remove":
    case "team.member.change_role":
    case "team.member.request_privileged_role":
    case "team.member.approve_privileged_role":
    case "team.governance.read":
    case "team.governance.edit":
    case "team.governance.approve":
    case "team.capability.bind":
    case "team.capability.unbind":
    case "team.audit.read":
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

function expectedResourceTypes(action: CheckPermissionRequest["action"]) {
  if (action === "team.read") {
    return ["tenant", "team"];
  }
  return [getResourceDefaults(action, { teamId: "", tenantId: "" }).resourceType];
}

function usesTenantResource(action: CheckPermissionRequest["action"], resourceType: string) {
  return action === "tenant.access" || action === "authz_center.read" || action === "runtime_scope.manage" || action === "team.create" || (action === "team.read" && resourceType.trim() === "tenant");
}

function usesTeamResource(action: CheckPermissionRequest["action"], resourceType: string) {
  if (action === "team.read") {
    return resourceType.trim() === "team";
  }
  switch (action) {
    case "team.access":
    case "team.update":
    case "team.disable":
    case "team.archive":
    case "team.restore":
    case "team.member.add":
    case "team.member.remove":
    case "team.member.change_role":
    case "team.member.request_privileged_role":
    case "team.member.approve_privileged_role":
    case "team.governance.read":
    case "team.governance.edit":
    case "team.governance.approve":
    case "team.capability.bind":
    case "team.capability.unbind":
    case "team.audit.read":
      return true;
    default:
      return false;
  }
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

  const expectedTypes = expectedResourceTypes(action);
  if (!expectedTypes.includes(resourceType.trim())) {
    return `动作 ${action} 需要资源类型 ${expectedTypes.join(" 或 ")}。`;
  }

  if (action === "console.access" && resourceId !== "web") {
    return "console.access 的资源 ID 应为 web。";
  }

  if (usesTenantResource(action, resourceType) && !resourceId) {
    return `动作 ${action} 需要租户资源 ID。`;
  }

  if (usesTeamResource(action, resourceType) && (!teamId || !resourceId)) {
    return `动作 ${action} 需要团队 ID 和团队资源 ID。`;
  }

  if (action === "task.claim" && !resourceId) {
    return "task.claim 需要任务资源 ID。";
  }

  return null;
}

function DiagnosticsResult({ result }: { result?: CheckPermissionResponse }) {
  if (!result) {
    return (
      <SoftCard className="p-5">
        <h2 className="text-base font-bold text-v3-ink">诊断结果</h2>
        <p className="mt-1 text-sm text-v3-ink-2">提交后展示授权引擎、命中规则和快照。</p>
        <div className="mt-6 rounded-v3-inner bg-v3-card-soft px-4 py-8 text-center text-sm text-v3-ink-3">
          暂无诊断结果。
        </div>
      </SoftCard>
    );
  }

  return (
    <SoftCard className="p-5">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-base font-bold text-v3-ink">诊断结果</h2>
          <p className="mt-1 text-sm text-v3-ink-2">{result.engine}</p>
        </div>
        <StatusPill tone={result.allowed ? "ok" : "danger"}>{result.allowed ? "允许" : "拒绝"}</StatusPill>
      </div>
      <div className="mt-5 rounded-v3-inner bg-v3-card-soft p-4">
        <p className="text-sm font-bold text-v3-ink">{result.reason}</p>
        <p className="mt-2 text-sm text-v3-ink-2">命中规则：{result.matched_rule || "-"}</p>
        <p className="mt-1 text-sm text-v3-ink-2">快照字段：{result.snapshot ? Object.keys(result.snapshot).length : 0}</p>
      </div>
    </SoftCard>
  );
}

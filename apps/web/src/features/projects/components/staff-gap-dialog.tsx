import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from "@/components/ui/select";
import { Button,
  notifySuccess
} from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  createDigitalEmployee,
  listDigitalEmployeeAvatarAssets,
  type DigitalEmployee
} from "@/lib/api/employees";
import { listEmployeeTemplates, type EmployeeTemplate } from "@/lib/api/employee-templates";
import {
  listProjectCastings,
  putProjectCastings,
} from "@/lib/api/casting";
import {
  getProject,
  listProjectMembers,
  replaceProjectMembers,
  resolveProjectDecision,
  type ProjectMember,
  type ProjectMemberInput,
  type ProjectTaskGraphBlockingFactGap
} from "@/lib/api/projects";

const DEFAULT_PROVIDER_TYPE = "claude-code";

export type StaffGapDialogProps = {
  apiOptions: ApiClientOptions;
  decisionRequestId: string;
  gap: ProjectTaskGraphBlockingFactGap;
  onOpenChange: (open: boolean) => void;
  onStaffed?: () => void;
  open: boolean;
  projectId: string;
  scenarioTemplateKey?: string;
};

/**
 * 一键补员：按系统模板创建员工（写入缺口角色）→ 入项目成员 → 写入编制 → resolve restaffed。
 */
export function StaffGapDialog({
  apiOptions,
  decisionRequestId,
  gap,
  onOpenChange,
  onStaffed,
  open,
  projectId,
  scenarioTemplateKey
}: StaffGapDialogProps) {
  const queryClient = useQueryClient();
  const [templateType, setTemplateType] = useState("");
  const [name, setName] = useState("");
  const [providerType, setProviderType] = useState(DEFAULT_PROVIDER_TYPE);
  const [error, setError] = useState("");
  const createdEmployeeRef = useRef<DigitalEmployee | null>(null);
  const membersDoneRef = useRef(false);
  const castingDoneRef = useRef(false);

  const templatesQuery = useQuery({
    enabled: open,
    queryFn: () => listEmployeeTemplates(apiOptions),
    queryKey: ["employee-templates", apiOptions.baseUrl]
  });
  const projectQuery = useQuery({
    enabled: open,
    queryFn: () => getProject(apiOptions, projectId),
    queryKey: ["staff-gap-project", apiOptions.baseUrl, projectId]
  });
  const avatarAssetsQuery = useQuery({
    enabled: open,
    queryFn: () => listDigitalEmployeeAvatarAssets(apiOptions),
    queryKey: ["digital-employee-avatar-assets", apiOptions.baseUrl]
  });

  const systemTemplates = useMemo(
    () => (templatesQuery.data ?? []).filter((template) => template.is_system),
    [templatesQuery.data],
  );
  const selectedTemplate = systemTemplates.find((template) => template.type === templateType);
  const restaffKeys = selectedTemplate
    ? restaffRoleKeys(selectedTemplate, gap)
    : [];

  useEffect(() => {
    if (!open) {
      setError("");
      setTemplateType("");
      createdEmployeeRef.current = null;
      membersDoneRef.current = false;
      castingDoneRef.current = false;
      return;
    }
    setName(`审查员-${randomSuffix()}`);
  }, [open]);

  useEffect(() => {
    if (!open || templateType || systemTemplates.length === 0) return;
    setTemplateType(preselectTemplateType(systemTemplates, gap));
  }, [gap, open, systemTemplates, templateType]);

  useEffect(() => {
    if (!selectedTemplate) return;
    const recommended = selectedTemplate.recommended_provider_types;
    if (recommended.length > 0 && !recommended.includes(providerType)) {
      setProviderType(recommended[0]);
    }
  }, [providerType, selectedTemplate]);

  const mutation = useMutation({
    mutationFn: async () => {
      let employee = createdEmployeeRef.current;
      if (!employee) {
        if (!selectedTemplate) {
          throw new Error("请选择补员模板");
        }
        const trimmedName = name.trim();
        if (!trimmedName) {
          throw new Error("请输入员工名称");
        }
        const avatarAssetId = avatarAssetsQuery.data?.[0]?.id;
        if (!avatarAssetId) {
          throw new Error("暂无可用头像资源，无法创建数字员工");
        }
        const projectTeamId = projectQuery.data?.team_id;
        if (!projectTeamId) {
          throw new Error("项目未绑定团队，无法补员：请先为项目绑定团队");
        }
        const roleKeys = restaffRoleKeys(selectedTemplate, gap);
        employee = await createDigitalEmployee(apiOptions, {
          avatar_asset_id: avatarAssetId,
          capability_bindings: selectedTemplate.capability_bindings,
          employee_type: selectedTemplate.type,
          name: trimmedName,
          persona_memory_markdown: selectedTemplate.persona_memory_markdown,
          provider_type: providerType,
          role: selectedTemplate.default_role,
          role_keys: roleKeys,
          team_id: projectTeamId
        });
        createdEmployeeRef.current = employee;
      }

      if (!membersDoneRef.current) {
        const existingMembers = await listProjectMembers(apiOptions, projectId);
        const alreadyMember = existingMembers.some(
          (member) => member.principal_id === employee.id,
        );
        if (!alreadyMember) {
          const nextMembers: ProjectMemberInput[] = [
            ...existingMembers.map(toMemberInput),
            {
              display_name_snapshot: employee.name,
              principal_id: employee.id,
              principal_type: "digital_employee",
              project_role: "executor",
              settings: {}
            },
          ];
          await replaceProjectMembers(apiOptions, projectId, nextMembers);
        }
        membersDoneRef.current = true;
      }

      const templateKey = scenarioTemplateKey?.trim();
      const roleKeys = employee.role_keys?.length
        ? employee.role_keys
        : selectedTemplate
          ? restaffRoleKeys(selectedTemplate, gap)
          : [];
      if (!castingDoneRef.current && templateKey && roleKeys.length > 0) {
        const existing = await listProjectCastings(apiOptions, projectId, templateKey);
        const kept = existing
          .filter((row) => !roleKeys.includes(row.role_key))
          .map((row) => ({
            role_key: row.role_key,
            digital_employee_id: row.digital_employee_id,
          }));
        await putProjectCastings(apiOptions, projectId, {
          scenario_template_key: templateKey,
          assignments: [
            ...kept,
            ...roleKeys.map((role_key) => ({
              role_key,
              digital_employee_id: employee.id,
            })),
          ],
        });
        castingDoneRef.current = true;
      }

      await resolveProjectDecision(apiOptions, projectId, decisionRequestId, {
        decision: "restaffed"
      });
      return employee;
    },
    onError: (mutationError: unknown) => {
      setError(mutationError instanceof Error ? mutationError.message : "补员失败");
    },
    onSuccess: async (employee) => {
      notifySuccess(`已创建数字员工「${employee.name}」，重新规划已触发`);
      setError("");
      createdEmployeeRef.current = null;
      membersDoneRef.current = false;
      castingDoneRef.current = false;
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["workflow-task-graph"] }),
        queryClient.invalidateQueries({ queryKey: ["workflow-detail"] }),
        queryClient.invalidateQueries({ queryKey: ["digital-employees"] }),
        queryClient.invalidateQueries({ queryKey: ["project-castings"] }),
      ]);
      onOpenChange(false);
      onStaffed?.();
    }
  });

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="sm:max-w-[520px]">
        <DialogHeader>
          <DialogTitle>从标准模板补员</DialogTitle>
          <DialogDescription>
            按系统模板创建数字员工、绑定缺口角色并写入编制，成功后自动重新规划。
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-2">
          <div className="grid gap-2">
            <Label>标准模板</Label>
            <Select onValueChange={setTemplateType} value={templateType}>
              <SelectTrigger className="w-full">
                <SelectValue placeholder="选择模板" />
              </SelectTrigger>
              <SelectContent>
                {systemTemplates.map((template) => (
                  <SelectItem key={template.type} value={template.type}>
                    {template.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {restaffKeys.length > 0 ? (
            <p className="text-xs text-ink-3">
              将绑定剧本角色 {restaffKeys.join("、")}
              {scenarioTemplateKey ? `，并写入场景「${scenarioTemplateKey}」编制` : ""}。
            </p>
          ) : null}
          <div className="grid gap-2">
            <Label htmlFor="staff-gap-employee-name">员工名称</Label>
            <Input
              id="staff-gap-employee-name"
              onChange={(event) => setName(event.target.value)}
              value={name}
            />
          </div>
          <div className="grid gap-2">
            <Label>Provider</Label>
            <Select onValueChange={setProviderType} value={providerType}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {(selectedTemplate?.recommended_provider_types.length
                  ? selectedTemplate.recommended_provider_types
                  : [DEFAULT_PROVIDER_TYPE]
                ).map((provider) => (
                  <SelectItem key={provider} value={provider}>
                    {provider}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {error ? <p className="text-sm font-semibold text-danger">{error}</p> : null}
        </div>
        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} variant="outline">
            取消
          </Button>
          <Button
            disabled={mutation.isPending || !templateType || !name.trim()}
            onClick={() => mutation.mutate()}
          >
            创建并补员
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function toMemberInput(member: ProjectMember): ProjectMemberInput {
  return {
    display_name_snapshot: member.display_name_snapshot,
    principal_id: member.principal_id,
    principal_type: member.principal_type,
    project_role: member.project_role,
    settings: member.settings
  };
}

export function restaffRoleKeys(
  template: EmployeeTemplate,
  gap: ProjectTaskGraphBlockingFactGap,
): string[] {
  const defaults = template.default_role_keys ?? [];
  const gapRoles = gap.roles ?? [];
  const overlap = defaults.filter((key) => gapRoles.includes(key));
  if (overlap.length > 0) return overlap;
  if (defaults.length > 0) return defaults;
  if (gapRoles.length > 0) return [gapRoles[0]!];
  return [];
}

function preselectTemplateType(
  templates: EmployeeTemplate[],
  gap: ProjectTaskGraphBlockingFactGap,
): string {
  for (const role of gap.roles ?? []) {
    const byRole = templates.find((template) =>
      (template.default_role_keys ?? []).includes(role),
    );
    if (byRole) return byRole.type;
  }
  for (const capability of gap.required_capabilities) {
    const match = templates.find((template) =>
      (template.capability_bindings?.external_capabilities ?? []).includes(capability),
    );
    if (match) return match.type;
  }
  return templates[0]?.type ?? "";
}

function randomSuffix(): string {
  return Math.random().toString(36).slice(2, 6);
}

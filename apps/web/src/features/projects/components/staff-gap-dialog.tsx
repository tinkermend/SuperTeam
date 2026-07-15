import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { V3Button } from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  createDigitalEmployee,
  listDigitalEmployeeAvatarAssets,
  type DigitalEmployee,
} from "@/lib/api/employees";
import { listEmployeeTemplates, type EmployeeTemplate } from "@/lib/api/employee-templates";
import {
  listProjectMembers,
  replaceProjectMembers,
  resolveProjectDecision,
  type ProjectMember,
  type ProjectMemberInput,
  type ProjectTaskGraphBlockingFactGap,
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
};

/**
 * 一键补员对话框：从系统内置数字员工模板选一个、命名、确认 provider，创建后追加进
 * 项目成员，并把 planning_gap 决策 resolve 为 restaffed（触发需求重开+重规划）。
 * 提交链严格依次调用 createDigitalEmployee → replaceProjectMembers（读改写现有成员
 * 列表）→ resolveProjectDecision，任一步失败都不进入下一步。
 */
export function StaffGapDialog({
  apiOptions,
  decisionRequestId,
  gap,
  onOpenChange,
  onStaffed,
  open,
  projectId,
}: StaffGapDialogProps) {
  const queryClient = useQueryClient();
  const [templateType, setTemplateType] = useState("");
  const [name, setName] = useState("");
  const [providerType, setProviderType] = useState(DEFAULT_PROVIDER_TYPE);
  const [error, setError] = useState("");
  // 重试幂等锚点：createDigitalEmployee 成功后立即记录已创建员工，链条中段
  // （成员 PUT / resolve）失败后用户重试时跳过创建、从成员步续跑，避免重复建员。
  // 对话框关闭或整链成功后清空。用 ref 而非 state：mutationFn 内同步写入即生效，
  // 不依赖 React 渲染周期。
  const createdEmployeeRef = useRef<DigitalEmployee | null>(null);

  const templatesQuery = useQuery({
    enabled: open,
    queryFn: () => listEmployeeTemplates(apiOptions),
    queryKey: ["employee-templates", apiOptions.baseUrl],
  });
  const avatarAssetsQuery = useQuery({
    enabled: open,
    queryFn: () => listDigitalEmployeeAvatarAssets(apiOptions),
    queryKey: ["digital-employee-avatar-assets", apiOptions.baseUrl],
  });

  const systemTemplates = useMemo(
    () => (templatesQuery.data ?? []).filter((template) => template.is_system),
    [templatesQuery.data],
  );
  const selectedTemplate = systemTemplates.find((template) => template.type === templateType);

  useEffect(() => {
    if (!open) {
      setError("");
      setTemplateType("");
      createdEmployeeRef.current = null;
      return;
    }
    setName(`审查员-${randomSuffix()}`);
  }, [open]);

  useEffect(() => {
    if (!open || templateType || systemTemplates.length === 0) return;
    setTemplateType(preselectTemplateType(systemTemplates, gap.required_capabilities));
  }, [gap.required_capabilities, open, systemTemplates, templateType]);

  useEffect(() => {
    if (!selectedTemplate) return;
    const recommended = selectedTemplate.recommended_provider_types;
    if (recommended.length > 0 && !recommended.includes(providerType)) {
      setProviderType(recommended[0]);
    }
  }, [providerType, selectedTemplate]);

  const mutation = useMutation({
    mutationFn: async () => {
      // Step 1: 创建员工——重试时若上次已创建成功则复用，绝不重复创建。
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
        employee = await createDigitalEmployee(apiOptions, {
          avatar_asset_id: avatarAssetId,
          capability_bindings: selectedTemplate.capability_bindings,
          employee_type: selectedTemplate.type,
          name: trimmedName,
          persona_memory_markdown: selectedTemplate.persona_memory_markdown,
          provider_type: providerType,
          role: selectedTemplate.default_role,
        });
        createdEmployeeRef.current = employee;
      }
      // Step 2: 追加进项目成员（读改写）——若上次重试已写入成功则跳过，追加本身幂等。
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
            settings: {},
          },
        ];
        await replaceProjectMembers(apiOptions, projectId, nextMembers);
      }
      // Step 3: resolve planning_gap 决策为 restaffed，触发需求重开+重规划。
      await resolveProjectDecision(apiOptions, projectId, decisionRequestId, {
        decision: "restaffed",
      });
      return employee;
    },
    onError: (mutationError: unknown) => {
      setError(mutationError instanceof Error ? mutationError.message : "补员失败");
    },
    onSuccess: async (employee) => {
      toast.success(`已创建数字员工「${employee.name}」，重新规划已触发`);
      setError("");
      createdEmployeeRef.current = null;
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["workflow-task-graph"] }),
        queryClient.invalidateQueries({ queryKey: ["workflow-detail"] }),
        queryClient.invalidateQueries({ queryKey: ["digital-employees"] }),
      ]);
      onOpenChange(false);
      onStaffed?.();
    },
  });

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="sm:max-w-[520px]">
        <DialogHeader>
          <DialogTitle>从标准模板补员</DialogTitle>
          <DialogDescription>
            按系统内置模板一键创建数字员工并加入项目成员，成功后自动重新规划该需求。
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
          {error ? <p className="text-sm font-semibold text-v3-danger">{error}</p> : null}
        </div>
        <DialogFooter>
          <V3Button onClick={() => onOpenChange(false)} variant="outline">
            取消
          </V3Button>
          <V3Button
            disabled={mutation.isPending || !templateType || !name.trim()}
            onClick={() => mutation.mutate()}
          >
            创建并补员
          </V3Button>
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
    settings: member.settings,
  };
}

/** 按 gap.required_capabilities 命中系统模板的 external_capabilities，命中第一个就选它
 * （如 code_review → standard_code_reviewer）；都不命中则退回第一个系统模板。 */
function preselectTemplateType(
  templates: EmployeeTemplate[],
  requiredCapabilities: string[],
): string {
  for (const capability of requiredCapabilities) {
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

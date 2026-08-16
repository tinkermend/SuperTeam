import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Eye, Pencil, Save } from "lucide-react";
import { useState } from "react";
import { Button, MarkdownProse, notifyError, notifySuccess, SectionHeader, SoftCard } from "@/components/superteam";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  createDigitalEmployeeConfigRevision,
  type CapabilityBindings,
  type CreateDigitalEmployeeConfigRevisionInput,
  type DigitalEmployee,
} from "@/lib/api/employees";
import { statusLabel } from "@/lib/status-labels";
import { ChipsEditor } from "./config-chips-editor";
import {
  budgetPolicyFromDailyTokenLimit,
  budgetPolicyValue,
  RESERVED_CAPABILITY_KEYS,
  sameStringArray,
  stringArray,
} from "../config-utils";
import { useDirtyReport, useTabRehydrate } from "./use-config-tab-state";

export function ConfigExecutionTab({
  apiOptions,
  employee,
  onDirtyChange,
}: {
  apiOptions: ApiClientOptions;
  employee: DigitalEmployee;
  onDirtyChange: (dirty: boolean) => void;
}) {
  const queryClient = useQueryClient();
  const initialBindings = employee.capability_bindings ?? {};
  const [personaMemoryMarkdown, setPersonaMemoryMarkdown] = useState(
    employee.persona_memory_markdown ?? "",
  );
  const [personaPreview, setPersonaPreview] = useState(false);
  const [externalCapabilities, setExternalCapabilities] = useState<string[]>(
    () => stringArray(initialBindings.external_capabilities),
  );
  const [environmentVariableRefs, setEnvironmentVariableRefs] = useState<string[]>(
    () => stringArray(initialBindings.environment_variable_refs),
  );
  const [dailyTokenLimit, setDailyTokenLimit] = useState(() =>
    budgetPolicyValue(employee.budget_policy ?? {}),
  );
  const [otherCapabilityKeys, setOtherCapabilityKeys] = useState<Record<string, unknown>>(() =>
    Object.fromEntries(
      Object.entries(initialBindings).filter(([key]) => !RESERVED_CAPABILITY_KEYS.includes(key)),
    ),
  );
  const [budgetError, setBudgetError] = useState("");

  const bindings = employee.capability_bindings ?? {};
  const dirty =
    personaMemoryMarkdown !== (employee.persona_memory_markdown ?? "") ||
    !sameStringArray(externalCapabilities, stringArray(bindings.external_capabilities)) ||
    !sameStringArray(environmentVariableRefs, stringArray(bindings.environment_variable_refs)) ||
    dailyTokenLimit !== budgetPolicyValue(employee.budget_policy ?? {});

  useTabRehydrate(
    employee.id,
    dirty,
    [employee.persona_memory_markdown, employee.capability_bindings, employee.budget_policy],
    () => {
      const nextBindings = employee.capability_bindings ?? {};
      setPersonaMemoryMarkdown(employee.persona_memory_markdown ?? "");
      setExternalCapabilities(stringArray(nextBindings.external_capabilities));
      setEnvironmentVariableRefs(stringArray(nextBindings.environment_variable_refs));
      setOtherCapabilityKeys(
        Object.fromEntries(
          Object.entries(nextBindings).filter(([key]) => !RESERVED_CAPABILITY_KEYS.includes(key)),
        ),
      );
      setDailyTokenLimit(budgetPolicyValue(employee.budget_policy ?? {}));
      setBudgetError("");
    },
  );
  useDirtyReport(dirty, onDirtyChange);

  const createRevision = useMutation({
    mutationFn: (input: CreateDigitalEmployeeConfigRevisionInput) =>
      createDigitalEmployeeConfigRevision(apiOptions, employee.id, input),
    onSuccess: () => {
      setBudgetError("");
      queryClient.invalidateQueries({ queryKey: ["digital-employee", employee.id] });
      notifySuccess("执行配置已保存并生效");
    },
    onError: (error) => {
      notifyError(error instanceof Error ? error.message : "保存执行配置失败");
    },
  });

  const effectiveStatus = employee.metadata?.effective_config_status;

  const handleSubmit = (event: React.FormEvent) => {
    event.preventDefault();
    setBudgetError("");
    const budgetPolicy = budgetPolicyFromDailyTokenLimit(dailyTokenLimit);
    if (!budgetPolicy) {
      setBudgetError("每日 Token 预算上限必须是正整数");
      return;
    }
    const capabilityBindings: CapabilityBindings = {
      ...otherCapabilityKeys,
      external_capabilities: externalCapabilities,
      environment_variable_refs: environmentVariableRefs,
    };
    createRevision.mutate({
      persona_memory_markdown: personaMemoryMarkdown.trim(),
      capability_bindings: capabilityBindings,
      budget_policy: budgetPolicy,
    });
  };

  return (
    <form className="space-y-4" noValidate onSubmit={handleSubmit}>
      <SectionHeader
        title="执行配置"
        description="保存后生成新配置版本并即时生效"
      />
      {effectiveStatus ? (
        <p className="text-xs text-ink-3">
          当前生效配置：{employee.metadata?.effective_config_label ?? "—"}（{statusLabel(String(effectiveStatus))}）
        </p>
      ) : null}

      <SoftCard className="space-y-3 p-5">
        <div className="flex items-center justify-between">
          <div className="text-sm font-medium text-ink">人格记忆.md</div>
          <Button type="button" variant="ghost" size="sm" onClick={() => setPersonaPreview((value) => !value)}>
            {personaPreview ? <Pencil /> : <Eye />}
            {personaPreview ? "编辑" : "预览"}
          </Button>
        </div>
        {personaPreview ? (
          personaMemoryMarkdown.trim() ? (
            <MarkdownProse className="rounded-inner border border-line bg-card-soft p-3">
              {personaMemoryMarkdown}
            </MarkdownProse>
          ) : (
            <p className="rounded-inner border border-line bg-card-soft p-3 text-sm text-ink-3">未设置</p>
          )
        ) : (
          <Textarea
            id="persona-memory-markdown"
            aria-label="人格记忆.md"
            value={personaMemoryMarkdown}
            onChange={(event) => {
              setPersonaMemoryMarkdown(event.target.value);
            }}
            rows={10}
            className="font-mono text-xs"
          />
        )}
        <p className="text-xs text-ink-3">
          人格记忆随任务注入；项目宪法由所属项目在执行时注入，不属于数字员工配置。
        </p>
      </SoftCard>

      <SoftCard className="space-y-4 p-5">
        <div className="text-sm font-medium text-ink">能力声明（用于选角佐证，非实际绑定）</div>
        <ChipsEditor
          label="外部能力"
          placeholder="输入能力标识后回车添加"
          values={externalCapabilities}
          onChange={(next) => {
            setExternalCapabilities(next);
          }}
        />
        <ChipsEditor
          label="环境变量引用"
          placeholder="输入环境变量名后回车添加"
          values={environmentVariableRefs}
          onChange={(next) => {
            setEnvironmentVariableRefs(next);
          }}
        />
      </SoftCard>

      <SoftCard className="space-y-2 p-5">
        <div className="text-sm font-medium text-ink">预算策略</div>
        <Label htmlFor="config-daily-token-limit" className="text-xs text-ink-3">
          每日 Token 预算上限
        </Label>
        <Input
          id="config-daily-token-limit"
          inputMode="numeric"
          min={1}
          onChange={(event) => {
            setDailyTokenLimit(event.target.value);
            setBudgetError("");
          }}
          placeholder="不填写表示无预算上限"
          type="number"
          aria-invalid={Boolean(budgetError)}
          value={dailyTokenLimit}
        />
        {budgetError ? <p className="text-sm text-destructive">{budgetError}</p> : null}
      </SoftCard>

      <div className="flex justify-end">
        <Button type="submit" disabled={!dirty || createRevision.isPending}>
          <Save />
          保存
        </Button>
      </div>
    </form>
  );
}

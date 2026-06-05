import type { ReactNode } from "react";
import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { ArrowLeft, Bot, Check, ChevronLeft, ChevronRight, Loader2, Plus } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Textarea } from "@/components/ui/textarea";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import type {
  DigitalEmployeeCreateOptions,
  DigitalEmployeeRuntimeProviderOption,
  DigitalEmployeeTypeOption,
} from "@/lib/api/employees";
import {
  createDigitalEmployee,
  getDigitalEmployeeCreateOptions,
} from "@/lib/api/employees";
import { listTeams } from "@/lib/api/teams";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { cn } from "@/lib/utils";

const steps = ["身份", "能力", "治理", "运行"] as const;
type StepName = (typeof steps)[number];

type WizardDraft = {
  capability_selection: {
    enabled_external_capabilities: string[];
    enabled_mcp_servers: string[];
    enabled_skills: string[];
  };
  context_policy_override: Record<string, unknown>;
  approval_policy_override: Record<string, unknown>;
  description: string;
  employee_type: string;
  name: string;
  risk_level: string;
  role: string;
  runtime_binding: string;
  runtime_node_id: string;
  provider_type: string;
  team_id: string;
};

type ValidationErrors = Partial<Record<"employee_type" | "name" | "role" | "runtime" | "team_id", string>>;

type CreateEmployeeViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
};

const emptyDraft: WizardDraft = {
  approval_policy_override: {},
  capability_selection: {
    enabled_external_capabilities: [],
    enabled_mcp_servers: [],
    enabled_skills: [],
  },
  context_policy_override: {},
  description: "",
  employee_type: "",
  name: "",
  provider_type: "",
  risk_level: "medium",
  role: "",
  runtime_binding: "",
  runtime_node_id: "",
  team_id: "",
};

export function CreateEmployeePage() {
  const apiBaseUrl = resolveControlPlaneUrl();

  return <CreateEmployeeView apiBaseUrl={apiBaseUrl} />;
}

export function CreateEmployeeView({ apiBaseUrl, fetcher }: CreateEmployeeViewProps) {
  const navigate = useNavigate();
  const [stepIndex, setStepIndex] = useState(0);
  const [draft, setDraft] = useState<WizardDraft>(emptyDraft);
  const [errors, setErrors] = useState<ValidationErrors>({});

  const teams = useQuery({
    queryKey: ["teams"],
    queryFn: () => listTeams({ baseUrl: apiBaseUrl, fetcher }),
  });

  useEffect(() => {
    const firstTeamId = teams.data?.[0]?.id;
    if (!draft.team_id && firstTeamId) {
      setDraft((current) => ({ ...current, team_id: firstTeamId }));
    }
  }, [draft.team_id, teams.data]);

  const createOptions = useQuery({
    enabled: Boolean(draft.team_id),
    queryKey: ["digital-employee-create-options", draft.team_id],
    queryFn: () => getDigitalEmployeeCreateOptions({ baseUrl: apiBaseUrl, fetcher }, draft.team_id),
  });

  const selectedType = useMemo(
    () => createOptions.data?.employee_types.find((item) => item.type === draft.employee_type),
    [createOptions.data?.employee_types, draft.employee_type],
  );

  useEffect(() => {
    const optionsData = createOptions.data;
    const firstType = optionsData?.employee_types[0];
    if (!firstType) return;
    if (!draft.employee_type || !optionsData.employee_types.some((item) => item.type === draft.employee_type)) {
      setDraft((current) => applyTypeDefaults(current, firstType));
    }
  }, [createOptions.data, draft.employee_type]);

  useEffect(() => {
    const runtimeOptions = createOptions.data?.runtime_provider_options ?? [];
    const availableOptions = runtimeOptions.filter((option) => option.available);
    setDraft((current) => {
      if (availableOptions.length === 1) {
        return {
          ...current,
          runtime_binding: runtimeBinding(availableOptions[0]),
          provider_type: availableOptions[0].provider_type,
          runtime_node_id: availableOptions[0].runtime_node_id,
        };
      }
      if (current.runtime_binding && !availableOptions.some((option) => runtimeBinding(option) === current.runtime_binding)) {
        return { ...current, provider_type: "", runtime_binding: "", runtime_node_id: "" };
      }
      return current;
    });
  }, [createOptions.data?.runtime_provider_options]);

  const createEmployee = useMutation({
    mutationFn: () => {
      const runtimeOption = findRuntimeOption(createOptions.data, draft.runtime_binding);
      if (!runtimeOption) {
        throw new Error("请选择 Runtime");
      }

      return createDigitalEmployee(
        { baseUrl: apiBaseUrl, fetcher },
        {
          team_id: draft.team_id,
          employee_type: draft.employee_type,
          name: draft.name.trim(),
          role: draft.role.trim(),
          description: draft.description.trim() || undefined,
          risk_level: draft.risk_level,
          role_profile: {
            employee_type: draft.employee_type,
            role: draft.role.trim(),
            title: selectedType?.label ?? draft.employee_type,
          },
          capability_selection: draft.capability_selection,
          context_policy_override: draft.context_policy_override,
          approval_policy_override: draft.approval_policy_override,
          output_contract_addendum: {},
          runtime_node_id: runtimeOption.runtime_node_id,
          provider_type: runtimeOption.provider_type,
          session_policy: { mode: "reuse_latest" },
          workspace_policy: {},
        },
      );
    },
    onSuccess: (employee) => {
      void navigate({
        params: { employeeId: employee.id },
        to: "/employees/$employeeId",
      });
    },
  });

  const currentStep = steps[stepIndex];
  const teamOptions = teams.data ?? [];

  function updateDraft(patch: Partial<WizardDraft>) {
    setDraft((current) => ({ ...current, ...patch }));
  }

  function selectType(typeValue: string) {
    const nextType = createOptions.data?.employee_types.find((item) => item.type === typeValue);
    if (!nextType) {
      updateDraft({ employee_type: typeValue });
      return;
    }
    setDraft((current) => applyTypeDefaults(current, nextType));
  }

  function selectRuntime(runtimeBindingValue: string) {
    const runtimeOption = findRuntimeOption(createOptions.data, runtimeBindingValue);
    updateDraft({
      provider_type: runtimeOption?.provider_type ?? "",
      runtime_binding: runtimeBindingValue,
      runtime_node_id: runtimeOption?.runtime_node_id ?? "",
    });
    setErrors((current) => ({ ...current, runtime: undefined }));
  }

  function nextStep() {
    const nextErrors = validateStep(currentStep, draft);
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length === 0) {
      setStepIndex((current) => Math.min(current + 1, steps.length - 1));
    }
  }

  function submit() {
    const nextErrors = validateStep("运行", draft);
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length === 0) {
      createEmployee.mutate();
    }
  }

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main>
        <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-3">
            <div className="flex size-10 items-center justify-center rounded-md border bg-muted">
              <Bot />
            </div>
            <div>
              <h1 className="text-2xl font-bold tracking-tight">创建数字员工</h1>
              <p className="text-sm text-muted-foreground">创建后进入 ready 状态，再由任务运行链路调度执行。</p>
            </div>
          </div>
          <Button asChild type="button" variant="outline">
            <Link to="/employees">
              <ArrowLeft data-icon="inline-start" />
              返回列表
            </Link>
          </Button>
        </div>

        {teams.isError ? (
          <Alert className="mb-4" variant="destructive">
            <AlertTitle>团队列表加载失败</AlertTitle>
            <AlertDescription>{getErrorMessage(teams.error)}</AlertDescription>
          </Alert>
        ) : null}
        {!teams.isLoading && !teams.isError && teamOptions.length === 0 ? (
          <Alert className="mb-4">
            <AlertTitle>暂无可用团队</AlertTitle>
            <AlertDescription>需先创建团队并完成治理配置后再创建数字员工。</AlertDescription>
          </Alert>
        ) : null}
        {createOptions.isError ? (
          <Alert className="mb-4" variant="destructive">
            <AlertTitle>创建选项加载失败</AlertTitle>
            <AlertDescription>{getErrorMessage(createOptions.error)}</AlertDescription>
          </Alert>
        ) : null}

        <Card>
          <CardHeader>
            <CardTitle>创建向导</CardTitle>
            <CardDescription>按身份、能力、治理、运行绑定完成 ready 数字员工创建。</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-5">
            <div className="grid gap-3 md:grid-cols-[220px_1fr]">
              <StepRail currentStep={currentStep} />
              <div className="min-h-[420px] rounded-md border bg-background p-4">
                {teams.isLoading || (draft.team_id && createOptions.isLoading) ? (
                  <div className="flex min-h-[360px] items-center justify-center gap-2 text-sm text-muted-foreground">
                    <Loader2 className="animate-spin" />
                    加载创建选项
                  </div>
                ) : null}
                {!teams.isLoading && !createOptions.isLoading && currentStep === "身份" ? (
                  <IdentityStep
                    draft={draft}
                    errors={errors}
                    options={createOptions.data}
                    selectedType={selectedType}
                    teamOptions={teamOptions}
                    onSelectType={selectType}
                    onUpdate={updateDraft}
                  />
                ) : null}
                {!teams.isLoading && !createOptions.isLoading && currentStep === "能力" ? (
                  <CapabilityStep draft={draft} options={createOptions.data} onUpdate={updateDraft} />
                ) : null}
                {!teams.isLoading && !createOptions.isLoading && currentStep === "治理" ? (
                  <GovernanceStep draft={draft} options={createOptions.data} selectedType={selectedType} />
                ) : null}
                {!teams.isLoading && !createOptions.isLoading && currentStep === "运行" ? (
                  <RuntimeStep
                    draft={draft}
                    error={errors.runtime}
                    options={createOptions.data}
                    onSelectRuntime={selectRuntime}
                  />
                ) : null}
              </div>
            </div>

            {createEmployee.isError ? (
              <p className="text-sm text-destructive">{getErrorMessage(createEmployee.error)}</p>
            ) : null}
            <div className="flex justify-between gap-3 border-t pt-4">
              <Button
                disabled={stepIndex === 0 || createEmployee.isPending}
                onClick={() => setStepIndex((current) => Math.max(current - 1, 0))}
                type="button"
                variant="outline"
              >
                <ChevronLeft data-icon="inline-start" />
                上一步
              </Button>
              {stepIndex < steps.length - 1 ? (
                <Button
                  disabled={teamOptions.length === 0 || createOptions.isLoading || createOptions.isError}
                  onClick={nextStep}
                  type="button"
                >
                  下一步
                  <ChevronRight data-icon="inline-end" />
                </Button>
              ) : (
                <Button
                  disabled={
                    createEmployee.isPending ||
                    teamOptions.length === 0 ||
                    createOptions.isLoading ||
                    createOptions.isError ||
                    !draft.runtime_binding
                  }
                  onClick={submit}
                  type="button"
                >
                  {createEmployee.isPending ? <Loader2 className="animate-spin" data-icon="inline-start" /> : <Plus data-icon="inline-start" />}
                  创建数字员工
                </Button>
              )}
            </div>
          </CardContent>
        </Card>
      </Main>
    </>
  );
}

function StepRail({ currentStep }: { currentStep: StepName }) {
  const currentIndex = steps.indexOf(currentStep);

  return (
    <div className="rounded-md border bg-muted/30 p-3">
      <div className="flex flex-col gap-2">
        {steps.map((step, index) => {
          const active = step === currentStep;
          const done = index < currentIndex;

          return (
            <div
              className={cn(
                "flex items-center gap-2 rounded-md px-2 py-2 text-sm text-muted-foreground",
                active ? "bg-background font-medium text-foreground shadow-xs" : "",
                done ? "text-foreground" : "",
              )}
              key={step}
            >
              <span
                className={cn(
                  "flex size-6 items-center justify-center rounded-full border text-xs",
                  active ? "border-primary bg-primary text-primary-foreground" : "",
                  done ? "border-primary text-primary" : "",
                )}
              >
                {done ? <Check /> : index + 1}
              </span>
              <span>{step}</span>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function IdentityStep({
  draft,
  errors,
  options,
  selectedType,
  teamOptions,
  onSelectType,
  onUpdate,
}: {
  draft: WizardDraft;
  errors: ValidationErrors;
  options?: DigitalEmployeeCreateOptions;
  selectedType?: DigitalEmployeeTypeOption;
  teamOptions: Array<{ id: string; name: string }>;
  onSelectType: (value: string) => void;
  onUpdate: (patch: Partial<WizardDraft>) => void;
}) {
  return (
    <div className="flex flex-col gap-5">
      <div>
        <h2 className="text-lg font-semibold">身份</h2>
        <p className="text-sm text-muted-foreground">确定团队、业务类型和职责边界。负责人由后端按当前登录身份注入。</p>
      </div>
      <div className="grid gap-4 md:grid-cols-2">
        <Field label="归属团队" error={errors.team_id}>
          <select
            aria-invalid={Boolean(errors.team_id)}
            className={selectClassName}
            id="employee-team"
            onChange={(event) =>
              onUpdate({
                employee_type: "",
                provider_type: "",
                runtime_binding: "",
                runtime_node_id: "",
                team_id: event.target.value,
              })
            }
            value={draft.team_id}
          >
            {teamOptions.map((team) => (
              <option key={team.id} value={team.id}>
                {team.name}
              </option>
            ))}
          </select>
        </Field>
        <Field label="员工类型" error={errors.employee_type}>
          <select
            aria-invalid={Boolean(errors.employee_type)}
            className={selectClassName}
            id="employee-type"
            onChange={(event) => onSelectType(event.target.value)}
            value={draft.employee_type}
          >
            {(options?.employee_types ?? []).map((item) => (
              <option key={item.type} value={item.type}>
                {item.label}
              </option>
            ))}
          </select>
        </Field>
        <Field label="名称" error={errors.name}>
          <Input
            aria-invalid={Boolean(errors.name)}
            id="employee-name"
            onChange={(event) => onUpdate({ name: event.target.value })}
            value={draft.name}
          />
        </Field>
        <Field label="角色" error={errors.role}>
          <Input
            aria-invalid={Boolean(errors.role)}
            id="employee-role"
            onChange={(event) => onUpdate({ role: event.target.value })}
            value={draft.role}
          />
        </Field>
        <Field label="风险等级">
          <select
            className={selectClassName}
            id="employee-risk"
            onChange={(event) => onUpdate({ risk_level: event.target.value })}
            value={draft.risk_level}
          >
            <option value="low">low</option>
            <option value="medium">medium</option>
            <option value="high">high</option>
            <option value="critical">critical</option>
          </select>
        </Field>
        <Field label="描述">
          <Textarea
            id="employee-description"
            onChange={(event) => onUpdate({ description: event.target.value })}
            value={draft.description}
          />
        </Field>
      </div>
      {selectedType ? (
        <div className="rounded-md border bg-muted/30 p-3 text-sm">
          <div className="font-medium">{selectedType.label}</div>
          <div className="mt-1 text-muted-foreground">{selectedType.description}</div>
          <div className="mt-2 text-muted-foreground">默认角色：{selectedType.default_role || selectedType.type}</div>
        </div>
      ) : null}
    </div>
  );
}

function CapabilityStep({
  draft,
  options,
  onUpdate,
}: {
  draft: WizardDraft;
  options?: DigitalEmployeeCreateOptions;
  onUpdate: (patch: Partial<WizardDraft>) => void;
}) {
  const capabilityOptions = options?.capability_options;

  function toggle(kind: keyof WizardDraft["capability_selection"], value: string) {
    const currentValues = draft.capability_selection[kind];
    const nextValues = currentValues.includes(value)
      ? currentValues.filter((item) => item !== value)
      : [...currentValues, value];
    onUpdate({
      capability_selection: {
        ...draft.capability_selection,
        [kind]: nextValues,
      },
    });
  }

  return (
    <div className="flex flex-col gap-5">
      <div>
        <h2 className="text-lg font-semibold">能力</h2>
        <p className="text-sm text-muted-foreground">按团队治理配置选择技能、MCP Server 和外部能力。</p>
      </div>
      <CapabilityGroup
        checkedValues={draft.capability_selection.enabled_skills}
        label="技能"
        onToggle={(value) => toggle("enabled_skills", value)}
        values={capabilityOptions?.skills ?? []}
      />
      <CapabilityGroup
        checkedValues={draft.capability_selection.enabled_mcp_servers}
        label="MCP Server"
        onToggle={(value) => toggle("enabled_mcp_servers", value)}
        values={capabilityOptions?.mcp_servers ?? []}
      />
      <CapabilityGroup
        checkedValues={draft.capability_selection.enabled_external_capabilities}
        label="外部能力"
        onToggle={(value) => toggle("enabled_external_capabilities", value)}
        values={capabilityOptions?.external_capabilities ?? []}
      />
    </div>
  );
}

function CapabilityGroup({
  checkedValues,
  label,
  onToggle,
  values,
}: {
  checkedValues: string[];
  label: string;
  onToggle: (value: string) => void;
  values: string[];
}) {
  return (
    <fieldset className="rounded-md border p-3">
      <legend className="px-1 text-sm font-medium">{label}</legend>
      <div className="mt-3 grid gap-3 md:grid-cols-2">
        {values.map((value) => (
          <label className="flex items-center gap-2 text-sm" key={value}>
            <Checkbox checked={checkedValues.includes(value)} onCheckedChange={() => onToggle(value)} />
            <span>{value}</span>
          </label>
        ))}
        {values.length === 0 ? <p className="text-sm text-muted-foreground">暂无可选项</p> : null}
      </div>
    </fieldset>
  );
}

function GovernanceStep({
  draft,
  options,
  selectedType,
}: {
  draft: WizardDraft;
  options?: DigitalEmployeeCreateOptions;
  selectedType?: DigitalEmployeeTypeOption;
}) {
  const teamConfig = options?.team_config;

  return (
    <div className="flex flex-col gap-5">
      <div>
        <h2 className="text-lg font-semibold">治理</h2>
        <p className="text-sm text-muted-foreground">确认团队治理版本、上下文和审批默认值。这里不暴露原始 JSON 编辑。</p>
      </div>
      <div className="grid gap-3 md:grid-cols-2">
        <SummaryItem label="团队治理版本" value={teamConfig ? `#${teamConfig.revision_number} · ${teamConfig.status}` : "未加载"} />
        <SummaryItem label="风险触发" value={draft.risk_level} />
        <SummaryItem label="允许员工类型" value={`${teamConfig?.allowed_employee_types.length ?? 0} 项`} />
        <SummaryItem label="允许 Provider" value={(teamConfig?.allowed_provider_types ?? []).join(", ") || "暂无"} />
        <SummaryItem label="上下文策略" value={`覆盖项 ${Object.keys(draft.context_policy_override).length} 个`} />
        <SummaryItem label="审批策略" value={String(draft.approval_policy_override.min_risk_for_human ?? "按团队默认")} />
      </div>
      <div className="rounded-md border bg-muted/30 p-3">
        <div className="text-sm font-medium">创建摘要</div>
        <div className="mt-2 flex flex-wrap gap-2">
          <Badge variant="secondary">{selectedType?.label ?? draft.employee_type}</Badge>
          <Badge variant="secondary">{draft.role}</Badge>
          <Badge variant="secondary">技能 {draft.capability_selection.enabled_skills.length}</Badge>
          <Badge variant="secondary">MCP {draft.capability_selection.enabled_mcp_servers.length}</Badge>
          <Badge variant="secondary">外部能力 {draft.capability_selection.enabled_external_capabilities.length}</Badge>
        </div>
      </div>
    </div>
  );
}

function SummaryItem({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border p-3">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 text-sm font-medium">{value}</div>
    </div>
  );
}

function RuntimeStep({
  draft,
  error,
  options,
  onSelectRuntime,
}: {
  draft: WizardDraft;
  error?: string;
  options?: DigitalEmployeeCreateOptions;
  onSelectRuntime: (runtimeBindingValue: string) => void;
}) {
  const runtimeOptions = options?.runtime_provider_options ?? [];

  return (
    <div className="flex flex-col gap-5">
      <div>
        <h2 className="text-lg font-semibold">运行</h2>
        <p className="text-sm text-muted-foreground">绑定 Runtime 和 Provider。多个可用 Runtime 时必须显式选择。</p>
      </div>
      <RadioGroup onValueChange={onSelectRuntime} value={draft.runtime_binding}>
        <div className="grid gap-3">
          {runtimeOptions.map((option) => (
            <RuntimeOption
              key={runtimeBinding(option)}
              onSelectRuntime={onSelectRuntime}
              option={option}
            />
          ))}
        </div>
      </RadioGroup>
      {runtimeOptions.length === 0 ? <p className="text-sm text-muted-foreground">暂无可用 Runtime Provider。</p> : null}
      {error ? <p className="text-sm text-destructive">{error}</p> : null}
    </div>
  );
}

function RuntimeOption({
  onSelectRuntime,
  option,
}: {
  onSelectRuntime: (runtimeBindingValue: string) => void;
  option: DigitalEmployeeRuntimeProviderOption;
}) {
  const label = `${option.runtime_name} / ${option.provider_type}`;
  const binding = runtimeBinding(option);

  return (
    <label
      className={cn(
        "flex items-start gap-3 rounded-md border p-3 text-sm",
        option.available ? "cursor-pointer" : "cursor-not-allowed opacity-60",
      )}
      onClick={(event) => {
        event.preventDefault();
        if (option.available) onSelectRuntime(binding);
      }}
    >
      <RadioGroupItem disabled={!option.available} value={binding} />
      <span className="min-w-0 flex-1">
        <span className="block font-medium">{label}</span>
        <span className="mt-1 block text-muted-foreground">
          {option.node_id} · {option.runtime_status} · {option.provider_status} · {option.current_load}/{option.max_slots}
        </span>
        {!option.available && option.disabled_reason ? (
          <span className="mt-1 block text-destructive">{option.disabled_reason}</span>
        ) : null}
      </span>
    </label>
  );
}

function Field({
  children,
  error,
  label,
}: {
  children: ReactNode;
  error?: string;
  label: string;
}) {
  const id = labelId[label] ?? "";

  return (
    <div className="grid gap-2">
      <Label htmlFor={id}>{label}</Label>
      {children}
      {error ? <span className="text-sm text-destructive">{error}</span> : null}
    </div>
  );
}

const labelId: Record<string, string> = {
  员工类型: "employee-type",
  名称: "employee-name",
  归属团队: "employee-team",
  描述: "employee-description",
  角色: "employee-role",
  风险等级: "employee-risk",
};

const selectClassName =
  "h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-xs outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50";

function applyTypeDefaults(current: WizardDraft, typeOption: DigitalEmployeeTypeOption): WizardDraft {
  const defaultCapabilitySelection = typeOption.default_capability_selection ?? {};

  return {
    ...current,
    approval_policy_override: typeOption.default_approval_policy ?? {},
    capability_selection: {
      enabled_external_capabilities: stringList(defaultCapabilitySelection.enabled_external_capabilities),
      enabled_mcp_servers:
        typeOption.recommended_mcp_servers && typeOption.recommended_mcp_servers.length > 0
          ? typeOption.recommended_mcp_servers
          : stringList(defaultCapabilitySelection.enabled_mcp_servers),
      enabled_skills:
        typeOption.recommended_skills && typeOption.recommended_skills.length > 0
          ? typeOption.recommended_skills
          : stringList(defaultCapabilitySelection.enabled_skills),
    },
    context_policy_override: typeOption.default_context_policy_override ?? {},
    employee_type: typeOption.type,
    risk_level: stringValue(typeOption.default_approval_policy?.min_risk_for_human) || "medium",
    role: typeOption.default_role || typeOption.type,
  };
}

function validateStep(step: StepName, draft: WizardDraft): ValidationErrors {
  if (step === "身份") {
    const errors: ValidationErrors = {};
    if (!draft.team_id.trim()) errors.team_id = "团队不能为空";
    if (!draft.employee_type.trim()) errors.employee_type = "员工类型不能为空";
    if (!draft.name.trim()) errors.name = "名称不能为空";
    if (!draft.role.trim()) errors.role = "角色不能为空";
    return errors;
  }
  if (step === "运行" && !draft.runtime_binding) {
    return { runtime: "请选择 Runtime" };
  }
  return {};
}

function findRuntimeOption(options: DigitalEmployeeCreateOptions | undefined, runtimeBindingValue: string) {
  return options?.runtime_provider_options.find((option) => runtimeBinding(option) === runtimeBindingValue);
}

function runtimeBinding(option: DigitalEmployeeRuntimeProviderOption) {
  return `${option.runtime_node_id}:${option.provider_type}`;
}

function stringList(value: unknown) {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];
}

function stringValue(value: unknown) {
  return typeof value === "string" ? value : "";
}

function getErrorMessage(error: unknown) {
  return error instanceof Error ? error.message : "请求失败";
}

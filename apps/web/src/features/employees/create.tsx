import type { ReactNode } from "react";
import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import {
  ArrowLeft,
  Bot,
  Check,
  ChevronLeft,
  ChevronRight,
  ClipboardCheck,
  Code2,
  Cpu,
  FileText,
  Gauge,
  GitBranch,
  Loader2,
  Plus,
  ShieldCheck,
  Sparkles,
  Trash2
} from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Textarea } from "@/components/ui/textarea";
import {
  GlassCard,
  IconTile,
  StatusPill,
  Chip,
  Button,
  WorkSurface,
  type Tone
} from "@/components/superteam";
import { Main } from "@/components/layout/main";
import {
  ShellPageHeader,
  ShellPageHeaderBack
} from "@/components/layout/shell-page-header";
import type {
  DigitalEmployeeAvatarAsset,
  DigitalEmployeeCapabilityOptionItem,
  DigitalEmployeeCreateOptions,
  DigitalEmployeeTypeOption
} from "@/lib/api/employees";
import {
  createDigitalEmployee,
  getDigitalEmployeeCreateOptions,
  listDigitalEmployeeAvatarAssets
} from "@/lib/api/employees";
import { ApiRequestError } from "@/lib/api/client";
import { apiErrorMessage } from "@/lib/api/api-error";
import { listTeams } from "@/lib/api/teams";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { cn } from "@/lib/utils";
import { providerDisplayName } from "./provider-label";
import {
  findTemplateByType,
  orderedEmployeeTypes,
  riskSortValue,
  stringList,
  stringValue,
  templateCapabilityPreview,
  templateCapabilitySummary,
  templateDefaultInjectionLine,
  templateRisk,
  templateSearchText
} from "./template-utils";

const BLANK_CUSTOM_EMPLOYEE_TYPE = "custom_agent";
const BLANK_CUSTOM_TITLE = "自定义身份";

const configSteps = ["身份", "能力", "Provider 类型"] as const;
type StepName = (typeof configSteps)[number];
type CreateFlowStep = "template" | "configure" | "confirm";
type CreationMode = "template" | "blank_custom";

type WizardDraft = {
  creation_mode: CreationMode;
  capability_bindings: Record<string, unknown>;
  capability_binding_draft: {
    mcp_servers: string[];
    skills: string[];
  };
  daily_token_limit: string;
  description: string;
  employee_type: string;
  avatar_asset_id: string;
  name: string;
  persona_memory_markdown: string;
  risk_level: string;
  role: string;
  runtime_binding: string;
  runtime_node_id: string;
  provider_type: string;
  team_id: string;
  environment_variables: EnvironmentVariableDraftRow[];
};

type EnvironmentVariableDraftRow = {
  id: string;
  name: string;
  value: string;
  sensitive: boolean;
};

type ValidationErrors = Partial<
  Record<
    "avatar_asset_id" | "daily_token_limit" | "employee_type" | "name" | "role" | "runtime" | "team_id",
    string
  >
>;

type CreateEmployeeViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
};

const emptyDraft: WizardDraft = {
  capability_bindings: {},
  creation_mode: "template",
  capability_binding_draft: {
    mcp_servers: [],
    skills: []
},
  daily_token_limit: "",
  description: "",
  employee_type: "",
  avatar_asset_id: "",
  name: "",
  persona_memory_markdown: "",
  provider_type: "",
  risk_level: "medium",
  role: "",
  runtime_binding: "",
  runtime_node_id: "",
  team_id: "",
  environment_variables: []
};

export function CreateEmployeePage() {
  const apiBaseUrl = resolveControlPlaneUrl();

  return <CreateEmployeeView apiBaseUrl={apiBaseUrl} />;
}

export function CreateEmployeeView({ apiBaseUrl, fetcher }: CreateEmployeeViewProps) {
  const navigate = useNavigate();
  const search = useSearch({ strict: false });
  const requestedTemplate =
    typeof search === "object" && search && "template" in search ? stringValue(search.template) : "";
  const [flowStep, setFlowStep] = useState<CreateFlowStep>("template");
  const [draftTouched, setDraftTouched] = useState(false);
  const [stepIndex, setStepIndex] = useState(0);
  const [draft, setDraft] = useState<WizardDraft>(emptyDraft);
  const [errors, setErrors] = useState<ValidationErrors>({});
  const [templateQueryHandled, setTemplateQueryHandled] = useState("");

  const teams = useQuery({
    queryKey: ["teams"],
    queryFn: () => listTeams({ baseUrl: apiBaseUrl, fetcher })
});

  const createOptions = useQuery({
    enabled: !teams.isLoading,
    queryKey: ["digital-employee-create-options", draft.team_id || "team-less"],
    queryFn: () => getDigitalEmployeeCreateOptions({ baseUrl: apiBaseUrl, fetcher }, draft.team_id || undefined)
});

  const avatarAssets = useQuery({
    queryKey: ["digital-employee-avatar-assets"],
    queryFn: () => listDigitalEmployeeAvatarAssets({ baseUrl: apiBaseUrl, fetcher })
});
  // 头像独占：已被在册员工占用的头像不进入候选。
  const availableAvatarAssets = useMemo(
    () => (avatarAssets.data ?? []).filter((asset) => asset.status === "active" && !asset.in_use),
    [avatarAssets.data],
  );

  const selectedType = useMemo(
    () => createOptions.data?.employee_types.find((item) => item.type === draft.employee_type),
    [createOptions.data?.employee_types, draft.employee_type],
  );
  // 团队容量预检（来自 create-options）：满员团队在身份步即拦截，不再走完三步才失败。
  const teamCapacityCheck = createOptions.data?.creation_checks.find((check) => check.key === "team_capacity");
  const teamCapacityBlocked = teamCapacityCheck?.status === "blocked";
  const blankCustom = draft.creation_mode === "blank_custom";

  useEffect(() => {
    const optionsData = createOptions.data;
    if (!requestedTemplate || !optionsData || templateQueryHandled === requestedTemplate) return;
    const requestedType = findTemplateByType(optionsData, requestedTemplate);
    setTemplateQueryHandled(requestedTemplate);
    if (!requestedType) return;
    setDraft((current) => (
      current.creation_mode === "template" ? applyTypeDefaults(current, requestedType, optionsData) : current
    ));
  }, [createOptions.data, requestedTemplate, templateQueryHandled]);

  useEffect(() => {
    const firstAvatar = availableAvatarAssets[0];
    if (!draft.avatar_asset_id && firstAvatar) {
      setDraft((current) => ({ ...current, avatar_asset_id: firstAvatar.id }));
    }
  }, [availableAvatarAssets, draft.avatar_asset_id]);

  useEffect(() => {
    const candidateProviders = providerCandidates(createOptions.data);
    setDraft((current) => {
      if (candidateProviders.length === 1 && current.provider_type !== candidateProviders[0]) {
        return {
          ...current,
          runtime_binding: "",
          runtime_node_id: "",
          provider_type: candidateProviders[0]
};
      }
      if (current.provider_type && !candidateProviders.includes(current.provider_type)) {
        return { ...current, provider_type: "", runtime_binding: "", runtime_node_id: "" };
      }
      return current;
    });
  }, [createOptions.data]);

  const createEmployee = useMutation({
    mutationFn: async () => {
      if (!draft.provider_type) {
        throw new Error("请选择 Provider 类型");
      }

      try {
        return await createDigitalEmployee(
          { baseUrl: apiBaseUrl, fetcher },
          {
            team_id: draft.team_id || undefined,
            employee_type: draft.employee_type,
            name: draft.name.trim(),
            avatar_asset_id: draft.avatar_asset_id,
            role: draft.role.trim(),
            ...(draft.description.trim() ? { description: draft.description.trim() } : {}),
            ...(blankCustom ? { metadata: { creation_mode: "blank_custom" } } : {}),
            budget_policy: budgetPolicyFromDraft(draft),
            ...capabilitySelectionFromDraft(draft, createOptions.data),
            capability_bindings: capabilityBindingsFromDraft(draft),
            persona_memory_markdown: draft.persona_memory_markdown.trim(),
            risk_level: draft.risk_level,
            provider_type: draft.provider_type,
            environment_variables: draft.environment_variables
              .filter((row) => row.name.trim() && row.value)
              .map((row) => ({ name: row.name.trim(), value: row.value, sensitive: row.sensitive }))
},
        );
      } catch (err) {
        // 中文化落在 mutationFn 层：横幅与全局失败 toast（main.tsx onError 透传
        // error.message）共用后端权威中文 message（apiErrorMessage 读 coded error），
        // 不再靠英文错误文本关键词匹配。
        if (err instanceof ApiRequestError) {
          err.message = apiErrorMessage(err, CREATE_EMPLOYEE_FALLBACK_MESSAGE);
        }
        throw err;
      }
    },
    onSuccess: (employee) => {
      void navigate({
        params: { employeeId: employee.id },
        to: "/employees/$employeeId"
});
    }
});

  const currentStep = configSteps[stepIndex];
  const teamOptions = useMemo(() => (teams.data ?? []).filter((team) => team.status === "active"), [teams.data]);
  const selectedTeam = teamOptions.find((team) => team.id === draft.team_id);
  const isIdentityStepReady = !teams.isLoading && !avatarAssets.isLoading && currentStep === "身份";
  const shouldShowConfigureLoading =
    teams.isLoading ||
    avatarAssets.isLoading ||
    (currentStep !== "身份" && createOptions.isLoading);

  // 提交失败横幅不粘滞：任何草稿修改或步骤移动后清除上一次的失败状态。
  function clearCreateError() {
    if (createEmployee.isError) {
      createEmployee.reset();
    }
  }

  function updateDraft(patch: Partial<WizardDraft>) {
    if (flowStep === "configure") {
      setDraftTouched(true);
    }
    clearCreateError();
    setDraft((current) => ({ ...current, ...patch }));
  }

  function selectType(typeValue: string) {
    if (flowStep === "configure") {
      setDraftTouched(true);
    }
    const nextType = createOptions.data?.employee_types.find((item) => item.type === typeValue);
    if (!nextType) {
      updateDraft({ employee_type: typeValue });
      return;
    }
    setDraft((current) => applyTypeDefaults(current, nextType, createOptions.data));
  }

  function selectProvider(providerType: string) {
    updateDraft({
      provider_type: providerType,
      runtime_binding: "",
      runtime_node_id: ""
});
    setErrors((current) => ({ ...current, runtime: undefined }));
  }

  function nextStep() {
    const nextErrors = validateStep(currentStep, draft);
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length === 0) {
      clearCreateError();
      setStepIndex((current) => Math.min(current + 1, configSteps.length - 1));
    }
  }

  function submit() {
    const nextErrors = validateDraftForCreate(draft);
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length === 0) {
      createEmployee.mutate();
      return;
    }

    setFlowStep("configure");
    if (nextErrors.avatar_asset_id || nextErrors.employee_type || nextErrors.name || nextErrors.role) {
      setStepIndex(0);
    } else if (nextErrors.daily_token_limit) {
      setStepIndex(1);
    } else if (nextErrors.runtime) {
      setStepIndex(2);
    }
  }

  function enterConfiguration() {
    setErrors({});
    setFlowStep("configure");
    setStepIndex(0);
    setDraftTouched(false);
  }

  function enterConfirmCreation() {
    const nextErrors = validateStep("Provider 类型", draft);
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length === 0) {
      setFlowStep("confirm");
    }
  }

  function resetDraftForTeam(teamId: string, creationMode: CreationMode = draft.creation_mode) {
    setErrors({});
    setStepIndex(0);
    setDraftTouched(false);
    const nextDraft = { ...emptyDraft, creation_mode: creationMode, team_id: teamId };
    setDraft(creationMode === "blank_custom" ? applyBlankCustomDefaults(nextDraft) : nextDraft);
  }

  function requestCreationModeChange(nextMode: CreationMode) {
    if (nextMode === draft.creation_mode) {
      // 从"返回创建方式"回来后点击当前激活路径＝继续配置，草稿保留。
      if (nextMode === "blank_custom" && flowStep === "template") {
        setFlowStep("configure");
      }
      return;
    }
    if (draftTouched && !window.confirm("更换创建路径会重置当前配置草稿，是否继续？")) {
      return;
    }
    setErrors({});
    setStepIndex(0);
    setDraftTouched(false);
    const nextDraft = { ...emptyDraft, creation_mode: nextMode, team_id: draft.team_id };
    if (nextMode === "blank_custom") {
      setDraft(applyBlankCustomDefaults(nextDraft));
      setFlowStep("configure");
      setStepIndex(0);
      return;
    }
    setDraft(nextDraft);
    setFlowStep("template");
  }

  function requestTemplateChange() {
    if (draftTouched && !window.confirm("更换创建路径会重置当前配置草稿，是否继续？")) {
      return;
    }
    resetDraftForTeam(draft.team_id, "template");
    setFlowStep("template");
  }

  function requestTeamChange(nextTeamId: string) {
    if (nextTeamId === draft.team_id) {
      return;
    }
    updateDraft({ team_id: nextTeamId });
  }

  return (
    <>
      <ShellPageHeader
        back={
          flowStep === "template" ? (
            <ShellPageHeaderBack ariaLabel="返回数字员工列表" to="/employees" />
          ) : undefined
        }
        icon={flowStep === "template" ? undefined : <Bot />}
        iconTone="brand"
        title="创建数字员工"
        subtitle={flowStep === "template"
          ? "先选择创建方式，再完成配置并确认创建。"
          : "按职责定位、能力选择和必选 Provider 类型完成员工画像。"
        }
        actions={
          flowStep !== "template" ? (
            <Button onClick={requestTemplateChange} type="button" variant="outline">
              <ArrowLeft className="size-4" />
              返回
            </Button>
          ) : undefined
        }
      />
      <Main width="canvas">
        {teams.isError ? (
          <Alert className="mb-4" variant="destructive">
            <AlertTitle>团队列表加载失败</AlertTitle>
            <AlertDescription>{getErrorMessage(teams.error, "加载团队列表失败，请稍后重试。")}</AlertDescription>
          </Alert>
        ) : null}
        {!teams.isLoading && !teams.isError && teamOptions.length === 0 ? (
          <Alert className="mb-4">
            <AlertTitle>暂无可用团队</AlertTitle>
            <AlertDescription>可将归属团队选择为“无”，创建租户级独立数字员工。</AlertDescription>
          </Alert>
        ) : null}
        {createOptions.isError ? (
          <Alert className="mb-4" variant="destructive">
            <AlertTitle>创建选项加载失败</AlertTitle>
            <AlertDescription>请稍后重试，或切换团队后重新加载创建选项。</AlertDescription>
          </Alert>
        ) : null}
        {avatarAssets.isError ? (
          <Alert className="mb-4" variant="destructive">
            <AlertTitle>头像库加载失败</AlertTitle>
            <AlertDescription>{getErrorMessage(avatarAssets.error, "加载头像库失败，请稍后重试。")}</AlertDescription>
          </Alert>
        ) : null}

        <CreationStageProgress
          flowStep={flowStep}
          onNavigate={(stage) => {
            clearCreateError();
            setFlowStep(stage);
          }}
        />

        {flowStep === "template" ? (
          <div className="grid gap-4 xl:h-[calc(100vh-220px)] xl:min-h-[560px] xl:grid-cols-[260px_minmax(0,1fr)]">
            <CreationPathPanel creationMode={draft.creation_mode} onSelectMode={requestCreationModeChange} />

            {draft.creation_mode === "template" ? (
              <TemplateSelectionPanel
                draft={draft}
                options={createOptions.data}
                selectedTeamName={selectedTeam?.name}
                selectedType={selectedType}
                onEnterConfiguration={enterConfiguration}
                onSelectType={selectType}
              />
            ) : null}
          </div>
        ) : null}

        {flowStep === "configure" ? (
          <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_340px] xl:items-start">
            <GlassCard className="flex min-w-0 flex-col xl:max-h-[calc(100vh-220px)] xl:min-h-[560px]">
              <div className="border-b border-[color:var(--aurora-hairline)] p-4">
                <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                  <div className="flex items-center gap-2.5">
                    <IconTile tone="brand" size="sm">
                        <Bot />
                      </IconTile>
                      <div>
                        <h2 className="text-lg font-semibold text-ink">员工画像蓝图</h2>
                        <p className="mt-0.5 text-sm text-ink-3">
                        按职责定位、可用能力和 Provider 类型完成员工画像。
                      </p>
                    </div>
                  </div>
                  <StepTabs currentStep={currentStep} />
                </div>
              </div>

              <div className="grid min-h-0 flex-1 gap-4 overflow-y-auto p-4">
                <div className="min-h-0">
                  {shouldShowConfigureLoading ? (
                    <div className="flex min-h-[360px] items-center justify-center gap-2 text-sm text-ink-3">
                      <Loader2 className="size-4 animate-spin" />
                      加载创建选项
                    </div>
                  ) : null}
                  {isIdentityStepReady ? (
                    <IdentityStep
                      avatarAssets={availableAvatarAssets}
                      draft={draft}
                      errors={errors}
                      selectedType={selectedType}
                      teamCapacityError={teamCapacityBlocked ? teamCapacityCheck?.message : undefined}
                      teamOptions={teamOptions}
                      onSelectAvatar={(avatarAssetId) => updateDraft({ avatar_asset_id: avatarAssetId })}
                      onSelectTeam={requestTeamChange}
                      onUpdate={updateDraft}
                    />
                  ) : null}
                  {!teams.isLoading && !createOptions.isLoading && currentStep === "能力" ? (
                    <CapabilityStep draft={draft} errors={errors} options={createOptions.data} onUpdate={updateDraft} />
                  ) : null}
                  {!teams.isLoading && !createOptions.isLoading && currentStep === "Provider 类型" ? (
                    <ProviderStep
                      draft={draft}
                      error={errors.runtime}
                      options={createOptions.data}
                      onSelectProvider={selectProvider}
                      onUpdate={updateDraft}
                    />
                  ) : null}
                </div>
              </div>

              {createEmployee.isError ? (
                <p className="px-4 text-sm text-danger">{getErrorMessage(createEmployee.error, CREATE_EMPLOYEE_FALLBACK_MESSAGE)}</p>
              ) : null}
              <div
                className="flex justify-between gap-3 border-t border-[color:var(--aurora-hairline)] p-4"
                data-testid="employee-configure-actions"
              >
                <Button
                  disabled={createEmployee.isPending}
                  onClick={() => {
                    clearCreateError();
                    if (stepIndex === 0) {
                      setFlowStep("template");
                      return;
                    }
                    setStepIndex((current) => Math.max(current - 1, 0));
                  }}
                  type="button"
                  variant="glass"
                >
                  <ChevronLeft className="size-4" />
                  {stepIndex === 0 ? "返回创建方式" : "上一步"}
                </Button>
                {stepIndex < configSteps.length - 1 ? (
                  <Button
                    disabled={
                      createOptions.isLoading ||
                      createOptions.isError ||
                      avatarAssets.isLoading ||
                      avatarAssets.isError ||
                      teamCapacityBlocked
                    }
                    onClick={nextStep}
                    type="button"
                  >
                    下一步
                    <ChevronRight className="size-4" />
                  </Button>
                ) : (
                  <Button
                    disabled={
                      createEmployee.isPending ||
                      createOptions.isLoading ||
                      createOptions.isError ||
                      avatarAssets.isLoading ||
                      avatarAssets.isError ||
                      teamCapacityBlocked ||
                      !draft.avatar_asset_id ||
                      !draft.provider_type
                    }
                    onClick={enterConfirmCreation}
                    type="button"
                  >
                    进入确认创建
                    <ChevronRight className="size-4" />
                  </Button>
                )}
              </div>
            </GlassCard>

            <CreationPreflightPanel
              draft={draft}
              options={createOptions.data}
              selectedType={selectedType}
            />
          </div>
        ) : null}

        {flowStep === "confirm" ? (
          <ConfirmCreationStep
            createError={createEmployee.error}
            creating={createEmployee.isPending}
            draft={draft}
            selectedTeamName={selectedTeam?.name}
            selectedType={selectedType}
            onBack={() => setFlowStep("configure")}
            onSubmit={submit}
          />
        ) : null}
      </Main>
    </>
  );
}

function CreationStageProgress({
  flowStep,
  onNavigate
}: {
  flowStep: CreateFlowStep;
  onNavigate?: (stage: CreateFlowStep) => void;
}) {
  const stages: Array<{ key: CreateFlowStep; title: string; description: string }> = [
    { key: "template", title: "创建方式", description: "选择模板或自定义身份" },
    { key: "configure", title: "完成配置", description: "补齐身份、能力和 Provider" },
    { key: "confirm", title: "确认创建", description: "核对本次创建明细" },
  ];
  const activeIndex = stages.findIndex((stage) => stage.key === flowStep);
  const normalizedActiveIndex = activeIndex === -1 ? 2 : activeIndex;

  return (
    <GlassCard className="mb-4 px-4 py-3.5">
      <div className="grid gap-3 md:grid-cols-3">
        {stages.map((stage, index) => {
          const active = index === normalizedActiveIndex;
          const done = index < normalizedActiveIndex;
          const navigable = done && Boolean(onNavigate);
          const StageTag = navigable ? "button" : "div";

          return (
            <StageTag
              className={cn(
                "flex items-center gap-3 text-left",
                navigable && "cursor-pointer rounded-[12px] transition-opacity hover:opacity-80",
              )}
              key={stage.title}
              {...(navigable
                ? { type: "button" as const, onClick: () => onNavigate?.(stage.key), "aria-label": `返回${stage.title}` }
                : {})}
            >
              <span
                className={cn(
                  "flex size-8 shrink-0 items-center justify-center rounded-[11px] text-[13px] font-bold tabular-nums transition-colors",
                  active ? "bg-brand text-white shadow-card" : "",
                  done ? "bg-brand-soft text-brand-deep" : "",
                  !active && !done ? "bg-card-soft text-ink-3" : "",
                )}
              >
                {done ? <Check className="size-4" /> : index + 1}
              </span>
              <span className="min-w-0">
                <span className={cn("block text-sm font-semibold", active ? "text-brand-deep" : "text-ink")}>
                  {stage.title}
                </span>
                <span className="block truncate text-xs text-ink-3">{stage.description}</span>
              </span>
              {index < stages.length - 1 ? (
                <span
                  aria-hidden
                  className={cn(
                    "ml-auto hidden h-px w-6 shrink-0 md:block",
                    done ? "bg-brand/40" : "bg-line",
                  )}
                />
              ) : null}
            </StageTag>
          );
        })}
      </div>
    </GlassCard>
  );
}

function CreationPathPanel({
  creationMode,
  onSelectMode
}: {
  creationMode: CreationMode;
  onSelectMode: (mode: CreationMode) => void;
}) {
  const paths = [
    {
      title: "从专业模板创建",
      description: "按职责模板带出默认角色、能力建议和 Provider 默认值。",
      icon: Sparkles,
      mode: "template" as const,
      badge: "推荐",
      disabled: false
},
    {
      title: "空白自定义",
      description: "直接定义自定义身份，逐项手动配置职责定位、能力和 Provider 类型。",
      icon: FileText,
      mode: "blank_custom" as const,
      badge: "可用",
      disabled: false
},
    {
      title: "从团队角色复制",
      description: "复用团队内已验证的角色画像和能力配置。",
      icon: ClipboardCheck,
      mode: undefined,
      badge: "暂未开放",
      disabled: true
},
    {
      title: "从历史员工克隆",
      description: "基于已有员工配置生成新草稿，保留审计来源。",
      icon: GitBranch,
      mode: undefined,
      badge: "暂未开放",
      disabled: true
},
  ];

  return (
    <GlassCard className="flex flex-col p-3.5">
      <div className="mb-3.5 flex items-center gap-2.5 px-1">
        <IconTile tone="brand" size="sm">
          <Sparkles />
        </IconTile>
        <div>
          <h2 className="text-base font-semibold text-ink">创建路径</h2>
          <p className="text-xs text-ink-3">先选入口，再进入配置。</p>
        </div>
      </div>
      <div className="grid gap-2">
        {paths.map((path) => {
          const Icon = path.icon;
          const active = path.mode === creationMode;
          return (
            <button
              aria-pressed={active}
              className={cn(
                "rounded-[14px] border p-3 text-left transition-all duration-200",
                active
                  ? "border-brand/40 bg-brand-soft shadow-card"
                  : "border-line bg-card/70",
                path.disabled
                  ? "cursor-not-allowed opacity-60"
                  : "hover:-translate-y-0.5 hover:border-brand/30 hover:shadow-md",
              )}
              disabled={path.disabled}
              key={path.title}
              onClick={() => {
                if (path.mode) {
                  onSelectMode(path.mode);
                }
              }}
              type="button"
            >
              <span className="flex items-start gap-2.5">
                <IconTile tone={active ? "brand" : "mute"} size="sm">
                  <Icon />
                </IconTile>
                <span className="min-w-0 flex-1">
                  <span className="flex items-center justify-between gap-2">
                    <span className={cn("text-sm font-semibold", active ? "text-brand-deep" : "text-ink")}>
                      {path.title}
                    </span>
                    <Chip active={active}>{path.badge}</Chip>
                  </span>
                  <span className="mt-1 block text-xs leading-5 text-ink-3">{path.description}</span>
                </span>
              </span>
            </button>
          );
        })}
      </div>
      <div className="mt-3.5 flex items-start gap-2 rounded-[14px] border border-info/20 bg-info-soft p-3 text-xs leading-5 text-ink">
        <ShieldCheck className="mt-0.5 size-4 shrink-0 text-info" />
        <span>创建后进入 ready，不会自动执行任务；项目或任务调度可手动发起。</span>
      </div>
    </GlassCard>
  );
}

function TemplateSelectionPanel({
  draft,
  options,
  selectedTeamName,
  selectedType,
  onEnterConfiguration,
  onSelectType
}: {
  draft: WizardDraft;
  options?: DigitalEmployeeCreateOptions;
  selectedTeamName?: string;
  selectedType?: DigitalEmployeeTypeOption;
  onEnterConfiguration: () => void;
  onSelectType: (value: string) => void;
}) {
  const employeeTypes = useMemo(() => orderedEmployeeTypes(options?.employee_types ?? []), [options?.employee_types]);
  const [templateQuery, setTemplateQuery] = useState("");
  const [riskFilter, setRiskFilter] = useState("all");
  const riskOptions = useMemo(() => {
    const risks = new Set(employeeTypes.map((typeOption) => templateRisk(typeOption)));
    return ["all", ...Array.from(risks).sort((left, right) => riskSortValue(left) - riskSortValue(right))];
  }, [employeeTypes]);
  const filteredEmployeeTypes = useMemo(() => {
    const normalizedQuery = templateQuery.trim().toLowerCase();
    return employeeTypes.filter((typeOption) => {
      const matchesRisk = riskFilter === "all" || templateRisk(typeOption) === riskFilter;
      if (!matchesRisk) return false;
      if (!normalizedQuery) return true;
      return templateSearchText(typeOption).includes(normalizedQuery);
    });
  }, [employeeTypes, riskFilter, templateQuery]);

  return (
    <GlassCard className="@container/template flex min-w-0 flex-col">
      <div className="border-b border-[color:var(--aurora-hairline)] p-4">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex items-center gap-2.5">
            <IconTile tone="brand" size="sm">
              <Sparkles />
            </IconTile>
            <div>
              <h2 className="text-base font-semibold text-ink">选择内置模板</h2>
              <p className="mt-0.5 text-sm text-ink-3">模板只负责带出默认角色、模板能力和 Provider 默认值。</p>
            </div>
          </div>
          <Chip>
            {employeeTypes.length} 个模板 · 已筛选 {filteredEmployeeTypes.length}
          </Chip>
        </div>
        <div className="mt-4 grid gap-2 lg:grid-cols-[minmax(220px,1fr)_160px]">
          <Input
            aria-label="搜索专业模板"
            onChange={(event) => setTemplateQuery(event.target.value)}
            placeholder="搜索模板、角色、技能或 MCP"
            value={templateQuery}
          />
          <select
            aria-label="按风险等级筛选模板"
            className={selectClassName}
            onChange={(event) => setRiskFilter(event.target.value)}
            value={riskFilter}
          >
            {riskOptions.map((risk) => (
              <option key={risk} value={risk}>
                {risk === "all" ? "全部风险" : riskLabel(risk)}
              </option>
            ))}
          </select>
        </div>
      </div>
      {employeeTypes.length === 0 ? (
        <div className="m-4 flex min-h-[420px] flex-1 items-center justify-center rounded-[14px] border border-line bg-card-soft p-6 text-sm text-ink-3">
          当前创建选项未返回可用专业模板。
        </div>
      ) : (
        <div className="min-h-0 flex-1 p-4">
          <WorkSurface
            className="h-full"
            data-testid="template-selection-table"
            data-slot="template-selection-table"
          >
            <div className="h-full max-h-[min(680px,calc(100vh-360px))] overflow-auto">
              <table className="w-full min-w-[880px] border-separate border-spacing-0 text-sm">
                <thead>
                  <tr>
                    {[
                      { label: "模板", w: "" },
                      { label: "默认角色", w: "w-[150px]" },
                      { label: "模板能力", w: "w-[290px]" },
                      { label: "风险等级", w: "w-[112px]" },
                      { label: "默认注入", w: "w-[132px]" },
                    ].map(({ label, w }) => (
                      <th
                        key={label}
                        className={cn(
                          "sticky top-0 z-10 border-b border-line-strong bg-card-soft px-3 py-2.5 text-left text-[11px] font-bold uppercase tracking-wide text-ink-3",
                          w,
                        )}
                      >
                        {label}
                      </th>
                    ))}
                    <th className="sticky top-0 z-10 w-[100px] border-b border-line-strong bg-card-soft px-3 py-2.5 text-right text-[11px] font-bold uppercase tracking-wide text-ink-3">
                      选择
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {filteredEmployeeTypes.map((typeOption) => (
                    <TemplateTableRow
                      key={typeOption.type}
                      selected={typeOption.type === draft.employee_type}
                      typeOption={typeOption}
                      onSelect={() => onSelectType(typeOption.type)}
                    />
                  ))}
                </tbody>
              </table>
              {filteredEmployeeTypes.length === 0 ? (
                <div className="flex min-h-[220px] items-center justify-center border-t border-line bg-card-soft p-6 text-sm text-ink-3">
                  没有匹配当前筛选条件的专业模板。
                </div>
              ) : null}
            </div>
          </WorkSurface>
        </div>
      )}
      <div className="border-t border-[color:var(--aurora-hairline)] px-4 py-3.5">
        <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2 text-sm">
              <span className="font-semibold text-ink">已选模板摘要</span>
              <Chip>团队 {selectedTeamName || "无（租户级）"}</Chip>
              <Chip>模板 {selectedType?.label ?? (draft.employee_type || "未选择")}</Chip>
              <Chip>默认角色 {draft.role || selectedType?.default_role || "未生成"}</Chip>
              <Chip>风险 {riskLabel(draft.risk_level || "medium")}</Chip>
            </div>
            <p className="mt-2 text-sm text-ink-3">
              没有合适的模板？可在左侧选择空白自定义。
            </p>
          </div>
          <Button disabled={!draft.employee_type} onClick={onEnterConfiguration} type="button">
            进入完成配置
            <ChevronRight className="size-4" />
          </Button>
        </div>
      </div>
      {selectedType ? <span className="sr-only">当前选择：{selectedType.label}</span> : null}
    </GlassCard>
  );
}

function TemplateTableRow({
  selected,
  typeOption,
  onSelect
}: {
  selected: boolean;
  typeOption: DigitalEmployeeTypeOption;
  onSelect: () => void;
}) {
  const risk = templateRisk(typeOption);
  const capability = templateCapabilitySummary(typeOption);

  return (
    <tr
      className={cn(
        "transition-colors [&>td]:border-b [&>td]:border-line [&:last-child>td]:border-b-0 [&:hover>td]:bg-card-inner",
        selected ? "[&>td]:bg-brand-soft [&>td:first-child]:shadow-[inset_3px_0_0_var(--brand)]" : "",
      )}
    >
      <td className="px-3 py-3 align-top">
        <div className="flex min-w-0 gap-3">
          <IconTile tone={selected ? "brand" : "mute"} size="sm">
            <Code2 />
          </IconTile>
          <div className="min-w-0">
            <div className="font-semibold text-ink">{typeOption.label}</div>
            <div className="mt-1 line-clamp-2 text-xs leading-5 text-ink-3">{typeOption.description}</div>
          </div>
        </div>
      </td>
      <td className="px-3 py-3 align-top">
        <span className="block max-w-[180px] truncate font-mono text-xs text-ink-2">
          {typeOption.default_role || typeOption.type}
        </span>
      </td>
      <td className="px-3 py-3 align-top">
        <div className="grid gap-1.5">
          <div className="flex flex-wrap gap-1.5">
            <Chip>{`技能 ${capability.skills.length}`}</Chip>
            <Chip>{`MCP ${capability.mcpServers.length}`}</Chip>
            <Chip>{`Provider ${capability.providerTypes.length}`}</Chip>
          </div>
          <div className="truncate text-xs text-ink-3">{templateCapabilityPreview(typeOption)}</div>
        </div>
      </td>
      <td className="px-3 py-3 align-top">
        <RiskPill risk={risk} />
      </td>
      <td className="px-3 py-3 align-top">
        <span className="line-clamp-2 text-xs leading-5 text-ink-3">
          {templateDefaultInjectionLine(typeOption)}
        </span>
      </td>
      <td className="px-3 py-3 text-right align-top">
        <Button
          aria-label={`${selected ? "已选择" : "选择"}${typeOption.label}模板`}
          aria-pressed={selected}
          className="ml-auto whitespace-nowrap"
          onClick={onSelect}
          size="sm"
          type="button"
          variant={selected ? "primary" : "outline"}
        >
          {selected ? <Check className="size-4" /> : null}
          {selected ? "已选" : "选择"}
        </Button>
      </td>
    </tr>
  );
}

function isCapabilityPolicyCheck(check: { key: string }) {
  return check.key === "capability_policy" || check.key === "capability_boundary";
}

function displayablePreflightChecks<T extends { key: string }>(checks: T[]) {
  return checks.filter((check) => !isCapabilityPolicyCheck(check));
}

function InlineSummary({ label, value }: { label: string; value: string }) {
  return (
    <div className="glass-inner flex items-center justify-between gap-3 px-3 py-2">
      <span className="text-ink-3">{label}</span>
      <span className="max-w-[180px] truncate font-medium text-ink">{value}</span>
    </div>
  );
}

function StepTabs({ currentStep }: { currentStep: StepName }) {
  const currentIndex = configSteps.indexOf(currentStep);

  return (
    <div className="glass-inner flex flex-wrap gap-1 p-1.5">
        {configSteps.map((step, index) => {
          const active = step === currentStep;
          const done = index < currentIndex;

          return (
            <div
              className={cn(
                "flex h-8 items-center gap-2 rounded-[10px] px-2.5 text-xs font-semibold transition-colors",
                active ? "bg-brand-soft text-brand-deep" : "text-ink-3",
                done && !active ? "text-ink-2" : "",
              )}
              key={step}
            >
              <span
                className={cn(
                  "flex size-5 items-center justify-center rounded-full text-[11px] tabular-nums",
                  active ? "bg-brand text-white" : "",
                  done && !active ? "bg-brand-soft text-brand-deep" : "",
                  !active && !done ? "bg-card text-ink-3" : "",
                )}
              >
                {done ? <Check className="size-3" /> : index + 1}
              </span>
              <span>{step}</span>
            </div>
          );
        })}
    </div>
  );
}

function CreationPreflightPanel({
  draft,
  options,
  selectedType
}: {
  draft: WizardDraft;
  options?: DigitalEmployeeCreateOptions;
  selectedType?: DigitalEmployeeTypeOption;
}) {
  const checks = displayablePreflightChecks(options?.creation_checks ?? []);
  const providers = providerCandidates(options);

  return (
    <aside className="grid content-start">
      <GlassCard className="divide-y divide-[color:var(--aurora-hairline)]">
      <section className="p-4">
        <div className="mb-3 flex items-center gap-2">
          <IconTile tone="ok" size="sm">
            <ShieldCheck />
          </IconTile>
          <div>
            <h2 className="text-base font-semibold text-ink">预检项目</h2>
            <p className="text-xs text-ink-3">来自 Control Plane 创建候选接口。</p>
          </div>
        </div>
        <div className="grid gap-2">
          {checks.length === 0 ? (
            <p className="glass-inner p-3 text-sm text-ink-3">等待创建候选加载。</p>
          ) : (
            checks.map((check) => (
              <div
                className="glass-inner flex items-start gap-2 p-3"
                key={check.key}
              >
                <span className={cn("mt-1 size-2 shrink-0 rounded-full", checkDotClassName(check.status))} />
                <span className="min-w-0 flex-1">
                  <span className="flex items-center justify-between gap-2">
                    <span className="text-sm font-medium text-ink">{check.label}</span>
                    <StatusPill tone={checkTone(check.status)}>{checkStatusLabel(check.status)}</StatusPill>
                  </span>
                  <span className="mt-1 block text-xs leading-5 text-ink-3">{check.message}</span>
                </span>
              </div>
            ))
          )}
        </div>
      </section>

      <section className="p-4">
        <div className="mb-3 flex items-center gap-2">
          <IconTile tone="artifact" size="sm">
            <Gauge />
          </IconTile>
          <div>
            <h2 className="text-base font-semibold text-ink">画像摘要</h2>
            <p className="text-xs text-ink-3">随配置实时更新。</p>
          </div>
        </div>
        <div className="grid gap-3 text-sm">
          <SummaryItem
            label="专业类型"
            value={draft.creation_mode === "blank_custom" ? BLANK_CUSTOM_TITLE : (selectedType?.label ?? draft.employee_type) || "未选择"}
          />
          <SummaryItem label="职责定位" value={draft.role || "未填写"} />
          <SummaryItem label="风险等级" value={riskLabel(draft.risk_level || "medium")} />
          <SummaryItem
            label="能力选择"
            value={`技能 ${draft.capability_binding_draft.skills.length} · MCP ${draft.capability_binding_draft.mcp_servers.length}`}
          />
          <SummaryItem
            label="Provider 类型"
            value={draft.provider_type ? providerDisplayName(draft.provider_type) : `${providers.length} 个候选`}
          />
        </div>
      </section>

      <section className="p-4 text-xs leading-5 text-ink-3">
        <div className="mb-2 flex items-center gap-2 font-semibold text-ink">
          <Cpu className="size-4 text-brand" />
          创建后事实
        </div>
        <div className="grid gap-2">
          <div>1. 写入身份与初始配置修订</div>
          <div>2. 记录 Provider 类型</div>
          <div>3. Runtime 节点会在项目运行准备中决定，不在创建时绑定到员工。</div>
          <div>4. 进入 ready，等待任务调度</div>
        </div>
      </section>
      </GlassCard>
    </aside>
  );
}

function ConfirmCreationStep({
  createError,
  creating,
  draft,
  selectedTeamName,
  selectedType,
  onBack,
  onSubmit
}: {
  createError: unknown;
  creating: boolean;
  draft: WizardDraft;
  selectedTeamName?: string;
  selectedType?: DigitalEmployeeTypeOption;
  onBack: () => void;
  onSubmit: () => void;
}) {
  const environmentVariableCount = draft.environment_variables.filter((row) => row.name.trim() && row.value).length;

  return (
    <GlassCard className="flex flex-col">
      <div className="border-b border-[color:var(--aurora-hairline)] p-4">
        <div className="flex items-center gap-2.5">
          <IconTile tone="brand" size="sm">
            <ClipboardCheck />
          </IconTile>
          <div>
            <h2 className="text-lg font-semibold text-ink">确认创建</h2>
            <p className="mt-0.5 text-sm text-ink-3">
              核对本次将提交给 Control Plane 的员工配置；确认后创建 ready 状态数字员工。
            </p>
          </div>
        </div>
      </div>
      <div className="grid gap-4 p-4 lg:grid-cols-2">
        <section className="glass-inner p-4">
          <h3 className="text-sm font-semibold text-ink">身份与模板</h3>
          <div className="mt-3 grid gap-2 text-sm">
            <InlineSummary label="归属团队" value={selectedTeamName || "无（租户级）"} />
            <InlineSummary label="创建路径" value={draft.creation_mode === "blank_custom" ? BLANK_CUSTOM_TITLE : "专业模板"} />
            {draft.creation_mode !== "blank_custom" ? (
              <InlineSummary label="专业模板" value={selectedType?.label ?? (draft.employee_type || "未选择")} />
            ) : null}
            <InlineSummary label="名称" value={draft.name.trim() || "未填写"} />
            <InlineSummary label="职责定位" value={draft.role || "未填写"} />
            <InlineSummary
              label="员工说明"
              value={draft.description.trim() || "未填写"}
            />
            <InlineSummary label="风险等级" value={riskLabel(draft.risk_level || "medium")} />
          </div>
        </section>

        <section className="glass-inner p-4">
          <h3 className="text-sm font-semibold text-ink">能力与 Provider 类型</h3>
          <div className="mt-3 grid gap-2 text-sm">
            <InlineSummary
              label="能力选择"
              value={`技能 ${draft.capability_binding_draft.skills.length} · MCP ${draft.capability_binding_draft.mcp_servers.length}`}
            />
            <InlineSummary
              label="Provider 类型"
              value={draft.provider_type ? providerDisplayName(draft.provider_type) : "未选择"}
            />
            <InlineSummary label="Runtime 节点" value="会在项目运行准备中决定，不在创建时绑定到员工。" />
            <InlineSummary
              label="每日预算"
              value={draft.daily_token_limit.trim() ? `${draft.daily_token_limit.trim()} Token` : "无预算上限"}
            />
            <InlineSummary label="环境变量" value={`${environmentVariableCount} 个`} />
          </div>
        </section>
      </div>
      {createError ? (
        <p className="px-4 pb-2 text-sm text-danger">{getErrorMessage(createError, CREATE_EMPLOYEE_FALLBACK_MESSAGE)}</p>
      ) : null}
      <div className="flex justify-between gap-3 border-t border-[color:var(--aurora-hairline)] p-4">
        <Button disabled={creating} onClick={onBack} type="button" variant="glass">
          <ChevronLeft className="size-4" />
          返回配置
        </Button>
        <Button disabled={creating} onClick={onSubmit} type="button">
          {creating ? <Loader2 className="size-4 animate-spin" /> : <Plus className="size-4" />}
          确认创建
        </Button>
      </div>
    </GlassCard>
  );
}

function IdentityStep({
  avatarAssets,
  draft,
  errors,
  selectedType,
  teamCapacityError,
  teamOptions,
  onSelectTeam,
  onSelectAvatar,
  onUpdate
}: {
  avatarAssets: DigitalEmployeeAvatarAsset[];
  draft: WizardDraft;
  errors: ValidationErrors;
  selectedType?: DigitalEmployeeTypeOption;
  teamCapacityError?: string;
  teamOptions: Array<{ id: string; name: string }>;
  onSelectTeam: (value: string) => void;
  onSelectAvatar: (value: string) => void;
  onUpdate: (patch: Partial<WizardDraft>) => void;
}) {
  const isBlankCustom = draft.creation_mode === "blank_custom";

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h2 className="text-lg font-semibold text-ink">身份</h2>
        <p className="text-sm text-ink-3">确定团队、名称、职责定位与员工说明。负责人由后端按当前登录身份注入。</p>
      </div>
      <div className="grid gap-4 md:grid-cols-2">
        <Field label="归属团队" error={errors.team_id ?? teamCapacityError}>
          <select
            aria-invalid={Boolean(errors.team_id)}
            className={selectClassName}
            id="employee-team"
            onChange={(event) => onSelectTeam(event.target.value)}
            value={draft.team_id}
          >
            <option value="">无（暂不归属团队）</option>
            {teamOptions.map((team) => (
              <option key={team.id} value={team.id}>
                {team.name}
              </option>
            ))}
          </select>
          <p className="text-xs text-ink-3">选择“无”将创建租户级独立数字员工。</p>
        </Field>
        <Field label="名称" error={errors.name}>
          <Input
            aria-invalid={Boolean(errors.name)}
            id="employee-name"
            onChange={(event) => onUpdate({ name: event.target.value })}
            value={draft.name}
          />
        </Field>
        <Field label="职责定位" error={errors.role}>
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
            <option value="low">低</option>
            <option value="medium">中</option>
            <option value="high">高</option>
            <option value="critical">严重</option>
          </select>
        </Field>
        <div className="md:col-span-2">
          <Field label="员工说明">
            <Textarea
              id="employee-description"
              placeholder="简述这位数字员工负责什么、边界与协作方式，便于列表扫读识别。"
              rows={3}
              value={draft.description}
              onChange={(event) => onUpdate({ description: event.target.value })}
            />
            <p className="text-xs text-ink-3">可选。会出现在数字员工卡片上，超出两行以省略号截断。</p>
          </Field>
        </div>
      </div>
      <div className="glass-inner p-3 text-sm">
        <div className="font-semibold text-ink">
          {isBlankCustom ? BLANK_CUSTOM_TITLE : selectedType?.label ?? "专业模板"}
        </div>
        <div className="mt-1 text-ink-3">
          {isBlankCustom ? "从空白配置开始，后续按职责逐项补齐能力和 Provider。" : selectedType?.description}
        </div>
      </div>
      <AvatarSelection
        assets={avatarAssets}
        error={errors.avatar_asset_id}
        selectedAssetId={draft.avatar_asset_id}
        onSelect={onSelectAvatar}
      />
    </div>
  );
}

function AvatarSelection({
  assets,
  error,
  onSelect,
  selectedAssetId
}: {
  assets: DigitalEmployeeAvatarAsset[];
  error?: string;
  onSelect: (value: string) => void;
  selectedAssetId: string;
}) {
  return (
    <div className="flex flex-col gap-2.5">
      <div className="flex flex-wrap items-baseline gap-2">
        <span className="text-sm font-semibold text-ink">头像</span>
        <span className="text-xs text-ink-3">每个头像只能被一名数字员工使用，已占用的不再显示。</span>
      </div>
      <div className="flex flex-wrap gap-3">
        {assets.map((asset) => {
          const selected = asset.id === selectedAssetId;
          return (
            <button
              aria-pressed={selected}
              className={cn(
                "flex size-20 shrink-0 items-center justify-center rounded-full border bg-card-soft p-0.5 transition-all duration-200",
                selected ? "border-brand ring-2 ring-brand/30" : "hover:-translate-y-0.5 hover:border-brand/60",
              )}
              key={asset.id}
              onClick={() => onSelect(asset.id)}
              type="button"
            >
              <img alt={asset.label} className="size-full rounded-full object-cover" src={asset.thumbnail_url} />
            </button>
          );
        })}
      </div>
      {assets.length === 0 ? (
        <p className="mt-2 text-sm text-ink-3">头像库已全部被现有数字员工占用，请先扩充头像库再创建。</p>
      ) : null}
      {error ? <span className="mt-2 block text-sm text-danger">{error}</span> : null}
    </div>
  );
}

function CapabilityStep({
  draft,
  errors,
  options,
  onUpdate
}: {
  draft: WizardDraft;
  errors: ValidationErrors;
  options?: DigitalEmployeeCreateOptions;
  onUpdate: (patch: Partial<WizardDraft>) => void;
}) {
  const capabilityOptions = options?.capability_options;
  const inheritedCapabilities = inheritedCapabilityBindings(options);
  const templateBindingsSummary = formatTemplateBindingsSummary(draft);
  const extensionCapabilityOptions = {
    mcp_servers: withoutInheritedItems(capabilityOptions?.mcp_servers ?? [], inheritedCapabilities.mcp_servers),
    skills: withoutInheritedItems(capabilityOptions?.skills ?? [], inheritedCapabilities.skills)
};

  function toggle(kind: keyof WizardDraft["capability_binding_draft"], value: string) {
    const currentValues = draft.capability_binding_draft[kind];
    const nextValues = currentValues.includes(value)
      ? currentValues.filter((item) => item !== value)
      : [...currentValues, value];
    onUpdate({
      capability_binding_draft: {
        ...draft.capability_binding_draft,
        [kind]: nextValues
}
});
  }

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h2 className="text-lg font-semibold text-ink">能力</h2>
        <p className="text-sm text-ink-3">团队基线能力只读继承；这里只为员工补充个人技能和 MCP。</p>
      </div>
      <section className="glass-inner px-3.5 py-3">
        <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
          <span className="text-sm font-semibold text-ink">团队继承能力</span>
          <span className="text-xs text-ink-3">团队绑定能力只读展示，不会作为员工扩展能力重复提交。</span>
        </div>
        <div className="mt-2 flex flex-wrap items-center gap-x-5 gap-y-1.5">
          <CapabilityReadOnlyList label="技能" values={inheritedCapabilities.skills} />
          <CapabilityReadOnlyList label="MCP Server" values={inheritedCapabilities.mcp_servers} />
        </div>
      </section>
      <div>
        <div className="text-sm font-semibold text-ink">员工扩展能力</div>
        <p className="mt-1 text-xs text-ink-3">
          这里只提交员工个人扩展项。
          {templateBindingsSummary ? `模板另带出：${templateBindingsSummary}。` : null}
        </p>
      </div>
      <CapabilityGroup
        checkedValues={draft.capability_binding_draft.skills}
        emptyState={
          <p className="text-sm text-ink-3">
            注册表暂无可选技能。候选来自租户技能市场,先
            <Link className="text-brand hover:underline" to="/skills">
              去技能市场上架
            </Link>
            ,回到这里即可选用;创建后也可在员工详情"扩展能力"中加载。
          </p>
        }
        label="技能"
        onToggle={(value) => toggle("skills", value)}
        values={extensionCapabilityOptions.skills}
      />
      <CapabilityGroup
        checkedValues={draft.capability_binding_draft.mcp_servers}
        emptyState={
          <p className="text-sm text-ink-3">
            注册表暂无可选 MCP Server。候选来自 MCP 注册表,先
            <Link className="text-brand hover:underline" to="/mcp">
              去 MCP 注册表登记
            </Link>
            ,回到这里即可选用。
          </p>
        }
        label="MCP Server"
        onToggle={(value) => toggle("mcp_servers", value)}
        values={extensionCapabilityOptions.mcp_servers}
      />
      <Field label="人格记忆.md">
        <Textarea
          className="min-h-[160px] border-line bg-card font-mono text-xs text-ink"
          id="persona-memory-markdown"
          onChange={(event) => onUpdate({ persona_memory_markdown: event.target.value })}
          placeholder="# 人格画像"
          value={draft.persona_memory_markdown}
        />
      </Field>
      <section className="lg:max-w-md">
        <Field error={errors.daily_token_limit} label="每日 Token 预算上限">
          <Input
            aria-invalid={Boolean(errors.daily_token_limit)}
            id="daily-token-limit"
            inputMode="numeric"
            onChange={(event) => onUpdate({ daily_token_limit: event.target.value })}
            placeholder="例如 200000"
            value={draft.daily_token_limit}
          />
          <p className="text-xs text-ink-3">留空表示不设置每日预算上限；填写时必须为正整数。</p>
        </Field>
      </section>
    </div>
  );
}

function CapabilityReadOnlyList({ label, values }: { label: string; values: string[] }) {
  return (
    <span className="flex flex-wrap items-center gap-1.5">
      <span className="text-xs text-ink-3">{label}</span>
      {values.length === 0 ? <span className="text-sm text-ink-3">无</span> : null}
      {values.map((value) => (
        <Chip key={value}>
          {value}
        </Chip>
      ))}
    </span>
  );
}

function CapabilityGroup({
  checkedValues,
  emptyState,
  label,
  onToggle,
  values
}: {
  checkedValues: string[];
  emptyState: ReactNode;
  label: string;
  onToggle: (value: string) => void;
  values: DigitalEmployeeCapabilityOptionItem[];
}) {
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-baseline gap-2">
        <span className="text-sm font-medium text-ink">{label}</span>
        <span className="text-xs text-ink-3 tabular-nums">已选 {checkedValues.length}</span>
      </div>
      <div className="grid gap-2 md:grid-cols-[repeat(auto-fill,minmax(260px,1fr))]">
        {values.map((item) => {
          const checked = checkedValues.includes(item.key);
          const disabled = !item.available;
          return (
            <label
              className={cn(
                "flex items-start gap-2 rounded-[12px] border px-3 py-2 text-sm transition-colors",
                disabled
                  ? "cursor-not-allowed border-line bg-card-soft opacity-60"
                  : checked
                    ? "cursor-pointer border-brand/40 bg-brand-soft text-brand-deep"
                    : "cursor-pointer border-line bg-card text-ink hover:bg-card-soft",
              )}
              key={item.key}
            >
              <Checkbox
                checked={checked}
                className="mt-0.5"
                disabled={disabled}
                onCheckedChange={() => onToggle(item.key)}
              />
              <span className="flex min-w-0 flex-col gap-0.5">
                <span className="flex min-w-0 items-center gap-1.5">
                  <span className="min-w-0 truncate font-medium">{item.label}</span>
                  {item.recommended ? (
                    <Chip className="shrink-0">
                      推荐
                    </Chip>
                  ) : null}
                  {disabled ? (
                    <Chip className="shrink-0">
                      未上架
                    </Chip>
                  ) : null}
                </span>
                {item.label !== item.key ? (
                  <span className="truncate font-mono text-xs text-ink-3">{item.key}</span>
                ) : null}
                {item.description ? (
                  <span className="line-clamp-2 text-xs text-ink-3">{item.description}</span>
                ) : null}
              </span>
            </label>
          );
        })}
        {values.length === 0 ? <div className="md:col-span-full">{emptyState}</div> : null}
      </div>
    </div>
  );
}

function SummaryItem({ label, value }: { label: string; value: string }) {
  return (
    <div className="glass-inner p-3">
      <div className="text-xs text-ink-3">{label}</div>
      <div className="mt-1 text-sm font-medium text-ink">{value}</div>
    </div>
  );
}

function ProviderStep({
  draft,
  error,
  options,
  onSelectProvider,
  onUpdate
}: {
  draft: WizardDraft;
  error?: string;
  options?: DigitalEmployeeCreateOptions;
  onSelectProvider: (providerType: string) => void;
  onUpdate: (patch: Partial<WizardDraft>) => void;
}) {
  const providers = providerCandidates(options);
  const updateEnvironmentRow = (rowId: string, patch: Partial<EnvironmentVariableDraftRow>) => {
    onUpdate({
      environment_variables: draft.environment_variables.map((row) =>
        row.id === rowId ? { ...row, ...patch } : row,
      )
});
  };
  const removeEnvironmentRow = (rowId: string) => {
    onUpdate({ environment_variables: draft.environment_variables.filter((row) => row.id !== rowId) });
  };

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h2 className="text-lg font-semibold text-ink">Provider 类型</h2>
        <p className="text-sm text-ink-3">数字员工必须选择一个 Provider 类型；Runtime 节点会在项目运行准备中决定，不在创建时绑定到员工。</p>
      </div>
      <RadioGroup onValueChange={onSelectProvider} value={draft.provider_type}>
        <div className="grid gap-3">
          {providers.map((providerType) => (
            <ProviderOption
              key={providerType}
              options={options}
              providerType={providerType}
              selected={draft.provider_type === providerType}
              onSelectProvider={onSelectProvider}
            />
          ))}
        </div>
      </RadioGroup>
      {providers.length === 0 ? (
        <p className="rounded-inner border border-dashed border-line bg-card-soft p-3 text-sm text-ink-3">
          当前创建选项没有返回可选 Provider 类型。
        </p>
      ) : null}
      {error ? <p className="text-sm text-danger">{error}</p> : null}
      <section className="glass-inner p-3.5">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h3 className="text-sm font-medium text-ink">员工环境变量</h3>
            <p className="text-xs text-ink-3">用于技能运行依赖；创建请求会提交值，接口不会回显明文。</p>
          </div>
          <Button
            onClick={() =>
              onUpdate({ environment_variables: [...draft.environment_variables, newEnvironmentVariableRow()] })
            }
            size="sm"
            type="button"
            variant="outline"
          >
            <Plus className="size-4" />
            添加环境变量
          </Button>
        </div>
        <div className="mt-3 grid gap-2">
          {draft.environment_variables.length === 0 ? (
            <p className="rounded-[12px] border border-dashed border-line p-3 text-sm text-ink-3">暂无环境变量。</p>
          ) : null}
          {draft.environment_variables.map((row, index) => (
            <div
              className="grid gap-2 rounded-[12px] border border-line bg-card p-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto_auto] md:items-end"
              key={row.id}
            >
              <div className="grid gap-1.5">
                <Label htmlFor={`employee-env-name-${row.id}`}>环境变量名称 {index + 1}</Label>
                <Input
                  id={`employee-env-name-${row.id}`}
                  onChange={(event) => updateEnvironmentRow(row.id, { name: event.target.value })}
                  placeholder="GH_TOKEN"
                  value={row.name}
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor={`employee-env-value-${row.id}`}>环境变量值 {index + 1}</Label>
                <Input
                  id={`employee-env-value-${row.id}`}
                  onChange={(event) => updateEnvironmentRow(row.id, { value: event.target.value })}
                  type="password"
                  value={row.value}
                />
              </div>
              <label className="flex h-10 items-center gap-2 rounded-xl border border-line px-3 text-sm text-ink">
                <Checkbox
                  checked={row.sensitive}
                  onCheckedChange={(checked) => updateEnvironmentRow(row.id, { sensitive: checked === true })}
                />
                敏感
              </label>
              <Button
                aria-label={`移除环境变量 ${index + 1}`}
                onClick={() => removeEnvironmentRow(row.id)}
                size="icon"
                type="button"
                variant="ghost"
              >
                <Trash2 />
              </Button>
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}

function ProviderOption({
  onSelectProvider,
  options,
  providerType,
  selected
}: {
  onSelectProvider: (providerType: string) => void;
  options?: DigitalEmployeeCreateOptions;
  providerType: string;
  selected?: boolean;
}) {
  const preview = providerDispatchPreview(options, providerType);
  const dispatchPreviewText = providerDispatchPreviewText(preview);

  return (
    <label
      className={cn(
        "flex cursor-pointer items-start gap-3 rounded-inner border p-3 text-sm transition-colors",
        selected
          ? "border-brand/40 bg-brand-soft"
          : "border-line bg-card hover:bg-card-soft",
      )}
      onClick={(event) => {
        event.preventDefault();
        onSelectProvider(providerType);
      }}
    >
      <RadioGroupItem aria-label={providerDisplayName(providerType)} value={providerType} />
      <span className="min-w-0 flex-1">
        <span className={cn("block font-semibold", selected ? "text-brand-deep" : "text-ink")}>
          {providerDisplayName(providerType)}
        </span>
        <span className="mt-1 block text-ink-3">{dispatchPreviewText}</span>
      </span>
    </label>
  );
}

function Field({
  children,
  error,
  label
}: {
  children: ReactNode;
  error?: string;
  label: string;
}) {
  const id = labelId[label] ?? "";

  return (
    <div className="grid gap-2">
      <Label className="text-ink" htmlFor={id}>{label}</Label>
      {children}
      {error ? <span className="text-sm text-danger">{error}</span> : null}
    </div>
  );
}

const labelId: Record<string, string> = {
  "人格记忆.md": "persona-memory-markdown",
  名称: "employee-name",
  归属团队: "employee-team",
  职责定位: "employee-role",
  员工说明: "employee-description",
  风险等级: "employee-risk",
  "每日 Token 预算上限": "daily-token-limit"
};

const selectClassName =
  "h-10 w-full rounded-xl border border-line bg-card px-3 py-1 text-sm text-ink shadow-sm outline-none transition-[color,box-shadow] focus-visible:border-brand focus-visible:ring-2 focus-visible:ring-brand/40 disabled:cursor-not-allowed disabled:opacity-50";

function applyTypeDefaults(
  current: WizardDraft,
  typeOption: DigitalEmployeeTypeOption,
  options: DigitalEmployeeCreateOptions | undefined,
): WizardDraft {
  const policyDefaults = options?.policy_defaults;
  const budgetPolicy = typeOption.budget_policy ?? {};
  const dailyTokenLimit = budgetPolicyValue(budgetPolicy);

  return {
    ...current,
    capability_bindings: capabilityBindingDefaults(typeOption.capability_bindings),
    capability_binding_draft: recommendedCapabilitySelection(typeOption, options),
    daily_token_limit: dailyTokenLimit,
    description: typeOption.description ?? "",
    employee_type: typeOption.type,
    persona_memory_markdown: typeOption.persona_memory_markdown ?? "",
    risk_level: stringValue(policyDefaults?.approval_policy?.min_risk_for_human) || "medium",
    role: typeOption.default_role || typeOption.type
};
}

function applyBlankCustomDefaults(current: WizardDraft): WizardDraft {
  return {
    ...current,
    creation_mode: "blank_custom",
    capability_bindings: {},
    capability_binding_draft: {
      mcp_servers: [],
      skills: []
},
    description: "",
    employee_type: BLANK_CUSTOM_EMPLOYEE_TYPE,
    persona_memory_markdown: "",
    risk_level: "medium",
    role: ""
};
}

// recommendedCapabilitySelection preselects the template's recommended
// skills/MCP servers, limited to keys that are actually available in the
// tenant registry — template defaults are no longer merged server-side.
function recommendedCapabilitySelection(
  typeOption: DigitalEmployeeTypeOption,
  options: DigitalEmployeeCreateOptions | undefined,
): WizardDraft["capability_binding_draft"] {
  const availableSkills = availableOptionKeys(options?.capability_options.skills);
  const availableMCPServers = availableOptionKeys(options?.capability_options.mcp_servers);
  const recommendedSkills = uniqueStringList([
    ...(typeOption.recommended_skills ?? []),
    ...stringList(typeOption.capability_bindings?.skills),
  ]);
  const recommendedMCPServers = uniqueStringList([
    ...(typeOption.recommended_mcp_servers ?? []),
    ...stringList(typeOption.capability_bindings?.mcp_servers),
  ]);
  return {
    mcp_servers: recommendedMCPServers.filter((key) => availableMCPServers.has(key)),
    skills: recommendedSkills.filter((key) => availableSkills.has(key))
};
}

function availableOptionKeys(items: DigitalEmployeeCapabilityOptionItem[] | undefined): Set<string> {
  return new Set((items ?? []).filter((item) => item.available).map((item) => item.key));
}

function inheritedCapabilityBindings(options: DigitalEmployeeCreateOptions | undefined): WizardDraft["capability_binding_draft"] {
  const teamConfig = options?.team_config as Record<string, unknown> | undefined;

  return {
    mcp_servers: uniqueStringList(teamConfig?.mcp_servers),
    skills: uniqueStringList(teamConfig?.skills)
};
}

function employeeExtensionCapabilityBindings(
  selection: WizardDraft["capability_binding_draft"],
  options: DigitalEmployeeCreateOptions | undefined,
): WizardDraft["capability_binding_draft"] {
  const inherited = inheritedCapabilityBindings(options);

  return {
    mcp_servers: withoutValues(selection.mcp_servers, inherited.mcp_servers),
    skills: withoutValues(selection.skills, inherited.skills)
};
}

// capabilitySelectionFromDraft is the top-level skills/mcp_servers payload:
// the employee's own logical-binding selections, with team-inherited keys
// removed. capability_bindings no longer carries skills/mcp_servers — the
// server strips those keys and persists bindings in the binding tables.
function capabilitySelectionFromDraft(
  draft: WizardDraft,
  options: DigitalEmployeeCreateOptions | undefined,
): { skills: string[]; mcp_servers: string[] } {
  const extension = employeeExtensionCapabilityBindings(draft.capability_binding_draft, options);
  return {
    skills: uniqueStringList(extension.skills),
    mcp_servers: uniqueStringList(extension.mcp_servers)
};
}

function capabilityBindingsFromDraft(draft: WizardDraft): Record<string, unknown> {
  return capabilityBindingDefaults(draft.capability_bindings);
}

function capabilityBindingDefaults(value: Record<string, unknown> | undefined): Record<string, unknown> {
  const source = structuredCloneSafe(value ?? {});
  const bindings: Record<string, unknown> = {};

  for (const key of ["external_capabilities", "environment_variable_refs"]) {
    if (Object.prototype.hasOwnProperty.call(source, key)) {
      bindings[key] = source[key];
    }
  }

  return bindings;
}

function budgetPolicyValue(value: Record<string, unknown>) {
  const rawValue = value.daily_token_limit;
  if (typeof rawValue === "number" && Number.isInteger(rawValue) && rawValue > 0) {
    return String(rawValue);
  }
  if (typeof rawValue === "string") {
    const trimmed = rawValue.trim();
    if (trimmed) {
      return trimmed;
    }
  }
  return "";
}

function structuredCloneSafe<T>(value: T): T {
  return JSON.parse(JSON.stringify(value ?? {})) as T;
}

function uniqueStringList(value: unknown): string[] {
  return Array.from(new Set(stringList(value)));
}

function withoutValues(values: string[], excludedValues: string[]) {
  if (excludedValues.length === 0) return values;
  const excluded = new Set(excludedValues);
  return values.filter((value) => !excluded.has(value));
}

function withoutInheritedItems(
  items: DigitalEmployeeCapabilityOptionItem[],
  inheritedKeys: string[],
) {
  if (inheritedKeys.length === 0) return items;
  const excluded = new Set(inheritedKeys);
  return items.filter((item) => !excluded.has(item.key));
}

function parseDailyTokenLimit(rawValue: string) {
  const trimmed = rawValue.trim();
  if (!trimmed) return { value: undefined };
  const parsed = Number(trimmed);
  if (!Number.isInteger(parsed) || parsed <= 0) {
    return { error: "每日 Token 预算上限必须是正整数" };
  }
  return { value: parsed };
}

// formatTemplateBindingsSummary 汇总模板带出的、本步不可编辑的逻辑绑定
// （外部能力/环境变量引用）；两者皆空时返回空串不占版面。
// 技能/MCP 计数不在此重复——右侧"画像摘要"实时展示。
function formatTemplateBindingsSummary(draft: WizardDraft) {
  const bindings = capabilityBindingsFromDraft(draft);
  const parts: string[] = [];
  const externalCount = stringList(bindings.external_capabilities).length;
  const envRefCount = stringList(bindings.environment_variable_refs).length;
  if (externalCount > 0) parts.push(`外部能力 ${externalCount}`);
  if (envRefCount > 0) parts.push(`环境变量引用 ${envRefCount}`);
  return parts.join(" · ");
}

function validateStep(step: StepName, draft: WizardDraft): ValidationErrors {
  if (step === "身份") {
    const errors: ValidationErrors = {};
    if (!draft.avatar_asset_id.trim()) errors.avatar_asset_id = "头像不能为空";
    if (!draft.employee_type.trim()) errors.employee_type = "员工类型不能为空";
    if (!draft.name.trim()) errors.name = "名称不能为空";
    if (!draft.role.trim()) errors.role = "职责定位不能为空";
    return errors;
  }
  if (step === "能力") {
    const budget = parseDailyTokenLimit(draft.daily_token_limit);
    return budget.error ? { daily_token_limit: budget.error } : {};
  }
  if (step === "Provider 类型" && !draft.provider_type) {
    return { runtime: "请选择 Provider 类型" };
  }
  return {};
}

function validateDraftForCreate(draft: WizardDraft): ValidationErrors {
  return {
    ...validateStep("身份", draft),
    ...validateStep("能力", draft),
    ...validateStep("Provider 类型", draft)
};
}

function budgetPolicyFromDraft(draft: WizardDraft) {
  const budget = parseDailyTokenLimit(draft.daily_token_limit);
  if (budget.value === undefined) return {};
  return budget.error ? {} : { daily_token_limit: budget.value };
}

function providerCandidates(options: DigitalEmployeeCreateOptions | undefined) {
  const configuredValues = [...(options?.capability_options.provider_types ?? [])]
    .map(normalizeProviderValue)
    .filter((value) => value && canonicalProviderTypes.includes(value as (typeof canonicalProviderTypes)[number]));
  const runtimeValues =
    options?.runtime_provider_options
      .map((option) => normalizeProviderValue(option.provider_type))
      .filter(
        (value) =>
          value &&
          canonicalProviderTypes.includes(value as (typeof canonicalProviderTypes)[number]),
      ) ?? [];
  const values = [...configuredValues, ...runtimeValues];
  return Array.from(new Set(values)).sort(
    (left, right) => canonicalProviderTypes.indexOf(left as any) - canonicalProviderTypes.indexOf(right as any),
  );
}

function providerDispatchPreview(options: DigitalEmployeeCreateOptions | undefined, providerType: string) {
  const matchingOptions = (options?.runtime_provider_options ?? []).filter(
    (option) => normalizeProviderValue(option.provider_type) === providerType,
  );
  const inactiveSessionCount = matchingOptions.filter(
    (option) => !option.available && option.disabled_reason === "runtime_session_inactive",
  ).length;
  const onlineHealthyCount = matchingOptions.filter(
    (option) =>
      option.runtime_status === "online" &&
      option.provider_status === "healthy" &&
      option.health_status === "healthy",
  ).length;
  return {
    matchingCount: matchingOptions.length,
    availableCount: matchingOptions.filter((option) => option.available).length,
    inactiveSessionCount,
    onlineHealthyCount
};
}

function providerDispatchPreviewText(preview: ReturnType<typeof providerDispatchPreview>) {
  if (preview.availableCount > 0) {
    if (preview.availableCount === preview.matchingCount) {
      return `${preview.matchingCount} 个 Runtime 节点候选会在项目运行准备中评估`;
    }
    return `${preview.availableCount}/${preview.matchingCount} 个 Runtime 节点当前可用于调度，仅用于项目运行准备参考`;
  }
  if (preview.inactiveSessionCount > 0) {
    return `${preview.inactiveSessionCount} 个 Runtime 节点已上报该 Provider，但当前会话未激活。`;
  }
  if (preview.onlineHealthyCount > 0) {
    return `${preview.onlineHealthyCount} 个 Runtime 节点已上报该 Provider，但当前不可用于调度。`;
  }
  return "当前没有在线 Runtime 节点支持该 Provider；创建时仍会记录必选 Provider 类型";
}

function newEnvironmentVariableRow(): EnvironmentVariableDraftRow {
  return {
    id: globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random()}`,
    name: "",
    value: "",
    sensitive: true
};
}

const riskTone: Record<string, Tone> = {
  critical: "danger",
  high: "danger",
  medium: "warn",
  low: "ok"
};

const riskLabels: Record<string, string> = {
  low: "低",
  medium: "中",
  high: "高",
  critical: "严重"
};

const canonicalProviderTypes = ["codex", "opencode", "claude-code"] as const;

function riskLabel(value: string) {
  return riskLabels[value] ?? value;
}

function normalizeProviderValue(value: string) {
  const normalized = value.trim().toLowerCase();
  return normalized === "claude_code" ? "claude-code" : normalized;
}

function RiskPill({ risk }: { risk: string }) {
  return <StatusPill tone={riskTone[risk] ?? "mute"}>{riskLabel(risk)}</StatusPill>;
}

function checkTone(status: string): Tone {
  if (status === "passed") return "ok";
  if (status === "warning") return "warn";
  return "danger";
}

function checkDotClassName(status: string) {
  if (status === "passed") return "bg-ok";
  if (status === "warning") return "bg-warn";
  return "bg-danger";
}

function checkStatusLabel(status: string) {
  if (status === "passed") return "通过";
  if (status === "warning") return "提醒";
  return "阻断";
}

// getErrorMessage 统一走 apiErrorMessage：结构化 coded error 展示后端权威中文，
// 未结构化时用调用方给定的中文兜底，绝不透传英文错误壳（如 "request failed
// with status 500"）。加载类横幅与创建横幅共用此出口。
function getErrorMessage(error: unknown, fallback = "请求失败，请稍后重试。") {
  return apiErrorMessage(error, fallback);
}

// 后端未返回结构化 code 时的兜底文案；有 code 时用后端权威中文 message（apiErrorMessage）。
const CREATE_EMPLOYEE_FALLBACK_MESSAGE = "创建失败，请稍后重试；若持续出现请联系管理员。";

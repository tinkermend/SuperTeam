import type { ReactNode } from "react";
import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useNavigate, useSearch } from "@tanstack/react-router";
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
  Trash2,
} from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Textarea } from "@/components/ui/textarea";
import {
  GlassCard,
  IconTile,
  SoftCard,
  StatusPill,
  V3Button,
  WorkSurface,
  type V3Tone,
} from "@/components/superteam";
import { Main } from "@/components/layout/main";
import {
  ShellPageHeader,
  ShellPageHeaderBack,
} from "@/components/layout/shell-page-header";
import type {
  DigitalEmployeeAvatarAsset,
  DigitalEmployeeCreateOptions,
  DigitalEmployeeTypeOption,
} from "@/lib/api/employees";
import {
  createDigitalEmployee,
  getDigitalEmployeeCreateOptions,
  listDigitalEmployeeAvatarAssets,
} from "@/lib/api/employees";
import { listTeams } from "@/lib/api/teams";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { cn } from "@/lib/utils";
import {
  findTemplateByType,
  firstPreferredEmployeeType,
  orderedEmployeeTypes,
  riskSortValue,
  stringList,
  stringValue,
  templateCapabilityPreview,
  templateCapabilitySummary,
  templateDefaultInjectionLine,
  templateRisk,
  templateSearchText,
} from "./template-utils";

const BLANK_CUSTOM_EMPLOYEE_TYPE = "custom_agent";
const BLANK_CUSTOM_TITLE = "自定义身份";

const configSteps = ["身份", "能力", "治理", "Provider 类型"] as const;
type StepName = (typeof configSteps)[number];
type CreateFlowStep = "template" | "configure" | "confirm";
type CreationMode = "template" | "blank_custom";

type WizardDraft = {
  creation_mode: CreationMode;
  capability_selection: {
    enabled_external_capabilities: string[];
    enabled_mcp_servers: string[];
    enabled_skills: string[];
  };
  context_policy_override: Record<string, unknown>;
  daily_token_limit: string;
  approval_policy_override: Record<string, unknown>;
  description: string;
  employee_type: string;
  avatar_asset_id: string;
  name: string;
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
  approval_policy_override: {},
  creation_mode: "template",
  capability_selection: {
    enabled_external_capabilities: [],
    enabled_mcp_servers: [],
    enabled_skills: [],
  },
  context_policy_override: {},
  daily_token_limit: "",
  description: "",
  employee_type: "",
  avatar_asset_id: "",
  name: "",
  provider_type: "",
  risk_level: "medium",
  role: "",
  runtime_binding: "",
  runtime_node_id: "",
  team_id: "",
  environment_variables: [],
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
    queryFn: () => listTeams({ baseUrl: apiBaseUrl, fetcher }),
  });

  const createOptions = useQuery({
    enabled: !teams.isLoading,
    queryKey: ["digital-employee-create-options", draft.team_id || "team-less"],
    queryFn: () => getDigitalEmployeeCreateOptions({ baseUrl: apiBaseUrl, fetcher }, draft.team_id || undefined),
  });

  const avatarAssets = useQuery({
    queryKey: ["digital-employee-avatar-assets"],
    queryFn: () => listDigitalEmployeeAvatarAssets({ baseUrl: apiBaseUrl, fetcher }),
  });

  const selectedType = useMemo(
    () => createOptions.data?.employee_types.find((item) => item.type === draft.employee_type),
    [createOptions.data?.employee_types, draft.employee_type],
  );
  const blankCustom = draft.creation_mode === "blank_custom";

  useEffect(() => {
    const optionsData = createOptions.data;
    const employeeTypes = optionsData?.employee_types ?? [];
    const firstType = firstPreferredEmployeeType(employeeTypes);
    if (!firstType) return;
    setDraft((current) => {
      if (current.creation_mode !== "template") return current;
      if (!current.employee_type || !employeeTypes.some((item) => item.type === current.employee_type)) {
        return applyTypeDefaults(current, firstType);
      }
      return current;
    });
  }, [createOptions.data, draft.creation_mode, draft.employee_type]);

  useEffect(() => {
    const optionsData = createOptions.data;
    if (!requestedTemplate || !optionsData || templateQueryHandled === requestedTemplate) return;
    const requestedType = findTemplateByType(optionsData, requestedTemplate);
    setTemplateQueryHandled(requestedTemplate);
    if (!requestedType) return;
    setDraft((current) => (current.creation_mode === "template" ? applyTypeDefaults(current, requestedType) : current));
  }, [createOptions.data, requestedTemplate, templateQueryHandled]);

  useEffect(() => {
    const firstAvatar = avatarAssets.data?.find((asset) => asset.status === "active");
    if (!draft.avatar_asset_id && firstAvatar) {
      setDraft((current) => ({ ...current, avatar_asset_id: firstAvatar.id }));
    }
  }, [avatarAssets.data, draft.avatar_asset_id]);

  useEffect(() => {
    const candidateProviders = providerCandidates(createOptions.data);
    setDraft((current) => {
      if (candidateProviders.length === 1 && current.provider_type !== candidateProviders[0]) {
        return {
          ...current,
          runtime_binding: "",
          runtime_node_id: "",
          provider_type: candidateProviders[0],
        };
      }
      if (current.provider_type && !candidateProviders.includes(current.provider_type)) {
        return { ...current, provider_type: "", runtime_binding: "", runtime_node_id: "" };
      }
      return current;
    });
  }, [createOptions.data]);

  const createEmployee = useMutation({
    mutationFn: () => {
      if (!draft.provider_type) {
        throw new Error("请选择 Provider 类型");
      }

      return createDigitalEmployee(
        { baseUrl: apiBaseUrl, fetcher },
        {
          team_id: draft.team_id || undefined,
          employee_type: draft.employee_type,
          name: draft.name.trim(),
          avatar_asset_id: draft.avatar_asset_id,
          role: draft.role.trim(),
          description: draft.description.trim() || undefined,
          risk_level: draft.risk_level,
          role_profile: {
            employee_type: draft.employee_type,
            role: draft.role.trim(),
            title: selectedType?.label ?? draft.employee_type,
          },
          ...(blankCustom ? { metadata: { creation_mode: "blank_custom" } } : {}),
          capability_selection: draft.capability_selection,
          context_policy_override: draft.context_policy_override,
          approval_policy_override: draft.approval_policy_override,
          budget_policy: budgetPolicyFromDraft(draft),
          output_contract_addendum: {},
          provider_type: draft.provider_type,
          session_policy: { mode: "reuse_latest" },
          workspace_policy: {},
          environment_variables: draft.environment_variables
            .filter((row) => row.name.trim() && row.value)
            .map((row) => ({ name: row.name.trim(), value: row.value, sensitive: row.sensitive })),
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

  const currentStep = configSteps[stepIndex];
  const teamOptions = useMemo(() => (teams.data ?? []).filter((team) => team.status === "active"), [teams.data]);
  const selectedTeam = teamOptions.find((team) => team.id === draft.team_id);

  function updateDraft(patch: Partial<WizardDraft>) {
    if (flowStep === "configure") {
      setDraftTouched(true);
    }
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
    setDraft((current) => applyTypeDefaults(current, nextType));
  }

  function selectProvider(providerType: string) {
    updateDraft({
      provider_type: providerType,
      runtime_binding: "",
      runtime_node_id: "",
    });
    setErrors((current) => ({ ...current, runtime: undefined }));
  }

  function nextStep() {
    const nextErrors = validateStep(currentStep, draft);
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length === 0) {
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
      setStepIndex(2);
    } else if (nextErrors.runtime) {
      setStepIndex(3);
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
    if (draftTouched && !window.confirm("更换团队会重置当前配置草稿，是否继续？")) {
      return;
    }
    resetDraftForTeam(nextTeamId, draft.creation_mode);
    if (draft.creation_mode === "blank_custom") {
      setFlowStep("configure");
    }
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
          ? "先选择模板，再分步完成预检、配置和确认。"
          : "按职责定位、能力选择、治理策略和必选 Provider 类型完成员工画像。"
        }
        actions={
          flowStep !== "template" ? (
            <V3Button onClick={requestTemplateChange} type="button" variant="outline">
              <ArrowLeft className="size-4" />
              返回
            </V3Button>
          ) : undefined
        }
      />
      <Main>
        {teams.isError ? (
          <Alert className="mb-4" variant="destructive">
            <AlertTitle>团队列表加载失败</AlertTitle>
            <AlertDescription>{getErrorMessage(teams.error)}</AlertDescription>
          </Alert>
        ) : null}
        {!teams.isLoading && !teams.isError && teamOptions.length === 0 ? (
          <Alert className="mb-4">
            <AlertTitle>暂无可用团队</AlertTitle>
            <AlertDescription>可将归属团队选择为“无”，创建租户级独立数字员工；治理按内置默认（全部允许）。</AlertDescription>
          </Alert>
        ) : null}
        {createOptions.isError ? (
          <Alert className="mb-4" variant="destructive">
            <AlertTitle>创建选项加载失败</AlertTitle>
            <AlertDescription>{getErrorMessage(createOptions.error)}</AlertDescription>
          </Alert>
        ) : null}
        {avatarAssets.isError ? (
          <Alert className="mb-4" variant="destructive">
            <AlertTitle>头像库加载失败</AlertTitle>
            <AlertDescription>{getErrorMessage(avatarAssets.error)}</AlertDescription>
          </Alert>
        ) : null}

        <CreationStageProgress flowStep={flowStep} />

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
          <div className="grid gap-4 xl:h-[calc(100vh-220px)] xl:min-h-[560px] xl:grid-cols-[minmax(0,1fr)_340px]">
            <section className="flex min-w-0 flex-col overflow-hidden rounded-v3-card border border-v3-line bg-v3-card shadow-v3">
              <div className="border-b border-v3-line p-4">
                <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                  <div className="flex items-center gap-2.5">
                    <IconTile tone="brand" size="sm">
                        <Bot />
                      </IconTile>
                      <div>
                        <h2 className="text-lg font-semibold text-v3-ink">员工画像蓝图</h2>
                        <p className="mt-0.5 text-sm text-v3-ink-3">
                        按职责定位、可用能力、治理边界和 Provider 类型完成员工画像。
                      </p>
                    </div>
                  </div>
                  <StepTabs currentStep={currentStep} />
                </div>
              </div>

              <div className="grid min-h-0 flex-1 gap-4 overflow-y-auto p-4">
                <div className="min-h-0 rounded-[14px] border border-v3-line bg-v3-card-inner p-4">
                  {teams.isLoading || avatarAssets.isLoading || createOptions.isLoading ? (
                    <div className="flex min-h-[360px] items-center justify-center gap-2 text-sm text-v3-ink-3">
                      <Loader2 className="size-4 animate-spin" />
                      加载创建选项
                    </div>
                  ) : null}
                  {!teams.isLoading && !avatarAssets.isLoading && !createOptions.isLoading && currentStep === "身份" ? (
                    <IdentityStep
                      avatarAssets={avatarAssets.data ?? []}
                      draft={draft}
                      errors={errors}
                      options={createOptions.data}
                      selectedType={selectedType}
                      teamOptions={teamOptions}
                      onSelectAvatar={(avatarAssetId) => updateDraft({ avatar_asset_id: avatarAssetId })}
                      onSelectTeam={requestTeamChange}
                      onUpdate={updateDraft}
                    />
                  ) : null}
                  {!teams.isLoading && !createOptions.isLoading && currentStep === "能力" ? (
                    <CapabilityStep draft={draft} options={createOptions.data} onUpdate={updateDraft} />
                  ) : null}
                  {!teams.isLoading && !createOptions.isLoading && currentStep === "治理" ? (
                    <GovernanceStep
                      draft={draft}
                      errors={errors}
                      options={createOptions.data}
                      selectedType={selectedType}
                      onUpdate={updateDraft}
                    />
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
                <p className="px-4 text-sm text-v3-danger">{getErrorMessage(createEmployee.error)}</p>
              ) : null}
              <div
                className="sticky bottom-0 z-10 flex justify-between gap-3 border-t border-v3-line bg-v3-card p-4 shadow-[0_-12px_24px_rgba(15,23,42,0.06)]"
                data-testid="employee-configure-actions"
              >
                <V3Button
                  disabled={stepIndex === 0 || createEmployee.isPending}
                  onClick={() => setStepIndex((current) => Math.max(current - 1, 0))}
                  type="button"
                  variant="outline"
                >
                  <ChevronLeft className="size-4" />
                  上一步
                </V3Button>
                {stepIndex < configSteps.length - 1 ? (
                  <V3Button
                    disabled={
                      createOptions.isLoading ||
                      createOptions.isError ||
                      avatarAssets.isLoading ||
                      avatarAssets.isError
                    }
                    onClick={nextStep}
                    type="button"
                  >
                    下一步
                    <ChevronRight className="size-4" />
                  </V3Button>
                ) : (
                  <V3Button
                    disabled={
                      createEmployee.isPending ||
                      createOptions.isLoading ||
                      createOptions.isError ||
                      avatarAssets.isLoading ||
                      avatarAssets.isError ||
                      !draft.avatar_asset_id ||
                      !draft.provider_type
                    }
                    onClick={enterConfirmCreation}
                    type="button"
                  >
                    进入确认创建
                    <ChevronRight className="size-4" />
                  </V3Button>
                )}
              </div>
            </section>

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

function CreationStageProgress({ flowStep }: { flowStep: CreateFlowStep }) {
  const stages = [
    { key: "template", title: "创建方式", description: "选择模板或自定义身份" },
    { key: "configure", title: "完成配置", description: "补齐身份、能力、治理和 Provider" },
    { key: "confirm", title: "确认创建", description: "核对本次创建明细" },
  ];
  const activeIndex = stages.findIndex((stage) => stage.key === flowStep);
  const normalizedActiveIndex = activeIndex === -1 ? 2 : activeIndex;

  return (
    <SoftCard className="mb-4 px-4 py-3.5">
      <div className="grid gap-3 md:grid-cols-3">
        {stages.map((stage, index) => {
          const active = index === normalizedActiveIndex;
          const done = index < normalizedActiveIndex;

          return (
            <div className="flex items-center gap-3" key={stage.title}>
              <span
                className={cn(
                  "flex size-8 shrink-0 items-center justify-center rounded-[11px] text-[13px] font-bold tabular-nums transition-colors",
                  active ? "bg-v3-brand text-white shadow-v3" : "",
                  done ? "bg-v3-brand-soft text-v3-brand-deep" : "",
                  !active && !done ? "bg-v3-card-soft text-v3-ink-3" : "",
                )}
              >
                {done ? <Check className="size-4" /> : index + 1}
              </span>
              <span className="min-w-0">
                <span className={cn("block text-sm font-semibold", active ? "text-v3-brand-deep" : "text-v3-ink")}>
                  {stage.title}
                </span>
                <span className="block truncate text-xs text-v3-ink-3">{stage.description}</span>
              </span>
              {index < stages.length - 1 ? (
                <span
                  aria-hidden
                  className={cn(
                    "ml-auto hidden h-px w-6 shrink-0 md:block",
                    done ? "bg-v3-brand/40" : "bg-v3-line",
                  )}
                />
              ) : null}
            </div>
          );
        })}
      </div>
    </SoftCard>
  );
}

function CreationPathPanel({
  creationMode,
  onSelectMode,
}: {
  creationMode: CreationMode;
  onSelectMode: (mode: CreationMode) => void;
}) {
  const paths = [
    {
      title: "从专业模板创建",
      description: "按职责模板带出默认角色、能力建议和治理策略。",
      icon: Sparkles,
      mode: "template" as const,
      badge: "推荐",
      disabled: false,
    },
    {
      title: "空白自定义",
      description: "直接定义自定义身份，逐项手动配置职责定位、能力和 Provider 类型。",
      icon: FileText,
      mode: "blank_custom" as const,
      badge: "可用",
      disabled: false,
    },
    {
      title: "从团队角色复制",
      description: "复用团队内已验证的角色画像和能力配置。",
      icon: ClipboardCheck,
      mode: undefined,
      badge: "暂未开放",
      disabled: true,
    },
    {
      title: "从历史员工克隆",
      description: "基于已有员工配置生成新草稿，保留审计来源。",
      icon: GitBranch,
      mode: undefined,
      badge: "暂未开放",
      disabled: true,
    },
  ];

  return (
    <GlassCard className="flex flex-col p-3.5">
      <div className="mb-3.5 flex items-center gap-2.5 px-1">
        <IconTile tone="brand" size="sm">
          <Sparkles />
        </IconTile>
        <div>
          <h2 className="text-base font-semibold text-v3-ink">创建路径</h2>
          <p className="text-xs text-v3-ink-3">先选入口，再进入配置。</p>
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
                  ? "border-v3-brand/40 bg-v3-brand-soft shadow-v3"
                  : "border-v3-line bg-v3-card/70",
                path.disabled
                  ? "cursor-not-allowed opacity-60"
                  : "hover:-translate-y-0.5 hover:border-v3-brand/30 hover:shadow-md",
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
                    <span className={cn("text-sm font-semibold", active ? "text-v3-brand-deep" : "text-v3-ink")}>
                      {path.title}
                    </span>
                    <Badge variant={active ? "default" : "secondary"}>{path.badge}</Badge>
                  </span>
                  <span className="mt-1 block text-xs leading-5 text-v3-ink-3">{path.description}</span>
                </span>
              </span>
            </button>
          );
        })}
      </div>
      <div className="mt-3.5 flex items-start gap-2 rounded-[14px] border border-v3-info/20 bg-v3-info-soft p-3 text-xs leading-5 text-v3-ink">
        <ShieldCheck className="mt-0.5 size-4 shrink-0 text-v3-info" />
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
  onSelectType,
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
    <section className="@container/template flex min-w-0 flex-col overflow-hidden rounded-v3-card border border-v3-line bg-v3-card shadow-v3">
      <div className="border-b border-v3-line p-4">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex items-center gap-2.5">
            <IconTile tone="brand" size="sm">
              <Sparkles />
            </IconTile>
            <div>
              <h2 className="text-base font-semibold text-v3-ink">选择内置模板</h2>
              <p className="mt-0.5 text-sm text-v3-ink-3">模板只负责带出默认角色、模板能力和治理默认值。</p>
            </div>
          </div>
          <Badge variant="secondary">
            {employeeTypes.length} 个模板 · 已筛选 {filteredEmployeeTypes.length}
          </Badge>
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
        <div className="m-4 flex min-h-[420px] flex-1 items-center justify-center rounded-[14px] border border-v3-line bg-v3-card-soft p-6 text-sm text-v3-ink-3">
          当前团队治理配置未返回可用专业模板。
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
                          "sticky top-0 z-10 border-b border-v3-line-strong bg-v3-card-soft px-3 py-2.5 text-left text-[11px] font-bold uppercase tracking-wide text-v3-ink-3",
                          w,
                        )}
                      >
                        {label}
                      </th>
                    ))}
                    <th className="sticky top-0 z-10 w-[100px] border-b border-v3-line-strong bg-v3-card-soft px-3 py-2.5 text-right text-[11px] font-bold uppercase tracking-wide text-v3-ink-3">
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
                <div className="flex min-h-[220px] items-center justify-center border-t border-v3-line bg-v3-card-soft p-6 text-sm text-v3-ink-3">
                  没有匹配当前筛选条件的专业模板。
                </div>
              ) : null}
            </div>
          </WorkSurface>
        </div>
      )}
      <div className="border-t border-v3-line px-4 py-3.5">
        <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2 text-sm">
              <span className="font-semibold text-v3-ink">已选模板摘要</span>
              <Badge variant="secondary">团队 {selectedTeamName || "无（租户级）"}</Badge>
              <Badge variant="secondary">模板 {selectedType?.label ?? (draft.employee_type || "未选择")}</Badge>
              <Badge variant="secondary">默认角色 {draft.role || selectedType?.default_role || "未生成"}</Badge>
              <Badge variant="secondary">风险 {riskLabel(draft.risk_level || "medium")}</Badge>
            </div>
            <p className="mt-2 text-sm text-v3-ink-3">
              没有合适的模板？
              <button className="ml-2 cursor-not-allowed font-medium text-v3-ink-3" disabled type="button">
                选择空白自定义（暂未开放）
              </button>
            </p>
          </div>
          <V3Button disabled={!draft.employee_type} onClick={onEnterConfiguration} type="button">
            进入完成配置
            <ChevronRight className="size-4" />
          </V3Button>
        </div>
      </div>
      {selectedType ? <span className="sr-only">当前选择：{selectedType.label}</span> : null}
    </section>
  );
}

function TemplateTableRow({
  selected,
  typeOption,
  onSelect,
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
        "transition-colors [&>td]:border-b [&>td]:border-v3-line [&:last-child>td]:border-b-0 [&:hover>td]:bg-v3-card-inner",
        selected ? "[&>td]:bg-v3-brand-soft [&>td:first-child]:shadow-[inset_3px_0_0_var(--v3-brand)]" : "",
      )}
    >
      <td className="px-3 py-3 align-top">
        <div className="flex min-w-0 gap-3">
          <IconTile tone={selected ? "brand" : "mute"} size="sm">
            <Code2 />
          </IconTile>
          <div className="min-w-0">
            <div className="font-semibold text-v3-ink">{typeOption.label}</div>
            <div className="mt-1 line-clamp-2 text-xs leading-5 text-v3-ink-3">{typeOption.description}</div>
          </div>
        </div>
      </td>
      <td className="px-3 py-3 align-top">
        <span className="block max-w-[180px] truncate font-mono text-xs text-v3-ink-2">
          {typeOption.default_role || typeOption.type}
        </span>
      </td>
      <td className="px-3 py-3 align-top">
        <div className="grid gap-1.5">
          <div className="flex flex-wrap gap-1.5">
            <Badge variant="secondary">{`技能 ${capability.skills.length}`}</Badge>
            <Badge variant="secondary">{`MCP ${capability.mcpServers.length}`}</Badge>
            <Badge variant="secondary">{`Provider ${capability.providerTypes.length}`}</Badge>
          </div>
          <div className="truncate text-xs text-v3-ink-3">{templateCapabilityPreview(typeOption)}</div>
        </div>
      </td>
      <td className="px-3 py-3 align-top">
        <RiskPill risk={risk} />
      </td>
      <td className="px-3 py-3 align-top">
        <span className="line-clamp-2 text-xs leading-5 text-v3-ink-3">
          {templateDefaultInjectionLine(typeOption)}
        </span>
      </td>
      <td className="px-3 py-3 text-right align-top">
        <V3Button
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
        </V3Button>
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
    <div className="flex items-center justify-between gap-3 rounded-[12px] border border-v3-line bg-v3-card px-3 py-2">
      <span className="text-v3-ink-3">{label}</span>
      <span className="max-w-[180px] truncate font-medium text-v3-ink">{value}</span>
    </div>
  );
}

function StepTabs({ currentStep }: { currentStep: StepName }) {
  const currentIndex = configSteps.indexOf(currentStep);

  return (
    <div className="flex flex-wrap gap-1 rounded-[14px] bg-v3-card-soft p-1.5">
        {configSteps.map((step, index) => {
          const active = step === currentStep;
          const done = index < currentIndex;

          return (
            <div
              className={cn(
                "flex h-8 items-center gap-2 rounded-[10px] px-2.5 text-xs font-semibold transition-colors",
                active ? "bg-v3-brand-soft text-v3-brand-deep" : "text-v3-ink-3",
                done && !active ? "text-v3-ink-2" : "",
              )}
              key={step}
            >
              <span
                className={cn(
                  "flex size-5 items-center justify-center rounded-full text-[11px] tabular-nums",
                  active ? "bg-v3-brand text-white" : "",
                  done && !active ? "bg-v3-brand-soft text-v3-brand-deep" : "",
                  !active && !done ? "bg-v3-card text-v3-ink-3" : "",
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
  selectedType,
}: {
  draft: WizardDraft;
  options?: DigitalEmployeeCreateOptions;
  selectedType?: DigitalEmployeeTypeOption;
}) {
  const checks = displayablePreflightChecks(options?.creation_checks ?? []);
  const providers = providerCandidates(options);

  return (
    <aside className="grid content-start gap-4">
      <GlassCard className="p-4">
        <div className="mb-3 flex items-center gap-2">
          <IconTile tone="ok" size="sm">
            <ShieldCheck />
          </IconTile>
          <div>
            <h2 className="text-base font-semibold text-v3-ink">预检项目</h2>
            <p className="text-xs text-v3-ink-3">来自 Control Plane 创建候选接口。</p>
          </div>
        </div>
        <div className="grid gap-2">
          {checks.length === 0 ? (
            <p className="rounded-[12px] border border-v3-line bg-v3-card-soft p-3 text-sm text-v3-ink-3">等待创建候选加载。</p>
          ) : (
            checks.map((check) => (
              <div
                className="flex items-start gap-2 rounded-[12px] border border-v3-line bg-v3-card-inner p-3"
                key={check.key}
              >
                <span className={cn("mt-1 size-2 shrink-0 rounded-full", checkDotClassName(check.status))} />
                <span className="min-w-0 flex-1">
                  <span className="flex items-center justify-between gap-2">
                    <span className="text-sm font-medium text-v3-ink">{check.label}</span>
                    <StatusPill tone={checkTone(check.status)}>{checkStatusLabel(check.status)}</StatusPill>
                  </span>
                  <span className="mt-1 block text-xs leading-5 text-v3-ink-3">{check.message}</span>
                </span>
              </div>
            ))
          )}
        </div>
      </GlassCard>

      <GlassCard className="p-4">
        <div className="mb-3 flex items-center gap-2">
          <IconTile tone="artifact" size="sm">
            <Gauge />
          </IconTile>
          <div>
            <h2 className="text-base font-semibold text-v3-ink">画像摘要</h2>
            <p className="text-xs text-v3-ink-3">随配置实时更新。</p>
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
            value={`技能 ${draft.capability_selection.enabled_skills.length} · MCP ${draft.capability_selection.enabled_mcp_servers.length} · 外部 ${draft.capability_selection.enabled_external_capabilities.length}`}
          />
          <SummaryItem
            label="Provider 类型"
            value={draft.provider_type ? providerLabel(draft.provider_type) : `${providers.length} 个候选`}
          />
        </div>
      </GlassCard>

      <section className="rounded-[14px] border border-v3-line bg-v3-card-soft p-4 text-xs leading-5 text-v3-ink-3">
        <div className="mb-2 flex items-center gap-2 font-semibold text-v3-ink">
          <Cpu className="size-4 text-v3-brand" />
          创建后事实
        </div>
        <div className="grid gap-2">
          <div>1. 写入身份与初始配置修订</div>
          <div>2. 记录 Provider 类型</div>
          <div>3. Runtime 节点会在项目运行准备中决定，不在创建时绑定到员工。</div>
          <div>4. 进入 ready，等待任务调度</div>
        </div>
      </section>
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
  onSubmit,
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
      <div className="border-b border-v3-line p-4">
        <div className="flex items-center gap-2.5">
          <IconTile tone="brand" size="sm">
            <ClipboardCheck />
          </IconTile>
          <div>
            <h2 className="text-lg font-semibold text-v3-ink">确认创建</h2>
            <p className="mt-0.5 text-sm text-v3-ink-3">
              核对本次将提交给 Control Plane 的员工配置；确认后创建 ready 状态数字员工。
            </p>
          </div>
        </div>
      </div>
      <div className="grid gap-4 p-4 lg:grid-cols-2">
        <section className="rounded-[14px] border border-v3-line bg-v3-card-inner p-4">
          <h3 className="text-sm font-semibold text-v3-ink">身份与模板</h3>
          <div className="mt-3 grid gap-2 text-sm">
            <InlineSummary label="归属团队" value={selectedTeamName || "无（租户级）"} />
            <InlineSummary label="创建路径" value={draft.creation_mode === "blank_custom" ? BLANK_CUSTOM_TITLE : "专业模板"} />
            {draft.creation_mode !== "blank_custom" ? (
              <InlineSummary label="专业模板" value={selectedType?.label ?? (draft.employee_type || "未选择")} />
            ) : null}
            <InlineSummary label="名称" value={draft.name.trim() || "未填写"} />
            <InlineSummary label="职责定位" value={draft.role || "未填写"} />
            <InlineSummary label="风险等级" value={riskLabel(draft.risk_level || "medium")} />
          </div>
        </section>

        <section className="rounded-[14px] border border-v3-line bg-v3-card-inner p-4">
          <h3 className="text-sm font-semibold text-v3-ink">能力与 Provider 类型</h3>
          <div className="mt-3 grid gap-2 text-sm">
            <InlineSummary
              label="能力选择"
              value={`技能 ${draft.capability_selection.enabled_skills.length} · MCP ${draft.capability_selection.enabled_mcp_servers.length} · 外部 ${draft.capability_selection.enabled_external_capabilities.length}`}
            />
            <InlineSummary
              label="Provider 类型"
              value={draft.provider_type ? providerLabel(draft.provider_type) : "未选择"}
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
        <p className="px-4 pb-2 text-sm text-v3-danger">{getErrorMessage(createError)}</p>
      ) : null}
      <div className="flex justify-between gap-3 border-t border-v3-line p-4">
        <V3Button disabled={creating} onClick={onBack} type="button" variant="glass">
          <ChevronLeft className="size-4" />
          返回配置
        </V3Button>
        <V3Button disabled={creating} onClick={onSubmit} type="button">
          {creating ? <Loader2 className="size-4 animate-spin" /> : <Plus className="size-4" />}
          确认创建
        </V3Button>
      </div>
    </GlassCard>
  );
}

function IdentityStep({
  avatarAssets,
  draft,
  errors,
  options,
  selectedType,
  teamOptions,
  onSelectTeam,
  onSelectAvatar,
  onUpdate,
}: {
  avatarAssets: DigitalEmployeeAvatarAsset[];
  draft: WizardDraft;
  errors: ValidationErrors;
  options?: DigitalEmployeeCreateOptions;
  selectedType?: DigitalEmployeeTypeOption;
  teamOptions: Array<{ id: string; name: string }>;
  onSelectTeam: (value: string) => void;
  onSelectAvatar: (value: string) => void;
  onUpdate: (patch: Partial<WizardDraft>) => void;
}) {
  const isBlankCustom = draft.creation_mode === "blank_custom";

  return (
    <div className="flex flex-col gap-5">
      <div>
        <h2 className="text-lg font-semibold text-v3-ink">身份</h2>
        <p className="text-sm text-v3-ink-3">确定团队、业务类型和职责边界。负责人由后端按当前登录身份注入。</p>
      </div>
      <div className="grid gap-4 md:grid-cols-2">
        <Field label="归属团队" error={errors.team_id}>
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
          <p className="text-xs text-v3-ink-3">选择“无”将创建租户级独立数字员工，治理按内置默认（全部允许）。</p>
        </Field>
        <Field label="员工类型" error={errors.employee_type}>
          <select
            aria-invalid={Boolean(errors.employee_type)}
            className={selectClassName}
            disabled
            id="employee-type"
            value={draft.employee_type}
          >
            {isBlankCustom ? <option value={BLANK_CUSTOM_EMPLOYEE_TYPE}>{BLANK_CUSTOM_TITLE}</option> : null}
            {(options?.employee_types ?? []).map((item) => (
              <option key={item.type} value={item.type}>
                {item.label}
              </option>
            ))}
          </select>
          <p className="mt-1 text-xs text-v3-ink-3">
            {isBlankCustom
              ? "如需切换创建路径或员工类型，请使用右上角“返回”并重新生成配置草稿。"
              : "如需切换模板，请使用右上角“返回”并重新生成配置草稿。"}
          </p>
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
      <AvatarSelection
        assets={avatarAssets}
        error={errors.avatar_asset_id}
        selectedAssetId={draft.avatar_asset_id}
        onSelect={onSelectAvatar}
      />
      {selectedType ? (
        <div className="rounded-[14px] border border-v3-line bg-v3-card-soft p-3 text-sm">
          <div className="font-semibold text-v3-ink">{selectedType.label}</div>
          <div className="mt-1 text-v3-ink-3">{selectedType.description}</div>
          <div className="mt-2 text-v3-ink-3">默认职责定位：{selectedType.default_role || selectedType.type}</div>
        </div>
      ) : null}
    </div>
  );
}

function AvatarSelection({
  assets,
  error,
  onSelect,
  selectedAssetId,
}: {
  assets: DigitalEmployeeAvatarAsset[];
  error?: string;
  onSelect: (value: string) => void;
  selectedAssetId: string;
}) {
  return (
    <fieldset className="rounded-[14px] border border-v3-line p-3">
      <legend className="px-1 text-sm font-medium text-v3-ink">头像</legend>
      <div className="mt-3 flex flex-wrap gap-3">
        {assets.map((asset) => {
          const selected = asset.id === selectedAssetId;
          return (
            <button
              aria-pressed={selected}
              className={cn(
                "flex size-20 shrink-0 items-center justify-center rounded-full border bg-v3-card-soft p-0.5 transition-all duration-200",
                selected ? "border-v3-brand ring-2 ring-v3-brand/30" : "hover:-translate-y-0.5 hover:border-v3-brand/60",
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
      {assets.length === 0 ? <p className="mt-2 text-sm text-v3-ink-3">暂无可选头像</p> : null}
      {error ? <span className="mt-2 block text-sm text-v3-danger">{error}</span> : null}
    </fieldset>
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
        <h2 className="text-lg font-semibold text-v3-ink">能力</h2>
        <p className="text-sm text-v3-ink-3">按团队治理配置选择技能、MCP Server 和外部能力。</p>
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
    <fieldset className="rounded-[14px] border border-v3-line p-3">
      <legend className="px-1 text-sm font-medium text-v3-ink">{label}</legend>
      <div className="mt-3 grid gap-2.5 md:grid-cols-2">
        {values.map((value) => {
          const checked = checkedValues.includes(value);
          return (
            <label
              className={cn(
                "flex cursor-pointer items-center gap-2 rounded-[12px] border px-3 py-2 text-sm transition-colors",
                checked
                  ? "border-v3-brand/40 bg-v3-brand-soft text-v3-brand-deep"
                  : "border-v3-line bg-v3-card text-v3-ink hover:bg-v3-card-soft",
              )}
              key={value}
            >
              <Checkbox checked={checked} onCheckedChange={() => onToggle(value)} />
              <span className="min-w-0 truncate">{value}</span>
            </label>
          );
        })}
        {values.length === 0 ? <p className="text-sm text-v3-ink-3">暂无可选项</p> : null}
      </div>
    </fieldset>
  );
}

function GovernanceStep({
  draft,
  errors,
  options,
  selectedType,
  onUpdate,
}: {
  draft: WizardDraft;
  errors: ValidationErrors;
  options?: DigitalEmployeeCreateOptions;
  selectedType?: DigitalEmployeeTypeOption;
  onUpdate: (patch: Partial<WizardDraft>) => void;
}) {
  const teamConfig = options?.team_config;

  return (
    <div className="flex flex-col gap-5">
      <div>
        <h2 className="text-lg font-semibold text-v3-ink">治理</h2>
        <p className="text-sm text-v3-ink-3">确认团队治理版本、上下文和审批默认值。这里不暴露原始 JSON 编辑。</p>
      </div>
      <div className="grid gap-3 md:grid-cols-2">
        <SummaryItem label="团队治理版本" value={teamConfig ? `#${teamConfig.revision_number} · ${teamConfig.status}` : "未加载"} />
        <SummaryItem label="风险等级" value={riskLabel(draft.risk_level)} />
        <SummaryItem label="允许员工类型" value={`${teamConfig?.allowed_employee_types.length ?? 0} 项`} />
        <SummaryItem
          label="允许 Provider"
          value={
            (teamConfig?.allowed_provider_types ?? [])
              .map((value) => providerLabel(normalizeProviderValue(value)))
              .join(", ") || "暂无"
          }
        />
        <SummaryItem label="上下文策略" value={`覆盖项 ${Object.keys(draft.context_policy_override).length} 个`} />
        <SummaryItem label="审批策略" value={String(draft.approval_policy_override.min_risk_for_human ?? "按团队默认")} />
      </div>
      <Field label="每日 Token 预算上限" error={errors.daily_token_limit}>
        <Input
          aria-invalid={Boolean(errors.daily_token_limit)}
          id="daily-token-limit"
          inputMode="numeric"
          min={1}
          onChange={(event) => onUpdate({ daily_token_limit: event.target.value })}
          placeholder="不填写表示无预算上限"
          type="number"
          value={draft.daily_token_limit}
        />
        <p className="text-xs text-v3-ink-3">不填写表示无预算上限。填写后，达到当日上限会阻止发起新的运行。</p>
      </Field>
      <div className="rounded-[14px] border border-v3-line bg-v3-card-soft p-3">
        <div className="text-sm font-medium text-v3-ink">创建摘要</div>
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
    <div className="rounded-[12px] border border-v3-line bg-v3-card p-3">
      <div className="text-xs text-v3-ink-3">{label}</div>
      <div className="mt-1 text-sm font-medium text-v3-ink">{value}</div>
    </div>
  );
}

function ProviderStep({
  draft,
  error,
  options,
  onSelectProvider,
  onUpdate,
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
      ),
    });
  };
  const removeEnvironmentRow = (rowId: string) => {
    onUpdate({ environment_variables: draft.environment_variables.filter((row) => row.id !== rowId) });
  };

  return (
    <div className="flex flex-col gap-5">
      <div>
        <h2 className="text-lg font-semibold text-v3-ink">Provider 类型</h2>
        <p className="text-sm text-v3-ink-3">数字员工必须选择一个 Provider 类型；Runtime 节点会在项目运行准备中决定，不在创建时绑定到员工。</p>
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
        <p className="rounded-[14px] border border-dashed border-v3-line bg-v3-card-soft p-3 text-sm text-v3-ink-3">
          当前团队治理没有返回可选 Provider，请检查团队能力边界配置。
        </p>
      ) : null}
      {error ? <p className="text-sm text-v3-danger">{error}</p> : null}
      <section className="rounded-[14px] border border-v3-line bg-v3-card-soft p-3">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h3 className="text-sm font-medium text-v3-ink">员工环境变量</h3>
            <p className="text-xs text-v3-ink-3">用于技能运行依赖；创建请求会提交值，接口不会回显明文。</p>
          </div>
          <V3Button
            onClick={() =>
              onUpdate({ environment_variables: [...draft.environment_variables, newEnvironmentVariableRow()] })
            }
            size="sm"
            type="button"
            variant="outline"
          >
            <Plus className="size-4" />
            添加环境变量
          </V3Button>
        </div>
        <div className="mt-3 grid gap-2">
          {draft.environment_variables.length === 0 ? (
            <p className="rounded-[12px] border border-dashed border-v3-line p-3 text-sm text-v3-ink-3">暂无环境变量。</p>
          ) : null}
          {draft.environment_variables.map((row, index) => (
            <div
              className="grid gap-2 rounded-[12px] border border-v3-line bg-v3-card p-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto_auto] md:items-end"
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
              <label className="flex h-10 items-center gap-2 rounded-xl border border-v3-line px-3 text-sm text-v3-ink">
                <Checkbox
                  checked={row.sensitive}
                  onCheckedChange={(checked) => updateEnvironmentRow(row.id, { sensitive: checked === true })}
                />
                敏感
              </label>
              <V3Button
                aria-label={`移除环境变量 ${index + 1}`}
                onClick={() => removeEnvironmentRow(row.id)}
                size="icon"
                type="button"
                variant="ghost"
              >
                <Trash2 />
              </V3Button>
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
  selected,
}: {
  onSelectProvider: (providerType: string) => void;
  options?: DigitalEmployeeCreateOptions;
  providerType: string;
  selected?: boolean;
}) {
  const preview = providerDispatchPreview(options, providerType);

  return (
    <label
      className={cn(
        "flex cursor-pointer items-start gap-3 rounded-[14px] border p-3 text-sm transition-colors",
        selected
          ? "border-v3-brand/40 bg-v3-brand-soft"
          : "border-v3-line bg-v3-card hover:bg-v3-card-soft",
      )}
      onClick={(event) => {
        event.preventDefault();
        onSelectProvider(providerType);
      }}
    >
      <RadioGroupItem value={providerType} />
      <span className="min-w-0 flex-1">
        <span className={cn("block font-semibold", selected ? "text-v3-brand-deep" : "text-v3-ink")}>
          {providerLabel(providerType)}
        </span>
        <span className="mt-1 block text-v3-ink-3">
          {preview.availableCount > 0
            ? preview.availableCount === preview.matchingCount
              ? `${preview.matchingCount} 个 Runtime 节点候选会在项目运行准备中评估`
              : `${preview.availableCount}/${preview.matchingCount} 个 Runtime 节点当前在线，仅用于项目运行准备参考`
            : "当前没有在线 Runtime 节点支持该 Provider；创建时仍会记录必选 Provider 类型"}
        </span>
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
      <Label className="text-v3-ink" htmlFor={id}>{label}</Label>
      {children}
      {error ? <span className="text-sm text-v3-danger">{error}</span> : null}
    </div>
  );
}

const labelId: Record<string, string> = {
  员工类型: "employee-type",
  名称: "employee-name",
  归属团队: "employee-team",
  描述: "employee-description",
  职责定位: "employee-role",
  风险等级: "employee-risk",
  "每日 Token 预算上限": "daily-token-limit",
};

const selectClassName =
  "h-10 w-full rounded-xl border border-v3-line bg-v3-card px-3 py-1 text-sm text-v3-ink shadow-sm outline-none transition-[color,box-shadow] focus-visible:border-v3-brand focus-visible:ring-2 focus-visible:ring-v3-brand/40 disabled:cursor-not-allowed disabled:opacity-50";

function applyTypeDefaults(current: WizardDraft, typeOption: DigitalEmployeeTypeOption): WizardDraft {
  const defaultCapabilitySelection = typeOption.default_capability_selection ?? {};

  return {
    ...current,
    approval_policy_override: typeOption.default_approval_policy ?? {},
    capability_selection: {
      enabled_external_capabilities: stringList(defaultCapabilitySelection.enabled_external_capabilities),
      enabled_mcp_servers: stringList(defaultCapabilitySelection.enabled_mcp_servers),
      enabled_skills: stringList(defaultCapabilitySelection.enabled_skills),
    },
    context_policy_override: typeOption.default_context_policy_override ?? {},
    employee_type: typeOption.type,
    risk_level: stringValue(typeOption.default_approval_policy?.min_risk_for_human) || "medium",
    role: typeOption.default_role || typeOption.type,
  };
}

function applyBlankCustomDefaults(current: WizardDraft): WizardDraft {
  return {
    ...current,
    creation_mode: "blank_custom",
    approval_policy_override: {},
    capability_selection: {
      enabled_external_capabilities: [],
      enabled_mcp_servers: [],
      enabled_skills: [],
    },
    context_policy_override: {},
    employee_type: BLANK_CUSTOM_EMPLOYEE_TYPE,
    risk_level: "medium",
    role: "",
  };
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
  if (step === "治理") {
    const errors: ValidationErrors = {};
    if (draft.daily_token_limit.trim()) {
      const parsed = Number(draft.daily_token_limit);
      if (!Number.isInteger(parsed) || parsed <= 0) {
        errors.daily_token_limit = "每日 Token 预算上限必须是正整数";
      }
    }
    return errors;
  }
  if (step === "Provider 类型" && !draft.provider_type) {
    return { runtime: "请选择 Provider 类型" };
  }
  return {};
}

function validateDraftForCreate(draft: WizardDraft): ValidationErrors {
  return {
    ...validateStep("身份", draft),
    ...validateStep("治理", draft),
    ...validateStep("Provider 类型", draft),
  };
}

function budgetPolicyFromDraft(draft: WizardDraft) {
  const trimmed = draft.daily_token_limit.trim();
  if (!trimmed) return {};
  return { daily_token_limit: Number(trimmed) };
}

function providerCandidates(options: DigitalEmployeeCreateOptions | undefined) {
  const governanceValues = [
    ...(options?.team_config.allowed_provider_types ?? []),
    ...(options?.capability_options.provider_types ?? []),
  ]
    .map(normalizeProviderValue)
    .filter((value) => value && canonicalProviderTypes.includes(value as (typeof canonicalProviderTypes)[number]));
  const governanceSet = new Set(governanceValues);
  const runtimeValues =
    options?.runtime_provider_options
      .map((option) => normalizeProviderValue(option.provider_type))
      .filter(
        (value) =>
          value &&
          canonicalProviderTypes.includes(value as (typeof canonicalProviderTypes)[number]) &&
          (governanceSet.size === 0 || governanceSet.has(value)),
      ) ?? [];
  const values = [...governanceValues, ...runtimeValues];
  return Array.from(new Set(values)).sort(
    (left, right) => canonicalProviderTypes.indexOf(left as any) - canonicalProviderTypes.indexOf(right as any),
  );
}

function providerDispatchPreview(options: DigitalEmployeeCreateOptions | undefined, providerType: string) {
  const matchingOptions = (options?.runtime_provider_options ?? []).filter(
    (option) => normalizeProviderValue(option.provider_type) === providerType,
  );
  return {
    matchingCount: matchingOptions.length,
    availableCount: matchingOptions.filter((option) => option.available).length,
  };
}

function newEnvironmentVariableRow(): EnvironmentVariableDraftRow {
  return {
    id: globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random()}`,
    name: "",
    value: "",
    sensitive: true,
  };
}

const riskTone: Record<string, V3Tone> = {
  critical: "danger",
  high: "danger",
  medium: "warn",
  low: "ok",
};

const riskLabels: Record<string, string> = {
  low: "低",
  medium: "中",
  high: "高",
  critical: "严重",
};

const providerLabels: Record<string, string> = {
  codex: "Codex",
  opencode: "OpenCode",
  "claude-code": "Claude Code",
};

const canonicalProviderTypes = ["codex", "opencode", "claude-code"] as const;

function riskLabel(value: string) {
  return riskLabels[value] ?? value;
}

function providerLabel(value: string) {
  return providerLabels[value] ?? value;
}

function normalizeProviderValue(value: string) {
  const normalized = value.trim().toLowerCase();
  return normalized === "claude_code" ? "claude-code" : normalized;
}

function RiskPill({ risk }: { risk: string }) {
  return <StatusPill tone={riskTone[risk] ?? "mute"}>{riskLabel(risk)}</StatusPill>;
}

function checkTone(status: string): V3Tone {
  if (status === "passed") return "ok";
  if (status === "warning") return "warn";
  return "danger";
}

function checkDotClassName(status: string) {
  if (status === "passed") return "bg-v3-ok";
  if (status === "warning") return "bg-v3-warn";
  return "bg-v3-danger";
}

function checkStatusLabel(status: string) {
  if (status === "passed") return "通过";
  if (status === "warning") return "提醒";
  return "阻断";
}

function getErrorMessage(error: unknown) {
  return error instanceof Error ? error.message : "请求失败";
}

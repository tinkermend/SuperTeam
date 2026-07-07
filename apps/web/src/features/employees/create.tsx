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
  Trash2,
} from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Textarea } from "@/components/ui/textarea";
import { IconTile } from "@/components/superteam";
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
  isTeamGovernanceConfigRequiredError,
  listDigitalEmployeeAvatarAssets,
} from "@/lib/api/employees";
import { listTeamMcpBindings } from "@/lib/api/capabilities";
import { listSkills, listTeamSkills } from "@/lib/api/skills";
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
const canonicalProviderValues = ["claude-code", "codex", "opencode"] as const;
type CanonicalProviderType = (typeof canonicalProviderValues)[number];
const providerLabels: Record<string, string> = {
  "claude-code": "Claude Code",
  codex: "Codex",
  opencode: "OpenCode",
};

const configSteps = ["身份", "能力", "治理", "Provider 类型"] as const;
type StepName = (typeof configSteps)[number];
type CreateFlowStep = "template" | "preflight" | "configure" | "confirm";
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

  const visibleSkills = useQuery({
    queryKey: ["skills", "employee-create-visible"],
    queryFn: () => listSkills({ baseUrl: apiBaseUrl, fetcher }),
    staleTime: 30_000,
  });

  const teamGovernanceBlocked = isTeamGovernanceConfigRequiredError(createOptions.error);

  const teamSkills = useQuery({
    enabled: Boolean(draft.team_id) && !teamGovernanceBlocked,
    queryKey: ["team-skills", draft.team_id],
    queryFn: () => listTeamSkills({ baseUrl: apiBaseUrl, fetcher }, draft.team_id),
  });

  const teamMcpBindings = useQuery({
    enabled: Boolean(draft.team_id) && !teamGovernanceBlocked,
    queryKey: ["team-mcp-bindings", draft.team_id],
    queryFn: () => listTeamMcpBindings({ baseUrl: apiBaseUrl, fetcher }, draft.team_id),
  });
  const inheritedSkillKeys = useMemo(
    () => new Set((teamSkills.data ?? []).map(skillKey).filter(Boolean)),
    [teamSkills.data],
  );
  const inheritedMcpKeys = useMemo(
    () => new Set((teamMcpBindings.data ?? []).map(mcpBindingKey).filter(Boolean)),
    [teamMcpBindings.data],
  );
  const visibleSkillKeys = useMemo(
    () => new Set((visibleSkills.data ?? []).map(skillKey).filter(Boolean)),
    [visibleSkills.data],
  );

  const selectedType = useMemo(
    () => createOptions.data?.employee_types.find((item) => item.type === draft.employee_type),
    [createOptions.data?.employee_types, draft.employee_type],
  );
  const blankCustom = draft.creation_mode === "blank_custom";
  const roleTitle = blankCustom ? BLANK_CUSTOM_TITLE : selectedType?.label ?? draft.employee_type;

  useEffect(() => {
    if (draft.creation_mode !== "template") return;
    const optionsData = createOptions.data;
    const employeeTypes = optionsData?.employee_types ?? [];
    const firstType = firstPreferredEmployeeType(employeeTypes);
    if (!firstType) return;
    if (!draft.employee_type || !employeeTypes.some((item) => item.type === draft.employee_type)) {
      setDraft((current) => applyTypeDefaults(current, firstType));
    }
  }, [createOptions.data, draft.creation_mode, draft.employee_type]);

  useEffect(() => {
    if (draft.creation_mode !== "template") return;
    const optionsData = createOptions.data;
    if (!requestedTemplate || !optionsData || templateQueryHandled === requestedTemplate) return;
    const requestedType = findTemplateByType(optionsData, requestedTemplate);
    setTemplateQueryHandled(requestedTemplate);
    if (!requestedType) return;
    setDraft((current) => applyTypeDefaults(current, requestedType));
  }, [createOptions.data, draft.creation_mode, requestedTemplate, templateQueryHandled]);

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
        throw new Error("请选择 Provider");
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
            title: roleTitle,
          },
          ...(blankCustom ? { metadata: { creation_mode: "blank_custom" } } : {}),
          capability_selection: {
            enabled_external_capabilities: draft.capability_selection.enabled_external_capabilities.filter((value) =>
              (createOptions.data?.capability_options.external_capabilities ?? []).includes(value),
            ),
            enabled_mcp_servers: withoutInherited(draft.capability_selection.enabled_mcp_servers, inheritedMcpKeys),
            enabled_skills: withoutInherited(
              draft.capability_selection.enabled_skills.filter((value) => visibleSkillKeys.size === 0 || visibleSkillKeys.has(value)),
              inheritedSkillKeys,
            ),
          },
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
    if (teamGovernanceBlocked) {
      const blockedErrors = { ...nextErrors, team_id: "该团队尚未启用治理配置" };
      setErrors(blockedErrors);
      return;
    }
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

  function enterPreflight() {
    setErrors({});
    setFlowStep("preflight");
  }

  function enterConfiguration() {
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
    setDraft({ ...emptyDraft, creation_mode: creationMode, team_id: teamId });
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
      setFlowStep("preflight");
      return;
    }
    setDraft(nextDraft);
    setFlowStep("template");
  }

  function requestTemplateChange() {
    if (draftTouched && !window.confirm("更换创建路径会重置当前配置草稿，是否继续？")) {
      return;
    }
    resetDraftForTeam(draft.team_id, draft.creation_mode);
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
      setDraft((current) => applyBlankCustomDefaults(current));
      setFlowStep("preflight");
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
          : "按职责定位、能力选择、治理策略和 Provider 类型完成员工画像。"
        }
        actions={
          flowStep !== "template" ? (
            <Button onClick={requestTemplateChange} type="button" variant="outline">
              <ArrowLeft data-icon="inline-start" />
              返回
            </Button>
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
        {createOptions.isError && !teamGovernanceBlocked ? (
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
                onEnterPreflight={enterPreflight}
                onSelectType={selectType}
              />
            ) : null}
          </div>
        ) : null}

        {flowStep === "preflight" ? (
          <PreflightStep
            draft={draft}
            options={createOptions.data}
            selectedTeamName={selectedTeam?.name}
            selectedType={selectedType}
            onBack={() => setFlowStep("template")}
            onContinue={enterConfiguration}
          />
        ) : null}

        {flowStep === "configure" ? (
          <div className="grid gap-4 xl:h-[calc(100vh-220px)] xl:min-h-[560px] xl:grid-cols-[minmax(0,1fr)_340px]">
            <section className="flex min-w-0 flex-col overflow-hidden rounded-md border bg-card/95 shadow-xs">
              <div className="border-b p-4">
                <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                  <div>
                    <h2 className="text-lg font-semibold">员工画像蓝图</h2>
                    <p className="mt-1 text-sm text-muted-foreground">
                      按职责定位、可用能力、治理边界和 Provider 类型完成员工画像。
                    </p>
                  </div>
                  <StepTabs currentStep={currentStep} />
                </div>
              </div>

              <div className="grid min-h-0 flex-1 gap-4 overflow-y-auto p-4">
                <SelectedTemplateSummary
                  draft={draft}
                  selectedType={selectedType}
                  onChangeTemplate={requestTemplateChange}
                />

                <div className="min-h-0 rounded-md border bg-background p-4">
                  {teams.isLoading || avatarAssets.isLoading || createOptions.isLoading ? (
                    <div className="flex min-h-[360px] items-center justify-center gap-2 text-sm text-muted-foreground">
                      <Loader2 className="animate-spin" />
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
                    <CapabilityStep
                      draft={draft}
                      options={createOptions.data}
                      teamSkills={teamSkills.data}
                      teamMcpBindings={teamMcpBindings.data}
                      visibleSkills={visibleSkills.data}
                      onUpdate={updateDraft}
                    />
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
                <p className="px-4 text-sm text-destructive">{getErrorMessage(createEmployee.error)}</p>
              ) : null}
              {teamGovernanceBlocked ? (
                <div className="px-4">
                  <Alert variant="destructive">
                    <AlertTitle>团队治理未启用</AlertTitle>
                    <AlertDescription>
                      该团队尚未启用治理配置，不能在此团队下创建数字员工。
                    </AlertDescription>
                    <div className="mt-3 flex flex-wrap gap-2">
                      <Button
                        type="button"
                        variant="secondary"
                        onClick={() => {
                          setDraft((current) => ({
                            ...current,
                            team_id: "",
                            capability_selection: {
                              ...current.capability_selection,
                              enabled_skills: [],
                              enabled_mcp_servers: [],
                            },
                          }));
                        }}
                      >
                        先不归属团队创建
                      </Button>
                      <Button asChild type="button" variant="outline">
                        <Link params={{ teamId: draft.team_id }} to="/teams/$teamId">
                          前往团队治理配置
                        </Link>
                      </Button>
                    </div>
                  </Alert>
                </div>
              ) : null}
              <div
                className="sticky bottom-0 z-10 flex justify-between gap-3 border-t bg-card/95 p-4 shadow-[0_-12px_24px_rgba(15,23,42,0.06)]"
                data-testid="employee-configure-actions"
              >
                <Button
                  disabled={stepIndex === 0 || createEmployee.isPending}
                  onClick={() => setStepIndex((current) => Math.max(current - 1, 0))}
                  type="button"
                  variant="outline"
                >
                  <ChevronLeft data-icon="inline-start" />
                  上一步
                </Button>
                {stepIndex < configSteps.length - 1 ? (
                  <Button
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
                    <ChevronRight data-icon="inline-end" />
                  </Button>
                ) : (
                  <Button
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
                    <ChevronRight data-icon="inline-end" />
                  </Button>
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
    { key: "template", title: "选择模板", description: "选择创建方式和专业模板" },
    { key: "preflight", title: "配置预检", description: "检查治理策略和 Provider 类型候选" },
    { key: "configure", title: "完成配置", description: "进入详细配置向导" },
    { key: "confirm", title: "确认创建", description: "核对本次创建明细" },
  ];
  const activeIndex = stages.findIndex((stage) => stage.key === flowStep);
  const normalizedActiveIndex = activeIndex === -1 ? 2 : activeIndex;

  return (
    <section className="mb-4 rounded-md border bg-card/95 px-4 py-3 shadow-xs">
      <div className="grid gap-3 md:grid-cols-4">
        {stages.map((stage, index) => {
          const active = index === normalizedActiveIndex;
          const done = index < normalizedActiveIndex;

          return (
            <div className="flex items-center gap-3" key={stage.title}>
              <span
                className={cn(
                  "flex size-7 shrink-0 items-center justify-center rounded-full text-xs font-semibold",
                  active ? "bg-primary text-primary-foreground" : "",
                  done ? "bg-primary/15 text-primary" : "",
                  !active && !done ? "bg-muted text-muted-foreground" : "",
                )}
              >
                {done ? <Check className="size-4" /> : index + 1}
              </span>
              <span className="min-w-0">
                <span className={cn("block text-sm font-semibold", active ? "text-primary" : "")}>{stage.title}</span>
                <span className="block text-xs text-muted-foreground">{stage.description}</span>
              </span>
            </div>
          );
        })}
      </div>
    </section>
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
    <aside className="rounded-md border bg-card/95 p-3 shadow-xs">
      <div className="mb-3 flex items-center gap-2 px-1">
        <IconTile tone="brand" size="sm">
          <Sparkles />
        </IconTile>
        <div>
          <h2 className="text-base font-semibold">创建路径</h2>
          <p className="text-xs text-muted-foreground">先选入口，再进入配置。</p>
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
                "rounded-md border p-3 text-left transition",
                active
                  ? "border-primary/40 bg-primary/10 text-foreground shadow-xs"
                  : "border-border/70 bg-background/80 text-muted-foreground",
                path.disabled ? "cursor-not-allowed opacity-65" : "hover:border-primary/30 hover:bg-primary/5",
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
              <span className="flex items-start gap-2">
                <span
                  className={cn(
                    "mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-md border",
                    active ? "border-primary/30 bg-primary/15 text-primary" : "bg-muted text-muted-foreground",
                  )}
                >
                  <Icon className="size-4" />
                </span>
                <span className="min-w-0 flex-1">
                  <span className="flex items-center justify-between gap-2">
                    <span className="text-sm font-medium">{path.title}</span>
                    <Badge variant={active ? "default" : "secondary"}>{path.badge}</Badge>
                  </span>
                  <span className="mt-1 block text-xs leading-5">{path.description}</span>
                </span>
              </span>
            </button>
          );
        })}
      </div>
      <div className="mt-3 rounded-md border bg-muted/30 p-3 text-xs leading-5 text-muted-foreground">
        创建后进入 ready，不会自动执行任务；项目或任务调度可手动发起。
      </div>
    </aside>
  );
}

function TemplateSelectionPanel({
  draft,
  options,
  selectedTeamName,
  selectedType,
  onEnterPreflight,
  onSelectType,
}: {
  draft: WizardDraft;
  options?: DigitalEmployeeCreateOptions;
  selectedTeamName?: string;
  selectedType?: DigitalEmployeeTypeOption;
  onEnterPreflight: () => void;
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
    <section className="@container/template flex min-w-0 flex-col overflow-hidden rounded-md border bg-card/95 shadow-xs">
      <div className="border-b p-4">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <h2 className="text-base font-semibold">选择内置模板</h2>
            <p className="mt-1 text-sm text-muted-foreground">模板只负责带出默认角色、模板能力和治理默认值。</p>
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
                {risk === "all" ? "全部风险" : risk}
              </option>
            ))}
          </select>
        </div>
      </div>
      {employeeTypes.length === 0 ? (
        <div className="m-4 flex min-h-[420px] flex-1 items-center justify-center rounded-md border bg-muted/30 p-6 text-sm text-muted-foreground">
          当前团队治理配置未返回可用专业模板。
        </div>
      ) : (
        <div className="min-h-0 flex-1 p-4">
          <div
            className="h-full overflow-hidden rounded-md border bg-background"
            data-testid="template-selection-table"
            data-slot="template-selection-table"
          >
            <div className="h-full max-h-[min(680px,calc(100vh-360px))] overflow-auto">
              <table className="w-full min-w-[860px] border-collapse text-sm">
                <thead className="sticky top-0 z-10 border-b bg-muted text-xs font-medium text-muted-foreground">
                  <tr>
                    <th className="w-[34%] px-3 py-2 text-left">模板</th>
                    <th className="w-[18%] px-3 py-2 text-left">默认角色</th>
                    <th className="w-[18%] px-3 py-2 text-left">模板能力</th>
                    <th className="w-[12%] px-3 py-2 text-left">风险等级</th>
                    <th className="w-[12%] px-3 py-2 text-left">默认注入</th>
                    <th className="w-[6%] px-3 py-2 text-right">选择</th>
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
                <div className="flex min-h-[220px] items-center justify-center border-t bg-muted/20 p-6 text-sm text-muted-foreground">
                  没有匹配当前筛选条件的专业模板。
                </div>
              ) : null}
            </div>
          </div>
        </div>
      )}
      <div className="border-t bg-card/95 px-4 py-3">
        <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2 text-sm">
              <span className="font-medium text-foreground">已选模板摘要</span>
              <Badge variant="secondary">团队 {selectedTeamName || "无（租户级）"}</Badge>
              <Badge variant="secondary">模板 {selectedType?.label ?? (draft.employee_type || "未选择")}</Badge>
              <Badge variant="secondary">默认角色 {draft.role || selectedType?.default_role || "未生成"}</Badge>
              <Badge variant="secondary">风险 {draft.risk_level || "medium"}</Badge>
            </div>
            <p className="mt-2 text-sm text-muted-foreground">
              没有合适的模板？
              <button className="ml-2 cursor-not-allowed font-medium text-muted-foreground" disabled type="button">
                选择空白自定义（暂未开放）
              </button>
            </p>
          </div>
          <Button disabled={!draft.employee_type} onClick={onEnterPreflight} type="button">
            进入配置预检
            <ChevronRight data-icon="inline-end" />
          </Button>
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
        "border-b transition last:border-b-0 hover:bg-muted/30",
        selected ? "bg-primary/5 [box-shadow:inset_3px_0_0_var(--v3-brand)]" : "",
      )}
    >
      <td className="px-3 py-3 align-top">
        <div className="flex min-w-0 gap-3">
          <span
            className={cn(
              "mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-md border",
              selected ? "border-primary/30 bg-primary/15 text-primary" : "bg-muted text-muted-foreground",
            )}
          >
            <Code2 className="size-4" />
          </span>
          <div className="min-w-0">
            <div className="font-semibold">{typeOption.label}</div>
            <div className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">{typeOption.description}</div>
          </div>
        </div>
      </td>
      <td className="px-3 py-3 align-top">
        <div className="max-w-[180px] truncate rounded-md border bg-muted/30 px-2 py-1 font-mono text-xs">
          {typeOption.default_role || typeOption.type}
        </div>
      </td>
      <td className="px-3 py-3 align-top">
        <div className="grid gap-1.5">
          <div className="flex flex-wrap gap-1.5">
            <Badge className="gap-1" variant="secondary">
              {`技能 ${capability.skills.length}`}
            </Badge>
            <Badge className="gap-1" variant="secondary">
              {`MCP ${capability.mcpServers.length}`}
            </Badge>
            <Badge className="gap-1" variant="secondary">
              {`Provider ${capability.providerTypes.length}`}
            </Badge>
          </div>
          <div className="truncate text-xs text-muted-foreground">{templateCapabilityPreview(typeOption)}</div>
        </div>
      </td>
      <td className="px-3 py-3 align-top">
        <div className="flex items-center gap-1.5">
          <span className="text-xs text-muted-foreground">风险</span>
          <Badge
            className={cn(
              "font-mono",
              risk === "high" || risk === "critical" ? "bg-amber-100 text-amber-800" : "",
              risk === "low" || risk === "medium" ? "bg-emerald-100 text-emerald-800" : "",
            )}
            variant="secondary"
          >
            {risk}
          </Badge>
        </div>
      </td>
      <td className="px-3 py-3 align-top text-xs leading-5 text-muted-foreground">
        {templateDefaultInjectionLine(typeOption)}
      </td>
      <td className="px-3 py-3 text-right align-top">
        <Button
          aria-label={`${selected ? "已选择" : "选择"}${typeOption.label}模板`}
          aria-pressed={selected}
          onClick={onSelect}
          size="sm"
          type="button"
          variant={selected ? "default" : "outline"}
        >
          {selected ? <Check data-icon="inline-start" /> : null}
          {selected ? "已选" : "选择"}
        </Button>
      </td>
    </tr>
  );
}

function CheckListPanel({ options }: { options?: DigitalEmployeeCreateOptions }) {
  const checks = displayablePreflightChecks(options?.creation_checks ?? []);

  return (
    <section className="rounded-md border bg-card/95 p-4 shadow-xs">
      <h2 className="text-base font-semibold">预检项目</h2>
      <p className="mt-1 text-xs text-muted-foreground">检查治理策略、模板与 Provider 类型候选；能力选择在下一步配置。</p>
      <div className="mt-4 grid gap-2">
        {checks.length === 0 ? (
          <p className="rounded-md border bg-muted/30 p-3 text-sm text-muted-foreground">等待创建候选加载。</p>
        ) : (
          checks.map((check) => (
            <div className="flex items-center gap-3 rounded-md border bg-background p-3" key={check.key}>
              <span className={cn("size-2 rounded-full", checkDotClassName(check.status))} />
              <span className="min-w-0 flex-1">
                <span className="block text-sm font-medium">{check.label}</span>
                <span className="block truncate text-xs text-muted-foreground">{check.message}</span>
              </span>
              <Badge variant={check.status === "blocked" ? "destructive" : "secondary"}>{checkStatusLabel(check.status)}</Badge>
            </div>
          ))
        )}
      </div>
    </section>
  );
}

function PreflightStep({
  draft,
  options,
  selectedTeamName,
  selectedType,
  onBack,
  onContinue,
}: {
  draft: WizardDraft;
  options?: DigitalEmployeeCreateOptions;
  selectedTeamName?: string;
  selectedType?: DigitalEmployeeTypeOption;
  onBack: () => void;
  onContinue: () => void;
}) {
  const isBlankCustom = draft.creation_mode === "blank_custom";
  const blockedChecks = displayablePreflightChecks(options?.creation_checks ?? []).filter(
    (check) => check.status === "blocked",
  );
  const hasBlockedChecks = blockedChecks.length > 0;

  return (
    <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_340px]">
      <section className="min-w-0 rounded-md border bg-card/95 shadow-xs">
        <div className="border-b p-4">
          <h2 className="text-lg font-semibold">配置预检</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            先确认后端创建候选返回的治理策略、{isBlankCustom ? BLANK_CUSTOM_TITLE : "模板"}和 Provider 类型候选；技能、MCP 和外部能力在配置页选择。
          </p>
        </div>
        <div className="grid gap-4 p-4">
          <CheckListPanel options={options} />
          {hasBlockedChecks ? (
            <Alert variant="destructive">
              <AlertTitle>当前配置暂不能继续</AlertTitle>
              <AlertDescription>
                {blockedChecks.map((check) => `${check.label}: ${check.message}`).join("；")}
              </AlertDescription>
            </Alert>
          ) : (
            <Alert>
              <AlertTitle>预检通过</AlertTitle>
              <AlertDescription>可以继续补充员工身份、能力、治理和 Provider 类型。</AlertDescription>
            </Alert>
          )}
        </div>
        <div className="flex justify-between gap-3 border-t p-4">
          <Button onClick={onBack} type="button" variant="outline">
            <ChevronLeft data-icon="inline-start" />
            返回创建路径
          </Button>
          <Button disabled={hasBlockedChecks || !draft.employee_type} onClick={onContinue} type="button">
            预检通过，继续配置
            <ChevronRight data-icon="inline-end" />
          </Button>
        </div>
      </section>

      <aside className="grid content-start gap-4">
        <section className="rounded-md border bg-card/95 p-4 shadow-xs">
          <h2 className="text-base font-semibold">本次草稿</h2>
          <p className="mt-1 text-xs text-muted-foreground">
            {isBlankCustom ? "能力和治理覆盖将在配置页手动补齐。" : "模板默认值将在配置页继续编辑。"}
          </p>
          <div className="mt-4 grid gap-2 text-sm">
            <InlineSummary label="归属团队" value={selectedTeamName || "无（租户级）"} />
            <InlineSummary label="创建路径" value={draft.creation_mode === "blank_custom" ? BLANK_CUSTOM_TITLE : "专业模板"} />
            {!isBlankCustom ? (
              <InlineSummary label="专业模板" value={selectedType?.label ?? (draft.employee_type || "未选择")} />
            ) : null}
            <InlineSummary label="职责定位" value={draft.role || selectedType?.default_role || "未生成"} />
            <InlineSummary label="风险等级" value={draft.risk_level || "medium"} />
            <InlineSummary
              label="默认注入"
              value={`技能 ${draft.capability_selection.enabled_skills.length} · MCP ${draft.capability_selection.enabled_mcp_servers.length}`}
            />
          </div>
        </section>
      </aside>
    </div>
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
    <div className="flex items-center justify-between gap-3 rounded-md border bg-background px-3 py-2">
      <span className="text-muted-foreground">{label}</span>
      <span className="max-w-[180px] truncate font-medium">{value}</span>
    </div>
  );
}

function StepTabs({ currentStep }: { currentStep: StepName }) {
  const currentIndex = configSteps.indexOf(currentStep);

  return (
    <div className="flex flex-wrap gap-2 rounded-md border bg-muted/30 p-1">
        {configSteps.map((step, index) => {
          const active = step === currentStep;
          const done = index < currentIndex;

          return (
            <div
              className={cn(
                "flex h-8 items-center gap-2 rounded-md px-2 text-xs text-muted-foreground",
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
  );
}

function SelectedTemplateSummary({
  draft,
  selectedType,
  onChangeTemplate,
}: {
  draft: WizardDraft;
  selectedType?: DigitalEmployeeTypeOption;
  onChangeTemplate: () => void;
}) {
  const isBlankCustom = draft.creation_mode === "blank_custom";

  return (
    <section className="rounded-md border bg-background p-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
            {isBlankCustom ? "空白自定义草稿" : "已选模板"}
          </p>
          <h2 className="mt-1 text-lg font-semibold">
            {isBlankCustom ? BLANK_CUSTOM_TITLE : selectedType?.label ?? (draft.employee_type || "未选择模板")}
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {isBlankCustom
              ? "能力、上下文覆盖、审批覆盖和 Provider 类型由你手动配置。"
              : selectedType?.description ?? "模板只作为初始草稿来源，Provider 在最后一步选择。"}
          </p>
        </div>
        <Button onClick={onChangeTemplate} type="button" variant="outline">
          <ArrowLeft data-icon="inline-start" />
          更换创建路径
        </Button>
      </div>
      <div className="mt-3 flex flex-wrap gap-1.5">
        <Badge variant="secondary">职责定位 {selectedType?.default_role || draft.role || "未生成"}</Badge>
        {isBlankCustom ? (
          <>
            <Badge variant="secondary">能力手动配置</Badge>
            <Badge variant="secondary">治理覆盖 0</Badge>
          </>
        ) : (
          <>
            <Badge variant="secondary">技能 {selectedType?.recommended_skills?.length ?? 0}</Badge>
            <Badge variant="secondary">MCP {selectedType?.recommended_mcp_servers?.length ?? 0}</Badge>
          </>
        )}
      </div>
    </section>
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
      <section className="rounded-md border bg-card/95 p-4 shadow-xs">
        <div className="mb-3 flex items-center gap-2">
          <IconTile tone="ok" size="sm">
            <ShieldCheck />
          </IconTile>
          <div>
            <h2 className="text-base font-semibold">预检项目</h2>
            <p className="text-xs text-muted-foreground">来自 Control Plane 创建候选接口。</p>
          </div>
        </div>
        <div className="grid gap-2">
          {checks.length === 0 ? (
            <p className="rounded-md border bg-muted/30 p-3 text-sm text-muted-foreground">等待创建候选加载。</p>
          ) : (
            checks.map((check) => (
              <div className="flex items-start gap-2 rounded-md border bg-background p-3" key={check.key}>
                <span className={cn("mt-1 size-2 rounded-full", checkDotClassName(check.status))} />
                <span className="min-w-0 flex-1">
                  <span className="flex items-center justify-between gap-2">
                    <span className="text-sm font-medium">{check.label}</span>
                    <Badge variant={check.status === "blocked" ? "destructive" : "secondary"}>
                      {checkStatusLabel(check.status)}
                    </Badge>
                  </span>
                  <span className="mt-1 block text-xs leading-5 text-muted-foreground">{check.message}</span>
                </span>
              </div>
            ))
          )}
        </div>
      </section>

      <section className="rounded-md border bg-card/95 p-4 shadow-xs">
        <div className="mb-3 flex items-center gap-2">
          <IconTile tone="artifact" size="sm">
            <Gauge />
          </IconTile>
          <div>
            <h2 className="text-base font-semibold">画像摘要</h2>
            <p className="text-xs text-muted-foreground">随配置实时更新。</p>
          </div>
        </div>
        <div className="grid gap-3 text-sm">
          <SummaryItem label={draft.creation_mode === "blank_custom" ? "创建路径" : "专业模板"} value={draft.creation_mode === "blank_custom" ? BLANK_CUSTOM_TITLE : (selectedType?.label ?? draft.employee_type) || "未选择"} />
          <SummaryItem label="职责定位" value={draft.role || "未填写"} />
          <SummaryItem label="风险等级" value={draft.risk_level || "medium"} />
          <SummaryItem
            label="能力选择"
            value={`技能 ${draft.capability_selection.enabled_skills.length} · MCP ${draft.capability_selection.enabled_mcp_servers.length} · 外部 ${draft.capability_selection.enabled_external_capabilities.length}`}
          />
          <SummaryItem label="Provider" value={draft.provider_type || `${providers.length} 个候选`} />
        </div>
      </section>

      <section className="rounded-md border bg-muted/30 p-4 text-xs leading-5 text-muted-foreground">
        <div className="mb-2 flex items-center gap-2 font-medium text-foreground">
          <Cpu className="size-4 text-primary" />
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
    <section className="rounded-md border bg-card/95 shadow-xs">
      <div className="border-b p-4">
        <h2 className="text-lg font-semibold">确认创建</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          核对本次将提交给 Control Plane 的员工配置；确认后创建 ready 状态数字员工。
        </p>
      </div>
      <div className="grid gap-4 p-4 lg:grid-cols-2">
        <section className="rounded-md border bg-background p-4">
          <h3 className="text-sm font-semibold">身份与模板</h3>
          <div className="mt-3 grid gap-2 text-sm">
            <InlineSummary label="归属团队" value={selectedTeamName || "无（租户级）"} />
            <InlineSummary label="创建路径" value={draft.creation_mode === "blank_custom" ? BLANK_CUSTOM_TITLE : "专业模板"} />
            {draft.creation_mode !== "blank_custom" ? (
              <InlineSummary label="专业模板" value={selectedType?.label ?? (draft.employee_type || "未选择")} />
            ) : null}
            <InlineSummary label="名称" value={draft.name.trim() || "未填写"} />
            <InlineSummary label="职责定位" value={draft.role || "未填写"} />
            <InlineSummary label="风险等级" value={draft.risk_level || "medium"} />
          </div>
        </section>

        <section className="rounded-md border bg-background p-4">
          <h3 className="text-sm font-semibold">能力与 Provider 类型</h3>
          <div className="mt-3 grid gap-2 text-sm">
            <InlineSummary
              label="能力选择"
              value={`技能 ${draft.capability_selection.enabled_skills.length} · MCP ${draft.capability_selection.enabled_mcp_servers.length} · 外部 ${draft.capability_selection.enabled_external_capabilities.length}`}
            />
            <InlineSummary label="Provider 类型" value={draft.provider_type ? providerLabel(draft.provider_type) : "未选择"} />
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
        <p className="px-4 pb-2 text-sm text-destructive">{getErrorMessage(createError)}</p>
      ) : null}
      <div className="flex justify-between gap-3 border-t p-4">
        <Button disabled={creating} onClick={onBack} type="button" variant="outline">
          <ChevronLeft data-icon="inline-start" />
          返回配置
        </Button>
        <Button disabled={creating} onClick={onSubmit} type="button">
          {creating ? <Loader2 className="animate-spin" data-icon="inline-start" /> : <Plus data-icon="inline-start" />}
          确认创建
        </Button>
      </div>
    </section>
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
        <h2 className="text-lg font-semibold">身份</h2>
        <p className="text-sm text-muted-foreground">确定团队、业务类型和职责边界。负责人由后端按当前登录身份注入。</p>
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
          <p className="text-xs text-muted-foreground">选择“无”将创建租户级独立数字员工，治理按内置默认（全部允许）。</p>
        </Field>
        {!isBlankCustom ? (
          <Field label="员工类型" error={errors.employee_type}>
            <select
              aria-invalid={Boolean(errors.employee_type)}
              className={selectClassName}
              disabled
              id="employee-type"
              value={draft.employee_type}
            >
              {(options?.employee_types ?? []).map((item) => (
                <option key={item.type} value={item.type}>
                  {item.label}
                </option>
              ))}
            </select>
            <p className="mt-1 text-xs text-muted-foreground">
              如需切换模板，请使用上方“更换创建路径”并重新生成配置草稿。
            </p>
          </Field>
        ) : null}
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
            placeholder="例如：负责跨系统问题诊断与交付验证"
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
        <div className="rounded-md border bg-muted/30 p-3 text-sm">
          <div className="font-medium">{selectedType.label}</div>
          <div className="mt-1 text-muted-foreground">{selectedType.description}</div>
          <div className="mt-2 text-muted-foreground">默认职责定位：{selectedType.default_role || selectedType.type}</div>
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
    <fieldset className="rounded-md border p-3">
      <legend className="px-1 text-sm font-medium">头像</legend>
      <div className="mt-3 flex flex-wrap gap-3">
        {assets.map((asset) => {
          const selected = asset.id === selectedAssetId;
          return (
            <button
              aria-pressed={selected}
              className={cn(
                "flex size-20 shrink-0 items-center justify-center rounded-full border bg-muted p-0.5 transition",
                selected ? "border-primary ring-2 ring-primary/30" : "hover:border-primary/60",
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
      {assets.length === 0 ? <p className="mt-2 text-sm text-muted-foreground">暂无可选头像</p> : null}
      {error ? <span className="mt-2 block text-sm text-destructive">{error}</span> : null}
    </fieldset>
  );
}

function CapabilityStep({
  draft,
  options,
  teamMcpBindings,
  teamSkills,
  visibleSkills,
  onUpdate,
}: {
  draft: WizardDraft;
  options?: DigitalEmployeeCreateOptions;
  teamMcpBindings?: Array<{ server_key?: string; server_name?: string }>;
  teamSkills?: Array<{ slug?: string; name?: string }>;
  visibleSkills?: Array<{ slug?: string; name?: string }>;
  onUpdate: (patch: Partial<WizardDraft>) => void;
}) {
  const capabilityOptions = options?.capability_options;
  const inheritedSkills = (teamSkills ?? []).map((skill) => ({
    key: skillKey(skill),
    label: skill.name || skill.slug || "",
  }));
  const inheritedMcpServers = (teamMcpBindings ?? []).map((binding) => ({
    key: mcpBindingKey(binding),
    label: binding.server_name || binding.server_key || "",
  }));
  const inheritedSkillKeys = new Set(inheritedSkills.map((skill) => skill.key).filter(Boolean));
  const inheritedMcpKeys = new Set(inheritedMcpServers.map((binding) => binding.key).filter(Boolean));
  const teamPolicySkillKeys = new Set((capabilityOptions?.skills ?? []).filter(Boolean));
  const extensionSkills = withoutInherited(
    (visibleSkills ?? [])
      .map(skillKey)
      .filter((value) => Boolean(value) && teamPolicySkillKeys.has(value)),
    inheritedSkillKeys,
  );
  const extensionMcpServers = withoutInherited(capabilityOptions?.mcp_servers ?? [], inheritedMcpKeys);

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
      <div className="rounded-md border bg-muted/20 p-4">
        <div className="flex items-center justify-between gap-3">
          <div>
            <h3 className="text-base font-semibold">团队继承能力</h3>
            <p className="mt-1 text-sm text-muted-foreground">以下能力由团队治理统一继承，当前员工只读使用。</p>
          </div>
          <Badge variant="secondary">团队继承</Badge>
        </div>
        <div className="mt-4 grid gap-3 md:grid-cols-2">
          <InheritedCapabilityGroup label="技能" values={inheritedSkills.map((skill) => skill.label).filter(Boolean)} />
          <InheritedCapabilityGroup
            label="MCP Server"
            values={inheritedMcpServers.map((binding) => binding.label).filter(Boolean)}
          />
        </div>
      </div>
      <div className="rounded-md border p-4">
        <div>
          <h3 className="text-base font-semibold">员工扩展能力</h3>
          <p className="mt-1 text-sm text-muted-foreground">仅选择当前员工额外需要的能力，不重复团队继承项。</p>
        </div>
        <div className="mt-4 grid gap-5">
      <CapabilityGroup
        checkedValues={draft.capability_selection.enabled_skills}
        label="技能"
        onToggle={(value) => toggle("enabled_skills", value)}
        values={extensionSkills}
      />
      <CapabilityGroup
        checkedValues={draft.capability_selection.enabled_mcp_servers}
        label="MCP Server"
        onToggle={(value) => toggle("enabled_mcp_servers", value)}
        values={extensionMcpServers}
      />
      <CapabilityGroup
        checkedValues={draft.capability_selection.enabled_external_capabilities}
        label="外部能力"
        onToggle={(value) => toggle("enabled_external_capabilities", value)}
        values={capabilityOptions?.external_capabilities ?? []}
      />
        </div>
      </div>
    </div>
  );
}

function InheritedCapabilityGroup({ label, values }: { label: string; values: string[] }) {
  return (
    <fieldset className="rounded-md border bg-background p-3">
      <legend className="px-1 text-sm font-medium">{label}</legend>
      <div className="mt-3 grid gap-2">
        {values.map((value) => (
          <div className="flex items-center justify-between gap-3 rounded-md border bg-muted/20 px-3 py-2 text-sm" key={value}>
            <span>{value}</span>
            <Badge variant="secondary">团队继承</Badge>
          </div>
        ))}
        {values.length === 0 ? <p className="text-sm text-muted-foreground">暂无继承项</p> : null}
      </div>
    </fieldset>
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

function skillKey(skill: { slug?: string; name?: string }): string {
  return skill.slug || skill.name || "";
}

function mcpBindingKey(binding: { server_key?: string; server_name?: string }): string {
  return binding.server_key || binding.server_name || "";
}

function withoutInherited(values: string[], inherited: Set<string>): string[] {
  return values.filter((value) => !inherited.has(value));
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
        <h2 className="text-lg font-semibold">治理</h2>
        <p className="text-sm text-muted-foreground">确认团队治理版本、上下文和审批默认值。这里不暴露原始 JSON 编辑。</p>
      </div>
      <div className="grid gap-3 md:grid-cols-2">
        <SummaryItem label="团队治理版本" value={teamConfig ? `#${teamConfig.revision_number} · ${teamConfig.status}` : "未加载"} />
        <SummaryItem label="风险等级" value={draft.risk_level} />
        <SummaryItem label="允许员工类型" value={`${teamConfig?.allowed_employee_types.length ?? 0} 项`} />
        <SummaryItem label="允许 Provider" value={(teamConfig?.allowed_provider_types ?? []).join(", ") || "暂无"} />
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
        <p className="text-xs text-muted-foreground">不填写表示无预算上限。填写后，达到当日上限会阻止发起新的运行。</p>
      </Field>
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
        <h2 className="text-lg font-semibold">Provider 类型</h2>
        <p className="text-sm text-muted-foreground">Runtime 节点会在项目运行准备中决定，不在创建时绑定到员工。</p>
      </div>
      <RadioGroup onValueChange={onSelectProvider} value={draft.provider_type}>
        <div className="grid gap-3">
          {providers.map((providerType) => (
            <ProviderOption
              key={providerType}
              options={options}
              providerType={providerType}
              onSelectProvider={onSelectProvider}
            />
          ))}
        </div>
      </RadioGroup>
      {providers.length === 0 ? (
        <p className="rounded-md border border-dashed bg-muted/20 p-3 text-sm text-muted-foreground">
          当前团队治理没有返回可选 Provider，请检查团队能力边界配置。
        </p>
      ) : null}
      {error ? <p className="text-sm text-destructive">{error}</p> : null}
      <section className="rounded-md border bg-card/80 p-3">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h3 className="text-sm font-medium">员工环境变量</h3>
            <p className="text-xs text-muted-foreground">用于技能运行依赖；创建请求会提交值，接口不会回显明文。</p>
          </div>
          <Button
            onClick={() =>
              onUpdate({ environment_variables: [...draft.environment_variables, newEnvironmentVariableRow()] })
            }
            size="sm"
            type="button"
            variant="outline"
          >
            <Plus data-icon="inline-start" />
            添加环境变量
          </Button>
        </div>
        <div className="mt-3 grid gap-2">
          {draft.environment_variables.length === 0 ? (
            <p className="rounded-md border border-dashed p-3 text-sm text-muted-foreground">暂无环境变量。</p>
          ) : null}
          {draft.environment_variables.map((row, index) => (
            <div
              className="grid gap-2 rounded-md border bg-background p-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto_auto] md:items-end"
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
              <label className="flex h-9 items-center gap-2 rounded-md border px-3 text-sm">
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
}: {
  onSelectProvider: (providerType: string) => void;
  options?: DigitalEmployeeCreateOptions;
  providerType: string;
}) {
  const preview = providerDispatchPreview(options, providerType);

  return (
    <label
      className="flex cursor-pointer items-start gap-3 rounded-md border p-3 text-sm"
      onClick={(event) => {
        event.preventDefault();
        onSelectProvider(providerType);
      }}
    >
      <RadioGroupItem value={providerType} />
      <span className="min-w-0 flex-1">
        <span className="block font-medium">{providerLabel(providerType)}</span>
        <span className="mt-1 block text-muted-foreground">
          {preview.availableCount > 0
            ? preview.availableCount === preview.matchingCount
              ? `${preview.matchingCount} 个 Runtime 节点候选会在项目运行准备中评估`
              : `${preview.availableCount}/${preview.matchingCount} 个 Runtime 节点当前在线，仅用于项目运行准备参考`
            : "当前没有在线 Runtime 节点支持该 Provider；创建后运行前需要可用 Runtime。"}
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
  职责定位: "employee-role",
  风险等级: "employee-risk",
  "每日 Token 预算上限": "daily-token-limit",
};

const selectClassName =
  "h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-xs outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50";

function applyTypeDefaults(current: WizardDraft, typeOption: DigitalEmployeeTypeOption): WizardDraft {
  const defaultCapabilitySelection = typeOption.default_capability_selection ?? {};

  return {
    ...current,
    approval_policy_override: typeOption.default_approval_policy ?? {},
    capability_selection: {
      enabled_external_capabilities: [],
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
    .map(canonicalizeProviderType)
    .filter((value): value is CanonicalProviderType => Boolean(value));
  const governanceSet = new Set(governanceValues);
  const runtimeValues = options?.runtime_provider_options
    .map((option) => canonicalizeProviderType(option.provider_type))
    .filter(
      (value): value is CanonicalProviderType => Boolean(value) && (governanceSet.size === 0 || governanceSet.has(value)),
    ) ?? [];
  const values = [...governanceValues, ...runtimeValues];
  return Array.from(new Set(values)).sort((left, right) => left.localeCompare(right));
}

function providerDispatchPreview(options: DigitalEmployeeCreateOptions | undefined, providerType: string) {
  const matchingOptions = (options?.runtime_provider_options ?? []).filter(
    (option) => option.provider_type === providerType,
  );
  return {
    matchingCount: matchingOptions.length,
    availableCount: matchingOptions.filter((option) => option.available).length,
  };
}

function providerLabel(providerType: string): string {
  const canonical = canonicalizeProviderType(providerType);
  return canonical ? providerLabels[canonical] : providerType;
}

function canonicalizeProviderType(providerType: string): CanonicalProviderType | "" {
  const trimmed = providerType.trim();
  if (!trimmed) return "";
  if (trimmed === "claude_code") return "claude-code";
  return canonicalProviderValues.includes(trimmed as CanonicalProviderType) ? (trimmed as CanonicalProviderType) : "";
}

function newEnvironmentVariableRow(): EnvironmentVariableDraftRow {
  return {
    id: globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random()}`,
    name: "",
    value: "",
    sensitive: true,
  };
}

function checkDotClassName(status: string) {
  if (status === "passed") return "bg-v3-ok";
  if (status === "warning") return "bg-v3-warn";
  return "bg-destructive";
}

function checkStatusLabel(status: string) {
  if (status === "passed") return "通过";
  if (status === "warning") return "提醒";
  return "阻断";
}

function getErrorMessage(error: unknown) {
  return error instanceof Error ? error.message : "请求失败";
}

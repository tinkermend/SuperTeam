import type { ReactNode } from "react";
import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
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
} from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Textarea } from "@/components/ui/textarea";
import { SemanticIconTile } from "@/components/superteam/liquid-components";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import type {
  DigitalEmployeeAvatarAsset,
  DigitalEmployeeCreateOptions,
  DigitalEmployeeRuntimeProviderOption,
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

const steps = ["身份", "能力", "治理", "运行"] as const;
type StepName = (typeof steps)[number];

type WizardDraft = {
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
};

export function CreateEmployeePage() {
  const apiBaseUrl = resolveControlPlaneUrl();

  return <CreateEmployeeView apiBaseUrl={apiBaseUrl} />;
}

export function CreateEmployeeView({ apiBaseUrl, fetcher }: CreateEmployeeViewProps) {
  const navigate = useNavigate();
  const [workbenchMode, setWorkbenchMode] = useState<"select" | "configure">("select");
  const [stepIndex, setStepIndex] = useState(0);
  const [draft, setDraft] = useState<WizardDraft>(emptyDraft);
  const [errors, setErrors] = useState<ValidationErrors>({});

  const teams = useQuery({
    queryKey: ["teams"],
    queryFn: () => listTeams({ baseUrl: apiBaseUrl, fetcher }),
  });

  useEffect(() => {
    const firstTeamId = teams.data?.find((team) => team.status === "active")?.id;
    if (!draft.team_id && firstTeamId) {
      setDraft((current) => ({ ...current, team_id: firstTeamId }));
    }
  }, [draft.team_id, teams.data]);

  const createOptions = useQuery({
    enabled: Boolean(draft.team_id),
    queryKey: ["digital-employee-create-options", draft.team_id],
    queryFn: () => getDigitalEmployeeCreateOptions({ baseUrl: apiBaseUrl, fetcher }, draft.team_id),
  });

  const avatarAssets = useQuery({
    queryKey: ["digital-employee-avatar-assets"],
    queryFn: () => listDigitalEmployeeAvatarAssets({ baseUrl: apiBaseUrl, fetcher }),
  });

  const selectedType = useMemo(
    () => createOptions.data?.employee_types.find((item) => item.type === draft.employee_type),
    [createOptions.data?.employee_types, draft.employee_type],
  );

  useEffect(() => {
    const optionsData = createOptions.data;
    const employeeTypes = optionsData?.employee_types ?? [];
    const firstType = firstPreferredEmployeeType(employeeTypes);
    if (!firstType) return;
    if (!draft.employee_type || !employeeTypes.some((item) => item.type === draft.employee_type)) {
      setDraft((current) => applyTypeDefaults(current, firstType));
    }
  }, [createOptions.data, draft.employee_type]);

  useEffect(() => {
    const firstAvatar = avatarAssets.data?.find((asset) => asset.status === "active");
    if (!draft.avatar_asset_id && firstAvatar) {
      setDraft((current) => ({ ...current, avatar_asset_id: firstAvatar.id }));
    }
  }, [avatarAssets.data, draft.avatar_asset_id]);

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
          avatar_asset_id: draft.avatar_asset_id,
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
          budget_policy: budgetPolicyFromDraft(draft),
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
  const teamOptions = useMemo(() => (teams.data ?? []).filter((team) => team.status === "active"), [teams.data]);
  const selectedTeam = teamOptions.find((team) => team.id === draft.team_id);

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

  function enterConfiguration() {
    setWorkbenchMode("configure");
    setStepIndex(0);
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
            <SemanticIconTile tone="primary" size="lg">
              <Bot />
            </SemanticIconTile>
            <div>
              <h1 className="text-2xl font-bold tracking-tight">创建数字员工</h1>
              <p className="text-sm text-muted-foreground">
                {workbenchMode === "select"
                  ? "选择创建路径，确认职责边界、治理策略和运行绑定后再进入配置。"
                  : "先定义职责画像、能力边界与运行约束，再生成 ready 员工。"}
              </p>
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            {workbenchMode === "configure" ? (
              <Button onClick={() => setWorkbenchMode("select")} type="button" variant="outline">
                <ArrowLeft data-icon="inline-start" />
                返回
              </Button>
            ) : (
              <Button asChild type="button" variant="outline">
                <Link to="/employees">
                  <ArrowLeft data-icon="inline-start" />
                  返回数字员工
                </Link>
              </Button>
            )}
            <Button
              disabled={
                workbenchMode === "configure" ||
                teamOptions.length === 0 ||
                createOptions.isLoading ||
                createOptions.isError ||
                !draft.employee_type
              }
              onClick={enterConfiguration}
              type="button"
            >
              进入配置
              <ChevronRight data-icon="inline-end" />
            </Button>
          </div>
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
        {avatarAssets.isError ? (
          <Alert className="mb-4" variant="destructive">
            <AlertTitle>头像库加载失败</AlertTitle>
            <AlertDescription>{getErrorMessage(avatarAssets.error)}</AlertDescription>
          </Alert>
        ) : null}

        <CreationStageProgress mode={workbenchMode} currentStep={currentStep} />

        {workbenchMode === "select" ? (
          <>
            <div className="grid gap-4 xl:grid-cols-[260px_minmax(0,1fr)_340px] min-[1760px]:grid-cols-[260px_minmax(0,1fr)_320px]">
              <CreationPathPanel />

              <TemplateSelectionPanel
                draft={draft}
                options={createOptions.data}
                selectedType={selectedType}
                onSelectType={selectType}
              />

              <CreationReadinessPanel
                draft={draft}
                options={createOptions.data}
                selectedTeamName={selectedTeam?.name}
                selectedType={selectedType}
                onEnterConfiguration={enterConfiguration}
              />
            </div>
            <CreationFactsBand />
          </>
        ) : (
          <div className="grid gap-4 xl:grid-cols-[260px_minmax(0,1fr)_340px]">
            <BlueprintSidebar
              draft={draft}
              options={createOptions.data}
              selectedType={selectedType}
              onSelectType={selectType}
            />

          <section className="min-w-0 rounded-md border bg-card/95 shadow-xs">
            <div className="border-b p-4">
              <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                <div>
                  <h2 className="text-lg font-semibold">员工画像蓝图</h2>
                  <p className="mt-1 text-sm text-muted-foreground">
                    按职责目标、可用能力、治理边界和运行绑定完成员工画像。
                  </p>
                </div>
                <StepTabs currentStep={currentStep} />
              </div>
            </div>

            <div className="grid gap-4 p-4">
              <TemplateOverview
                draft={draft}
                options={createOptions.data}
                selectedType={selectedType}
                onSelectType={selectType}
              />

              <div className="min-h-[420px] rounded-md border bg-background p-4">
                {teams.isLoading || avatarAssets.isLoading || (draft.team_id && createOptions.isLoading) ? (
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
                    onSelectType={selectType}
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
              <p className="px-4 text-sm text-destructive">{getErrorMessage(createEmployee.error)}</p>
            ) : null}
            <div className="flex justify-between gap-3 border-t p-4">
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
                  disabled={
                    teamOptions.length === 0 ||
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
                    teamOptions.length === 0 ||
                    createOptions.isLoading ||
                    createOptions.isError ||
                    avatarAssets.isLoading ||
                    avatarAssets.isError ||
                    !draft.avatar_asset_id ||
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
          </section>

          <CreationPreflightPanel
            draft={draft}
            options={createOptions.data}
            selectedType={selectedType}
          />
          </div>
        )}
      </Main>
    </>
  );
}

function CreationStageProgress({ currentStep, mode }: { currentStep: StepName; mode: "select" | "configure" }) {
  const activeIndex = mode === "select" ? 0 : currentStep === "运行" ? 2 : 1;
  const stages = [
    { title: "选择路径", description: "选择创建方式和专业模板" },
    { title: "预检治理", description: "检查治理策略和运行条件" },
    { title: "完成配置", description: "进入详细配置向导" },
  ];

  return (
    <section className="mb-4 rounded-md border bg-card/95 px-4 py-3 shadow-xs">
      <div className="grid gap-3 md:grid-cols-3">
        {stages.map((stage, index) => {
          const active = index === activeIndex;
          const done = index < activeIndex;

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

function CreationPathPanel() {
  const paths = [
    {
      title: "从专业模板创建",
      description: "按职责模板带出默认角色、能力建议和治理策略。",
      icon: Sparkles,
      active: true,
      badge: "推荐",
    },
    {
      title: "从团队角色复制",
      description: "复用团队内已验证的角色画像和能力边界。",
      icon: ClipboardCheck,
      active: false,
      badge: "可用",
    },
    {
      title: "从历史员工克隆",
      description: "基于已有员工配置生成新草稿，保留审计来源。",
      icon: GitBranch,
      active: false,
      badge: "可用",
    },
    {
      title: "空白自定义",
      description: "从空白身份开始逐项配置职责、能力和运行绑定。",
      icon: FileText,
      active: false,
      badge: "高级",
    },
  ];

  return (
    <aside className="rounded-md border bg-card/95 p-3 shadow-xs">
      <div className="mb-3 flex items-center gap-2 px-1">
        <SemanticIconTile tone="primary" size="sm">
          <Sparkles />
        </SemanticIconTile>
        <div>
          <h2 className="text-base font-semibold">创建路径</h2>
          <p className="text-xs text-muted-foreground">先选入口，再进入配置。</p>
        </div>
      </div>
      <div className="grid gap-2">
        {paths.map((path) => {
          const Icon = path.icon;
          return (
            <button
              aria-pressed={path.active}
              className={cn(
                "rounded-md border p-3 text-left transition",
                path.active
                  ? "border-primary/40 bg-primary/10 text-foreground shadow-xs"
                  : "border-border/70 bg-background/80 text-muted-foreground hover:border-primary/30 hover:bg-primary/5",
              )}
              key={path.title}
              type="button"
            >
              <span className="flex items-start gap-2">
                <span
                  className={cn(
                    "mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-md border",
                    path.active ? "border-primary/30 bg-primary/15 text-primary" : "bg-muted text-muted-foreground",
                  )}
                >
                  <Icon className="size-4" />
                </span>
                <span className="min-w-0 flex-1">
                  <span className="flex items-center justify-between gap-2">
                    <span className="text-sm font-medium">{path.title}</span>
                    <Badge variant={path.active ? "default" : "secondary"}>{path.badge}</Badge>
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
  selectedType,
  onSelectType,
}: {
  draft: WizardDraft;
  options?: DigitalEmployeeCreateOptions;
  selectedType?: DigitalEmployeeTypeOption;
  onSelectType: (value: string) => void;
}) {
  const employeeTypes = orderedEmployeeTypes(options?.employee_types ?? []);

  return (
    <section className="@container/template min-w-0 rounded-md border bg-card/95 p-4 shadow-xs">
      <div className="mb-4 flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h2 className="text-base font-semibold">选择专业类型</h2>
          <p className="mt-1 text-sm text-muted-foreground">选择最贴合业务场景的专业类型，系统将提供推荐配置。</p>
        </div>
        <Badge variant="secondary">全部模板 ({employeeTypes.length})</Badge>
      </div>
      {employeeTypes.length === 0 ? (
        <div className="flex min-h-[420px] items-center justify-center rounded-md border bg-muted/30 p-6 text-sm text-muted-foreground">
          当前团队治理配置未返回可用专业模板。
        </div>
      ) : (
        <div className="grid gap-3 @[640px]/template:grid-cols-2 @[980px]/template:grid-cols-3">
          {employeeTypes.map((typeOption) => (
            <TemplateCard
              key={typeOption.type}
              selected={typeOption.type === draft.employee_type}
              typeOption={typeOption}
              onSelect={() => onSelectType(typeOption.type)}
            />
          ))}
        </div>
      )}
      <div className="mt-3 text-sm text-muted-foreground">
        没有合适的模板？
        <button className="ml-2 font-medium text-primary hover:underline" type="button">
          选择空白自定义
        </button>
      </div>
      {selectedType ? <span className="sr-only">当前选择：{selectedType.label}</span> : null}
    </section>
  );
}

function TemplateCard({
  selected,
  typeOption,
  onSelect,
}: {
  selected: boolean;
  typeOption: DigitalEmployeeTypeOption;
  onSelect: () => void;
}) {
  const risk = String(typeOption.default_approval_policy?.min_risk_for_human ?? "medium");
  const providerLabel = typeOption.recommended_provider_types?.join(" / ") || "按团队策略";

  return (
    <button
      aria-pressed={selected}
      className={cn(
        "flex h-full min-h-[214px] flex-col rounded-md border p-3 text-left transition sm:p-4",
        selected ? "border-primary/70 bg-primary/5 shadow-xs ring-1 ring-primary/20" : "bg-background hover:border-primary/40",
      )}
      onClick={onSelect}
      type="button"
    >
      <span className="flex items-start justify-between gap-3">
        <span className="flex items-start gap-3">
          <span className="flex size-9 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
            <Code2 className="size-5" />
          </span>
          <span className="min-w-0">
            <span className="block text-base font-semibold">{typeOption.label}</span>
            <span className="mt-1 line-clamp-2 block text-sm leading-6 text-muted-foreground">{typeOption.description}</span>
          </span>
        </span>
        {selected ? (
          <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-primary text-primary-foreground">
            <Check className="size-3.5" />
          </span>
        ) : null}
      </span>
      <span className="mt-4 flex min-w-0 items-center gap-2 rounded-md border bg-muted/30 px-3 py-2 text-xs">
        <span className="text-muted-foreground">默认角色</span>
        <span className="truncate font-medium text-foreground">{typeOption.default_role || typeOption.type}</span>
      </span>
      <span className="mt-auto grid grid-cols-2 gap-x-3 gap-y-2 border-t pt-3 text-xs @[640px]/template:grid-cols-4 @[980px]/template:grid-cols-2">
        <MetricPill label="技能" value={String(typeOption.recommended_skills?.length ?? 0)} />
        <MetricPill label="MCP" value={String(typeOption.recommended_mcp_servers?.length ?? 0)} />
        <MetricPill label="风险" value={risk} tone={risk === "high" || risk === "critical" ? "warning" : "success"} />
        <MetricPill label="Provider" value={providerLabel} />
      </span>
    </button>
  );
}

function MetricPill({ label, tone, value }: { label: string; tone?: "success" | "warning"; value: string }) {
  return (
    <span className="min-w-0">
      <span className="block text-muted-foreground">{label}</span>
      <span
        className={cn(
          "mt-1 block truncate font-semibold text-foreground",
          tone === "success" ? "text-emerald-700 dark:text-emerald-300" : "",
          tone === "warning" ? "text-amber-700 dark:text-amber-300" : "",
        )}
      >
        {value}
      </span>
    </span>
  );
}

function CreationReadinessPanel({
  draft,
  options,
  selectedTeamName,
  selectedType,
  onEnterConfiguration,
}: {
  draft: WizardDraft;
  options?: DigitalEmployeeCreateOptions;
  selectedTeamName?: string;
  selectedType?: DigitalEmployeeTypeOption;
  onEnterConfiguration: () => void;
}) {
  return (
    <aside className="grid content-start gap-4">
      <CheckListPanel options={options} />
      <section className="rounded-md border bg-card/95 p-4 shadow-xs">
        <h2 className="text-base font-semibold">即将创建</h2>
        <p className="mt-1 text-xs text-muted-foreground">确认以下信息后进入详细配置。</p>
        <div className="mt-4 grid gap-2 text-sm">
          <InlineSummary label="归属团队" value={selectedTeamName || "未选择"} />
          <InlineSummary label="Owner" value="当前用户" />
          <InlineSummary label="专业类型" value={selectedType?.label ?? (draft.employee_type || "未选择")} />
          <InlineSummary label="默认角色" value={draft.role || selectedType?.default_role || "未生成"} />
          <InlineSummary label="推荐 Provider" value={selectedType?.recommended_provider_types?.join(" / ") || "按团队策略"} />
          <InlineSummary label="风险等级" value={draft.risk_level || "medium"} />
        </div>
        <Button className="mt-4 w-full" disabled={!draft.employee_type} onClick={onEnterConfiguration} type="button">
          确认并进入配置
          <ChevronRight data-icon="inline-end" />
        </Button>
      </section>
    </aside>
  );
}

function CheckListPanel({ options }: { options?: DigitalEmployeeCreateOptions }) {
  const checks = options?.creation_checks ?? [];

  return (
    <section className="rounded-md border bg-card/95 p-4 shadow-xs">
      <h2 className="text-base font-semibold">创建预检</h2>
      <p className="mt-1 text-xs text-muted-foreground">检查治理策略与运行条件。</p>
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

function InlineSummary({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-md border bg-background px-3 py-2">
      <span className="text-muted-foreground">{label}</span>
      <span className="max-w-[180px] truncate font-medium">{value}</span>
    </div>
  );
}

function CreationFactsBand() {
  const facts = [
    {
      title: "创建后进入 ready，不会自动执行任务",
      description: "可被任务、项目或流程调度调用",
      icon: Check,
    },
    {
      title: "后续由任务或项目调度",
      description: "支持手动发起或规则自动驱动执行",
      icon: ClipboardCheck,
    },
    {
      title: "所有选择写入审计",
      description: "便于追溯开启角色审查",
      icon: ShieldCheck,
    },
  ];

  return (
    <section className="mt-4 grid gap-3 rounded-md border bg-primary/5 p-4 md:grid-cols-3">
      {facts.map((fact) => {
        const Icon = fact.icon;
        return (
          <div className="flex items-center gap-3" key={fact.title}>
            <span className="flex size-10 shrink-0 items-center justify-center rounded-full border border-primary/25 bg-background text-primary">
              <Icon className="size-5" />
            </span>
            <span>
              <span className="block text-sm font-semibold">{fact.title}</span>
              <span className="mt-1 block text-xs text-muted-foreground">{fact.description}</span>
            </span>
          </div>
        );
      })}
    </section>
  );
}

function BlueprintSidebar({
  draft,
  options,
  selectedType,
  onSelectType,
}: {
  draft: WizardDraft;
  options?: DigitalEmployeeCreateOptions;
  selectedType?: DigitalEmployeeTypeOption;
  onSelectType: (value: string) => void;
}) {
  const employeeTypes = orderedEmployeeTypes(options?.employee_types ?? []);

  return (
    <aside className="rounded-md border bg-card/95 p-3 shadow-xs">
      <h2 className="px-1 text-base font-semibold">推荐起步画像</h2>
      <p className="mt-1 px-1 text-xs text-muted-foreground">切换画像会同步默认角色与能力建议。</p>
      <div className="mt-3 grid gap-2">
        {employeeTypes.map((typeOption) => (
          <button
            aria-pressed={typeOption.type === draft.employee_type}
            className={cn(
              "rounded-md border p-3 text-left transition",
              typeOption.type === draft.employee_type ? "border-primary/60 bg-primary/10" : "bg-background hover:border-primary/40",
            )}
            key={typeOption.type}
            onClick={() => onSelectType(typeOption.type)}
            type="button"
          >
            <span className="flex items-center justify-between gap-2">
              <span className="font-medium">{typeOption.label}</span>
              {typeOption.type === selectedType?.type ? <Check className="size-4 text-primary" /> : null}
            </span>
            <span className="mt-1 line-clamp-2 block text-xs leading-5 text-muted-foreground">{typeOption.description}</span>
          </button>
        ))}
        <button className="rounded-md border border-dashed bg-background p-3 text-sm text-muted-foreground" type="button">
          <Plus className="mr-2 inline size-4" />
          从空白开始自定义
        </button>
      </div>
    </aside>
  );
}

function StepTabs({ currentStep }: { currentStep: StepName }) {
  const currentIndex = steps.indexOf(currentStep);

  return (
    <div className="flex flex-wrap gap-2 rounded-md border bg-muted/30 p-1">
        {steps.map((step, index) => {
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

function TemplateOverview({
  draft,
  options,
  selectedType,
  onSelectType,
}: {
  draft: WizardDraft;
  options?: DigitalEmployeeCreateOptions;
  selectedType?: DigitalEmployeeTypeOption;
  onSelectType: (value: string) => void;
}) {
  const employeeTypes = orderedEmployeeTypes(options?.employee_types ?? []);
  if (employeeTypes.length === 0) {
    return (
      <section className="rounded-md border bg-muted/30 p-4">
        <h2 className="text-base font-semibold">专业模板</h2>
        <p className="mt-1 text-sm text-muted-foreground">当前团队治理配置未返回可用专业模板。</p>
      </section>
    );
  }

  return (
    <section className="rounded-md border bg-background p-4">
      <div className="mb-3 flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 className="text-base font-semibold">专业模板</h2>
          <p className="text-sm text-muted-foreground">模板只提供默认值和推荐能力，最终提交仍由控制平面校验。</p>
        </div>
        <Badge variant="secondary">{(selectedType?.label ?? draft.employee_type) || "未选择"}</Badge>
      </div>
      <div className="grid gap-3 md:grid-cols-2">
        {employeeTypes.map((typeOption) => {
          const selected = typeOption.type === draft.employee_type;
          return (
            <button
              aria-pressed={selected}
              className={cn(
                "rounded-md border p-3 text-left transition",
                selected ? "border-primary/50 bg-primary/10 shadow-xs" : "bg-card hover:border-primary/40",
              )}
              key={typeOption.type}
              onClick={() => onSelectType(typeOption.type)}
              type="button"
            >
              <span className="flex items-start justify-between gap-3">
                <span>
                  <span className="block text-sm font-semibold">{typeOption.label}</span>
                  <span className="mt-1 line-clamp-2 block text-xs leading-5 text-muted-foreground">
                    {typeOption.description}
                  </span>
                </span>
                {selected ? <Check className="size-4 shrink-0 text-primary" /> : null}
              </span>
              <span className="mt-3 flex flex-wrap gap-1.5">
                <Badge variant="secondary">技能 {typeOption.recommended_skills?.length ?? 0}</Badge>
                <Badge variant="secondary">MCP {typeOption.recommended_mcp_servers?.length ?? 0}</Badge>
                <Badge variant="secondary">Provider {(typeOption.recommended_provider_types ?? []).join(", ") || "按团队"}</Badge>
              </span>
            </button>
          );
        })}
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
  const checks = options?.creation_checks ?? [];
  const runtimeOptions = options?.runtime_provider_options ?? [];
  const availableRuntimeCount = runtimeOptions.filter((option) => option.available).length;

  return (
    <aside className="grid content-start gap-4">
      <section className="rounded-md border bg-card/95 p-4 shadow-xs">
        <div className="mb-3 flex items-center gap-2">
          <SemanticIconTile tone="success" size="sm">
            <ShieldCheck />
          </SemanticIconTile>
          <div>
            <h2 className="text-base font-semibold">创建预检</h2>
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
          <SemanticIconTile tone="artifact" size="sm">
            <Gauge />
          </SemanticIconTile>
          <div>
            <h2 className="text-base font-semibold">画像摘要</h2>
            <p className="text-xs text-muted-foreground">随配置实时更新。</p>
          </div>
        </div>
        <div className="grid gap-3 text-sm">
          <SummaryItem label="专业类型" value={(selectedType?.label ?? draft.employee_type) || "未选择"} />
          <SummaryItem label="角色" value={draft.role || "未填写"} />
          <SummaryItem label="风险等级" value={draft.risk_level || "medium"} />
          <SummaryItem
            label="能力选择"
            value={`技能 ${draft.capability_selection.enabled_skills.length} · MCP ${draft.capability_selection.enabled_mcp_servers.length} · 外部 ${draft.capability_selection.enabled_external_capabilities.length}`}
          />
          <SummaryItem label="Runtime" value={draft.runtime_binding || `${availableRuntimeCount}/${runtimeOptions.length} 可用`} />
        </div>
      </section>

      <section className="rounded-md border bg-muted/30 p-4 text-xs leading-5 text-muted-foreground">
        <div className="mb-2 flex items-center gap-2 font-medium text-foreground">
          <Cpu className="size-4 text-primary" />
          创建后事实
        </div>
        <div className="grid gap-2">
          <div>1. 写入身份与初始配置修订</div>
          <div>2. 绑定 Runtime 执行实例</div>
          <div>3. 进入 ready，等待任务调度</div>
        </div>
      </section>
    </aside>
  );
}

function IdentityStep({
  avatarAssets,
  draft,
  errors,
  options,
  selectedType,
  teamOptions,
  onSelectType,
  onSelectAvatar,
  onUpdate,
}: {
  avatarAssets: DigitalEmployeeAvatarAsset[];
  draft: WizardDraft;
  errors: ValidationErrors;
  options?: DigitalEmployeeCreateOptions;
  selectedType?: DigitalEmployeeTypeOption;
  teamOptions: Array<{ id: string; name: string }>;
  onSelectType: (value: string) => void;
  onSelectAvatar: (value: string) => void;
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
          <div className="mt-2 text-muted-foreground">默认角色：{selectedType.default_role || selectedType.type}</div>
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
        <SummaryItem label="风险触发" value={draft.risk_level} />
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
  "每日 Token 预算上限": "daily-token-limit",
};

const selectClassName =
  "h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-xs outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50";

const preferredEmployeeTypeOrder = [
  "frontend_engineer",
  "backend_engineer",
  "database_admin",
  "devops_engineer",
  "fullstack_engineer",
  "implementation_engineer",
  "general_engineer",
];

function firstPreferredEmployeeType(employeeTypes: DigitalEmployeeTypeOption[]) {
  return orderedEmployeeTypes(employeeTypes)[0];
}

function orderedEmployeeTypes(employeeTypes: DigitalEmployeeTypeOption[]) {
  return [...employeeTypes].sort((left, right) => {
    const leftIndex = preferredEmployeeTypeOrder.indexOf(left.type);
    const rightIndex = preferredEmployeeTypeOrder.indexOf(right.type);
    const normalizedLeft = leftIndex === -1 ? Number.MAX_SAFE_INTEGER : leftIndex;
    const normalizedRight = rightIndex === -1 ? Number.MAX_SAFE_INTEGER : rightIndex;

    if (normalizedLeft !== normalizedRight) {
      return normalizedLeft - normalizedRight;
    }
    return employeeTypes.indexOf(left) - employeeTypes.indexOf(right);
  });
}

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

function validateStep(step: StepName, draft: WizardDraft): ValidationErrors {
  if (step === "身份") {
    const errors: ValidationErrors = {};
    if (!draft.team_id.trim()) errors.team_id = "团队不能为空";
    if (!draft.avatar_asset_id.trim()) errors.avatar_asset_id = "头像不能为空";
    if (!draft.employee_type.trim()) errors.employee_type = "员工类型不能为空";
    if (!draft.name.trim()) errors.name = "名称不能为空";
    if (!draft.role.trim()) errors.role = "角色不能为空";
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
  if (step === "运行" && !draft.runtime_binding) {
    return { runtime: "请选择 Runtime" };
  }
  return {};
}

function budgetPolicyFromDraft(draft: WizardDraft) {
  const trimmed = draft.daily_token_limit.trim();
  if (!trimmed) return {};
  return { daily_token_limit: Number(trimmed) };
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

function checkDotClassName(status: string) {
  if (status === "passed") return "bg-[color:var(--superteam-success)]";
  if (status === "warning") return "bg-[color:var(--superteam-warning)]";
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

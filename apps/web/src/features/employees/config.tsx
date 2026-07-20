import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Eye, Pencil, Save, Send, ShieldCheck, X } from "lucide-react";
import { useEffect, useState } from "react";
import { Main } from "@/components/layout/main";
import {
  ShellPageHeader,
  ShellPageHeaderBack,
} from "@/components/layout/shell-page-header";
import { MarkdownProse, SoftCard, StatusPill } from "@/components/superteam";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  createDigitalEmployeeConfigRevision,
  getDigitalEmployee,
  submitEmployeePermissionChange,
  type CapabilityBindings,
  type CreateDigitalEmployeeConfigRevisionInput,
  type DigitalEmployee,
  type SubmitPermissionChangeInput,
} from "@/lib/api/employees";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { riskLevelLabel, statusLabel } from "@/lib/status-labels";
import { EmployeeCapabilitiesPanel } from "./components/employee-capabilities-panel";
import { providerDisplayName } from "./provider-label";

export function EmployeeConfigPage({ employeeId }: { employeeId: string }) {
  const apiBaseUrl = resolveControlPlaneUrl();
  return <EmployeeConfigView apiBaseUrl={apiBaseUrl} employeeId={employeeId} />;
}

type EmployeeConfigViewProps = {
  apiBaseUrl: string;
  employeeId: string;
  fetcher?: typeof fetch;
};

// 提交时只覆盖 external_capabilities / environment_variable_refs 两个受管数组；
// 其余未知键透传（向后兼容 [key: string]: unknown）。skills / mcp_servers 是已废弃的
// 逻辑绑定键，服务端会拒绝非空回传，故一并从透传集中剥离，绝不重新发送。
const RESERVED_CAPABILITY_KEYS = [
  "external_capabilities",
  "environment_variable_refs",
  "skills",
  "mcp_servers",
];

export function EmployeeConfigView({ apiBaseUrl, employeeId, fetcher }: EmployeeConfigViewProps) {
  const apiOptions = { baseUrl: apiBaseUrl, fetcher };
  const queryClient = useQueryClient();

  // 层一 · 即时生效字段
  const [personaMemoryMarkdown, setPersonaMemoryMarkdown] = useState("");
  const [personaPreview, setPersonaPreview] = useState(false);
  const [externalCapabilities, setExternalCapabilities] = useState<string[]>([]);
  const [environmentVariableRefs, setEnvironmentVariableRefs] = useState<string[]>([]);
  const [dailyTokenLimit, setDailyTokenLimit] = useState("");
  const [otherCapabilityKeys, setOtherCapabilityKeys] = useState<Record<string, unknown>>({});

  const [immediateDirty, setImmediateDirty] = useState(false);
  const [budgetError, setBudgetError] = useState("");
  const [hydratedEmployeeId, setHydratedEmployeeId] = useState("");

  const employee = useQuery({
    queryKey: ["digital-employee", employeeId],
    queryFn: () => getDigitalEmployee(apiOptions, employeeId),
  });

  const createRevision = useMutation({
    mutationFn: (input: CreateDigitalEmployeeConfigRevisionInput) =>
      createDigitalEmployeeConfigRevision(apiOptions, employeeId, input),
    onSuccess: () => {
      setImmediateDirty(false);
      setBudgetError("");
      queryClient.invalidateQueries({ queryKey: ["digital-employee", employeeId] });
    },
  });

  useEffect(() => {
    if (!employee.data || hydratedEmployeeId === employee.data.id) return;
    const bindings = employee.data.capability_bindings ?? {};
    setPersonaMemoryMarkdown(employee.data.persona_memory_markdown ?? "");
    setExternalCapabilities(stringArray(bindings.external_capabilities));
    setEnvironmentVariableRefs(stringArray(bindings.environment_variable_refs));
    setOtherCapabilityKeys(
      Object.fromEntries(
        Object.entries(bindings).filter(([key]) => !RESERVED_CAPABILITY_KEYS.includes(key)),
      ),
    );
    setDailyTokenLimit(budgetPolicyValue(employee.data.budget_policy ?? {}));
    setImmediateDirty(false);
    setBudgetError("");
    setHydratedEmployeeId(employee.data.id);
  }, [employee.data, hydratedEmployeeId]);

  const markImmediateDirty = () => setImmediateDirty(true);

  const handleImmediateSubmit = (event: React.FormEvent) => {
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

    const input: CreateDigitalEmployeeConfigRevisionInput = {
      persona_memory_markdown: personaMemoryMarkdown.trim(),
      capability_bindings: capabilityBindings,
      budget_policy: budgetPolicy,
    };
    createRevision.mutate(input);
  };

  return (
    <>
      <ShellPageHeader
        back={
          <ShellPageHeaderBack
            ariaLabel="返回数字员工详情"
            params={{ employeeId }}
            to="/employees/$employeeId"
          />
        }
        title={employee.data?.name ?? "数字员工配置"}
        subtitle="即时生效配置与权限审批配置分层管理"
      />
      <Main width="contained" className="space-y-4">
        {employee.isLoading ? <p className="text-sm text-v3-ink-2">加载中</p> : null}
        {employee.isError ? <p className="text-sm text-destructive">加载失败</p> : null}

        {employee.data ? (
          <>
            <LocatorHeader employee={employee.data} />

            <section className="space-y-3">
              <TierHeading
                title="即时生效配置"
                hint="保存后即时生效，无需审批"
              />

              <form className="space-y-4" noValidate onSubmit={handleImmediateSubmit}>
                <SoftCard className="space-y-3 p-5">
                  <div className="flex items-center justify-between">
                    <div className="text-sm font-semibold text-v3-ink">人格记忆.md</div>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => setPersonaPreview((value) => !value)}
                    >
                      {personaPreview ? <Pencil /> : <Eye />}
                      {personaPreview ? "编辑" : "预览"}
                    </Button>
                  </div>
                  {personaPreview ? (
                    personaMemoryMarkdown.trim() ? (
                      <MarkdownProse className="rounded-[14px] border border-v3-line bg-v3-card-soft p-3">
                        {personaMemoryMarkdown}
                      </MarkdownProse>
                    ) : (
                      <p className="rounded-[14px] border border-v3-line bg-v3-card-soft p-3 text-sm text-v3-ink-3">
                        未设置
                      </p>
                    )
                  ) : (
                    <Textarea
                      id="persona-memory-markdown"
                      aria-label="人格记忆.md"
                      value={personaMemoryMarkdown}
                      onChange={(event) => {
                        setPersonaMemoryMarkdown(event.target.value);
                        markImmediateDirty();
                      }}
                      rows={10}
                      className="font-mono text-xs"
                    />
                  )}
                  <p className="text-xs text-v3-ink-3">
                    人格记忆随任务注入；项目宪法由所属项目在执行时注入，不属于数字员工配置。
                  </p>
                </SoftCard>

                <SoftCard className="space-y-4 p-5">
                  <div className="text-sm font-semibold text-v3-ink">能力绑定</div>
                  <ChipsEditor
                    label="外部能力（external_capabilities）"
                    placeholder="输入能力标识后回车添加"
                    values={externalCapabilities}
                    onChange={(next) => {
                      setExternalCapabilities(next);
                      markImmediateDirty();
                    }}
                  />
                  <ChipsEditor
                    label="环境变量引用（environment_variable_refs）"
                    placeholder="输入环境变量名后回车添加"
                    values={environmentVariableRefs}
                    onChange={(next) => {
                      setEnvironmentVariableRefs(next);
                      markImmediateDirty();
                    }}
                  />
                  <p className="text-xs text-v3-ink-3">
                    技能与 MCP 是逻辑绑定，请在下方「技能 / MCP / 环境变量」区管理。
                  </p>
                </SoftCard>

                <SoftCard className="space-y-2 p-5">
                  <div className="text-sm font-semibold text-v3-ink">预算策略</div>
                  <Label htmlFor="config-daily-token-limit" className="text-xs text-v3-ink-3">
                    每日 Token 预算上限
                  </Label>
                  <Input
                    id="config-daily-token-limit"
                    inputMode="numeric"
                    min={1}
                    onChange={(event) => {
                      setDailyTokenLimit(event.target.value);
                      markImmediateDirty();
                      setBudgetError("");
                    }}
                    placeholder="不填写表示无预算上限"
                    type="number"
                    aria-invalid={Boolean(budgetError)}
                    value={dailyTokenLimit}
                  />
                  {budgetError ? <p className="text-sm text-destructive">{budgetError}</p> : null}
                </SoftCard>

                <div className="flex items-center gap-3">
                  <Button type="submit" disabled={!immediateDirty || createRevision.isPending}>
                    <Save />
                    保存即时配置
                  </Button>
                  {createRevision.isSuccess ? (
                    <p className="text-sm text-green-600">已保存并生效</p>
                  ) : null}
                  {createRevision.isError ? (
                    <p className="text-sm text-destructive">保存失败</p>
                  ) : null}
                </div>
                <p className="text-xs text-v3-ink-3">
                  人格记忆 / 能力 / 预算保存后即时生效为新配置版本，无需审批；角色 / 权限变更走下方「权限审批配置」。
                </p>
              </form>
            </section>

            <section className="space-y-3">
              <TierHeading title="技能 / MCP / 环境变量" hint="即时生效，无需审批" />
              <EmployeeCapabilitiesPanel apiOptions={apiOptions} employeeId={employeeId} />
            </section>

            <PermissionTierSection apiOptions={apiOptions} employee={employee.data} />
          </>
        ) : null}
      </Main>
    </>
  );
}

function LocatorHeader({ employee }: { employee: DigitalEmployee }) {
  const effectiveStatus = employee.metadata?.effective_config_status;
  return (
    <SoftCard className="space-y-3 p-5">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-base font-semibold text-v3-ink">{employee.name}</span>
        <span className="font-mono text-xs text-v3-ink-3">{employee.id}</span>
        <StatusPill tone={statusTone(employee.status)}>{statusLabel(employee.status)}</StatusPill>
      </div>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <LocatorItem label="Provider（不可改）" value={providerDisplayName(employee.provider_type)} />
        <LocatorItem label="角色" value={employee.role || "未设置"} />
        <LocatorItem label="风险等级" value={riskLevelLabel(employee.risk_level)} />
        <LocatorItem label="所属团队" value={employee.team_id ? "已分配" : "未分配"} />
      </div>
      {effectiveStatus ? (
        <p className="text-xs text-v3-ink-3">
          当前生效配置：{employee.metadata?.effective_config_label ?? "—"}（{statusLabel(effectiveStatus)}）
        </p>
      ) : null}
    </SoftCard>
  );
}

function LocatorItem({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <p className="text-[11px] text-v3-ink-3">{label}</p>
      <p className="truncate text-sm font-medium text-v3-ink">{value}</p>
    </div>
  );
}

function TierHeading({ title, hint }: { title: string; hint: string }) {
  return (
    <div className="flex flex-wrap items-baseline gap-2">
      <h2 className="text-sm font-semibold text-v3-ink">{title}</h2>
      <span className="text-xs text-v3-ink-3">{hint}</span>
    </div>
  );
}

function PermissionTierSection({
  apiOptions,
  employee,
}: {
  apiOptions: ApiClientOptions;
  employee: DigitalEmployee;
}) {
  const queryClient = useQueryClient();
  const permissionPolicy = employee.permission_policy ?? {};
  const currentGrants = stringArray(permissionPolicy.grants);
  const currentAllowedActions = stringArray(
    (permissionPolicy as Record<string, unknown>).allowed_actions,
  );

  const [role, setRole] = useState(employee.role ?? "");
  const [grants, setGrants] = useState<string[]>(currentGrants);
  const [allowedActions, setAllowedActions] = useState<string[]>(currentAllowedActions);
  const [dirty, setDirty] = useState(false);
  const [hydratedEmployeeId, setHydratedEmployeeId] = useState(employee.id);

  // 员工切换/服务端数据刷新后重水合(与即时层同思路,但避免覆盖在编辑内容)。
  useEffect(() => {
    if (hydratedEmployeeId === employee.id && dirty) return;
    setRole(employee.role ?? "");
    setGrants(stringArray((employee.permission_policy ?? {}).grants));
    setAllowedActions(
      stringArray(((employee.permission_policy ?? {}) as Record<string, unknown>).allowed_actions),
    );
    setDirty(false);
    setHydratedEmployeeId(employee.id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [employee.id, employee.role, employee.permission_policy]);

  const submitChange = useMutation({
    mutationFn: (input: SubmitPermissionChangeInput) =>
      submitEmployeePermissionChange(apiOptions, employee.id, input),
    onSuccess: () => {
      setDirty(false);
      queryClient.invalidateQueries({ queryKey: ["digital-employee", employee.id] });
    },
  });

  const handleSubmit = (event: React.FormEvent) => {
    event.preventDefault();
    const input: SubmitPermissionChangeInput = {};
    if (role.trim() && role.trim() !== employee.role) {
      input.role = role.trim();
    }
    const policyChanged =
      !sameStringArray(grants, currentGrants) ||
      !sameStringArray(allowedActions, currentAllowedActions);
    if (policyChanged) {
      input.permission_policy = {
        ...permissionPolicy,
        grants,
        allowed_actions: allowedActions,
      };
    }
    if (!input.role && !input.permission_policy) return;
    submitChange.mutate(input);
  };

  return (
    <section className="space-y-3">
      <TierHeading title="权限审批配置" hint="变更需权限中心审批，批准后生效" />
      <form onSubmit={handleSubmit}>
        <SoftCard className="space-y-4 p-5">
          <div className="flex items-center gap-2">
            <ShieldCheck className="size-4 text-v3-ink-2" />
            <span className="text-sm font-semibold text-v3-ink">角色与权限</span>
          </div>
          <div className="space-y-2">
            <Label htmlFor="permission-role" className="text-xs text-v3-ink-3">
              角色（role）· 当前：{employee.role || "未设置"}
            </Label>
            <Input
              id="permission-role"
              value={role}
              onChange={(event) => {
                setRole(event.target.value);
                setDirty(true);
              }}
            />
          </div>
          <ChipsEditor
            label="资源授权（grants）· scope:resource 形式"
            placeholder="例如 database.read:dev_db，回车添加"
            values={grants}
            onChange={(next) => {
              setGrants(next);
              setDirty(true);
            }}
          />
          <ChipsEditor
            label="动作白名单（allowed_actions）· 员工可执行动作上限，留空不收敛"
            placeholder="例如 code.write，回车添加"
            values={allowedActions}
            onChange={(next) => {
              setAllowedActions(next);
              setDirty(true);
            }}
          />
          <div className="flex flex-wrap items-center gap-3">
            <Button type="submit" disabled={!dirty || submitChange.isPending}>
              <Send />
              提交权限变更
            </Button>
            {submitChange.isSuccess ? (
              <p className="text-sm text-green-600">
                已提交，待权限中心审批。
                <Link className="ml-1 underline" to="/permissions">
                  去权限中心
                </Link>
              </p>
            ) : null}
            {submitChange.isError ? (
              <p className="text-sm text-destructive">
                {submitPermissionErrorMessage(submitChange.error)}
              </p>
            ) : null}
          </div>
          <p className="text-xs text-v3-ink-3">
            提交后生成权限审批请求，由团队审批人在权限中心批准后写回生效；员工有进行中工作时会被拒绝提交。
          </p>
        </SoftCard>
      </form>
    </section>
  );
}

function sameStringArray(a: string[], b: string[]) {
  return a.length === b.length && a.every((value, index) => value === b[index]);
}

function submitPermissionErrorMessage(error: unknown) {
  const message = error instanceof Error ? error.message : "";
  if (message.includes("active work")) {
    return "员工当前有进行中的工作，请等待完成后再提交权限变更。";
  }
  if (message.includes("not configured")) {
    return "权限审批链路未配置，请联系管理员。";
  }
  if (message.includes("team")) {
    return "员工需先归属团队才能提交权限变更。";
  }
  return "提交失败，请稍后重试。";
}

function ChipsEditor({
  label,
  placeholder,
  values,
  onChange,
}: {
  label: string;
  placeholder: string;
  values: string[];
  onChange: (next: string[]) => void;
}) {
  const [draft, setDraft] = useState("");

  const commit = () => {
    const trimmed = draft.trim();
    if (!trimmed || values.includes(trimmed)) {
      setDraft("");
      return;
    }
    onChange([...values, trimmed]);
    setDraft("");
  };

  return (
    <div className="space-y-2">
      <Label className="text-xs text-v3-ink-3">{label}</Label>
      {values.length ? (
        <div className="flex flex-wrap gap-1.5">
          {values.map((value) => (
            <span
              key={value}
              className="inline-flex items-center gap-1 rounded-v3-inner border border-v3-line bg-v3-card px-2 py-1 text-xs text-v3-ink"
            >
              <span className="font-mono">{value}</span>
              <button
                type="button"
                aria-label={`移除 ${value}`}
                className="text-v3-ink-3 hover:text-v3-danger"
                onClick={() => onChange(values.filter((item) => item !== value))}
              >
                <X className="size-3" />
              </button>
            </span>
          ))}
        </div>
      ) : null}
      <Input
        value={draft}
        placeholder={placeholder}
        onChange={(event) => setDraft(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Enter") {
            event.preventDefault();
            commit();
          }
        }}
        onBlur={commit}
      />
    </div>
  );
}

function stringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.filter((item): item is string => typeof item === "string");
}

function statusTone(status: DigitalEmployee["status"]) {
  if (status === "ready" || status === "active") return "ok" as const;
  if (status === "error") return "danger" as const;
  if (status === "disabled") return "mute" as const;
  return "info" as const;
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

function budgetPolicyFromDailyTokenLimit(dailyTokenLimit: string) {
  const trimmed = dailyTokenLimit.trim();
  if (!trimmed) return {};

  const parsed = Number(trimmed);
  if (!Number.isInteger(parsed) || parsed <= 0) {
    return undefined;
  }

  return { daily_token_limit: parsed };
}

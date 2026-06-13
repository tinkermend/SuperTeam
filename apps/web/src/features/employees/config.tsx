import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowLeft, Bot, Save } from "lucide-react";
import { useState } from "react";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { LiquidTabsList, LiquidTabsTrigger, SemanticIconTile } from "@/components/superteam/liquid-components";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import {
  createDigitalEmployeeConfigRevision,
  getDigitalEmployee,
  type CreateDigitalEmployeeConfigRevisionInput,
} from "@/lib/api/employees";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { EmployeeCapabilitiesPanel } from "./components/employee-capabilities-panel";
import { InstructionFilesPanel } from "./components/instruction-files-panel";

export function EmployeeConfigPage({ employeeId }: { employeeId: string }) {
  const apiBaseUrl = resolveControlPlaneUrl();
  return <EmployeeConfigView apiBaseUrl={apiBaseUrl} employeeId={employeeId} />;
}

type EmployeeConfigViewProps = {
  apiBaseUrl: string;
  employeeId: string;
  fetcher?: typeof fetch;
};

type AdvancedJsonFieldKey = Exclude<
  keyof CreateDigitalEmployeeConfigRevisionInput,
  "budget_policy" | "status"
>;

const advancedJsonFieldLabels: Record<AdvancedJsonFieldKey, string> = {
  role_profile: "Role Profile",
  constitution_addendum: "Constitution Addendum",
  capability_selection: "Capability Selection",
  context_policy_override: "Context Policy Override",
  approval_policy_override: "Approval Policy Override",
  output_contract_addendum: "Output Contract Addendum",
};

const createAdvancedJsonFieldState = <T,>(value: T) => ({
  role_profile: value,
  constitution_addendum: value,
  capability_selection: value,
  context_policy_override: value,
  approval_policy_override: value,
  output_contract_addendum: value,
});

export function EmployeeConfigView({ apiBaseUrl, employeeId, fetcher }: EmployeeConfigViewProps) {
  const apiOptions = { baseUrl: apiBaseUrl, fetcher };
  const queryClient = useQueryClient();

  const [roleProfile, setRoleProfile] = useState("{}");
  const [constitutionAddendum, setConstitutionAddendum] = useState("{}");
  const [capabilitySelection, setCapabilitySelection] = useState("{}");
  const [contextPolicyOverride, setContextPolicyOverride] = useState("{}");
  const [approvalPolicyOverride, setApprovalPolicyOverride] = useState("{}");
  const [outputContractAddendum, setOutputContractAddendum] = useState("{}");
  const [dailyTokenLimit, setDailyTokenLimit] = useState("");
  const [advancedDirty, setAdvancedDirty] = useState<Record<AdvancedJsonFieldKey, boolean>>(
    createAdvancedJsonFieldState(false),
  );
  const [advancedErrors, setAdvancedErrors] = useState<Record<AdvancedJsonFieldKey, string>>(
    createAdvancedJsonFieldState(""),
  );
  const [budgetDirty, setBudgetDirty] = useState(false);
  const [budgetError, setBudgetError] = useState("");

  const employee = useQuery({
    queryKey: ["digital-employee", employeeId],
    queryFn: () => getDigitalEmployee(apiOptions, employeeId),
  });

  const createRevision = useMutation({
    mutationFn: (input: CreateDigitalEmployeeConfigRevisionInput) =>
      createDigitalEmployeeConfigRevision(apiOptions, employeeId, input),
    onSuccess: () => {
      setAdvancedDirty(createAdvancedJsonFieldState(false));
      setAdvancedErrors(createAdvancedJsonFieldState(""));
      setBudgetDirty(false);
      setBudgetError("");
      queryClient.invalidateQueries({ queryKey: ["digital-employee", employeeId] });
    },
  });

  const advancedJsonFields = [
    { key: "role_profile", value: roleProfile },
    { key: "constitution_addendum", value: constitutionAddendum },
    { key: "capability_selection", value: capabilitySelection },
    { key: "context_policy_override", value: contextPolicyOverride },
    { key: "approval_policy_override", value: approvalPolicyOverride },
    { key: "output_contract_addendum", value: outputContractAddendum },
  ] satisfies { key: AdvancedJsonFieldKey; value: string }[];
  const hasDirtyConfig = budgetDirty || Object.values(advancedDirty).some(Boolean);

  const updateAdvancedField = (
    key: AdvancedJsonFieldKey,
    value: string,
    setValue: React.Dispatch<React.SetStateAction<string>>,
  ) => {
    setValue(value);
    setAdvancedDirty((current) => ({ ...current, [key]: true }));
    setAdvancedErrors((current) => ({ ...current, [key]: "" }));
  };

  const handleSubmit = (event: React.FormEvent) => {
    event.preventDefault();
    const input: CreateDigitalEmployeeConfigRevisionInput = { status: "draft" };
    const nextAdvancedErrors = createAdvancedJsonFieldState("");
    let hasAdvancedError = false;

    advancedJsonFields.forEach((field) => {
      if (!advancedDirty[field.key]) return;

      try {
        input[field.key] = JSON.parse(field.value);
      } catch {
        nextAdvancedErrors[field.key] = `${advancedJsonFieldLabels[field.key]} 必须是有效 JSON`;
        hasAdvancedError = true;
      }
    });

    setAdvancedErrors(nextAdvancedErrors);
    if (hasAdvancedError) return;

    setBudgetError("");
    if (budgetDirty) {
      const budgetPolicy = budgetPolicyFromDailyTokenLimit(dailyTokenLimit);
      if (!budgetPolicy) {
        setBudgetError("每日 Token 预算上限必须是正整数");
        return;
      }
      input.budget_policy = budgetPolicy;
    }

    createRevision.mutate(input);
  };

  const advancedConfigForm = (
    <form className="space-y-4" noValidate onSubmit={handleSubmit}>
      <Card>
        <CardHeader>
          <CardTitle>角色配置</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div>
            <Label htmlFor="role-profile">Role Profile (JSON)</Label>
            <Textarea
              id="role-profile"
              value={roleProfile}
              onChange={(e) => updateAdvancedField("role_profile", e.target.value, setRoleProfile)}
              rows={4}
              className="font-mono text-xs"
              aria-invalid={Boolean(advancedErrors.role_profile)}
            />
            {advancedErrors.role_profile ? (
              <p className="text-sm text-destructive">{advancedErrors.role_profile}</p>
            ) : null}
          </div>
          <div>
            <Label htmlFor="constitution">Constitution Addendum (JSON)</Label>
            <Textarea
              id="constitution"
              value={constitutionAddendum}
              onChange={(e) =>
                updateAdvancedField("constitution_addendum", e.target.value, setConstitutionAddendum)
              }
              rows={4}
              className="font-mono text-xs"
              aria-invalid={Boolean(advancedErrors.constitution_addendum)}
            />
            {advancedErrors.constitution_addendum ? (
              <p className="text-sm text-destructive">{advancedErrors.constitution_addendum}</p>
            ) : null}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>能力与策略</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div>
            <Label htmlFor="capability">Capability Selection (JSON)</Label>
            <Textarea
              id="capability"
              value={capabilitySelection}
              onChange={(e) =>
                updateAdvancedField("capability_selection", e.target.value, setCapabilitySelection)
              }
              rows={4}
              className="font-mono text-xs"
              aria-invalid={Boolean(advancedErrors.capability_selection)}
            />
            {advancedErrors.capability_selection ? (
              <p className="text-sm text-destructive">{advancedErrors.capability_selection}</p>
            ) : null}
          </div>
          <div>
            <Label htmlFor="context-policy">Context Policy Override (JSON)</Label>
            <Textarea
              id="context-policy"
              value={contextPolicyOverride}
              onChange={(e) =>
                updateAdvancedField("context_policy_override", e.target.value, setContextPolicyOverride)
              }
              rows={4}
              className="font-mono text-xs"
              aria-invalid={Boolean(advancedErrors.context_policy_override)}
            />
            {advancedErrors.context_policy_override ? (
              <p className="text-sm text-destructive">{advancedErrors.context_policy_override}</p>
            ) : null}
          </div>
          <div>
            <Label htmlFor="approval-policy">Approval Policy Override (JSON)</Label>
            <Textarea
              id="approval-policy"
              value={approvalPolicyOverride}
              onChange={(e) =>
                updateAdvancedField("approval_policy_override", e.target.value, setApprovalPolicyOverride)
              }
              rows={4}
              className="font-mono text-xs"
              aria-invalid={Boolean(advancedErrors.approval_policy_override)}
            />
            {advancedErrors.approval_policy_override ? (
              <p className="text-sm text-destructive">{advancedErrors.approval_policy_override}</p>
            ) : null}
          </div>
          <div>
            <Label htmlFor="output-contract">Output Contract Addendum (JSON)</Label>
            <Textarea
              id="output-contract"
              value={outputContractAddendum}
              onChange={(e) =>
                updateAdvancedField("output_contract_addendum", e.target.value, setOutputContractAddendum)
              }
              rows={4}
              className="font-mono text-xs"
              aria-invalid={Boolean(advancedErrors.output_contract_addendum)}
            />
            {advancedErrors.output_contract_addendum ? (
              <p className="text-sm text-destructive">{advancedErrors.output_contract_addendum}</p>
            ) : null}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>预算策略</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-2">
          <Label htmlFor="config-daily-token-limit">每日 Token 预算上限</Label>
          <Input
            id="config-daily-token-limit"
            inputMode="numeric"
            min={1}
            onChange={(event) => {
              setDailyTokenLimit(event.target.value);
              setBudgetDirty(true);
              setBudgetError("");
            }}
            placeholder="不填写表示无预算上限"
            type="number"
            aria-invalid={Boolean(budgetError)}
            value={dailyTokenLimit}
          />
          {budgetError ? <p className="text-sm text-destructive">{budgetError}</p> : null}
          <p className="text-xs text-muted-foreground">预算会进入新的配置版本，批准后生效。</p>
        </CardContent>
      </Card>

      <div className="flex gap-3">
        <Button type="submit" disabled={!hasDirtyConfig || createRevision.isPending}>
          <Save />
          保存配置
        </Button>
        {createRevision.isSuccess ? (
          <p className="text-sm text-green-600">配置已保存</p>
        ) : null}
        {createRevision.isError ? (
          <p className="text-sm text-destructive">保存失败</p>
        ) : null}
      </div>
    </form>
  );

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main>
        <div className="mb-4 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <SemanticIconTile tone="primary" size="lg">
              <Bot />
            </SemanticIconTile>
            <div>
              <h1 className="text-2xl font-bold">{employee.data?.name ?? "数字员工配置"}</h1>
              <p className="text-sm text-muted-foreground">配置员工技能、策略和输出契约</p>
            </div>
          </div>
          <Button asChild variant="outline">
            <Link to="/employees/$employeeId" params={{ employeeId }}>
              <ArrowLeft />
              返回详情
            </Link>
          </Button>
        </div>

        {employee.isLoading ? <p className="text-sm text-muted-foreground">加载中</p> : null}
        {employee.isError ? <p className="text-sm text-destructive">加载失败</p> : null}

        {employee.data ? (
          <Tabs defaultValue="instructions" className="gap-4">
            <LiquidTabsList aria-label="数字员工配置视图" className="max-w-3xl">
              <LiquidTabsTrigger value="instructions">宪法/人格</LiquidTabsTrigger>
              <LiquidTabsTrigger value="capabilities">能力设置</LiquidTabsTrigger>
              <LiquidTabsTrigger value="advanced">高级配置</LiquidTabsTrigger>
            </LiquidTabsList>
            <TabsContent value="instructions">
              <InstructionFilesPanel apiOptions={apiOptions} employeeId={employeeId} />
            </TabsContent>
            <TabsContent value="capabilities">
              <EmployeeCapabilitiesPanel apiOptions={apiOptions} employeeId={employeeId} />
            </TabsContent>
            <TabsContent value="advanced">{advancedConfigForm}</TabsContent>
          </Tabs>
        ) : null}
      </Main>
    </>
  );
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

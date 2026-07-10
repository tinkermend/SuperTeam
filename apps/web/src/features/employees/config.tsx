import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Save } from "lucide-react";
import { useEffect, useState } from "react";
import { Main } from "@/components/layout/main";
import {
  ShellPageHeader,
  ShellPageHeaderBack,
} from "@/components/layout/shell-page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  createDigitalEmployeeConfigRevision,
  getDigitalEmployee,
  type CreateDigitalEmployeeConfigRevisionInput,
} from "@/lib/api/employees";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";

export function EmployeeConfigPage({ employeeId }: { employeeId: string }) {
  const apiBaseUrl = resolveControlPlaneUrl();
  return <EmployeeConfigView apiBaseUrl={apiBaseUrl} employeeId={employeeId} />;
}

type EmployeeConfigViewProps = {
  apiBaseUrl: string;
  employeeId: string;
  fetcher?: typeof fetch;
};

export function EmployeeConfigView({ apiBaseUrl, employeeId, fetcher }: EmployeeConfigViewProps) {
  const apiOptions = { baseUrl: apiBaseUrl, fetcher };
  const queryClient = useQueryClient();

  const [personaMemoryMarkdown, setPersonaMemoryMarkdown] = useState("");
  const [capabilityBindings, setCapabilityBindings] = useState("{}");
  const [dailyTokenLimit, setDailyTokenLimit] = useState("");
  const [personaDirty, setPersonaDirty] = useState(false);
  const [capabilityDirty, setCapabilityDirty] = useState(false);
  const [budgetDirty, setBudgetDirty] = useState(false);
  const [capabilityError, setCapabilityError] = useState("");
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
      setPersonaDirty(false);
      setCapabilityDirty(false);
      setBudgetDirty(false);
      setCapabilityError("");
      setBudgetError("");
      queryClient.invalidateQueries({ queryKey: ["digital-employee", employeeId] });
    },
  });

  useEffect(() => {
    if (!employee.data || hydratedEmployeeId === employee.data.id) return;
    setPersonaMemoryMarkdown(employee.data.persona_memory_markdown ?? "");
    setCapabilityBindings(formatJsonObject(employee.data.capability_bindings ?? {}));
    setDailyTokenLimit(budgetPolicyValue(employee.data.budget_policy ?? {}));
    setPersonaDirty(false);
    setCapabilityDirty(false);
    setBudgetDirty(false);
    setCapabilityError("");
    setBudgetError("");
    setHydratedEmployeeId(employee.data.id);
  }, [employee.data, hydratedEmployeeId]);

  const hasDirtyConfig = personaDirty || capabilityDirty || budgetDirty;

  const handleSubmit = (event: React.FormEvent) => {
    event.preventDefault();
    const input: CreateDigitalEmployeeConfigRevisionInput = { status: "draft" };
    setCapabilityError("");

    if (personaDirty) {
      input.persona_memory_markdown = personaMemoryMarkdown.trim();
    }
    if (capabilityDirty) {
      const parsed = parseJsonObject(capabilityBindings, "能力绑定");
      if (!parsed.ok) {
        setCapabilityError(parsed.error);
        return;
      }
      input.capability_bindings = parsed.value;
    }

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

  const configForm = (
    <form className="space-y-4" noValidate onSubmit={handleSubmit}>
      <Card>
        <CardHeader>
          <CardTitle>人格记忆.md</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div>
            <Label htmlFor="persona-memory-markdown">人格记忆.md</Label>
            <Textarea
              id="persona-memory-markdown"
              value={personaMemoryMarkdown}
              onChange={(event) => {
                setPersonaMemoryMarkdown(event.target.value);
                setPersonaDirty(true);
              }}
              rows={10}
              className="font-mono text-xs"
            />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>能力绑定</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div>
            <Label htmlFor="capability-bindings">能力绑定</Label>
            <Textarea
              id="capability-bindings"
              value={capabilityBindings}
              onChange={(event) => {
                setCapabilityBindings(event.target.value);
                setCapabilityDirty(true);
                setCapabilityError("");
              }}
              rows={10}
              className="font-mono text-xs"
              aria-invalid={Boolean(capabilityError)}
            />
            {capabilityError ? (
              <p className="text-sm text-destructive">{capabilityError}</p>
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
      <ShellPageHeader
        back={
          <ShellPageHeaderBack
            ariaLabel="返回数字员工详情"
            params={{ employeeId }}
            to="/employees/$employeeId"
          />
        }
        title={employee.data?.name ?? "数字员工配置"}
        subtitle="配置员工人格记忆、能力绑定和预算策略"
      />
      <Main>
        {employee.isLoading ? <p className="text-sm text-muted-foreground">加载中</p> : null}
        {employee.isError ? <p className="text-sm text-destructive">加载失败</p> : null}

        {employee.data ? configForm : null}
      </Main>
    </>
  );
}

function formatJsonObject(value: Record<string, unknown>) {
  return JSON.stringify(value, null, 2);
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

function parseJsonObject(
  value: string,
  label: string,
): { ok: true; value: Record<string, unknown> } | { ok: false; error: string } {
  try {
    const parsed = JSON.parse(value || "{}") as unknown;
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      return { ok: false, error: `${label}必须是有效 JSON object` };
    }
    return { ok: true, value: parsed as Record<string, unknown> };
  } catch {
    return { ok: false, error: `${label}必须是有效 JSON object` };
  }
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

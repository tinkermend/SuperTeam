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
import { SemanticIconTile } from "@/components/superteam/liquid-components";
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

  const [roleProfile, setRoleProfile] = useState("{}");
  const [constitutionAddendum, setConstitutionAddendum] = useState("{}");
  const [capabilitySelection, setCapabilitySelection] = useState("{}");
  const [contextPolicyOverride, setContextPolicyOverride] = useState("{}");
  const [approvalPolicyOverride, setApprovalPolicyOverride] = useState("{}");
  const [outputContractAddendum, setOutputContractAddendum] = useState("{}");

  const employee = useQuery({
    queryKey: ["digital-employee", employeeId],
    queryFn: () => getDigitalEmployee(apiOptions, employeeId),
  });

  const createRevision = useMutation({
    mutationFn: (input: CreateDigitalEmployeeConfigRevisionInput) =>
      createDigitalEmployeeConfigRevision(apiOptions, employeeId, input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["digital-employee", employeeId] });
    },
  });

  const handleSubmit = (event: React.FormEvent) => {
    event.preventDefault();
    const parseJson = (str: string) => {
      try {
        return JSON.parse(str);
      } catch {
        return {};
      }
    };

    createRevision.mutate({
      role_profile: parseJson(roleProfile),
      constitution_addendum: parseJson(constitutionAddendum),
      capability_selection: parseJson(capabilitySelection),
      context_policy_override: parseJson(contextPolicyOverride),
      approval_policy_override: parseJson(approvalPolicyOverride),
      output_contract_addendum: parseJson(outputContractAddendum),
      status: "draft",
    });
  };

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
          <form onSubmit={handleSubmit} className="space-y-4">
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
                    onChange={(e) => setRoleProfile(e.target.value)}
                    rows={4}
                    className="font-mono text-xs"
                  />
                </div>
                <div>
                  <Label htmlFor="constitution">Constitution Addendum (JSON)</Label>
                  <Textarea
                    id="constitution"
                    value={constitutionAddendum}
                    onChange={(e) => setConstitutionAddendum(e.target.value)}
                    rows={4}
                    className="font-mono text-xs"
                  />
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
                    onChange={(e) => setCapabilitySelection(e.target.value)}
                    rows={4}
                    className="font-mono text-xs"
                  />
                </div>
                <div>
                  <Label htmlFor="context-policy">Context Policy Override (JSON)</Label>
                  <Textarea
                    id="context-policy"
                    value={contextPolicyOverride}
                    onChange={(e) => setContextPolicyOverride(e.target.value)}
                    rows={4}
                    className="font-mono text-xs"
                  />
                </div>
                <div>
                  <Label htmlFor="approval-policy">Approval Policy Override (JSON)</Label>
                  <Textarea
                    id="approval-policy"
                    value={approvalPolicyOverride}
                    onChange={(e) => setApprovalPolicyOverride(e.target.value)}
                    rows={4}
                    className="font-mono text-xs"
                  />
                </div>
                <div>
                  <Label htmlFor="output-contract">Output Contract Addendum (JSON)</Label>
                  <Textarea
                    id="output-contract"
                    value={outputContractAddendum}
                    onChange={(e) => setOutputContractAddendum(e.target.value)}
                    rows={4}
                    className="font-mono text-xs"
                  />
                </div>
              </CardContent>
            </Card>

            <div className="flex gap-3">
              <Button type="submit" disabled={createRevision.isPending}>
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
        ) : null}
      </Main>
    </>
  );
}

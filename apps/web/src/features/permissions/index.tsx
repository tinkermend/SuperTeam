import {
  SoftTabs,
  SoftTabsContent,
  SoftTabsList,
  SoftTabsTrigger,
} from "@/components/superteam";
import { useMemo, useState } from "react";
import type { ApiClientOptions } from "@/lib/api";
import { ShieldCheck } from "lucide-react";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import { AuthorizationAuditTable } from "./components/authorization-audit-table";
import { AuthorizationOverview } from "./components/authorization-overview";
import { MemberRoles } from "./components/member-roles";
import { PermissionApprovalsQueue } from "./components/permission-approvals-queue";
import { PermissionDiagnostics } from "./components/permission-diagnostics";
import { RuntimeScopes } from "./components/runtime-scopes";

export type PermissionsCenterProps = {
  apiBaseUrl?: string;
  fetcher?: typeof fetch;
  /** 受控 tab：由路由层根据 URL search 提供;不传时组件内部自管(测试等场景无路由)。 */
  activeTab?: string;
  onTabChange?: (tab: string) => void;
};

const DEFAULT_TAB = "overview";

const tabItems = [
  { value: "overview", label: "授权概览" },
  { value: "permission-approvals", label: "权限审批" },
  { value: "audit", label: "授权审计" },
  { value: "runtime-scopes", label: "Runtime 范围" },
  { value: "member-roles", label: "成员角色" },
  { value: "diagnostics", label: "权限诊断" },
] as const;

export function PermissionsCenter({
  apiBaseUrl = resolveControlPlaneUrl(),
  fetcher,
  activeTab,
  onTabChange
}: PermissionsCenterProps) {
  const apiOptions = useMemo<ApiClientOptions>(() => ({ baseUrl: apiBaseUrl, fetcher }), [apiBaseUrl, fetcher]);
  const [internalTab, setInternalTab] = useState(DEFAULT_TAB);
  const value = activeTab ?? internalTab;
  const handleValueChange = (next: string) => {
    setInternalTab(next);
    onTabChange?.(next);
  };

  return (
    <>
      <ShellPageHeader
        icon={<ShieldCheck />}
        iconTone="artifact"
        title="权限中心"
        subtitle="集中查看授权决策、Runtime 执行范围和成员角色。"
      />
      <Main width="wide" className="min-w-0 text-ink">
        <SoftTabs value={value} onValueChange={handleValueChange} className="gap-4">
          <SoftTabsList className="h-auto max-w-full flex-wrap justify-start gap-1 overflow-x-auto rounded-[14px] bg-card p-1.5 text-ink-2 shadow-card">
            {tabItems.map((tab) => (
              <SoftTabsTrigger
                key={tab.value}
                value={tab.value}
                className="h-9 flex-none rounded-[10px] border-0 px-4 py-2 text-[13px] font-semibold text-ink-2 shadow-none transition-colors hover:bg-card-soft hover:text-ink focus-visible:ring-brand/60 focus-visible:ring-offset-background data-[state=active]:bg-brand-soft data-[state=active]:text-brand-deep data-[state=active]:shadow-none dark:text-ink-2 dark:data-[state=active]:bg-brand-soft dark:data-[state=active]:text-brand-deep"
              >
                {tab.label}
              </SoftTabsTrigger>
            ))}
          </SoftTabsList>
          <SoftTabsContent value="overview">
            <AuthorizationOverview apiOptions={apiOptions} />
          </SoftTabsContent>
          <SoftTabsContent value="permission-approvals">
            <PermissionApprovalsQueue apiOptions={apiOptions} />
          </SoftTabsContent>
          <SoftTabsContent value="audit">
            <AuthorizationAuditTable apiOptions={apiOptions} />
          </SoftTabsContent>
          <SoftTabsContent value="runtime-scopes">
            <RuntimeScopes apiOptions={apiOptions} />
          </SoftTabsContent>
          <SoftTabsContent value="member-roles">
            <MemberRoles apiOptions={apiOptions} />
          </SoftTabsContent>
          <SoftTabsContent value="diagnostics">
            <PermissionDiagnostics apiOptions={apiOptions} />
          </SoftTabsContent>
        </SoftTabs>
      </Main>
    </>
  );
}

import { useMemo } from "react";
import type { ApiClientOptions } from "@/lib/api";
import { ShieldCheck } from "lucide-react";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import { AuthorizationAuditTable } from "./components/authorization-audit-table";
import { AuthorizationOverview } from "./components/authorization-overview";
import { MemberRoles } from "./components/member-roles";
import { PermissionDiagnostics } from "./components/permission-diagnostics";
import { RuntimeScopes } from "./components/runtime-scopes";

export type PermissionsCenterProps = {
  apiBaseUrl?: string;
  fetcher?: typeof fetch;
};

const tabItems = [
  { value: "overview", label: "授权概览" },
  { value: "audit", label: "授权审计" },
  { value: "runtime-scopes", label: "Runtime 范围" },
  { value: "member-roles", label: "成员角色" },
  { value: "diagnostics", label: "权限诊断" },
] as const;

export function PermissionsCenter({ apiBaseUrl = resolveControlPlaneUrl(), fetcher }: PermissionsCenterProps) {
  const apiOptions = useMemo<ApiClientOptions>(() => ({ baseUrl: apiBaseUrl, fetcher }), [apiBaseUrl, fetcher]);

  return (
    <>
      <ShellPageHeader
        icon={<ShieldCheck />}
        iconTone="artifact"
        title="权限中心"
        subtitle="集中查看授权决策、Runtime 执行范围和成员角色。"
      />
      <Main fluid className="min-w-0 text-v3-ink">
        <Tabs defaultValue="overview" className="gap-4">
          <TabsList className="h-auto max-w-full flex-wrap justify-start gap-1 overflow-x-auto rounded-[14px] bg-v3-card p-1.5 text-v3-ink-2 shadow-v3">
            {tabItems.map((tab) => (
              <TabsTrigger
                key={tab.value}
                value={tab.value}
                className="h-9 flex-none rounded-[10px] border-0 px-4 py-2 text-[13px] font-semibold text-v3-ink-2 shadow-none transition-colors hover:bg-v3-card-soft hover:text-v3-ink focus-visible:ring-v3-brand/60 focus-visible:ring-offset-v3-bg data-[state=active]:bg-v3-brand-soft data-[state=active]:text-v3-brand-deep data-[state=active]:shadow-none dark:text-v3-ink-2 dark:data-[state=active]:bg-v3-brand-soft dark:data-[state=active]:text-v3-brand-deep"
              >
                {tab.label}
              </TabsTrigger>
            ))}
          </TabsList>
          <TabsContent value="overview">
            <AuthorizationOverview apiOptions={apiOptions} />
          </TabsContent>
          <TabsContent value="audit">
            <AuthorizationAuditTable apiOptions={apiOptions} />
          </TabsContent>
          <TabsContent value="runtime-scopes">
            <RuntimeScopes apiOptions={apiOptions} />
          </TabsContent>
          <TabsContent value="member-roles">
            <MemberRoles apiOptions={apiOptions} />
          </TabsContent>
          <TabsContent value="diagnostics">
            <PermissionDiagnostics apiOptions={apiOptions} />
          </TabsContent>
        </Tabs>
      </Main>
    </>
  );
}

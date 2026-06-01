import { useMemo } from "react";
import type { ApiClientOptions } from "@/lib/api";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
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
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main fluid>
        <div className="mb-4 flex flex-col gap-1">
          <h1 className="text-2xl font-bold tracking-tight">权限中心</h1>
          <p className="text-sm text-muted-foreground">集中查看授权决策、Runtime 执行范围和成员角色。</p>
        </div>
        <Tabs defaultValue="overview" className="gap-4">
          <TabsList className="flex h-auto w-full flex-wrap justify-start">
            {tabItems.map((tab) => (
              <TabsTrigger key={tab.value} value={tab.value}>
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

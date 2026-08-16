import { useQuery } from "@tanstack/react-query";
import { useBlocker, useNavigate } from "@tanstack/react-router";
import { useCallback, useEffect, useRef, useState } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Main } from "@/components/layout/main";
import { ShellPageHeader, ShellPageHeaderBack } from "@/components/layout/shell-page-header";
import {
  DetailSkeleton,
  ErrorState,
  SoftCard,
  SoftTabs,
  SoftTabsContent,
  SoftTabsList,
  SoftTabsTrigger,
  StatusPill,
} from "@/components/superteam";
import { getDigitalEmployee, type DigitalEmployee } from "@/lib/api/employees";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { riskLevelLabel, statusLabel } from "@/lib/status-labels";
import { EmployeeAvatar } from "./avatar";
import { employeeAvatarAsset } from "./avatar-library";
import { ConfigExecutionTab } from "./components/config-execution-tab";
import { ConfigIdentityTab } from "./components/config-identity-tab";
import { ConfigPermissionTab } from "./components/config-permission-tab";
import { EmployeeCapabilitiesPanel } from "./components/employee-capabilities-panel";
import { parseEmployeeConfigTab, type EmployeeConfigTab } from "./config-utils";
import { providerDisplayName } from "./provider-label";

export function EmployeeConfigPage({
  employeeId,
  tab,
}: {
  employeeId: string;
  tab?: EmployeeConfigTab;
}) {
  const apiBaseUrl = resolveControlPlaneUrl();
  const navigate = useNavigate();
  return (
    <EmployeeConfigView
      apiBaseUrl={apiBaseUrl}
      employeeId={employeeId}
      tab={parseEmployeeConfigTab(tab)}
      onTabChange={(next) => {
        void navigate({
          to: "/employees/$employeeId/config",
          params: { employeeId },
          search: { tab: next },
        });
      }}
    />
  );
}

type EmployeeConfigViewProps = {
  apiBaseUrl: string;
  employeeId: string;
  fetcher?: typeof fetch;
  tab?: EmployeeConfigTab;
  onTabChange?: (tab: EmployeeConfigTab) => void;
};

export function EmployeeConfigView({
  apiBaseUrl,
  employeeId,
  fetcher,
  tab = "identity",
  onTabChange,
}: EmployeeConfigViewProps) {
  const apiOptions = { baseUrl: apiBaseUrl, fetcher };
  const [identityDirty, setIdentityDirty] = useState(false);
  const [executionDirty, setExecutionDirty] = useState(false);
  const [permissionDirty, setPermissionDirty] = useState(false);
  const [pendingTab, setPendingTab] = useState<EmployeeConfigTab | null>(null);
  const dirty = identityDirty || executionDirty || permissionDirty;
  const dirtyRef = useRef(dirty);
  dirtyRef.current = dirty;

  const employee = useQuery({
    queryKey: ["digital-employee", employeeId],
    queryFn: () => getDigitalEmployee(apiOptions, employeeId),
  });

  const blocker = useBlocker({
    shouldBlockFn: ({ current, next }) => {
      if (!dirtyRef.current) return false;
      const sameEmployee =
        current.pathname === next.pathname &&
        String((current.params as { employeeId?: string }).employeeId ?? "") ===
          String((next.params as { employeeId?: string }).employeeId ?? "");
      const currentTab = (current.search as { tab?: string }).tab;
      const nextTab = (next.search as { tab?: string }).tab;
      if (sameEmployee && currentTab !== nextTab) return false;
      return true;
    },
    enableBeforeUnload: true,
    withResolver: true,
  });

  useEffect(() => {
    if (blocker.status !== "blocked") return;
  }, [blocker.status]);

  const requestTabChange = useCallback(
    (next: EmployeeConfigTab) => {
      if (next === tab) return;
      if (dirtyRef.current) {
        setPendingTab(next);
        return;
      }
      onTabChange?.(next);
    },
    [onTabChange, tab],
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
        subtitle="按生效方式分组：即时保存、行内即写、配置版本、权限审批"
      />
      <Main width="contained" className="space-y-4">
        {employee.isLoading && !employee.data ? <DetailSkeleton /> : null}
        {employee.isError ? (
          <ErrorState title="加载失败" description="无法加载数字员工配置" onRetry={() => void employee.refetch()} />
        ) : null}

        {employee.data ? (
          <>
            <LocatorHeader employee={employee.data} />
            <SoftTabs className="gap-4" value={tab} onValueChange={(value) => requestTabChange(value as EmployeeConfigTab)}>
              <SoftTabsList
                className="h-auto w-full max-w-full flex-wrap justify-start gap-0 rounded-none border-b border-line bg-transparent p-0 shadow-none"
                data-slot="page-tab-list"
              >
                <ConfigTab value="identity">身份</ConfigTab>
                <ConfigTab value="capabilities">能力</ConfigTab>
                <ConfigTab value="execution">执行配置</ConfigTab>
                <ConfigTab value="permission">权限</ConfigTab>
              </SoftTabsList>
              <SoftTabsContent forceMount hidden={tab !== "identity"} value="identity">
                <ConfigIdentityTab
                  apiOptions={apiOptions}
                  employee={employee.data}
                  onDirtyChange={setIdentityDirty}
                />
              </SoftTabsContent>
              <SoftTabsContent forceMount hidden={tab !== "capabilities"} value="capabilities">
                <EmployeeCapabilitiesPanel apiOptions={apiOptions} employeeId={employeeId} />
              </SoftTabsContent>
              <SoftTabsContent forceMount hidden={tab !== "execution"} value="execution">
                <ConfigExecutionTab
                  apiOptions={apiOptions}
                  employee={employee.data}
                  onDirtyChange={setExecutionDirty}
                />
              </SoftTabsContent>
              <SoftTabsContent forceMount hidden={tab !== "permission"} value="permission">
                <ConfigPermissionTab
                  apiOptions={apiOptions}
                  employee={employee.data}
                  onDirtyChange={setPermissionDirty}
                />
              </SoftTabsContent>
            </SoftTabs>
          </>
        ) : null}
      </Main>
      <ConfirmDialog
        open={pendingTab !== null || blocker.status === "blocked"}
        onOpenChange={(open) => {
          if (open) return;
          setPendingTab(null);
          if (blocker.status === "blocked") blocker.reset?.();
        }}
        title="放弃未保存的更改？"
        desc="当前 Tab 有未保存内容，离开后这些修改会丢失。"
        confirmText="离开"
        destructive
        handleConfirm={() => {
          if (pendingTab) {
            const next = pendingTab;
            setPendingTab(null);
            onTabChange?.(next);
            return;
          }
          if (blocker.status === "blocked") blocker.proceed?.();
        }}
      />
    </>
  );
}

function ConfigTab({ children, value }: { children: string; value: EmployeeConfigTab }) {
  return (
    <SoftTabsTrigger
      className="-mb-px h-auto rounded-none border-b-2 border-transparent px-3 py-2.5 text-[13px] font-semibold text-ink-2 shadow-none hover:bg-transparent hover:text-ink data-[state=active]:border-brand data-[state=active]:bg-transparent data-[state=active]:text-brand-deep data-[state=active]:shadow-none"
      data-slot="page-tab"
      value={value}
    >
      {children}
    </SoftTabsTrigger>
  );
}

function LocatorHeader({ employee }: { employee: DigitalEmployee }) {
  const effectiveStatus = employee.metadata?.effective_config_status;
  const configLabel = employee.metadata?.effective_config_label;
  return (
    <SoftCard className="overflow-hidden p-0">
      <div className="flex items-start gap-3.5 px-5 py-4">
        <EmployeeAvatar asset={employeeAvatarAsset(employee)} name={employee.name} size="lg" />
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <h2 className="truncate text-[17px] font-extrabold tracking-tight text-ink">{employee.name}</h2>
            {effectiveStatus ? (
              <StatusPill tone="info">{statusLabel(String(effectiveStatus))}</StatusPill>
            ) : null}
          </div>
          <p className="mt-1 line-clamp-2 text-[13px] leading-5 text-ink-2">
            {employee.description?.trim() || "尚未填写员工说明"}
          </p>
          {configLabel ? (
            <p className="mt-1 text-[12px] text-ink-3">当前生效配置 {configLabel}</p>
          ) : null}
        </div>
      </div>
      <div className="grid grid-cols-2 divide-x divide-y divide-line border-t border-line bg-card-soft sm:grid-cols-4 sm:divide-y-0">
        <LocatorItem label="Provider（不可改）" value={providerDisplayName(employee.provider_type)} />
        <LocatorItem label="职责描述" value={employee.role || "未设置"} />
        <LocatorItem label="风险等级" value={riskLevelLabel(employee.risk_level)} />
        <LocatorItem label="所属团队" value={employee.team_name?.trim() || "无团队归属"} />
      </div>
    </SoftCard>
  );
}

function LocatorItem({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 px-4 py-3">
      <p className="text-[11px] leading-4 text-ink-3">{label}</p>
      <p className="mt-1 truncate text-[13px] font-semibold tracking-tight text-ink">{value}</p>
    </div>
  );
}

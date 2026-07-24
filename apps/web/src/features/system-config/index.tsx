import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, RotateCcw, Settings2 } from "lucide-react";
import {
  StatusPill,
  Button,
  EmptyState,
  ErrorState,
  LoadingState,
  PageTab,
  PageTabList,
  DataTable,
  PageTabs,
  Td,
  Th,
  Tr,
  WorkSurface
} from "@/components/superteam";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import { ApiRequestError } from "@/lib/api/client";
import {
  listSystemConfigs,
  resetSystemConfig,
  isHighDangerConfig,
  type SystemConfigItem
} from "@/lib/api/system-config";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { EditSystemConfigDialog } from "./edit-dialog";
import { displayDefaultValue, displayEffectiveValue } from "./units";

/** domain → tab 标题。未知 domain 落"其他"，后端加配置项前端零改动。 */
const DOMAIN_LABELS: Record<string, string> = {
  artifact: "文件与工件",
  execution: "执行与调度",
  security: "安全与会话"
};
const DOMAIN_ORDER = ["artifact", "execution", "security"];
const FALLBACK_DOMAIN = "__other__";

function domainLabel(domain: string): string {
  return DOMAIN_LABELS[domain] ?? "其他";
}

export function SystemConfigPage() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const queryClient = useQueryClient();
  const [activeDomain, setActiveDomain] = useState<string | null>(null);
  const [editing, setEditing] = useState<SystemConfigItem | null>(null);
  const [pendingReset, setPendingReset] = useState<SystemConfigItem | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const configs = useQuery({
    queryKey: ["system-configs"],
    queryFn: () => listSystemConfigs({ baseUrl: apiBaseUrl })
});

  const resetMutation = useMutation({
    mutationFn: (key: string) => resetSystemConfig({ baseUrl: apiBaseUrl }, key),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["system-configs"] });
      setActionError(null);
    },
    onError: (error: unknown) => {
      setActionError(
        error instanceof ApiRequestError && error.detail
          ? error.detail
          : error instanceof Error
            ? error.message
            : "恢复默认失败",
      );
    }
});

  const items = useMemo(() => configs.data?.items ?? [], [configs.data]);

  const domains = useMemo(() => {
    const present = new Set(
      items.map((item) => (DOMAIN_LABELS[item.domain] ? item.domain : FALLBACK_DOMAIN)),
    );
    const ordered = DOMAIN_ORDER.filter((domain) => present.has(domain));
    if (present.has(FALLBACK_DOMAIN)) ordered.push(FALLBACK_DOMAIN);
    return ordered;
  }, [items]);

  const currentDomain = activeDomain ?? domains[0] ?? null;
  const rows = useMemo(
    () =>
      items.filter((item) =>
        currentDomain === FALLBACK_DOMAIN
          ? !DOMAIN_LABELS[item.domain]
          : item.domain === currentDomain,
      ),
    [items, currentDomain],
  );

  const isInitialLoading = configs.isPending && items.length === 0;
  const isBlockingError = configs.isError && items.length === 0;

  return (
    <>
      <ShellPageHeader
        icon={<Settings2 />}
        iconTone="brand"
        title="系统配置"
        subtitle="平台运行态参数：默认值由服务端注册表定义，此处只管理显式覆盖，改动全部留审计"
      />
      <Main className="min-w-0 overflow-x-hidden">
        <div className="flex min-w-0 flex-col gap-6">
          {domains.length > 1 ? (
            <PageTabs>
              <PageTabList>
                {domains.map((domain) => (
                  <PageTab
                    key={domain}
                    active={domain === currentDomain}
                    onClick={() => setActiveDomain(domain)}
                  >
                    {domain === FALLBACK_DOMAIN ? "其他" : domainLabel(domain)}
                  </PageTab>
                ))}
              </PageTabList>
            </PageTabs>
          ) : null}

          {actionError ? (
            <Alert variant="destructive">
              <AlertTitle>操作失败</AlertTitle>
              <AlertDescription>{actionError}</AlertDescription>
            </Alert>
          ) : null}

          <WorkSurface className="min-w-0">
            {isInitialLoading ? (
              <LoadingState label="加载系统配置…" />
            ) : isBlockingError ? (
              <ErrorState title="加载失败" description="无法加载系统配置，请确认管理员权限" />
            ) : rows.length === 0 ? (
              <EmptyState
                icon={<Settings2 />}
                title="暂无配置项"
                description="服务端注册表中还没有该领域的配置项。"
              />
            ) : (
              <DataTable>
                <thead>
                  <tr>
                    <Th>配置项</Th>
                    <Th className="w-36">生效值</Th>
                    <Th className="w-32">默认值</Th>
                    <Th className="w-44">状态</Th>
                    <Th className="w-28" aria-label="操作" />
                  </tr>
                </thead>
                <tbody>
                  {rows.map((item) => (
                    <SystemConfigRow
                      key={item.key}
                      item={item}
                      onEdit={() => {
                        setActionError(null);
                        setEditing(item);
                      }}
                      onReset={() => {
                        setActionError(null);
                        setPendingReset(item);
                      }}
                    />
                  ))}
                </tbody>
              </DataTable>
            )}
          </WorkSurface>
        </div>
      </Main>

      <EditSystemConfigDialog
        apiBaseUrl={apiBaseUrl}
        item={editing}
        onOpenChange={(open) => {
          if (!open) setEditing(null);
        }}
      />

      <ConfirmDialog
        open={pendingReset !== null}
        onOpenChange={(open) => {
          if (!open) setPendingReset(null);
        }}
        title={`恢复「${pendingReset?.label ?? ""}」默认值`}
        desc={
          pendingReset
            ? `将删除覆盖值，恢复为默认 ${displayDefaultValue(pendingReset)}（当前生效 ${displayEffectiveValue(pendingReset)}）。`
            : ""
        }
        confirmText="恢复默认"
        isLoading={resetMutation.isPending}
        handleConfirm={() => {
          if (pendingReset) {
            resetMutation.mutate(pendingReset.key, {
              onSettled: () => setPendingReset(null)
});
          }
        }}
      />
    </>
  );
}

function SystemConfigRow({
  item,
  onEdit,
  onReset
}: {
  item: SystemConfigItem;
  onEdit: () => void;
  onReset: () => void;
}) {
  return (
    <Tr>
      <Td>
        <div className="flex min-w-0 flex-col gap-0.5">
          <span className="font-medium">{item.label}</span>
          <span className="truncate font-mono text-xs text-muted-foreground">{item.key}</span>
          {isHighDangerConfig(item) ? (
            <span className="text-xs font-medium text-destructive">高危 · 不宜改动 · 改后不迁存量</span>
          ) : null}
          <span className="line-clamp-2 max-w-[36rem] text-xs text-muted-foreground">
            {item.description}
          </span>
        </div>
      </Td>
      <Td className="font-medium tabular-nums">
        {displayEffectiveValue(item)}
      </Td>
      <Td className="tabular-nums text-muted-foreground">
        {displayDefaultValue(item)}
      </Td>
      <Td>
        <div className="flex min-w-0 flex-col gap-1">
          <StatusPill tone={item.is_overridden ? "info" : "mute"}>
            {item.is_overridden ? "已修改" : "默认"}
          </StatusPill>
          {item.is_overridden && item.updated_at ? (
            <span
              className="text-xs text-muted-foreground tabular-nums"
              title={new Date(item.updated_at).toLocaleString()}
            >
              {item.updated_by_name ? `${item.updated_by_name} · ` : ""}
              {new Date(item.updated_at).toLocaleDateString()}
            </span>
          ) : null}
        </div>
      </Td>
      <Td>
        <div className="flex items-center gap-1">
          <Button variant="ghost" size="sm" aria-label={`修改 ${item.label}`} onClick={onEdit}>
            <Pencil />
          </Button>
          {item.is_overridden ? (
            <Button
              variant="ghost"
              size="sm"
              aria-label={`恢复 ${item.label} 默认值`}
              onClick={onReset}
            >
              <RotateCcw />
            </Button>
          ) : null}
        </div>
      </Td>
    </Tr>
  );
}

export default SystemConfigPage;

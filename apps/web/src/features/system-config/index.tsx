import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useLocation, useNavigate } from "@tanstack/react-router";
import { Pencil, RotateCcw, Settings2 } from "lucide-react";
import {
  StatusPill,
  Button,
  EmptyState,
  ErrorState,
  LoadingState,
  DataTable,
  Td,
  Th,
  Tr,
  WorkSurface,
  Callout,
  SoftTabs,
  SoftTabsContent,
  SoftTabsList,
  SoftTabsTrigger,
  CopyableMono,
  RelativeTime,
} from "@/components/superteam";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import { Switch } from "@/components/ui/switch";
import { ApiRequestError } from "@/lib/api/client";
import {
  listSystemConfigs,
  resetSystemConfig,
  isHighDangerConfig,
  type SystemConfigItem,
} from "@/lib/api/system-config";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { cn } from "@/lib/utils";
import { EditSystemConfigDialog } from "./edit-dialog";
import { MessageChannelsPanel, type ChannelSection } from "./message-channels-panel";
import { displayDefaultValue, displayEffectiveValue } from "./units";

type PrimaryTab = "channels" | "params";

/** domain → tab 标题。未知 domain 落"其他"，后端加配置项前端零改动。 */
const DOMAIN_LABELS: Record<string, string> = {
  artifact: "文件与工件",
  execution: "执行与调度",
  security: "安全与会话",
  organization: "组织与编制",
  retention: "数据保留",
};

const DOMAIN_BLURBS: Record<string, string> = {
  artifact: "工件与附件的大小上限、上传/下载链接有效期等边界。",
  execution: "Runtime 心跳超时、系统工作区与僵尸任务收敛等执行面参数。",
  security: "登录会话与 Runtime 会话有效期。",
  organization: "团队编制上限与宪法字符预算等组织边界。",
  retention: "各流水与事件的保留天数；到期由保留作业清理。",
  __other__: "未归入已知领域的注册表项。",
};

const DOMAIN_ORDER = ["artifact", "execution", "security", "organization", "retention"];
const FALLBACK_DOMAIN = "__other__";

const CHANNEL_SECTIONS = new Set<ChannelSection>(["access", "tokens"]);

function domainLabel(domain: string): string {
  return DOMAIN_LABELS[domain] ?? "其他";
}

function domainBlurb(domain: string): string {
  return DOMAIN_BLURBS[domain] ?? DOMAIN_BLURBS[FALLBACK_DOMAIN];
}

function parseHash(hash: string): { primary: PrimaryTab; domain?: string; channelSection?: ChannelSection } {
  const raw = hash.replace(/^#/, "").trim();
  if (!raw || raw === "channels") {
    return { primary: "channels", channelSection: "access" };
  }
  if (raw.startsWith("channels/")) {
    const section = raw.slice("channels/".length) as ChannelSection;
    return {
      primary: "channels",
      channelSection: CHANNEL_SECTIONS.has(section) ? section : "access",
    };
  }
  if (raw === "params") {
    return { primary: "params" };
  }
  if (raw.startsWith("params/")) {
    return { primary: "params", domain: raw.slice("params/".length) || undefined };
  }
  // 兼容旧 domain 直链（若有）
  if (DOMAIN_LABELS[raw] || raw === FALLBACK_DOMAIN) {
    return { primary: "params", domain: raw };
  }
  return { primary: "channels", channelSection: "access" };
}

function hashFor(primary: PrimaryTab, opts?: { domain?: string; channelSection?: ChannelSection }): string {
  if (primary === "channels") {
    const section = opts?.channelSection ?? "access";
    return section === "access" ? "#channels" : `#channels/${section}`;
  }
  if (opts?.domain) return `#params/${opts.domain}`;
  return "#params";
}

export function SystemConfigPage() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const queryClient = useQueryClient();
  const location = useLocation();
  const navigate = useNavigate();

  const parsed = useMemo(() => parseHash(location.hash), [location.hash]);
  const [primary, setPrimary] = useState<PrimaryTab>(() => parsed.primary);
  const [channelSection, setChannelSection] = useState<ChannelSection>(
    () => parsed.channelSection ?? "access",
  );
  const [activeDomain, setActiveDomain] = useState<string | null>(parsed.domain ?? null);
  const [onlyOverridden, setOnlyOverridden] = useState(false);
  const [editing, setEditing] = useState<SystemConfigItem | null>(null);
  const [pendingReset, setPendingReset] = useState<SystemConfigItem | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  useEffect(() => {
    const next = parseHash(location.hash);
    setPrimary(next.primary);
    if (next.channelSection) setChannelSection(next.channelSection);
    if (next.domain) setActiveDomain(next.domain);
  }, [location.hash]);

  const configs = useQuery({
    queryKey: ["system-configs"],
    queryFn: () => listSystemConfigs({ baseUrl: apiBaseUrl }),
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
    },
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

  const currentDomain =
    activeDomain && domains.includes(activeDomain) ? activeDomain : (domains[0] ?? null);

  const rows = useMemo(() => {
    if (!currentDomain) return [];
    const filtered = items.filter((item) =>
      currentDomain === FALLBACK_DOMAIN
        ? !DOMAIN_LABELS[item.domain]
        : item.domain === currentDomain,
    );
    return onlyOverridden ? filtered.filter((item) => item.is_overridden) : filtered;
  }, [items, currentDomain, onlyOverridden]);

  const overriddenCount = useMemo(
    () =>
      currentDomain
        ? items.filter((item) =>
            (currentDomain === FALLBACK_DOMAIN
              ? !DOMAIN_LABELS[item.domain]
              : item.domain === currentDomain) && item.is_overridden,
          ).length
        : 0,
    [items, currentDomain],
  );

  const overriddenCountByDomain = useMemo(() => {
    const counts = new Map<string, number>();
    for (const item of items) {
      if (!item.is_overridden) continue;
      const domain = DOMAIN_LABELS[item.domain] ? item.domain : FALLBACK_DOMAIN;
      counts.set(domain, (counts.get(domain) ?? 0) + 1);
    }
    return counts;
  }, [items]);

  const isInitialLoading = configs.isPending && items.length === 0;
  const isBlockingError = configs.isError && items.length === 0;

  const goPrimary = (next: PrimaryTab) => {
    setPrimary(next);
    const hash =
      next === "channels"
        ? hashFor("channels", { channelSection })
        : hashFor("params", { domain: currentDomain ?? undefined });
    void navigate({ to: "/system-config", hash: hash.replace(/^#/, ""), replace: true });
  };

  const goChannelSection = (section: ChannelSection) => {
    setChannelSection(section);
    void navigate({
      to: "/system-config",
      hash: hashFor("channels", { channelSection: section }).replace(/^#/, ""),
      replace: true,
    });
  };

  const goDomain = (domain: string) => {
    setActiveDomain(domain);
    void navigate({
      to: "/system-config",
      hash: hashFor("params", { domain }).replace(/^#/, ""),
      replace: true,
    });
  };

  const subtitle =
    primary === "channels"
      ? "外部 IM 通道的健康、接入与服务凭据"
      : "服务端注册表参数；默认值可覆盖，改动全部留审计";

  return (
    <>
      <ShellPageHeader
        icon={<Settings2 />}
        iconTone="brand"
        title="系统配置"
        subtitle={subtitle}
      />
      <Main className="min-w-0 overflow-x-hidden">
        <SoftTabs
          className="flex w-full min-w-0 flex-col gap-4"
          onValueChange={(value) => goPrimary(value as PrimaryTab)}
          value={primary}
        >
          <div className="w-full min-w-0 max-w-full overflow-x-auto overflow-y-hidden pb-0.5">
            <SoftTabsList aria-label="系统配置分区">
              <SoftTabsTrigger value="channels">消息通道</SoftTabsTrigger>
              <SoftTabsTrigger value="params">平台参数</SoftTabsTrigger>
            </SoftTabsList>
          </div>

          <SoftTabsContent value="channels" className="min-w-0">
            <MessageChannelsPanel
              section={channelSection}
              onSectionChange={goChannelSection}
            />
          </SoftTabsContent>

          <SoftTabsContent value="params" className="min-w-0">
            <div className="flex min-w-0 flex-col gap-4">
              {actionError ? (
                <Callout tone="danger" title="操作失败" description={actionError} />
              ) : null}

              {domains.length > 0 ? (
                <SoftTabs
                  className="flex w-full min-w-0 flex-col gap-3"
                  onValueChange={goDomain}
                  value={currentDomain ?? domains[0]}
                >
                  <div className="w-full min-w-0 max-w-full overflow-x-auto overflow-y-hidden pb-0.5">
                    <SoftTabsList aria-label="平台参数领域">
                      {domains.map((domain) => {
                        const count = overriddenCountByDomain.get(domain) ?? 0;
                        return (
                          <SoftTabsTrigger key={domain} value={domain}>
                            {domainLabel(domain)}
                            {count > 0 ? (
                              <span className="inline-flex min-w-5 items-center justify-center rounded-full bg-brand-soft px-1.5 text-[11px] font-semibold tabular-nums text-brand-deep">
                                {count}
                              </span>
                            ) : null}
                          </SoftTabsTrigger>
                        );
                      })}
                    </SoftTabsList>
                  </div>

                  {domains.map((domain) => (
                    <SoftTabsContent key={domain} value={domain} className="min-w-0">
                      <WorkSurface className="min-w-0">
                        <div className="mb-0 flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
                          <p className="min-w-0 text-xs leading-5 text-ink-2">{domainBlurb(domain)}</p>
                          <label className="flex shrink-0 cursor-pointer items-center gap-2 text-xs text-ink-2">
                            <Switch
                              checked={onlyOverridden}
                              onCheckedChange={setOnlyOverridden}
                              aria-label="仅看已修改"
                            />
                            <span>
                              仅看已修改
                              {overriddenCount > 0 ? (
                                <span className="ms-1 tabular-nums text-ink-3">({overriddenCount})</span>
                              ) : null}
                            </span>
                          </label>
                        </div>

                        {isInitialLoading ? (
                          <div className="p-4">
                            <LoadingState label="加载系统配置…" />
                          </div>
                        ) : isBlockingError ? (
                          <div className="p-4">
                            <ErrorState
                              title="加载失败"
                              description="无法加载系统配置，请确认管理员权限"
                            />
                          </div>
                        ) : rows.length === 0 ? (
                          <div className="p-4">
                            <EmptyState
                              icon={<Settings2 />}
                              title={onlyOverridden ? "没有已修改项" : "暂无配置项"}
                              description={
                                onlyOverridden
                                  ? "当前领域下全部使用默认值。"
                                  : "服务端注册表中还没有该领域的配置项。"
                              }
                            />
                          </div>
                        ) : (
                          <DataTable>
                            <thead>
                              <tr>
                                <Th>配置项</Th>
                                <Th className="w-44">当前值</Th>
                                <Th className="w-24" aria-label="操作" />
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
                    </SoftTabsContent>
                  ))}
                </SoftTabs>
              ) : isInitialLoading ? (
                <LoadingState label="加载系统配置…" />
              ) : isBlockingError ? (
                <ErrorState title="加载失败" description="无法加载系统配置，请确认管理员权限" />
              ) : (
                <EmptyState
                  icon={<Settings2 />}
                  title="暂无配置项"
                  description="服务端注册表中还没有任何配置项。"
                />
              )}
            </div>
          </SoftTabsContent>
        </SoftTabs>
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
              onSettled: () => setPendingReset(null),
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
  onReset,
}: {
  item: SystemConfigItem;
  onEdit: () => void;
  onReset: () => void;
}) {
  const highDanger = isHighDangerConfig(item);
  const overridden = item.is_overridden;

  return (
    <Tr
      tone={highDanger ? "danger" : undefined}
      className={cn(
        "group/row",
        !highDanger && overridden && "[&>td:first-child]:shadow-[inset_3px_0_0_var(--brand)]",
      )}
    >
      <Td>
        <div className="flex min-w-0 flex-col gap-1">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <span className="font-medium text-ink">{item.label}</span>
            {highDanger ? (
              <span className="text-[11px] font-medium text-danger">高危 · 不宜改动</span>
            ) : null}
          </div>
          <CopyableMono className="max-w-[min(100%,28rem)]" value={item.key} />
          <span className="line-clamp-1 max-w-[36rem] text-xs leading-5 text-ink-3" title={item.description}>
            {item.description}
          </span>
        </div>
      </Td>
      <Td>
        <div className="flex min-w-0 flex-col gap-1">
          <div className="flex flex-wrap items-center gap-2">
            <span
              className={cn(
                "tabular-nums text-ink",
                overridden ? "text-sm font-semibold" : "font-medium",
              )}
            >
              {displayEffectiveValue(item)}
            </span>
            {overridden ? <StatusPill tone="info">已覆盖</StatusPill> : null}
          </div>
          {overridden ? (
            <div className="flex min-w-0 flex-col gap-0.5 text-xs text-ink-3">
              <span className="tabular-nums">默认 {displayDefaultValue(item)}</span>
              {item.updated_at ? (
                <span className="truncate">
                  {item.updated_by_name ? `${item.updated_by_name} · ` : ""}
                  <RelativeTime value={item.updated_at} />
                </span>
              ) : null}
            </div>
          ) : null}
        </div>
      </Td>
      <Td className="text-end">
        <div
          className={cn(
            "flex flex-wrap items-center justify-end gap-0.5 transition-opacity",
            // 覆盖行满显；默认行低透明常显，hover/焦点时满显，避免触控与键盘找不到入口
            overridden
              ? "opacity-100"
              : "opacity-45 group-hover/row:opacity-100 group-focus-within/row:opacity-100",
          )}
        >
          <Button
            aria-label={`修改 ${item.label}`}
            className="size-8"
            onClick={onEdit}
            size="icon"
            type="button"
            variant="ghost"
          >
            <Pencil className="size-3.5" />
          </Button>
          {overridden ? (
            <Button
              aria-label={`恢复 ${item.label} 为默认值`}
              className="size-8"
              onClick={onReset}
              size="icon"
              type="button"
              variant="ghost"
            >
              <RotateCcw className="size-3.5" />
            </Button>
          ) : null}
        </div>
      </Td>
    </Tr>
  );
}

export default SystemConfigPage;

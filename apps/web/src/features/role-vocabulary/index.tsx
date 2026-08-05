import { useMemo, useState } from "react";
import {
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Pencil, Plus, Tags } from "lucide-react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import {
  Button,
  DataTable,
  EmptyState,
  ErrorState,
  LoadingState,
  MetricCard,
  StatusPill,
  Td,
  Th,
  Tr,
  WorkSurface,
} from "@/components/superteam";
import { ApiRequestError } from "@/lib/api/client";
import {
  createRoleVocabulary,
  getRoleVocabularyReferences,
  listRoleVocabulary,
  patchRoleVocabulary,
  type RoleVocabularyEntry,
  type RoleVocabularyReferences,
} from "@/lib/api/casting";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { statusLabel } from "@/lib/status-labels";
import { CreateRoleDialog, EditRoleDialog } from "./role-dialogs";

export function RoleVocabularyPage() {
  const apiBaseUrl = resolveControlPlaneUrl();
  return <RoleVocabularyView apiBaseUrl={apiBaseUrl} />;
}

export function RoleVocabularyView({ apiBaseUrl }: { apiBaseUrl: string }) {
  const apiOptions = { baseUrl: apiBaseUrl };
  const queryClient = useQueryClient();
  const [showCreate, setShowCreate] = useState(false);
  const [editRow, setEditRow] = useState<RoleVocabularyEntry | null>(null);
  const [disableRow, setDisableRow] = useState<RoleVocabularyEntry | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const vocabulary = useQuery({
    queryKey: ["role-vocabulary"],
    queryFn: () => listRoleVocabulary(apiOptions),
  });

  const rows = vocabulary.data ?? [];

  // §4.2 被引用列：按行拉 references（词表规模小；停用弹窗复用同 cache）
  const referenceQueries = useQueries({
    queries: rows.map((row) => ({
      queryKey: ["role-vocabulary-references", row.role_key],
      queryFn: () => getRoleVocabularyReferences(apiOptions, row.role_key),
      staleTime: 30_000,
    })),
  });

  const referencesByKey = useMemo(() => {
    const map = new Map<string, RoleVocabularyReferences | "loading" | "error">();
    rows.forEach((row, index) => {
      const q = referenceQueries[index];
      if (!q || q.isPending) {
        map.set(row.role_key, "loading");
      } else if (q.isError || !q.data) {
        map.set(row.role_key, "error");
      } else {
        map.set(row.role_key, q.data);
      }
    });
    return map;
  }, [rows, referenceQueries]);

  const references = useQuery({
    queryKey: ["role-vocabulary-references", disableRow?.role_key],
    queryFn: () => getRoleVocabularyReferences(apiOptions, disableRow!.role_key),
    enabled: disableRow !== null,
  });

  const createMutation = useMutation({
    mutationFn: (input: { role_key: string; title: string; description: string }) =>
      createRoleVocabulary(apiOptions, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["role-vocabulary"] });
      void queryClient.invalidateQueries({ queryKey: ["role-vocabulary-references"] });
      setShowCreate(false);
      setActionError(null);
    },
    onError: (error: unknown) => {
      setActionError(apiErrorMessage(error, "创建失败"));
    },
  });

  const patchMutation = useMutation({
    mutationFn: (input: {
      roleKey: string;
      title?: string;
      description?: string;
      status?: "active" | "disabled";
    }) =>
      patchRoleVocabulary(apiOptions, input.roleKey, {
        title: input.title,
        description: input.description,
        status: input.status,
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["role-vocabulary"] });
      void queryClient.invalidateQueries({ queryKey: ["role-vocabulary-references"] });
      setEditRow(null);
      setDisableRow(null);
      setActionError(null);
    },
    onError: (error: unknown) => {
      setActionError(apiErrorMessage(error, "更新失败"));
    },
  });

  const activeCount = rows.filter((row) => row.status === "active").length;
  const isInitialLoading = vocabulary.isPending && rows.length === 0;
  const isBlockingError = vocabulary.isError && rows.length === 0;

  const disableDesc = useMemo(
    () => buildDisableDescription(disableRow, references.data, references.isPending, references.isError),
    [disableRow, references.data, references.isPending, references.isError],
  );

  return (
    <>
      <ShellPageHeader
        icon={<Tags />}
        iconTone="brand"
        title="角色词表"
        subtitle="租户级剧本角色注册表：编制与扩编的候选单位；停用前可见引用影响"
      />
      <Main width="wide" className="min-w-0 overflow-x-hidden">
        <div className="flex min-w-0 flex-col gap-6">
          <div className="flex flex-wrap items-center justify-start gap-2 sm:justify-end">
            <Button className="h-11 self-start px-5" onClick={() => setShowCreate(true)}>
              <Plus data-icon="inline-start" />
              新建角色
            </Button>
          </div>

          <section className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3" aria-label="角色词表指标">
            <MetricCard label="角色总数" value={`${rows.length}`} iconTone="brand" />
            <MetricCard label="启用中" value={`${activeCount}`} iconTone="ok" />
          </section>

          {actionError ? (
            <p className="text-sm text-destructive" role="alert">
              {actionError}
            </p>
          ) : null}

          <WorkSurface className="min-w-0">
            {isInitialLoading ? (
              <LoadingState label="加载角色词表…" />
            ) : isBlockingError ? (
              <ErrorState title="加载失败" description="无法加载角色词表" />
            ) : rows.length === 0 ? (
              <EmptyState
                icon={<Tags />}
                title="还没有角色"
                description="新建角色后可绑定给数字员工，并在项目编制中作为候选单位。"
              />
            ) : (
              <DataTable>
                <thead>
                  <tr>
                    <Th>角色</Th>
                    <Th>说明</Th>
                    <Th>状态</Th>
                    <Th>被引用</Th>
                    <Th aria-label="操作" />
                  </tr>
                </thead>
                <tbody>
                  {rows.map((row) => (
                    <Tr key={row.id}>
                      <Td>
                        <div className="min-w-0">
                          <p className="truncate font-medium text-ink">{row.title}</p>
                          <p className="truncate font-mono text-xs text-ink-2">{row.role_key}</p>
                        </div>
                      </Td>
                      <Td className="max-w-xs truncate text-sm text-ink-2">
                        {row.description || "—"}
                      </Td>
                      <Td>
                        <StatusPill tone={row.status === "active" ? "ok" : "mute"}>
                          {statusLabel(row.status)}
                        </StatusPill>
                      </Td>
                      <Td>
                        <ReferenceSummary refs={referencesByKey.get(row.role_key)} />
                      </Td>
                      <Td>
                        <div className="flex justify-end gap-1">
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => {
                              setActionError(null);
                              setEditRow(row);
                            }}
                          >
                            <Pencil className="size-3.5" />
                            编辑
                          </Button>
                          {row.status === "active" ? (
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => {
                                setActionError(null);
                                setDisableRow(row);
                              }}
                            >
                              停用
                            </Button>
                          ) : (
                            <Button
                              variant="primary"
                              size="sm"
                              disabled={patchMutation.isPending}
                              onClick={() =>
                                patchMutation.mutate({
                                  roleKey: row.role_key,
                                  status: "active",
                                })
                              }
                            >
                              启用
                            </Button>
                          )}
                        </div>
                      </Td>
                    </Tr>
                  ))}
                </tbody>
              </DataTable>
            )}
          </WorkSurface>
        </div>
      </Main>

      <CreateRoleDialog
        open={showCreate}
        onOpenChange={setShowCreate}
        isSubmitting={createMutation.isPending}
        onSubmit={(input) => createMutation.mutate(input)}
        error={createMutation.isError ? apiErrorMessage(createMutation.error, "创建失败") : null}
      />

      <EditRoleDialog
        entry={editRow}
        onOpenChange={(open) => {
          if (!open) setEditRow(null);
        }}
        isSubmitting={patchMutation.isPending}
        onSubmit={(input) => {
          if (!editRow) return;
          patchMutation.mutate({
            roleKey: editRow.role_key,
            title: input.title,
            description: input.description,
          });
        }}
        error={
          editRow && patchMutation.isError
            ? apiErrorMessage(patchMutation.error, "更新失败")
            : null
        }
      />

      <ConfirmDialog
        open={disableRow !== null}
        onOpenChange={(open) => {
          if (!open) setDisableRow(null);
        }}
        title={`停用角色 ${disableRow?.title ?? ""}`}
        desc={disableDesc}
        confirmText="确认停用"
        destructive
        disabled={references.isPending || references.isError}
        isLoading={patchMutation.isPending}
        handleConfirm={() => {
          if (!disableRow) return;
          patchMutation.mutate({ roleKey: disableRow.role_key, status: "disabled" });
        }}
      />
    </>
  );
}

function ReferenceSummary({
  refs,
}: {
  refs: RoleVocabularyReferences | "loading" | "error" | undefined;
}) {
  if (!refs || refs === "loading") {
    return <span className="text-xs text-ink-3">统计中…</span>;
  }
  if (refs === "error") {
    return <span className="text-xs text-destructive">无法加载</span>;
  }
  const templateCount = refs.scenario_templates.length;
  return (
    <div className="text-xs text-ink-2 tabular-nums">
      <span title="引用该角色的剧本数">剧本 {templateCount}</span>
      <span className="mx-1 text-ink-3">·</span>
      <span title="持有该角色的员工数">员工 {refs.employee_count}</span>
      <span className="mx-1 text-ink-3">·</span>
      <span title="已有编制行数">编制 {refs.casting_count}</span>
    </div>
  );
}

function buildDisableDescription(
  row: RoleVocabularyEntry | null,
  refs: RoleVocabularyReferences | undefined,
  loading: boolean,
  errored: boolean,
) {
  if (!row) return "";
  if (loading) {
    return <p className="text-sm text-ink-2">正在统计引用影响…</p>;
  }
  if (errored || !refs) {
    return <p className="text-sm text-destructive">无法加载引用统计，请稍后重试。</p>;
  }

  const templateNames =
    refs.scenario_templates.length > 0
      ? refs.scenario_templates.map((t) => t.name || t.key).join("、")
      : "无";
  const employeeNames =
    refs.employees.length > 0 ? refs.employees.map((e) => e.name).join("、") : "无";

  return (
    <div className="flex flex-col gap-2 text-sm">
      <p>
        停用后该角色不再作为编制候选；历史编制行仍保留。请确认以下引用影响：
      </p>
      <ul className="list-disc space-y-1 pl-5 text-ink-2">
        <li>
          <span className="font-medium text-ink">剧本</span>（{refs.scenario_templates.length}）
          ：{templateNames}
          <span className="block text-xs">
            引用此角色的剧本改版时将无法通过角色词表校验
          </span>
        </li>
        <li>
          <span className="font-medium text-ink">持有员工</span>（{refs.employee_count}）
          ：{employeeNames}
          <span className="block text-xs">这些员工不再作为该角色的候选出现</span>
        </li>
        <li>
          <span className="font-medium text-ink">已有编制</span>：{refs.casting_count} 条
          <span className="block text-xs">
            编制行仍在（历史事实），但可达收口会认为该角色无人可用
          </span>
        </li>
      </ul>
    </div>
  );
}

function apiErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof ApiRequestError) {
    if (error.status === 409) {
      return error.detail || "role_key 已存在";
    }
    if (error.status === 400) {
      return error.detail || "输入不合法：role_key 须为下划线小写";
    }
    return error.detail || error.message || fallback;
  }
  if (error instanceof Error && error.message) {
    return error.message;
  }
  return fallback;
}

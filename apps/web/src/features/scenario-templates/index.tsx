import { Fragment, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { LayoutTemplate, Plus } from "lucide-react";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import {
  ActionMenu,
  Button,
  Chip,
  EmptyNoMatch,
  EmptyState,
  ErrorState,
  ListToolbar,
  Pagination,
  StatusPill,
  TableSkeleton,
  ToolbarSearch,
  WorkSurface,
  DataTable,
  Td,
  Th,
  Tr
} from "@/components/superteam";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { ApiRequestError } from "@/lib/api/client";
import {
  listScenarioTemplates,
  patchScenarioTemplate,
  deleteScenarioTemplate,
  type ScenarioTemplate
} from "@/lib/api/scenario-templates";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { formatRelativeTime } from "@/lib/format-time";
import { statusLabel } from "@/lib/status-labels";
import {
  listChainFromSpec,
  type ListChainGroup,
  type ListChainPreview,
  type ListChainStation
} from "./spec-composer";

type StatusFilter = "all" | "active" | "disabled";

const PAGE_SIZE_OPTIONS = [10, 20, 50];

function matchesNameQuery(row: ScenarioTemplate, query: string): boolean {
  const needle = query.trim().toLowerCase();
  if (!needle) return true;
  return (
    row.name.toLowerCase().includes(needle) ||
    row.template_key.toLowerCase().includes(needle)
  );
}

export function ScenarioTemplatesPage() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const queryClient = useQueryClient();
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [nameQuery, setNameQuery] = useState("");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [statusToggleRow, setStatusToggleRow] = useState<ScenarioTemplate | null>(null);
  const [statusToggleError, setStatusToggleError] = useState<string | null>(null);
  const [deleteRow, setDeleteRow] = useState<ScenarioTemplate | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const templates = useQuery({
    queryKey: ["scenario-templates"],
    queryFn: () => listScenarioTemplates({ baseUrl: apiBaseUrl })
  });

  const patchMutation = useMutation({
    mutationFn: (input: { key: string; status: "active" | "disabled" }) =>
      patchScenarioTemplate({ baseUrl: apiBaseUrl }, input.key, { status: input.status }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["scenario-templates"] });
      setStatusToggleError(null);
      setStatusToggleRow(null);
    },
    onError: (error: unknown) => {
      setStatusToggleError(
        error instanceof ApiRequestError && error.detail
          ? error.detail
          : error instanceof Error
            ? error.message
            : "更新状态失败",
      );
    }
  });

  const deleteMutation = useMutation({
    mutationFn: (key: string) => deleteScenarioTemplate({ baseUrl: apiBaseUrl }, key),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["scenario-templates"] });
      setDeleteError(null);
      setDeleteRow(null);
    },
    onError: (error: unknown) => {
      setDeleteError(
        error instanceof ApiRequestError && error.detail
          ? error.detail
          : error instanceof Error
            ? error.message
            : "删除失败",
      );
    }
  });

  const rows = templates.data ?? [];
  const isInitialLoading = templates.isPending && rows.length === 0;
  const isBlockingError = templates.isError && rows.length === 0;
  const activeCount = rows.filter((row) => row.status === "active").length;
  const disabledCount = rows.filter((row) => row.status === "disabled").length;
  const filteredRows = useMemo(() => {
    const byStatus =
      statusFilter === "all" ? rows : rows.filter((row) => row.status === statusFilter);
    return byStatus.filter((row) => matchesNameQuery(row, nameQuery));
  }, [nameQuery, rows, statusFilter]);
  const pageCount = Math.max(1, Math.ceil(filteredRows.length / pageSize));
  const activePage = Math.min(page, pageCount);
  const visibleRows = useMemo(
    () => filteredRows.slice((activePage - 1) * pageSize, activePage * pageSize),
    [activePage, filteredRows, pageSize],
  );
  const nextStatus = statusToggleRow?.status === "active" ? "disabled" : "active";
  const hasActiveFilters = statusFilter !== "all" || nameQuery.trim() !== "";

  const resetFilters = () => {
    setStatusFilter("all");
    setNameQuery("");
    setPage(1);
  };

  return (
    <>
      <ShellPageHeader
        icon={<LayoutTemplate />}
        iconTone="brand"
        title="场景模板"
        subtitle="声明席位与收口档位。新规划只匹配启用中的模板。"
      />
      <Main width="wide" className="min-w-0 overflow-x-hidden">
        <div className="flex min-w-0 flex-col gap-6">
          <div className="flex flex-wrap items-center justify-start gap-2 sm:justify-end">
            <Button asChild className="h-11 self-start px-5">
              <Link to="/scenario-templates/new">
                <Plus data-icon="inline-start" />
                新建模板
              </Link>
            </Button>
          </div>

          <WorkSurface className="min-w-0">
            {isInitialLoading ? (
              <div className="p-4">
                <TableSkeleton rows={6} cols={5} />
              </div>
            ) : isBlockingError ? (
              <ErrorState title="加载失败" description="无法加载场景模板" />
            ) : rows.length === 0 ? (
              <EmptyState
                icon={<LayoutTemplate />}
                title="还没有场景模板"
                description="新建一个场景模板，或依赖种子数据；发起需求或配置自动化时可选用其一驱动规划。"
                action={
                  <Button asChild>
                    <Link to="/scenario-templates/new">新建模板</Link>
                  </Button>
                }
              />
            ) : (
              <>
                <ListToolbar
                  search={
                    <ToolbarSearch
                      aria-label="搜索模板名称"
                      placeholder="搜索模板名称"
                      type="search"
                      value={nameQuery}
                      onChange={(event) => {
                        setNameQuery(event.target.value);
                        setPage(1);
                      }}
                    />
                  }
                  filters={
                    <>
                      <Chip
                        type="button"
                        active={statusFilter === "all"}
                        aria-pressed={statusFilter === "all"}
                        count={rows.length}
                        onClick={() => {
                          setStatusFilter("all");
                          setPage(1);
                        }}
                      >
                        全部
                      </Chip>
                      <Chip
                        type="button"
                        active={statusFilter === "active"}
                        aria-pressed={statusFilter === "active"}
                        count={activeCount}
                        onClick={() => {
                          setStatusFilter("active");
                          setPage(1);
                        }}
                      >
                        启用中
                      </Chip>
                      <Chip
                        type="button"
                        active={statusFilter === "disabled"}
                        aria-pressed={statusFilter === "disabled"}
                        count={disabledCount}
                        onClick={() => {
                          setStatusFilter("disabled");
                          setPage(1);
                        }}
                      >
                        已禁用
                      </Chip>
                    </>
                  }
                />
                {filteredRows.length === 0 ? (
                  <EmptyNoMatch
                    title="没有符合筛选的模板"
                    description={
                      hasActiveFilters
                        ? "没有名称或状态匹配的模板。已禁用的仍可在此启用或删除。"
                        : "当前状态下没有模板。"
                    }
                    action={
                      <Button variant="outline" onClick={resetFilters}>
                        查看全部
                      </Button>
                    }
                  />
                ) : (
                  <>
                    <DataTable>
                      <thead>
                        <tr>
                          <Th>模板</Th>
                          <Th>骨架</Th>
                          <Th className="hidden md:table-cell">最深收口</Th>
                          <Th>状态</Th>
                          <Th className="hidden md:table-cell">更新</Th>
                          <Th aria-label="操作" />
                        </tr>
                      </thead>
                      <tbody>
                        {visibleRows.map((row) => (
                          <ScenarioTemplateRow
                            key={row.id}
                            row={row}
                            onRequestStatusToggle={() => {
                              setStatusToggleError(null);
                              setStatusToggleRow(row);
                            }}
                            onRequestDelete={() => {
                              setDeleteError(null);
                              setDeleteRow(row);
                            }}
                          />
                        ))}
                      </tbody>
                    </DataTable>
                    <Pagination
                      total={filteredRows.length}
                      page={activePage}
                      pageSize={pageSize}
                      pageCount={pageCount}
                      pageSizeOptions={PAGE_SIZE_OPTIONS}
                      onPageChange={setPage}
                      onPageSizeChange={(size) => {
                        setPageSize(size);
                        setPage(1);
                      }}
                    />
                  </>
                )}
              </>
            )}
          </WorkSurface>
        </div>
      </Main>

      <ConfirmDialog
        open={statusToggleRow !== null}
        onOpenChange={(open) => {
          if (!open) {
            setStatusToggleRow(null);
            setStatusToggleError(null);
          }
        }}
        title={
          nextStatus === "disabled"
            ? `停用 ${statusToggleRow?.template_key ?? ""}`
            : `启用 ${statusToggleRow?.template_key ?? ""}`
        }
        desc={
          nextStatus === "disabled" ? (
            <div className="flex flex-col gap-2">
              <p>
                停用后新规划请求将回落到通用（generic）行为，不再匹配该场景模板的分解骨架与验收判据。已实例化的项目不受影响。
              </p>
              {statusToggleError ? (
                <p className="text-danger">{statusToggleError}</p>
              ) : null}
            </div>
          ) : (
            <div className="flex flex-col gap-2">
              <p>启用后新规划请求将重新匹配该场景模板的分解骨架与验收判据。</p>
              {statusToggleError ? (
                <p className="text-danger">{statusToggleError}</p>
              ) : null}
            </div>
          )
        }
        confirmText={nextStatus === "disabled" ? "确认停用" : "确认启用"}
        destructive={nextStatus === "disabled"}
        isLoading={patchMutation.isPending}
        handleConfirm={() => {
          if (statusToggleRow) {
            patchMutation.mutate({ key: statusToggleRow.template_key, status: nextStatus });
          }
        }}
      />
      <ConfirmDialog
        open={deleteRow !== null}
        onOpenChange={(open) => {
          if (!open) {
            setDeleteRow(null);
            setDeleteError(null);
          }
        }}
        title={`删除 ${deleteRow?.name ?? ""}？`}
        desc={
          <div className="flex flex-col gap-2">
            <p>
              删除后列表和任务发起不再出现「{deleteRow?.name}」。已经按它规划过的需求仍保留当时的标识。
            </p>
            {deleteError ? <p className="text-danger">{deleteError}</p> : null}
          </div>
        }
        confirmText="确认删除"
        destructive
        isLoading={deleteMutation.isPending}
        handleConfirm={() => {
          if (deleteRow) {
            deleteMutation.mutate(deleteRow.template_key);
          }
        }}
      />
    </>
  );
}

function ScenarioTemplateRow({
  row,
  onRequestStatusToggle,
  onRequestDelete
}: {
  row: ScenarioTemplate;
  onRequestStatusToggle: () => void;
  onRequestDelete: () => void;
}) {
  const navigate = useNavigate();
  const preview = listChainFromSpec(row.spec);
  const dimmed = row.status !== "active";

  return (
    <Tr
      className="cursor-pointer"
      onClick={() =>
        void navigate({
          to: "/scenario-templates/$templateKey/edit",
          params: { templateKey: row.template_key },
        })
      }
    >
      <Td>
        <div className="min-w-0">
          <Link
            className={dimmed ? "font-medium text-ink-2 hover:underline" : "font-medium text-ink hover:underline"}
            to="/scenario-templates/$templateKey/edit"
            params={{ templateKey: row.template_key }}
            onClick={(event) => event.stopPropagation()}
          >
            {row.name}
          </Link>
          {row.description ? (
            <p className={`mt-0.5 line-clamp-2 text-xs ${dimmed ? "text-ink-2" : "text-ink-2"}`}>
              {row.description}
            </p>
          ) : null}
          <p className="mt-1 truncate font-mono text-xs text-ink-3">{row.template_key}</p>
        </div>
      </Td>
      <Td>
        <TemplateChain preview={preview} />
      </Td>
      <Td className="hidden text-sm md:table-cell">
        {preview.deepestExit ? (
          preview.deepestExit
        ) : (
          <span className="text-ink-3">generic</span>
        )}
      </Td>
      <Td>
        <StatusPill tone={row.status === "active" ? "ok" : "mute"}>
          {statusLabel(row.status)}
        </StatusPill>
      </Td>
      <Td className="hidden text-xs text-ink-2 tabular-nums md:table-cell">
        {formatRelativeTime(row.updated_at)}
      </Td>
      <Td>
        <div className="flex justify-end gap-1" onClick={(event) => event.stopPropagation()}>
          <Button asChild variant="outline" size="sm">
            <Link
              to="/scenario-templates/$templateKey/edit"
              params={{ templateKey: row.template_key }}
            >
              编辑
            </Link>
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={onRequestStatusToggle}
          >
            {row.status === "active" ? "停用" : "启用"}
          </Button>
          <ActionMenu
            label="更多操作"
            items={[
              {
                key: "delete",
                label: "删除",
                destructive: true,
                onSelect: onRequestDelete
              }
            ]}
          />
        </div>
      </Td>
    </Tr>
  );
}

function TemplateChain({ preview }: { preview: ListChainPreview }) {
  if (preview.empty) {
    return <p className="text-xs italic text-ink-3">无骨架 · generic 行为</p>;
  }
  const overflow = preview.groups.length > 5;
  const visible = overflow ? preview.groups.slice(0, 4) : preview.groups;
  const hidden = overflow ? preview.groups.length - 4 : 0;
  return (
    <div className="flex max-w-[36rem] flex-wrap items-center gap-1" title={preview.tooltip}>
      {visible.map((group, index) => (
        <Fragment key={`${group.stations.map((item) => item.step).join("-")}-${index}`}>
          {index > 0 ? <span className="text-[11px] text-ink-3">→</span> : null}
          <ChainGroup group={group} />
        </Fragment>
      ))}
      {hidden > 0 ? <span className="text-xs text-ink-3">+{hidden}</span> : null}
    </div>
  );
}

function ChainGroup({ group }: { group: ListChainGroup }) {
  if (group.stations.length === 1) {
    return <ChainStation station={group.stations[0]!} />;
  }
  return (
    <span className="inline-flex items-center gap-1 rounded-[10px] border border-dashed border-line-strong bg-card-soft p-0.5">
      {group.stations.map((station, index) => (
        <Fragment key={station.step}>
          {index > 0 ? <span className="text-[11px] font-semibold text-ink-3">∥</span> : null}
          <ChainStation station={station} />
        </Fragment>
      ))}
    </span>
  );
}

function ChainStation({ station }: { station: ListChainStation }) {
  return (
    <span
      className={
        station.exit
          ? "inline-flex items-center gap-1.5 rounded-lg border border-line bg-card px-2 py-0.5 text-xs"
          : "inline-flex items-center gap-1.5 rounded-lg border border-line bg-card-soft px-2 py-0.5 text-xs"
      }
    >
      {station.exit ? (
        <span
          aria-label="可收口"
          title={station.exitLabel || "可收口"}
          className="size-1.5 shrink-0 rounded-full bg-brand ring-2 ring-brand-soft"
        />
      ) : null}
      <span className="font-medium text-ink">{station.title}</span>
    </span>
  );
}

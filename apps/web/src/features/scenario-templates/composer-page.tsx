import { useEffect, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { LayoutTemplate } from "lucide-react";
import { Main } from "@/components/layout/main";
import {
  ShellPageHeader,
  ShellPageHeaderBack,
} from "@/components/layout/shell-page-header";
import {
  Button,
  Callout,
  ErrorState,
  GlassCard,
  LoadingState,
  SoftCard,
  StatusPill,
} from "@/components/superteam";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { ApiRequestError } from "@/lib/api/client";
import { listRoleVocabulary, type RoleVocabularyEntry } from "@/lib/api/casting";
import {
  createScenarioTemplate,
  createScenarioTemplateVersion,
  deleteScenarioTemplate,
  getScenarioTemplate,
  listScenarioTemplates,
  patchScenarioTemplate,
  type ScenarioTemplate,
} from "@/lib/api/scenario-templates";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { cn } from "@/lib/utils";
import {
  composerFromSpec,
  emptyComposerDraft,
  exitPreviewRows,
  isSerialSkeleton,
  isValidTemplateKey,
  newComposerNode,
  nodeDisplayTitle,
  specFromComposer,
  suggestTemplateKey,
  validateComposerDraft,
  type ComposerDraft,
  type ComposerNode,
  type NodeVerify,
} from "./spec-composer";

const VERIFY_LABEL: Record<NodeVerify, string> = {
  none: "交出即可",
  independent: "另一角色独立验证",
  human: "必须人类确认",
};

const inputClass =
  "h-9 w-full rounded-[10px] border border-line bg-card-inner px-2.5 text-[13px] text-ink outline-none transition-colors focus:border-brand focus:ring-2 focus:ring-brand/25 disabled:opacity-60";
const selectClass = `${inputClass} text-ink`;

const FEATURED_COPY_LIMIT = 4;

type ComposerPageProps = {
  mode: "create" | "edit";
  templateKey?: string;
};

export function ScenarioTemplateComposerPage({ mode, templateKey }: ComposerPageProps) {
  const apiBaseUrl = resolveControlPlaneUrl();
  const apiOptions = { baseUrl: apiBaseUrl };
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const [name, setName] = useState("");
  const [key, setKey] = useState("");
  const [description, setDescription] = useState("");
  const [keyTouched, setKeyTouched] = useState(false);
  const [draft, setDraft] = useState<ComposerDraft>(emptyComposerDraft);
  const [selectedId, setSelectedId] = useState(() => emptyComposerDraft().nodes[0]?.id ?? "");
  const [copyFrom, setCopyFrom] = useState("");
  const [showJson, setShowJson] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [dirty, setDirty] = useState(false);
  const [leaveOpen, setLeaveOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [originalSpec, setOriginalSpec] = useState<Record<string, unknown>>({});

  const vocabulary = useQuery({
    queryKey: ["role-vocabulary"],
    queryFn: () => listRoleVocabulary(apiOptions),
  });
  const templates = useQuery({
    enabled: mode === "create",
    queryKey: ["scenario-templates"],
    queryFn: () => listScenarioTemplates(apiOptions),
  });
  const existing = useQuery({
    enabled: mode === "edit" && Boolean(templateKey),
    queryKey: ["scenario-template", templateKey],
    queryFn: () => getScenarioTemplate(apiOptions, templateKey ?? ""),
  });

  const serialLocked = Boolean(
    mode === "edit" && existing.data && !isSerialSkeleton(existing.data.spec),
  );

  useEffect(() => {
    if (mode !== "edit" || !existing.data) return;
    const next = composerFromSpec(existing.data.spec);
    setName(existing.data.name);
    setKey(existing.data.template_key);
    setDescription(existing.data.description);
    setDraft(next);
    setSelectedId(next.nodes[0]?.id ?? "");
    setOriginalSpec(existing.data.spec);
    setDirty(false);
  }, [existing.data, mode]);

  const vocab = vocabulary.data ?? [];
  const activeRoles = vocab.filter((entry) => entry.status === "active");
  const selected = draft.nodes.find((node) => node.id === selectedId) ?? draft.nodes[0];
  const specPreview = specFromComposer(draft, vocab, originalSpec);
  const previewRows = exitPreviewRows(draft, vocab);
  const copyOptions = (templates.data ?? []).filter(
    (item) => item.status === "active" && isSerialSkeleton(item.spec),
  );
  const featuredCopy = copyOptions.slice(0, FEATURED_COPY_LIMIT);
  const extraCopy = copyOptions.slice(FEATURED_COPY_LIMIT);

  function markDraft(next: ComposerDraft) {
    setDraft(next);
    setDirty(true);
  }

  function updateNode(id: string, patch: Partial<ComposerNode>) {
    markDraft({
      nodes: draft.nodes.map((node) => (node.id === id ? { ...node, ...patch } : node)),
    });
  }

  function moveNode(index: number, direction: -1 | 1) {
    const next = index + direction;
    if (next < 0 || next >= draft.nodes.length) return;
    const nodes = [...draft.nodes];
    const [removed] = nodes.splice(index, 1);
    nodes.splice(next, 0, removed!);
    markDraft({ nodes });
  }

  function addNode() {
    const previous = draft.nodes[draft.nodes.length - 1];
    const created = newComposerNode(draft.nodes.length + 1, previous?.step);
    markDraft({ nodes: [...draft.nodes, created] });
    setSelectedId(created.id);
  }

  function removeNode(id: string) {
    if (draft.nodes.length <= 1) return;
    const nodes = draft.nodes.filter((node) => node.id !== id);
    markDraft({ nodes });
    if (selectedId === id) setSelectedId(nodes[0]?.id ?? "");
  }

  function applyCopy(source: ScenarioTemplate) {
    const next = composerFromSpec(source.spec);
    setDraft(next);
    setSelectedId(next.nodes[0]?.id ?? "");
    setOriginalSpec(source.spec);
    setCopyFrom(source.template_key);
    setDirty(true);
  }

  function applyBlankStart() {
    setCopyFrom("");
    setDraft(emptyComposerDraft());
    setOriginalSpec({});
    setSelectedId(emptyComposerDraft().nodes[0]?.id ?? "");
    setDirty(true);
  }

  const saveMutation = useMutation({
    mutationFn: async () => {
      if (!name.trim() || (mode === "create" && !key.trim())) {
        throw new Error("请填写名称和内部标识");
      }
      if (mode === "create" && !isValidTemplateKey(key)) {
        throw new Error("内部标识只能用英文字母、数字和下划线，且必须以字母开头");
      }
      if (mode === "create") {
        const issues = validateComposerDraft(draft);
        if (issues.length > 0) {
          throw new Error(issues[0]?.message ?? "请补全关键节点");
        }
        return createScenarioTemplate(apiOptions, {
          template_key: key.trim(),
          name: name.trim(),
          description: description.trim(),
          spec: specFromComposer(draft, vocab, originalSpec),
        });
      }
      const target = templateKey ?? "";
      if (!serialLocked) {
        const issues = validateComposerDraft(draft);
        if (issues.length > 0) {
          throw new Error(issues[0]?.message ?? "请补全关键节点");
        }
        await createScenarioTemplateVersion(apiOptions, target, {
          spec: specFromComposer(draft, vocab, originalSpec),
        });
      }
      return patchScenarioTemplate(apiOptions, target, {
        name: name.trim(),
        description: description.trim(),
      });
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["scenario-templates"] });
      setDirty(false);
      void navigate({ to: "/scenario-templates" });
    },
    onError: (error: unknown) => {
      setFormError(
        error instanceof ApiRequestError && error.detail
          ? error.detail
          : error instanceof Error
            ? error.message
            : "保存失败",
      );
    },
  });

  const deleteMutation = useMutation({
    mutationFn: () => deleteScenarioTemplate(apiOptions, templateKey ?? ""),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["scenario-templates"] });
      void navigate({ to: "/scenario-templates" });
    },
    onError: (error: unknown) => {
      setFormError(
        error instanceof ApiRequestError && error.detail
          ? error.detail
          : error instanceof Error
            ? error.message
            : "删除失败",
      );
    },
  });

  function requestLeave() {
    if (dirty) {
      setLeaveOpen(true);
      return;
    }
    void navigate({ to: "/scenario-templates" });
  }

  const skeletonReadOnly = serialLocked;
  const saveDisabled = saveMutation.isPending || (mode === "edit" && !dirty);

  const actionButtons = (
    <div className="flex shrink-0 flex-wrap items-center gap-2">
      {mode === "edit" ? (
        <Button type="button" variant="outline" onClick={() => setDeleteOpen(true)}>
          删除
        </Button>
      ) : null}
      <Button type="button" variant="outline" onClick={requestLeave}>
        取消
      </Button>
      <Button
        type="button"
        disabled={saveDisabled}
        onClick={() => {
          setFormError(null);
          saveMutation.mutate();
        }}
      >
        {saveMutation.isPending ? "保存中…" : mode === "create" ? "保存" : "保存更改"}
      </Button>
    </div>
  );

  if (mode === "edit" && existing.isPending) {
    return (
      <>
        <ShellPageHeader
          back={<ShellPageHeaderBack ariaLabel="返回场景模板" to="/scenario-templates" />}
          icon={<LayoutTemplate />}
          iconTone="brand"
          title="编辑模板"
        />
        <Main width="contained">
          <LoadingState label="加载场景模板…" />
        </Main>
      </>
    );
  }

  if (mode === "edit" && existing.isError) {
    return (
      <>
        <ShellPageHeader
          back={<ShellPageHeaderBack ariaLabel="返回场景模板" to="/scenario-templates" />}
          icon={<LayoutTemplate />}
          iconTone="brand"
          title="编辑模板"
        />
        <Main width="contained">
          <ErrorState title="加载失败" description="无法加载该场景模板" />
        </Main>
      </>
    );
  }

  return (
    <>
      <ShellPageHeader
        back={<ShellPageHeaderBack ariaLabel="返回场景模板" to="/scenario-templates" />}
        icon={<LayoutTemplate />}
        iconTone="brand"
        title={mode === "create" ? "新建模板" : "编辑模板"}
      />
      <Main width="contained" className="min-w-0 pb-16">
        <div className="flex min-w-0 flex-col gap-4">
          {serialLocked ? (
            <Callout
              tone="warn"
              title="此模板含并行依赖"
              description="本期编辑器不能保存骨架。名称和描述仍可改。"
            />
          ) : null}

          <GlassCard className="p-4 @3xl/content:p-5">
            <div className="mb-3 flex flex-wrap items-start justify-between gap-3">
              <div className="min-w-0">
                <p className="text-[11px] font-semibold tracking-[0.08em] text-ink-3">模板身份</p>
                <p className="mt-0.5 text-[12.5px] text-ink-2">
                  名称和描述会出现在任务发起的模板卡片上。
                </p>
              </div>
              {actionButtons}
            </div>
            <div className="grid gap-3 @3xl/content:grid-cols-[minmax(0,1.2fr)_minmax(10rem,0.7fr)_minmax(0,1.4fr)]">
              <label className="flex min-w-0 flex-col gap-1">
                <span className="text-[11px] font-semibold text-ink-3">
                  名称<span className="ml-0.5 text-danger">*</span>
                </span>
                <input
                  aria-label="名称"
                  className={inputClass}
                  value={name}
                  onChange={(event) => {
                    const next = event.target.value;
                    setName(next);
                    setDirty(true);
                    if (mode === "create" && !keyTouched) {
                      setKey(suggestTemplateKey(next));
                    }
                  }}
                />
              </label>
              <label className="flex min-w-0 flex-col gap-1">
                <span className="text-[11px] font-semibold text-ink-3">
                  内部标识<span className="ml-0.5 text-danger">*</span>
                </span>
                <input
                  aria-label="内部标识"
                  className={`${inputClass} font-mono text-[12px]`}
                  value={key}
                  disabled={mode === "edit"}
                  placeholder="ops_review"
                  onChange={(event) => {
                    setKeyTouched(true);
                    setKey(event.target.value);
                    setDirty(true);
                  }}
                />
                <span className="text-[11px] leading-4 text-ink-3">
                  {mode === "edit"
                    ? "创建后不能改。系统和链接用这一串，列表和发起页仍显示上面的名称。"
                    : "只能用英文字母、数字和下划线，且必须以字母开头；创建后不能改。"}
                </span>
              </label>
              <label className="flex min-w-0 flex-col gap-1">
                <span className="text-[11px] font-semibold text-ink-3">描述</span>
                <input
                  aria-label="描述"
                  className={inputClass}
                  value={description}
                  onChange={(event) => {
                    setDescription(event.target.value);
                    setDirty(true);
                  }}
                />
              </label>
            </div>
            {mode === "create" ? (
              <fieldset className="mt-3 min-w-0">
                <legend className="text-[11px] font-semibold text-ink-3">起步方式</legend>
                <div className="mt-1.5 flex flex-wrap items-center gap-1.5" role="radiogroup" aria-label="起步方式">
                  <CopyChip
                    pressed={copyFrom === ""}
                    onClick={applyBlankStart}
                  >
                    空白开做
                  </CopyChip>
                  {featuredCopy.map((item) => (
                    <CopyChip
                      key={item.template_key}
                      pressed={copyFrom === item.template_key}
                      onClick={() => applyCopy(item)}
                    >
                      复制：{item.name}
                    </CopyChip>
                  ))}
                  {extraCopy.length > 0 ? (
                    <select
                      aria-label="更多可复制模板"
                      className="h-8 max-w-[12rem] rounded-full border border-line-strong bg-card px-3 text-[12px] text-ink-2 outline-none focus:border-brand"
                      value={extraCopy.some((item) => item.template_key === copyFrom) ? copyFrom : ""}
                      onChange={(event) => {
                        const value = event.target.value;
                        if (!value) return;
                        const source = extraCopy.find((item) => item.template_key === value);
                        if (source) applyCopy(source);
                      }}
                    >
                      <option value="">更多模板…</option>
                      {extraCopy.map((item) => (
                        <option key={item.template_key} value={item.template_key}>
                          {item.name}
                        </option>
                      ))}
                    </select>
                  ) : null}
                </div>
              </fieldset>
            ) : null}
          </GlassCard>

          <SoftCard className="@container/composer overflow-hidden p-0">
            <div className="border-b border-line bg-card-soft/60 px-4 py-3">
              <div className="mb-2 flex items-baseline justify-between gap-3">
                <h2 className="text-[13px] font-extrabold text-ink">关卡</h2>
                <span className="text-[11px] text-ink-3">
                  {draft.nodes.length} 站 · {draft.nodes.filter((node) => node.exit).length} 个出口 · 蓝点可收口
                </span>
              </div>
              <ChainOverview
                draft={draft}
                vocabulary={vocab}
                selectedId={selected?.id}
                onSelect={setSelectedId}
              />
            </div>

            <div className="grid min-w-0 items-stretch gap-0 @4xl/composer:grid-cols-[minmax(16.5rem,19.5rem)_minmax(0,1fr)]">
              <div className="flex min-w-0 flex-col border-b border-line p-3 @4xl/composer:border-r @4xl/composer:border-b-0">
                <div className="flex flex-col gap-1.5">
                  {draft.nodes.map((node, index) => (
                    <button
                      key={node.id}
                      type="button"
                      className={cn(
                        "flex w-full items-start gap-2.5 rounded-[12px] border px-3 py-2.5 text-left transition-colors",
                        node.id === selected?.id
                          ? "border-brand bg-brand-soft shadow-[inset_3px_0_0_0_var(--brand)]"
                          : "border-transparent bg-card hover:border-line-strong hover:bg-card-inner",
                      )}
                      onClick={() => setSelectedId(node.id)}
                    >
                      <span className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-[8px] bg-card-soft font-mono text-[11px] font-extrabold tabular-nums text-ink-3">
                        {String(index + 1).padStart(2, "0")}
                      </span>
                      <span className="min-w-0 flex-1">
                        <span className="flex min-w-0 items-center gap-1.5">
                          <span className="truncate text-[13px] font-extrabold leading-5 text-ink">
                            {nodeDisplayTitle(node, vocab)}
                          </span>
                          {node.exit ? (
                            <StatusPill className="shrink-0 px-1.5 py-0.5 text-[10px]" showDot={false} tone="brand">
                              出口
                            </StatusPill>
                          ) : null}
                          {node.verify === "human" ? (
                            <StatusPill className="shrink-0 px-1.5 py-0.5 text-[10px]" showDot={false} tone="warn">
                              人类门
                            </StatusPill>
                          ) : null}
                        </span>
                        <span className="mt-0.5 block truncate text-[11px] text-ink-3">
                          {roleTitle(node.roleKey, vocab)} · {VERIFY_LABEL[node.verify]}
                        </span>
                      </span>
                    </button>
                  ))}
                </div>
                <Button
                  className="mt-2 w-full border-dashed"
                  type="button"
                  variant="outline"
                  disabled={skeletonReadOnly}
                  onClick={addNode}
                >
                  + 添加一站
                </Button>
              </div>

              {selected ? (
                <div className="flex min-w-0 flex-col">
                  <Inspector
                    node={selected}
                    index={draft.nodes.findIndex((node) => node.id === selected.id)}
                    total={draft.nodes.length}
                    vocabulary={vocab}
                    activeRoles={activeRoles}
                    vocabularyLoading={vocabulary.isPending}
                    vocabularyError={vocabulary.isError}
                    readOnly={skeletonReadOnly}
                    onChange={(patch) => updateNode(selected.id, patch)}
                    onMove={(direction) =>
                      moveNode(draft.nodes.findIndex((node) => node.id === selected.id), direction)
                    }
                    onDelete={() => removeNode(selected.id)}
                  />
                  <ExitPreview rows={previewRows} />
                </div>
              ) : null}
            </div>
          </SoftCard>

          <button
            type="button"
            className="self-start text-[11px] text-ink-3 underline decoration-line-strong underline-offset-2"
            onClick={() => setShowJson((current) => !current)}
          >
            {showJson ? "收起 JSON 对照" : "高级：查看 JSON 对照"}
          </button>
          {showJson ? (
            <pre className="max-h-56 overflow-auto rounded-[12px] border border-line bg-card-inner px-3 py-2 font-mono text-[11px] text-ink-2">
              {JSON.stringify(specPreview, null, 2)}
            </pre>
          ) : null}

          {formError ? <p className="text-sm text-danger">{formError}</p> : null}
        </div>
      </Main>
      <ConfirmDialog
        open={leaveOpen}
        title="放弃未保存的更改？"
        desc="离开后当前编辑不会保存。"
        confirmText="放弃更改"
        onOpenChange={setLeaveOpen}
        handleConfirm={() => {
          setLeaveOpen(false);
          setDirty(false);
          void navigate({ to: "/scenario-templates" });
        }}
      />
      <ConfirmDialog
        open={deleteOpen}
        title={`删除 ${key || "此模板"}？`}
        desc="删除后列表和发起页不再出现这份模板。已经按它规划过的需求仍保留当时的标识，不会被改写。"
        confirmText="确认删除"
        destructive
        isLoading={deleteMutation.isPending}
        onOpenChange={setDeleteOpen}
        handleConfirm={() => {
          setFormError(null);
          deleteMutation.mutate();
        }}
      />
    </>
  );
}

function CopyChip({
  pressed,
  onClick,
  children,
}: {
  pressed: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      role="radio"
      aria-checked={pressed}
      className={cn(
        "h-8 rounded-full border px-3 text-[12px] font-semibold transition-colors",
        pressed
          ? "border-brand bg-brand-soft text-brand-deep"
          : "border-line-strong bg-card text-ink-2 hover:border-brand hover:text-ink",
      )}
      onClick={onClick}
    >
      {children}
    </button>
  );
}

function roleTitle(roleKey: string, vocabulary: RoleVocabularyEntry[]) {
  if (!roleKey) return "未分派";
  const hit = vocabulary.find((entry) => entry.role_key === roleKey);
  const title = hit?.title || roleKey;
  return hit?.status === "disabled" ? `${title}（已停用）` : title;
}

function ChainOverview({
  draft,
  vocabulary,
  selectedId,
  onSelect,
}: {
  draft: ComposerDraft;
  vocabulary: RoleVocabularyEntry[];
  selectedId?: string;
  onSelect: (id: string) => void;
}) {
  return (
    <div className="flex items-center gap-0 overflow-x-auto pb-0.5">
      {draft.nodes.map((node, index) => (
        <div key={node.id} className="flex items-center">
          {index > 0 ? (
            <span
              aria-hidden
              className="mx-0.5 h-[2px] w-5 shrink-0 rounded-full bg-line-strong"
            />
          ) : null}
          <button
            type="button"
            className={cn(
              "flex shrink-0 items-center gap-1.5 rounded-full border px-2.5 py-1 text-left transition-colors",
              node.id === selectedId
                ? "border-brand bg-card shadow-sm"
                : "border-transparent bg-card/80 hover:border-line-strong",
            )}
            onClick={() => onSelect(node.id)}
          >
            <span
              className={cn(
                "size-1.5 shrink-0 rounded-full",
                node.exit ? "bg-brand" : "bg-line-strong",
              )}
            />
            <span className="whitespace-nowrap text-[12.5px] font-semibold text-ink">
              {nodeDisplayTitle(node, vocabulary)}
            </span>
          </button>
        </div>
      ))}
    </div>
  );
}

function ExitPreview({
  rows,
}: {
  rows: ReturnType<typeof exitPreviewRows>;
}) {
  return (
    <div className="border-t border-line px-5 py-4">
      <div className="flex items-baseline justify-between gap-3">
        <h3 className="text-[13px] font-extrabold text-ink">收口档位</h3>
        <p className="text-[11px] text-ink-3">发起只选模板；规划确认卡再选停在哪一档</p>
      </div>
      {rows.length === 0 ? (
        <p className="mt-2 text-[12.5px] text-ink-3">还没有站被标成出口。</p>
      ) : (
        <div className="mt-2.5 flex flex-col gap-1.5">
          {rows.map((row) => (
            <div
              key={`${row.index}-${row.label}`}
              className="flex items-center gap-3 rounded-[12px] border border-line bg-card-soft px-3 py-2"
            >
              <span className="w-5 shrink-0 font-mono text-[11px] font-extrabold tabular-nums text-ink-3">
                {row.index}
              </span>
              <div className="min-w-0 flex-1">
                <p className="truncate text-[13px] font-extrabold text-ink">{row.label}</p>
                <p className="truncate text-[11px] text-ink-3">{row.path}</p>
              </div>
              <span className="shrink-0 text-[11px] text-ink-3">
                {row.humanGates > 0 ? `${row.humanGates} 道人类门` : "无人类门"}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function Inspector({
  node,
  index,
  total,
  vocabulary,
  activeRoles,
  vocabularyLoading,
  vocabularyError,
  readOnly,
  onChange,
  onMove,
  onDelete,
}: {
  node: ComposerNode;
  index: number;
  total: number;
  vocabulary: RoleVocabularyEntry[];
  activeRoles: RoleVocabularyEntry[];
  vocabularyLoading: boolean;
  vocabularyError: boolean;
  readOnly: boolean;
  onChange: (patch: Partial<ComposerNode>) => void;
  onMove: (direction: -1 | 1) => void;
  onDelete: () => void;
}) {
  const selectedRole = vocabulary.find((entry) => entry.role_key === node.roleKey);
  const roleMissingActive = Boolean(node.roleKey) && selectedRole?.status !== "active";

  return (
    <div className="min-w-0 flex-1 px-5 py-4">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <p className="text-[11px] font-semibold tracking-[0.08em] text-ink-3">第 {index + 1} 站</p>
          <h2 className="mt-0.5 text-[15px] font-extrabold text-ink">
            {nodeDisplayTitle(node, vocabulary)}
          </h2>
        </div>
        <div className="flex flex-wrap gap-1.5">
          <Button type="button" size="sm" variant="outline" disabled={readOnly || index === 0} onClick={() => onMove(-1)}>
            ↑ 上移
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={readOnly || index === total - 1}
            onClick={() => onMove(1)}
          >
            ↓ 下移
          </Button>
          <Button type="button" size="sm" variant="outline" disabled={readOnly || total <= 1} onClick={onDelete}>
            删除
          </Button>
        </div>
      </div>
      {vocabularyLoading ? (
        <p className="mt-3 text-sm text-ink-2">加载角色词表…</p>
      ) : vocabularyError ? (
        <p className="mt-3 text-sm text-danger">角色词表加载失败，请稍后重试。</p>
      ) : (
        <>
          {activeRoles.length === 0 ? (
            <p className="mt-3 text-sm text-ink-2">
              还没有启用中的剧本角色。请先到{" "}
              <Link className="underline" to="/role-vocabulary">
                角色词表
              </Link>{" "}
              登记席位，再回来编排节点。
            </p>
          ) : null}
          <div className="mt-4 grid gap-3 @xl/composer:grid-cols-2">
            <label className="flex flex-col gap-1">
              <span className="text-[11px] font-semibold text-ink-3">这一站叫什么</span>
              <input
                aria-label="节点名称"
                className={inputClass}
                disabled={readOnly}
                placeholder="如 定界、采集、审查"
                value={node.title}
                onChange={(event) => onChange({ title: event.target.value })}
              />
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-[11px] font-semibold text-ink-3">谁来接收</span>
              <select
                aria-label={`节点 ${index + 1} 接收角色`}
                className={selectClass}
                disabled={readOnly}
                value={node.roleKey}
                onChange={(event) => onChange({ roleKey: event.target.value })}
              >
                <option value="">选择角色…</option>
                {roleMissingActive && selectedRole ? (
                  <option value={selectedRole.role_key}>
                    {selectedRole.title}（已停用）
                  </option>
                ) : null}
                {activeRoles.map((entry) => (
                  <option key={entry.role_key} value={entry.role_key}>
                    {entry.title}
                  </option>
                ))}
              </select>
            </label>
            {roleMissingActive ? (
              <p className="text-xs text-warn-text @xl/composer:col-span-2">
                当前席位已停用，保存前请换成启用中的角色。
              </p>
            ) : null}
            <label className="flex flex-col gap-1">
              <span className="text-[11px] font-semibold text-ink-3">怎么验证</span>
              <select
                aria-label={`节点 ${index + 1} 验证方式`}
                className={selectClass}
                disabled={readOnly}
                value={node.verify}
                onChange={(event) => onChange({ verify: event.target.value as NodeVerify })}
              >
                <option value="none">本角色交出即可</option>
                <option value="independent">另一角色独立验证</option>
                <option value="human">必须人类确认</option>
              </select>
            </label>
            {node.verify === "independent" ? (
              <label className="flex flex-col gap-1">
                <span className="text-[11px] font-semibold text-ink-3">独立验证角色</span>
                <select
                  aria-label={`节点 ${index + 1} 独立验证角色`}
                  className={selectClass}
                  disabled={readOnly}
                  value={node.independentRoleKey}
                  onChange={(event) => onChange({ independentRoleKey: event.target.value })}
                >
                  <option value="">选择验证角色</option>
                  {activeRoles
                    .filter((entry) => entry.role_key !== node.roleKey)
                    .map((entry) => (
                      <option key={entry.role_key} value={entry.role_key}>
                        {entry.title}
                      </option>
                    ))}
                </select>
              </label>
            ) : null}
            <label className="flex items-center gap-2 text-[13px] text-ink @xl/composer:col-span-2">
              <input
                type="checkbox"
                className="size-3.5 accent-brand"
                disabled={readOnly}
                checked={node.required}
                onChange={(event) => onChange({ required: event.target.checked })}
              />
              必须经过（规划不能漏掉）
            </label>
            <label className="flex items-center gap-2 text-[13px] text-ink @xl/composer:col-span-2">
              <input
                type="checkbox"
                className="size-3.5 accent-brand"
                disabled={readOnly}
                checked={node.exit}
                onChange={(event) => onChange({ exit: event.target.checked })}
              />
              可作出口（规划确认卡可选停在这）
            </label>
            {node.exit ? (
              <label className="flex flex-col gap-1 @xl/composer:col-span-2">
                <span className="text-[11px] font-semibold text-ink-3">收口怎么叫（可选）</span>
                <input
                  aria-label="收口怎么叫"
                  className={inputClass}
                  disabled={readOnly}
                  placeholder={node.title.trim() || "默认用站名"}
                  value={node.exitLabel}
                  onChange={(event) => onChange({ exitLabel: event.target.value })}
                />
              </label>
            ) : null}
          </div>
        </>
      )}
    </div>
  );
}

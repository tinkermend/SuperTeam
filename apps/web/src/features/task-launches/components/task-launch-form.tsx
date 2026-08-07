import {
  type KeyboardEvent,
  type ReactNode,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { keepPreviousData, useMutation, useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  ChevronsUpDown,
  FolderOpen,
  ListChecks,
  MessagesSquare,
  RefreshCw,
  SendHorizontal,
  Sparkles,
} from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Button, GlassCard } from "@/components/superteam";
import {
  launchModeLabel,
  missingObjectLabel,
  projectStatusLabel,
  shortObjectId,
} from "@/lib/status-labels";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  getProject,
  getProjectBudgetSummary,
  listProjects,
  type Project,
  type ProjectDemandSourceType,
  type SubmitProjectDemandInput,
} from "@/lib/api/projects";
import { PromptTemplateDialog } from "./prompt-template-dialog";
import { applyPromptTemplate } from "@/lib/api/prompt-templates";

export type LaunchMode = "plan" | "loop" | "chat";

/** 项目选择回调：始终带上 Project 实体，便于父层缓存搜索选出的「首页 50 条之外」项。 */
export type ProjectChangeHandler = (project: Project) => void;

const MODE_CARDS: Array<{
  badge?: string;
  desc: string;
  icon: ReactNode;
  tone: "brand" | "info" | "warn";
  value: LaunchMode;
}> = [
  {
    badge: "默认",
    desc: "遇上游阻塞时暂停，提案报你决策后再补做",
    icon: <ListChecks aria-hidden />,
    tone: "brand",
    value: "plan",
  },
  {
    desc: "遇上游阻塞时自动补做上游任务并重跑下游",
    icon: <RefreshCw aria-hidden />,
    tone: "info",
    value: "loop",
  },
  {
    desc: "与指定数字员工单次对话，结果不进入项目流转",
    icon: <MessagesSquare aria-hidden />,
    tone: "warn",
    value: "chat",
  },
];

const MODE_ORDER: LaunchMode[] = MODE_CARDS.map((card) => card.value);

export type SubmitSuccessResult = {
  demandId: string;
  mode: LaunchMode;
  projectId: string;
  projectName: string;
  title: string;
};

type TaskLaunchFormProps = {
  apiOptions: ApiClientOptions;
  chatPanel?: ReactNode;
  content: string;
  isSubmitting?: boolean;
  mode: LaunchMode;
  onContentChange: (content: string) => void;
  onModeChange: (mode: LaunchMode) => void;
  onProjectChange: ProjectChangeHandler;
  onSubmit: (projectId: string, input: SubmitProjectDemandInput) => void;
  onSuccessDismiss?: () => void;
  projects: Project[];
  projectsLoading?: boolean;
  /** 父层缓存的已选项目（含搜索选出、不在 browse 首页的项），供触发器显示名称。 */
  resolvedProject?: Project | null;
  selectedProjectId?: string;
  submitError?: string;
  successResult?: SubmitSuccessResult | null;
};

function deriveTitle(content: string): string {
  return content.trim().split(/\n+/)[0]?.slice(0, 80) ?? "";
}

export function TaskLaunchForm({
  apiOptions,
  chatPanel,
  content,
  isSubmitting = false,
  mode,
  onContentChange,
  onModeChange,
  onProjectChange,
  onSubmit,
  onSuccessDismiss,
  projects,
  projectsLoading = false,
  resolvedProject = null,
  selectedProjectId,
  submitError,
  successResult = null,
}: TaskLaunchFormProps) {
  const activeProjects = useMemo(
    () => projects.filter((project) => project.status !== "archived"),
    [projects],
  );
  const [error, setError] = useState("");
  const [templateDialogOpen, setTemplateDialogOpen] = useState(false);
  const [pendingTemplate, setPendingTemplate] = useState<{
    text: string;
    templateId: string;
  } | null>(null);
  const modeGroupRef = useRef<HTMLDivElement>(null);
  const pendingModeFocus = useRef<LaunchMode | null>(null);

  useEffect(() => {
    if (pendingModeFocus.current !== mode) {
      return;
    }
    const next = modeGroupRef.current?.querySelector<HTMLElement>(
      `[role="radio"][data-mode="${mode}"]`,
    );
    next?.focus();
    pendingModeFocus.current = null;
  }, [mode]);

  const { mutate: applyTemplate } = useMutation({
    mutationFn: (id: string) => applyPromptTemplate(apiOptions, id),
  });
  const projectId = selectedProjectId || activeProjects[0]?.id || "";
  const hasNoProjects = !projectsLoading && activeProjects.length === 0;

  // Token 预算熔断(P1-A):选中项目预算耗尽时禁止发起新任务。前端禁用是 UX,真正的
  // 强制在后端派发前闸;两者一致(自动化不走前端,只受后端闸约束)。
  const { data: budget } = useQuery({
    enabled: Boolean(projectId),
    queryKey: ["project-budget-summary", projectId],
    queryFn: () => getProjectBudgetSummary(apiOptions, projectId),
  });
  const budgetExhausted = budget?.exhausted ?? false;

  function handleProjectChange(project: Project) {
    setError("");
    onProjectChange(project);
  }

  function handleSubmit() {
    const trimmedContent = content.trim();
    const resolvedTitle = deriveTitle(trimmedContent);

    if (!trimmedContent) {
      setError("需求描述不能为空");
      return;
    }
    if (!projectId) {
      setError(hasNoProjects ? "请先新建项目后再提交" : "请选择项目");
      return;
    }
    if (budgetExhausted) {
      setError("该项目 token 预算已耗尽，提高额度后才能发起新任务");
      return;
    }

    setError("");
    onSubmit(projectId, {
      attachments: [],
      content: trimmedContent,
      source_refs: {},
      source_type: "manual" as ProjectDemandSourceType,
      title: resolvedTitle,
    });
  }

  function handleInsertTemplate(text: string, templateId: string) {
    if (content.trim()) {
      // 先关模板库，避免与覆盖/追加/取消 Dialog 叠两层
      setTemplateDialogOpen(false);
      setPendingTemplate({ text, templateId });
      return;
    }
    setTemplateDialogOpen(false);
    onContentChange(text);
    applyTemplate(templateId);
  }

  function resolveTemplateInsert(action: "overwrite" | "append" | "cancel") {
    if (!pendingTemplate) {
      return;
    }
    const { text, templateId } = pendingTemplate;
    setPendingTemplate(null);
    if (action === "cancel") {
      return;
    }
    if (action === "overwrite") {
      onContentChange(text);
    } else {
      onContentChange(content + "\n\n" + text);
    }
    applyTemplate(templateId);
  }

  function handleModeKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const key = event.key;
    if (
      key !== "ArrowLeft" &&
      key !== "ArrowRight" &&
      key !== "ArrowUp" &&
      key !== "ArrowDown"
    ) {
      return;
    }
    event.preventDefault();
    const currentIndex = MODE_ORDER.indexOf(mode);
    const delta = key === "ArrowLeft" || key === "ArrowUp" ? -1 : 1;
    const nextIndex = (currentIndex + delta + MODE_ORDER.length) % MODE_ORDER.length;
    const nextMode = MODE_ORDER[nextIndex];
    pendingModeFocus.current = nextMode;
    onModeChange(nextMode);
  }

  if (successResult) {
    return (
      <SubmitSuccessPanel result={successResult} onAgain={onSuccessDismiss} />
    );
  }

  const displayError = error || submitError;

  return (
    <>
      <div className="tl-hero">
        <h2 className="tl-title">提出任务</h2>
        <p className="tl-sub">
          先把目标说清楚，编排、上下文切片和执行分派会在提交后由系统完成。
        </p>
      </div>

      <div
        ref={modeGroupRef}
        aria-label="任务模式"
        className="tl-modes"
        onKeyDown={handleModeKeyDown}
        role="radiogroup"
      >
        {MODE_CARDS.map((card) => {
          const label = launchModeLabel(card.value);
          const selected = mode === card.value;
          return (
            <button
              aria-checked={selected}
              aria-label={label}
              className="tl-mode-card"
              data-active={selected || undefined}
              data-mode={card.value}
              data-tone={card.tone}
              key={card.value}
              onClick={() => onModeChange(card.value)}
              role="radio"
              tabIndex={selected ? 0 : -1}
              type="button"
            >
              <span aria-hidden className="tl-mode-check" />
              <span className="tl-mode-head">
                <span className="tl-mode-icon">{card.icon}</span>
                <span className="tl-mode-name">{label}</span>
                {card.badge ? <span className="tl-mode-def">{card.badge}</span> : null}
              </span>
              <span className="tl-mode-desc">{card.desc}</span>
            </button>
          );
        })}
      </div>

      <GlassCard>
        {mode === "chat" ? (
          <div data-testid="chat-panel-slot">{chatPanel}</div>
        ) : (
          <>
            <div className="tl-cmd">
              <div className="tl-cmd-top">
                <div className="tl-cmd-t">
                  <span>需求描述</span>
                  <span className="tl-req">*</span>
                </div>
              </div>
              <textarea
                aria-label="需求描述"
                className="tl-textarea"
                onChange={(event) => onContentChange(event.target.value)}
                placeholder="描述你希望项目协调线程处理的目标或问题场景"
                value={content}
              />
              <div className="tl-cmd-foot">
                <button
                  className="tl-ghost"
                  onClick={() => setTemplateDialogOpen(true)}
                  type="button"
                >
                  <Sparkles className="size-3.5" aria-hidden />
                  浏览模板库
                </button>
                <span className="tl-counter">{content.length} 字</span>
              </div>
            </div>

            <div className="tl-params" data-testid="task-launch-parameters">
              <LaunchChip icon={<FolderOpen aria-hidden />} label="项目" required>
                {hasNoProjects ? (
                  <NoProjectsEmptyState />
                ) : (
                  <ProjectPicker
                    apiOptions={apiOptions}
                    onChange={handleProjectChange}
                    projects={activeProjects}
                    resolvedProject={resolvedProject}
                    value={projectId}
                  />
                )}
              </LaunchChip>
            </div>

            {budgetExhausted ? (
              <div className="tl-err" data-testid="budget-exhausted-notice">
                ⚠ 该项目 token 预算已耗尽（已用 {budget?.consumed_tokens ?? 0}
                {budget?.token_limit != null ? ` / 上限 ${budget.token_limit}` : ""}
                ），提高额度后才能发起新任务。
              </div>
            ) : null}
            {hasNoProjects ? (
              <div className="tl-err" data-testid="no-projects-notice">
                ⚠ 当前没有可用项目，请先新建项目后再提交任务。
              </div>
            ) : null}
            {displayError ? <div className="tl-err">⚠ {displayError}</div> : null}

            <div className="tl-actions">
              <button
                className="tl-btn-send"
                disabled={isSubmitting || budgetExhausted || hasNoProjects}
                onClick={handleSubmit}
                type="button"
              >
                提交任务
                <SendHorizontal className="size-4" aria-hidden />
              </button>
            </div>
          </>
        )}
      </GlassCard>

      <PromptTemplateDialog
        open={templateDialogOpen}
        onOpenChange={setTemplateDialogOpen}
        onInsert={handleInsertTemplate}
      />

      <Dialog
        open={pendingTemplate !== null}
        onOpenChange={(open) => {
          if (!open) {
            resolveTemplateInsert("cancel");
          }
        }}
      >
        <DialogContent className="sm:max-w-md" showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>插入模板</DialogTitle>
            <DialogDescription>
              当前已有内容。请选择如何处理模板文本。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="flex-col gap-2 sm:flex-col sm:space-x-0">
            <Button
              type="button"
              onClick={() => resolveTemplateInsert("overwrite")}
            >
              覆盖
            </Button>
            <Button
              type="button"
              variant="secondary"
              onClick={() => resolveTemplateInsert("append")}
            >
              追加到末尾
            </Button>
            <Button
              type="button"
              variant="ghost"
              onClick={() => resolveTemplateInsert("cancel")}
            >
              取消
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function SubmitSuccessPanel({
  result,
  onAgain,
}: {
  result: SubmitSuccessResult;
  onAgain?: () => void;
}) {
  return (
    <>
      <div className="tl-hero">
        <h2 className="tl-title">需求已提交</h2>
        <p className="tl-sub">任务已写入项目需求卷宗，可继续提交下一条，或前往卷宗查看详情。</p>
      </div>
      <GlassCard>
        <div className="tl-result" data-testid="submit-success-panel">
          <dl className="tl-result-meta">
            <div>
              <dt>需求标题</dt>
              <dd>{result.title}</dd>
            </div>
            <div>
              <dt>所属项目</dt>
              <dd>{result.projectName}</dd>
            </div>
            <div>
              <dt>模式</dt>
              <dd>{launchModeLabel(result.mode)}</dd>
            </div>
          </dl>
          <div className="tl-actions">
            <Link
              className="tl-btn-secondary"
              params={{ projectId: result.projectId }}
              search={{ demand: result.demandId, tab: "demands" }}
              to="/projects/$projectId"
            >
              查看需求卷宗
            </Link>
            <button className="tl-btn-send" onClick={onAgain} type="button">
              再提一个
            </button>
          </div>
        </div>
      </GlassCard>
    </>
  );
}

export function NoProjectsEmptyState() {
  return (
    <div className="tl-proj-none" data-testid="no-projects-empty">
      <p className="tl-proj-none-text">还没有可用项目，无法提交任务。</p>
      <Link className="tl-proj-none-link" to="/projects">
        新建项目
      </Link>
    </div>
  );
}

export function ProjectPicker({
  apiOptions,
  onChange,
  projects,
  resolvedProject = null,
  value,
}: {
  apiOptions?: ApiClientOptions;
  onChange: ProjectChangeHandler;
  projects: Project[];
  /** 父层缓存的已选项目（搜索越界）；关闭 popover 后 list 回到父列表仍能显示名称。 */
  resolvedProject?: Project | null;
  value: string;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  // 本地钉住最近点选，防父层 resolvedProject 尚未回填时的一帧空名。
  const [selectedCache, setSelectedCache] = useState<Project | undefined>();

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setDebouncedQuery(query.trim());
    }, 280);
    return () => window.clearTimeout(timer);
  }, [query]);

  useEffect(() => {
    if (!value) setSelectedCache(undefined);
  }, [value]);

  const baseUrl = apiOptions?.baseUrl ?? "";
  const searchEnabled = debouncedQuery.length > 0;
  const searchQuery = useQuery({
    enabled: open && searchEnabled && Boolean(baseUrl),
    placeholderData: keepPreviousData,
    queryFn: () =>
      listProjects(apiOptions, { limit: 50, offset: 0, q: debouncedQuery }),
    queryKey: [
      "task-launch-project-search",
      baseUrl,
      debouncedQuery,
    ],
  });

  const list = useMemo(() => {
    if (searchEnabled) {
      return (searchQuery.data ?? []).filter(
        (project) => project.status !== "archived",
      );
    }
    return projects;
  }, [projects, searchEnabled, searchQuery.data]);

  const selectedFromList =
    projects.find((project) => project.id === value) ??
    list.find((project) => project.id === value) ??
    (resolvedProject?.id === value ? resolvedProject : undefined) ??
    (selectedCache?.id === value ? selectedCache : undefined);

  // 深链/会话恢复等只有 id 时再按 id 补实体，避免触发器裸 UUID。
  const selectedByIdQuery = useQuery({
    enabled: Boolean(value) && !selectedFromList && Boolean(baseUrl),
    queryFn: () => getProject(apiOptions, value),
    queryKey: ["task-launch-project", baseUrl, value],
    retry: false,
    staleTime: 5 * 60 * 1000,
  });

  const selected = selectedFromList ?? selectedByIdQuery.data;
  const listFetching = searchEnabled && searchQuery.isFetching;

  function pickProject(project: Project) {
    setSelectedCache(project);
    onChange(project);
  }

  function handleListKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const key = event.key;
    if (key === "Escape") {
      event.preventDefault();
      setOpen(false);
      return;
    }
    if (list.length === 0) {
      return;
    }
    if (key === "ArrowDown" || key === "ArrowUp") {
      event.preventDefault();
      const currentIndex = Math.max(
        0,
        list.findIndex((project) => project.id === value),
      );
      const delta = key === "ArrowDown" ? 1 : -1;
      const nextIndex = (currentIndex + delta + list.length) % list.length;
      pickProject(list[nextIndex]);
      return;
    }
    if (key === "Enter") {
      event.preventDefault();
      if (value) {
        setOpen(false);
      }
    }
  }

  return (
    <Popover
      onOpenChange={(next) => {
        setOpen(next);
        if (next) {
          setQuery("");
          setDebouncedQuery("");
        }
      }}
      open={open}
    >
      <PopoverTrigger asChild>
        <button aria-label="项目" className="tl-proj-trigger" type="button">
          {selected ? (
            <>
              <span className="tl-proj-dot" data-status={selected.status} />
              <span className="tl-proj-trigger-name">{selected.name}</span>
              <span className="tl-proj-status">
                {projectStatusLabel(selected.status)}
              </span>
            </>
          ) : value ? (
            <span className="tl-proj-trigger-name">
              {selectedByIdQuery.isLoading
                ? `项目 (${shortObjectId(value)})`
                : missingObjectLabel("project", value)}
            </span>
          ) : (
            <span className="tl-proj-trigger-placeholder">选择项目</span>
          )}
          <ChevronsUpDown aria-hidden className="tl-proj-trigger-caret" />
        </button>
      </PopoverTrigger>
      <PopoverContent align="start" className="tl-proj-pop">
        <input
          aria-label="搜索项目"
          className="tl-proj-search"
          onChange={(event) => setQuery(event.target.value)}
          placeholder="搜索项目名称或目标…"
          value={query}
        />
        <div
          aria-busy={listFetching || undefined}
          aria-label="项目列表"
          className="tl-proj-list"
          data-fetching={listFetching || undefined}
          onKeyDown={handleListKeyDown}
          role="radiogroup"
        >
          {listFetching && list.length === 0 ? (
            <p className="tl-proj-empty">搜索中…</p>
          ) : null}
          {list.map((project) => {
            const selectedItem = value === project.id;
            return (
              <button
                aria-checked={selectedItem}
                aria-label={project.name}
                className="tl-proj"
                data-active={selectedItem || undefined}
                key={project.id}
                onClick={() => {
                  pickProject(project);
                  setOpen(false);
                }}
                role="radio"
                tabIndex={selectedItem ? 0 : -1}
                type="button"
              >
                <span className="tl-proj-dot" data-status={project.status} />
                <span className="tl-proj-main">
                  <span className="tl-proj-name">{project.name}</span>
                  {project.goal ? (
                    <span className="tl-proj-goal">{project.goal}</span>
                  ) : null}
                </span>
                <span className="tl-proj-status">
                  {projectStatusLabel(project.status)}
                </span>
                <span aria-hidden className="tl-proj-check" />
              </button>
            );
          })}
          {list.length === 0 && searchEnabled && !listFetching ? (
            <div className="tl-proj-empty">无匹配项目</div>
          ) : null}
          {list.length === 0 && !searchEnabled && projects.length === 0 ? (
            <div className="tl-proj-empty">无可用项目</div>
          ) : null}
        </div>
      </PopoverContent>
    </Popover>
  );
}

export function LaunchChip({
  children,
  icon,
  label,
  required,
}: {
  children: ReactNode;
  icon: ReactNode;
  label: string;
  required?: boolean;
}) {
  return (
    <div className="tl-chip">
      <div className="tl-chip-label">
        {icon}
        {label}
        {required ? <span className="tl-req">*</span> : null}
      </div>
      {children}
    </div>
  );
}

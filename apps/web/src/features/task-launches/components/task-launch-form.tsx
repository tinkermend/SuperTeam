import { type ReactNode, useMemo, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import {
  ChevronsUpDown,
  FolderOpen,
  ListChecks,
  MessagesSquare,
  PencilLine,
  RefreshCw,
  SendHorizontal,
  Sparkles
} from "lucide-react";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { GlassCard } from "@/components/superteam";
import { projectStatusLabel } from "@/lib/status-labels";
import {
  getProjectBudgetSummary,
  type Project,
  type ProjectDemandSourceType,
  type SubmitProjectDemandInput
} from "@/lib/api/projects";
import { PromptTemplateDialog } from "./prompt-template-dialog";
import { applyPromptTemplate } from "@/lib/api/prompt-templates";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";

export type LaunchMode = "plan" | "loop" | "chat";

const MODE_CARDS: Array<{
  badge?: string;
  desc: string;
  icon: ReactNode;
  label: string;
  tone: "brand" | "info" | "warn";
  value: LaunchMode;
}> = [
  {
    badge: "默认",
    desc: "遇上游阻塞时暂停，提案报你决策后再补做",
    icon: <ListChecks aria-hidden />,
    label: "Plan 任务",
    tone: "brand",
    value: "plan"
},
  {
    desc: "遇上游阻塞时自动补做上游任务并重跑下游",
    icon: <RefreshCw aria-hidden />,
    label: "Loop 任务",
    tone: "info",
    value: "loop"
},
  {
    desc: "与指定数字员工单次对话，结果不进入项目流转",
    icon: <MessagesSquare aria-hidden />,
    label: "对话",
    tone: "warn",
    value: "chat"
},
];

type TaskLaunchFormProps = {
  chatPanel?: ReactNode;
  content: string;
  isSubmitting?: boolean;
  mode: LaunchMode;
  onContentChange: (content: string) => void;
  onModeChange: (mode: LaunchMode) => void;
  onProjectChange: (projectId: string) => void;
  onSubmit: (projectId: string, input: SubmitProjectDemandInput) => void;
  projects: Project[];
  selectedProjectId?: string;
};

function deriveTitle(content: string): string {
  return content.trim().split(/\n+/)[0]?.slice(0, 80) ?? "";
}

export function TaskLaunchForm({
  chatPanel,
  content,
  isSubmitting = false,
  mode,
  onContentChange,
  onModeChange,
  onProjectChange,
  onSubmit,
  projects,
  selectedProjectId
}: TaskLaunchFormProps) {
  const activeProjects = useMemo(
    () => projects.filter((project) => project.status !== "archived"),
    [projects],
  );
  const [error, setError] = useState("");
  const [templateDialogOpen, setTemplateDialogOpen] = useState(false);

  const apiOptions = useMemo(() => ({ baseUrl: resolveControlPlaneUrl() }), []);
  const { mutate: applyTemplate } = useMutation({
    mutationFn: (id: string) => applyPromptTemplate(apiOptions, id)
});
  const projectId = selectedProjectId || activeProjects[0]?.id || "";

  // Token 预算熔断(P1-A):选中项目预算耗尽时禁止发起新任务。前端禁用是 UX,真正的
  // 强制在后端派发前闸;两者一致(自动化不走前端,只受后端闸约束)。
  const { data: budget } = useQuery({
    enabled: Boolean(projectId),
    queryKey: ["project-budget-summary", projectId],
    queryFn: () => getProjectBudgetSummary(apiOptions, projectId),
  });
  const budgetExhausted = budget?.exhausted ?? false;

  function handleProjectChange(nextProjectId: string) {
    setError("");
    onProjectChange(nextProjectId);
  }

  function handleSubmit() {
    const trimmedContent = content.trim();
    const resolvedTitle = deriveTitle(trimmedContent);

    if (!trimmedContent) {
      setError("需求描述不能为空");
      return;
    }
    if (!projectId) {
      setError("请选择项目");
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
      title: resolvedTitle
});
  }

  function handleInsertTemplate(text: string, templateId: string) {
    if (content.trim()) {
      if (window.confirm("当前内容已存在。\n点击「确定」将覆盖当前内容？\n点击「取消」将继续。")) {
        onContentChange(text);
        applyTemplate(templateId);
      } else if (window.confirm("是否追加到末尾？\n点击「取消」放弃插入模板。")) {
        onContentChange(content + "\n\n" + text);
        applyTemplate(templateId);
      }
    } else {
      onContentChange(text);
      applyTemplate(templateId);
    }
  }

  return (
    <>
      <div className="tl-hero">
        <h1 className="tl-title">提出任务</h1>
        <p className="tl-sub">
          先把目标说清楚，编排、上下文切片和执行分派会在提交后由系统完成。
        </p>
      </div>

      <div aria-label="任务模式" className="tl-modes" role="radiogroup">
        {MODE_CARDS.map((card) => (
          <button
            aria-checked={mode === card.value}
            aria-label={card.label}
            className="tl-mode-card"
            data-active={mode === card.value || undefined}
            data-tone={card.tone}
            key={card.value}
            onClick={() => onModeChange(card.value)}
            role="radio"
            type="button"
          >
            <span aria-hidden className="tl-mode-check" />
            <span className="tl-mode-head">
              <span className="tl-mode-icon">{card.icon}</span>
              <span className="tl-mode-name">{card.label}</span>
              {card.badge ? <span className="tl-mode-def">{card.badge}</span> : null}
            </span>
            <span className="tl-mode-desc">{card.desc}</span>
          </button>
        ))}
      </div>

      <GlassCard>
        {mode === "chat" ? (
          <div data-testid="chat-panel-slot">{chatPanel}</div>
        ) : (
          <>
            <div className="tl-cmd">
              <div className="tl-cmd-top">
                <div className="tl-cmd-t">
                  <span>中枢指令区</span>
                  <span className="tl-req">*</span>
                </div>
                <span className="tl-pill">命令中心</span>
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
                <span className="tl-counter">{content.length} / 5000</span>
              </div>
            </div>

            <div className="tl-params" data-testid="task-launch-parameters">
              <LaunchChip icon={<FolderOpen aria-hidden />} label="项目" required>
                <ProjectPicker
                  onChange={handleProjectChange}
                  projects={activeProjects}
                  value={projectId}
                />
              </LaunchChip>
            </div>

            {budgetExhausted ? (
              <div className="tl-err" data-testid="budget-exhausted-notice">
                ⚠ 该项目 token 预算已耗尽（已用 {budget?.consumed_tokens ?? 0}
                {budget?.token_limit != null ? ` / 上限 ${budget.token_limit}` : ""}
                ），提高额度后才能发起新任务。
              </div>
            ) : null}
            {error ? <div className="tl-err">⚠ {error}</div> : null}

            <div className="tl-actions">
              <button className="tl-btn-draft" type="button">
                <PencilLine className="size-[15px]" aria-hidden />
                保存草稿
              </button>
              <button
                className="tl-btn-send"
                disabled={isSubmitting || budgetExhausted}
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
    </>
  );
}

export function ProjectPicker({
  onChange,
  projects,
  value
}: {
  onChange: (projectId: string) => void;
  projects: Project[];
  value: string;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const selected = projects.find((project) => project.id === value);
  const filtered = useMemo(() => {
    const keyword = query.trim().toLowerCase();
    if (!keyword) return projects;
    return projects.filter(
      (project) =>
        project.name.toLowerCase().includes(keyword) ||
        (project.goal ?? "").toLowerCase().includes(keyword),
    );
  }, [projects, query]);

  return (
    <Popover
      onOpenChange={(next) => {
        setOpen(next);
        if (next) setQuery("");
      }}
      open={open}
    >
      <PopoverTrigger asChild>
        <button aria-label="项目" className="tl-proj-trigger" type="button">
          {selected ? (
            <>
              <span className="tl-proj-dot" data-status={selected.status} />
              <span className="tl-proj-trigger-name">{selected.name}</span>
              <span className="tl-proj-status">{projectStatusLabel(selected.status)}</span>
            </>
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
        <div aria-label="项目列表" className="tl-proj-list" role="radiogroup">
          {filtered.map((project) => (
            <button
              aria-checked={value === project.id}
              aria-label={project.name}
              className="tl-proj"
              data-active={value === project.id || undefined}
              key={project.id}
              onClick={() => {
                onChange(project.id);
                setOpen(false);
              }}
              role="radio"
              type="button"
            >
              <span className="tl-proj-dot" data-status={project.status} />
              <span className="tl-proj-main">
                <span className="tl-proj-name">{project.name}</span>
                {project.goal ? <span className="tl-proj-goal">{project.goal}</span> : null}
              </span>
              <span className="tl-proj-status">{projectStatusLabel(project.status)}</span>
              <span aria-hidden className="tl-proj-check" />
            </button>
          ))}
          {filtered.length === 0 ? <div className="tl-proj-empty">无匹配项目</div> : null}
        </div>
      </PopoverContent>
    </Popover>
  );
}

export function LaunchChip({
  children,
  icon,
  label,
  required
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

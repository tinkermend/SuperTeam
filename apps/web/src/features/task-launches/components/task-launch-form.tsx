import { type ReactNode, useMemo, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { FolderOpen, PencilLine, SendHorizontal, Sparkles } from "lucide-react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { GlassCard, V3Segmented } from "@/components/superteam";
import type {
  Project,
  ProjectDemandSourceType,
  SubmitProjectDemandInput,
} from "@/lib/api/projects";
import { PromptTemplateDialog } from "./prompt-template-dialog";
import { applyPromptTemplate } from "@/lib/api/prompt-templates";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";

export type LaunchMode = "plan" | "loop" | "chat";

const MODE_OPTIONS: Array<{ label: string; value: LaunchMode }> = [
  { label: "Plan 任务", value: "plan" },
  { label: "Loop 任务", value: "loop" },
  { label: "对话", value: "chat" },
];

const MODE_EXPLAINER: Record<LaunchMode, string> = {
  plan: "遇上游阻塞时暂停，提案报你决策后再补做",
  loop: "遇上游阻塞时自动补做上游任务并重跑下游",
  chat: "与指定数字员工单次对话，结果不进入项目流转",
};

type TaskLaunchFormProps = {
  chatPanel?: ReactNode;
  isSubmitting?: boolean;
  mode: LaunchMode;
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
  isSubmitting = false,
  mode,
  onModeChange,
  onProjectChange,
  onSubmit,
  projects,
  selectedProjectId,
}: TaskLaunchFormProps) {
  const activeProjects = useMemo(
    () => projects.filter((project) => project.status !== "archived"),
    [projects],
  );
  const [content, setContent] = useState("");
  const [error, setError] = useState("");
  const [templateDialogOpen, setTemplateDialogOpen] = useState(false);

  const apiOptions = useMemo(() => ({ baseUrl: resolveControlPlaneUrl() }), []);
  const { mutate: applyTemplate } = useMutation({
    mutationFn: (id: string) => applyPromptTemplate(apiOptions, id),
  });
  const projectId = selectedProjectId || activeProjects[0]?.id || "";

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
      if (window.confirm("当前内容已存在。\n点击「确定」将覆盖当前内容？\n点击「取消」将继续。")) {
        setContent(text);
        applyTemplate(templateId);
      } else if (window.confirm("是否追加到末尾？\n点击「取消」放弃插入模板。")) {
        setContent(content + "\n\n" + text);
        applyTemplate(templateId);
      }
    } else {
      setContent(text);
      applyTemplate(templateId);
    }
  }

  return (
    <>
      <div className="tl-hero">
        <span className="tl-eyebrow">
          <Sparkles className="size-3.5" aria-hidden />
          提交后由协调线程动态编排
        </span>
        <h1 className="tl-title">提出任务</h1>
        <p className="tl-sub">
          先把目标说清楚，编排、上下文切片和执行分派会在提交后由系统完成。
        </p>
      </div>

      <V3Segmented<LaunchMode> options={MODE_OPTIONS} value={mode} onChange={onModeChange} />
      <p className="tl-sub">{MODE_EXPLAINER[mode]}</p>

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
                onChange={(event) => setContent(event.target.value)}
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
                <Select value={projectId} onValueChange={handleProjectChange}>
                  <SelectTrigger aria-label="项目" className="tl-chip-select">
                    <SelectValue placeholder="选择项目" />
                  </SelectTrigger>
                  <SelectContent>
                    {activeProjects.map((item) => (
                      <SelectItem key={item.id} value={item.id}>
                        {item.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </LaunchChip>
            </div>

            {error ? <div className="tl-err">⚠ {error}</div> : null}

            <div className="tl-actions">
              <button className="tl-btn-draft" type="button">
                <PencilLine className="size-[15px]" aria-hidden />
                保存草稿
              </button>
              <button
                className="tl-btn-send"
                disabled={isSubmitting}
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

function LaunchChip({
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

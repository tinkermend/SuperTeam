import { type ReactNode, useMemo, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import {
  CircleAlert,
  FolderOpen,
  GitBranch,
  PencilLine,
  SendHorizontal,
  Sparkles,
} from "lucide-react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type {
  Project,
  ProjectDemandSourceType,
  SubmitProjectDemandInput,
} from "@/lib/api/projects";
import { PromptTemplateDialog } from "./prompt-template-dialog";
import { applyPromptTemplate } from "@/lib/api/prompt-templates";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";

type TaskLaunchFormProps = {
  isSubmitting?: boolean;
  onProjectChange: (projectId: string) => void;
  onSubmit: (projectId: string, input: SubmitProjectDemandInput) => void;
  projects: Project[];
  selectedProjectId?: string;
};

function deriveTitle(content: string): string {
  return content.trim().split(/\n+/)[0]?.slice(0, 80) ?? "";
}

export function TaskLaunchForm({
  isSubmitting = false,
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
  const [priority, setPriority] = useState("high");
  const [riskLevel, setRiskLevel] = useState("medium");
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

      <div className="tl-glass">
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

          <LaunchChip icon={<GitBranch aria-hidden />} label="优先级" required>
            <Select value={priority} onValueChange={setPriority}>
              <SelectTrigger aria-label="优先级" className="tl-chip-select">
                <SelectValue placeholder="选择优先级" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="high">高</SelectItem>
                <SelectItem value="medium">中</SelectItem>
                <SelectItem value="low">低</SelectItem>
              </SelectContent>
            </Select>
          </LaunchChip>

          <LaunchChip icon={<CircleAlert aria-hidden />} label="风险级别" required>
            <Select value={riskLevel} onValueChange={setRiskLevel}>
              <SelectTrigger aria-label="风险级别" className="tl-chip-select">
                <SelectValue placeholder="选择风险级别" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="medium">中风险</SelectItem>
                <SelectItem value="low">低风险</SelectItem>
                <SelectItem value="high">高风险</SelectItem>
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
      </div>

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

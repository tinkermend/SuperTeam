import { type ReactNode, useEffect, useMemo, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import {
  CircleAlert,
  FolderOpen,
  GitBranch,
  PencilLine,
  SendHorizontal,
  Sparkles,
  UserRoundCheck,
} from "lucide-react";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { SoftCard, V3Button } from "@/components/superteam";
import type {
  Project,
  ProjectDemandSourceType,
  ProjectMember,
  ReviewerSelectionReason,
  SubmitProjectDemandInput,
} from "@/lib/api/projects";
import { PromptTemplateDialog } from "./prompt-template-dialog";
import { applyPromptTemplate } from "@/lib/api/prompt-templates";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";

export type ReviewerDefaultResolution = {
  member?: ProjectMember;
  reason: ReviewerSelectionReason;
  requiresChoice: boolean;
};

type TaskLaunchFormProps = {
  isSubmitting?: boolean;
  isReviewerLoading?: boolean;
  members: ProjectMember[];
  onProjectChange: (projectId: string) => void;
  onSubmit: (projectId: string, input: SubmitProjectDemandInput) => void;
  projects: Project[];
  selectedProjectId?: string;
};

export function resolveDefaultReviewer(
  project: Project | undefined,
  members: ProjectMember[],
): ReviewerDefaultResolution | undefined {
  if (!project) {
    return undefined;
  }

  const activeHumanMembers = members.filter(
    (member) => member.principal_type === "human_user" && member.status === "active",
  );
  const reviewers = activeHumanMembers.filter(
    (member) => member.project_role === "reviewer",
  );

  if (reviewers.length === 1) {
    return {
      member: reviewers[0],
      reason: "project_reviewer_default",
      requiresChoice: false,
    };
  }

  if (reviewers.length > 1) {
    return {
      reason: "user_selected",
      requiresChoice: true,
    };
  }

  const owner = activeHumanMembers.find(
    (member) => member.principal_id === project.human_owner_user_id,
  );
  if (!owner) {
    return undefined;
  }

  return {
    member: owner,
    reason: "project_human_owner_fallback",
    requiresChoice: false,
  };
}

function deriveTitle(content: string): string {
  return content.trim().split(/\n+/)[0]?.slice(0, 80) ?? "";
}

function reviewerSortKey(member: ProjectMember): string {
  return member.display_name_snapshot || member.principal_id;
}

function reviewerLabel(member: ProjectMember): string {
  const identity =
    member.display_name_snapshot || `${member.principal_id.slice(0, 8)}...`;
  return `${identity} · ${member.project_role}`;
}

export function TaskLaunchForm({
  isSubmitting = false,
  isReviewerLoading = false,
  members,
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
  const [reviewerId, setReviewerId] = useState("");
  const [error, setError] = useState("");
  const [templateDialogOpen, setTemplateDialogOpen] = useState(false);

  const apiOptions = useMemo(() => ({ baseUrl: resolveControlPlaneUrl() }), []);
  const { mutate: applyTemplate } = useMutation({
    mutationFn: (id: string) => applyPromptTemplate(apiOptions, id),
  });
  const projectId = selectedProjectId || activeProjects[0]?.id || "";
  const project = activeProjects.find((item) => item.id === projectId);
  const currentProjectMembers = useMemo(
    () => members.filter((member) => member.project_id === projectId),
    [members, projectId],
  );
  const reviewerDefault = useMemo(
    () => resolveDefaultReviewer(project, currentProjectMembers),
    [project, currentProjectMembers],
  );
  const selectedReviewerId = reviewerId || reviewerDefault?.member?.principal_id || "";
  const selectedReason: ReviewerSelectionReason | undefined = reviewerId
    ? "user_selected"
    : reviewerDefault?.reason;
  const humanReviewers = useMemo(() => {
    const activeHumanMembers = currentProjectMembers.filter(
      (member) => member.principal_type === "human_user" && member.status === "active",
    );
    return [...activeHumanMembers].sort((left, right) => {
      if (left.project_role === "reviewer" && right.project_role !== "reviewer") {
        return -1;
      }
      if (left.project_role !== "reviewer" && right.project_role === "reviewer") {
        return 1;
      }
      return reviewerSortKey(left).localeCompare(reviewerSortKey(right));
    });
  }, [currentProjectMembers]);

  useEffect(() => {
    setReviewerId("");
  }, [selectedProjectId]);

  function handleProjectChange(nextProjectId: string) {
    setReviewerId("");
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
    if (!selectedReviewerId || !selectedReason) {
      setError("请选择审核人");
      return;
    }

    setError("");
    onSubmit(projectId, {
      attachments: [],
      content: trimmedContent,
      reviewer_selection_reason: selectedReason,
      reviewer_user_id: selectedReviewerId,
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
    <SoftCard className="mx-auto w-full max-w-[1120px] overflow-hidden p-0">
      <div className="grid gap-6 p-4 sm:p-6 xl:p-7">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm font-bold text-v3-brand-deep">
            <Sparkles className="size-4" />
            提交后由协调线程动态编排
          </div>
          <h2 className="mt-5 text-2xl font-extrabold tracking-normal text-v3-ink">
            提出任务
          </h2>
          <p className="mt-2 max-w-3xl text-sm leading-6 text-v3-ink-2">
            先把目标说清楚，编排、上下文切片和执行分派会在提交后由系统完成。
          </p>
        </div>

        <Label className="grid gap-3">
          <div className="flex flex-wrap items-end justify-between gap-3">
            <div className="min-w-0">
              <span className="block text-base font-extrabold tracking-normal text-v3-ink">
                中枢指令区
              </span>
              <span className="mt-1 block text-sm text-v3-ink-2">
                描述你的需求 <span className="text-destructive">*</span>
              </span>
            </div>
            <span className="rounded-lg bg-v3-brand-soft px-2.5 py-1 text-xs font-bold text-v3-brand-deep">
              命令中心
            </span>
          </div>
          <div className="rounded-v3-inner border border-v3-line-strong bg-v3-card p-4 transition-colors focus-within:border-v3-ink-3">
            <textarea
              aria-label="需求描述"
              className="block min-h-[clamp(10rem,27dvh,16rem)] w-full resize-y bg-transparent px-1 py-1 text-base leading-8 text-v3-ink outline-none placeholder:text-v3-ink-3"
              onChange={(event) => setContent(event.target.value)}
              placeholder="描述你希望项目协调线程处理的目标或问题场景"
              value={content}
            />
            <div className="mt-3 flex items-center justify-between border-t border-v3-line pt-3">
              <V3Button
                variant="ghost"
                size="sm"
                className="h-8 gap-1 text-xs text-v3-brand hover:text-v3-brand-deep"
                onClick={() => setTemplateDialogOpen(true)}
                type="button"
              >
                <Sparkles className="size-3.5" />
                浏览模板库
              </V3Button>
              <span className="text-xs tabular-nums text-v3-ink-3">
                {content.length} / 5000
              </span>
            </div>
          </div>
        </Label>

        <section className="grid gap-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="min-w-0">
              <h3 className="text-sm font-extrabold tracking-normal text-v3-ink">编排参数</h3>
              <p className="mt-1 text-xs leading-5 text-v3-ink-2">
                先固定项目、审核与风险边界，再交给协调线程拆解。
              </p>
            </div>
            <span className="rounded-lg border border-v3-line-strong bg-v3-card px-2.5 py-1 text-xs font-bold text-v3-ink-2">
              项目路由
            </span>
          </div>
          <div
            className="grid gap-3 md:grid-cols-2 xl:grid-cols-[minmax(15rem,1.1fr)_minmax(14rem,1fr)_minmax(9rem,0.55fr)_minmax(10rem,0.65fr)] xl:items-end"
            data-testid="task-launch-parameters"
          >
            <LaunchSelect
              icon={<FolderOpen className="size-4" />}
              label="项目"
              required
            >
              <Select value={projectId} onValueChange={handleProjectChange}>
                <SelectTrigger aria-label="项目" className="h-11 w-full min-w-0 rounded-xl border-v3-line-strong bg-v3-card px-3 text-base font-semibold text-v3-ink shadow-none">
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
            </LaunchSelect>

            <LaunchSelect
              icon={<UserRoundCheck className="size-4" />}
              label="审核人"
              required
            >
              <Select value={selectedReviewerId} onValueChange={setReviewerId}>
                <SelectTrigger aria-label="审核人" className="h-11 w-full min-w-0 rounded-xl border-v3-line-strong bg-v3-card px-3 text-base font-semibold text-v3-ink shadow-none">
                  <SelectValue placeholder="选择审核人" />
                </SelectTrigger>
                <SelectContent>
                  {humanReviewers.map((member) => (
                    <SelectItem key={member.id} value={member.principal_id}>
                      {reviewerLabel(member)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </LaunchSelect>

            <LaunchSelect
              icon={<GitBranch className="size-4" />}
              label="优先级"
              required
            >
              <Select value={priority} onValueChange={setPriority}>
                <SelectTrigger aria-label="优先级" className="h-11 w-full min-w-0 rounded-xl border-v3-line-strong bg-v3-card px-3 text-base font-semibold text-v3-ink shadow-none">
                  <SelectValue placeholder="选择优先级" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="high">高</SelectItem>
                  <SelectItem value="medium">中</SelectItem>
                  <SelectItem value="low">低</SelectItem>
                </SelectContent>
              </Select>
            </LaunchSelect>

            <LaunchSelect
              icon={<CircleAlert className="size-4" />}
              label="风险级别"
              required
            >
              <Select value={riskLevel} onValueChange={setRiskLevel}>
                <SelectTrigger aria-label="风险级别" className="h-11 w-full min-w-0 rounded-xl border-v3-line-strong bg-v3-card px-3 text-base font-semibold text-v3-ink shadow-none">
                  <SelectValue placeholder="选择风险级别" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="medium">中风险</SelectItem>
                  <SelectItem value="low">低风险</SelectItem>
                  <SelectItem value="high">高风险</SelectItem>
                </SelectContent>
              </Select>
            </LaunchSelect>
          </div>
        </section>

        {error ? <p className="text-sm text-destructive">{error}</p> : null}
      </div>
      <div className="flex flex-col-reverse gap-3 border-t border-v3-line bg-v3-card-soft px-4 py-4 sm:flex-row sm:items-center sm:justify-end sm:px-6 xl:px-7">
        <V3Button
          className="w-full sm:w-auto"
          type="button"
          variant="outline"
        >
          <PencilLine className="size-4" />
          保存草稿
        </V3Button>
        <V3Button
          className="w-full sm:w-auto"
          disabled={isSubmitting || isReviewerLoading}
          onClick={handleSubmit}
          type="button"
        >
          <SendHorizontal className="size-4" />
          提交任务
        </V3Button>
      </div>
      <PromptTemplateDialog
        open={templateDialogOpen}
        onOpenChange={setTemplateDialogOpen}
        onInsert={handleInsertTemplate}
      />
    </SoftCard>
  );
}
function LaunchSelect({
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
    <Label className="grid min-w-0 gap-2">
      <span className="flex min-w-0 items-center gap-2 text-sm font-semibold text-v3-ink">
        <span className="text-v3-brand">{icon}</span>
        {label}
        {required ? <span className="text-destructive">*</span> : null}
      </span>
      {children}
    </Label>
  );
}

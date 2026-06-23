import { type ReactNode, useEffect, useMemo, useState } from "react";
import {
  Bookmark,
  CircleAlert,
  DatabaseZap,
  FilePlus2,
  FolderOpen,
  GitBranch,
  Link2,
  Paperclip,
  PencilLine,
  Route,
  SendHorizontal,
  ShieldCheck,
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
import { Textarea } from "@/components/ui/textarea";
import { IconTile, SignatureCard, SoftCard, V3Button } from "@/components/superteam";
import type {
  Project,
  ProjectDemandSourceType,
  ProjectMember,
  ReviewerSelectionReason,
  SubmitProjectDemandInput,
} from "@/lib/api/projects";

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

  return (
    <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1fr)_390px]">
      <SoftCard className="p-4 sm:p-5 xl:p-6">
        <div className="grid gap-5">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div className="min-w-0">
              <div className="flex items-center gap-2 text-sm font-medium text-v3-brand-deep">
                <Sparkles className="size-4" />
                提交后由协调线程动态编排
              </div>
              <h2 className="mt-2 text-2xl font-extrabold tracking-normal text-v3-ink">提出需求</h2>
              <p className="mt-1 text-sm text-v3-ink-2">
                先把目标说清楚，编排、上下文切片和执行分派会在提交后由系统完成。
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <V3Button type="button" variant="outline">
                <Bookmark className="size-4" />
                使用模板
              </V3Button>
              <V3Button type="button" variant="outline">
                <PencilLine className="size-4" />
                保存草稿
              </V3Button>
              <V3Button
                disabled={isSubmitting || isReviewerLoading}
                onClick={handleSubmit}
                type="button"
              >
                <SendHorizontal className="size-4" />
                提交需求
              </V3Button>
            </div>
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
                Command Center
              </span>
            </div>
            <div className="rounded-v3-inner border border-v3-line-strong bg-v3-card p-3 shadow-[inset_3px_0_0_var(--v3-brand)] transition-shadow focus-within:border-v3-brand focus-within:ring-2 focus-within:ring-v3-brand/25">
              <Textarea
                aria-label="需求描述"
                className="min-h-[clamp(8rem,24dvh,14rem)] resize-y border-0 bg-transparent px-1 py-1 text-base leading-8 text-v3-ink shadow-none placeholder:text-v3-ink-3 focus-visible:ring-0"
                onChange={(event) => setContent(event.target.value)}
                placeholder="描述你希望项目协调线程处理的目标或问题场景"
                value={content}
              />
              <div className="mt-3 flex flex-wrap items-center justify-between gap-3 border-t border-v3-line pt-3">
                <div className="flex items-center gap-2">
                  <IconGhostButton label="添加附件">
                    <Paperclip className="size-4" />
                  </IconGhostButton>
                  <IconGhostButton label="关联链接">
                    <Link2 className="size-4" />
                  </IconGhostButton>
                  <IconGhostButton label="导入资料">
                    <FilePlus2 className="size-4" />
                  </IconGhostButton>
                </div>
                <span className="text-xs text-v3-ink-3">
                  {content.length} / 5000
                </span>
              </div>
            </div>
          </Label>

          <section className="grid gap-3 rounded-v3-inner border border-v3-line bg-v3-card-soft p-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="min-w-0">
                <h3 className="text-sm font-extrabold tracking-normal text-v3-ink">编排参数</h3>
                <p className="mt-0.5 text-xs text-v3-ink-2">
                  先固定项目、审核与风险边界，再交给协调线程拆解。
                </p>
              </div>
              <span className="rounded-lg border border-v3-line-strong bg-v3-card px-2.5 py-1 text-xs font-bold text-v3-ink-2">
                Project routing
              </span>
            </div>
            <div
              className="grid gap-3 md:grid-cols-[minmax(16rem,1.25fr)_minmax(13rem,1fr)_minmax(7rem,0.34fr)_minmax(9rem,0.42fr)] xl:items-end"
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

          <div className="grid gap-3">
            <div className="min-w-0">
              <h3 className="text-sm font-extrabold tracking-normal text-v3-ink">协同资料</h3>
              <p className="mt-0.5 text-xs text-v3-ink-2">
                把执行需要的附件、链接或知识库材料挂到同一条需求上。
              </p>
            </div>
            <div className="grid gap-3 md:grid-cols-3">
              <ReferenceButton
                description="上传文件或截图"
                icon={<Paperclip className="size-5" />}
                title="添加附件"
              />
              <ReferenceButton
                description="GitHub / 文档 / 任务等"
                icon={<Link2 className="size-5" />}
                title="关联链接"
              />
              <ReferenceButton
                description="从知识库导入"
                icon={<FilePlus2 className="size-5" />}
                title="导入资料"
              />
            </div>
          </div>

          {error ? <p className="text-sm text-destructive">{error}</p> : null}

        </div>
      </SoftCard>

      <LaunchGuidance />
    </div>
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

function IconGhostButton({
  children,
  label,
}: {
  children: ReactNode;
  label: string;
}) {
  return (
    <V3Button aria-label={label} size="icon" type="button" variant="outline">
      {children}
    </V3Button>
  );
}

function ReferenceButton({
  description,
  icon,
  title,
}: {
  description: string;
  icon: ReactNode;
  title: string;
}) {
  return (
    <button
      className="flex min-h-20 items-center gap-3 rounded-v3-inner border border-v3-line bg-v3-card p-3 text-left transition-colors hover:border-v3-brand/35 hover:bg-v3-brand-soft"
      type="button"
    >
      <span className="text-v3-brand">{icon}</span>
      <span className="min-w-0">
        <span className="block text-sm font-semibold text-v3-ink">{title}</span>
        <span className="block truncate text-xs text-v3-ink-2">{description}</span>
      </span>
    </button>
  );
}

function LaunchGuidance() {
  const steps = [
    {
      description: "系统记录目标、来源与审核人，作为后续协调事实入口。",
      icon: <DatabaseZap />,
      title: "写入项目需求",
      tone: "brand" as const,
    },
    {
      description: "协调线程读取项目规则、负责人和可调度成员。",
      icon: <Route />,
      title: "启动协调线程",
      tone: "info" as const,
    },
    {
      description: "根据目标拆分执行任务，并确定协作顺序。",
      icon: <GitBranch />,
      title: "生成编排决策",
      tone: "ok" as const,
    },
    {
      description: "编排完成后展示数字员工状态、事件和工件。",
      icon: <ShieldCheck />,
      title: "进入运行视图",
      tone: "artifact" as const,
    },
  ];

  return (
    <aside className="grid content-start gap-4">
      <SignatureCard className="p-6 xl:p-7">
        <div className="flex items-center gap-2 text-v3-brand-deep">
          <Sparkles className="size-4" />
          <h2 className="text-lg font-bold tracking-normal text-v3-ink">提交后会发生什么</h2>
        </div>
        <p className="mt-2 text-sm leading-6 text-v3-ink-2">
          一次需求提交，触发四步自动协调编排。
        </p>
        <ol className="mt-7 grid gap-6">
          {steps.map((step, index) => (
            <li className="grid grid-cols-[auto_1fr] gap-4" key={step.title}>
              <div className="flex flex-col items-center">
                <span className="grid size-11 place-items-center rounded-2xl bg-v3-brand-soft text-lg font-extrabold tabular-nums text-v3-brand-deep ring-1 ring-[color:var(--v3-signature-border)] [&_svg]:size-5">
                  {String(index + 1).padStart(2, "0")}
                </span>
                {index < steps.length - 1 ? (
                  <span aria-hidden className="mt-1 w-px flex-1 bg-v3-line-strong" />
                ) : null}
              </div>
              <div className="min-w-0 pb-1">
                <div className="flex items-center gap-2 text-v3-ink">
                  <span className="[&_svg]:size-4 text-v3-brand">{step.icon}</span>
                  <h3 className="text-[15px] font-bold tracking-normal">{step.title}</h3>
                </div>
                <p className="mt-1.5 text-[13px] leading-6 text-v3-ink-2">
                  {step.description}
                </p>
              </div>
            </li>
          ))}
        </ol>
      </SignatureCard>

      <SoftCard className="p-5">
        <div className="flex items-start gap-3">
          <IconTile className="mt-0.5" size="sm" tone="mute">
            <ShieldCheck />
          </IconTile>
          <div>
            <h2 className="text-base font-bold tracking-normal text-v3-ink">提交前确认</h2>
            <p className="mt-2 text-sm leading-6 text-v3-ink-2">
              请确认目标清楚、项目选择正确、审核人可以处理后续确认。权限校验、上下文切片和审计写入会在提交后由系统完成。
            </p>
          </div>
        </div>
      </SoftCard>
    </aside>
  );
}

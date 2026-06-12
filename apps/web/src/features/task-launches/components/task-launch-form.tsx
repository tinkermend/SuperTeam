import { useEffect, useMemo, useState } from "react";
import { ChevronDown, SendHorizontal } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { LiquidCard } from "@/components/superteam";
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

export function TaskLaunchForm({
  isSubmitting = false,
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
  const [title, setTitle] = useState("");
  const [reviewerId, setReviewerId] = useState("");
  const [error, setError] = useState("");
  const projectId = selectedProjectId || activeProjects[0]?.id || "";
  const project = activeProjects.find((item) => item.id === projectId);
  const reviewerDefault = useMemo(
    () => resolveDefaultReviewer(project, members),
    [project, members],
  );
  const selectedReviewerId = reviewerId || reviewerDefault?.member?.principal_id || "";
  const selectedReason: ReviewerSelectionReason | undefined = reviewerId
    ? "user_selected"
    : reviewerDefault?.reason;
  const humanReviewers = members.filter(
    (member) => member.principal_type === "human_user" && member.status === "active",
  );

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
    const resolvedTitle = title.trim() || deriveTitle(trimmedContent);

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
    <LiquidCard className="mx-auto w-full max-w-3xl rounded-xl p-5">
      <div className="grid gap-4">
        <Label className="grid gap-2">
          <span>需求描述</span>
          <Textarea
            aria-label="需求描述"
            className="min-h-36 resize-y"
            onChange={(event) => setContent(event.target.value)}
            placeholder="描述你希望项目协调线程处理的需求"
            value={content}
          />
        </Label>

        <Label className="grid gap-2">
          <span>标题</span>
          <Input
            aria-label="标题"
            onChange={(event) => setTitle(event.target.value)}
            placeholder={deriveTitle(content) || "自动使用需求描述首行"}
            value={title}
          />
        </Label>

        <Label className="grid gap-2">
          <span>项目</span>
          <Select value={projectId} onValueChange={handleProjectChange}>
            <SelectTrigger aria-label="项目">
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
        </Label>

        <details className="rounded-lg border border-border/70 bg-background/50 p-3">
          <summary className="flex cursor-pointer items-center gap-2 text-sm font-medium">
            <ChevronDown className="size-4" />
            高级选项
          </summary>
          <Label className="mt-3 grid gap-2">
            <span>审核人</span>
            <Select value={selectedReviewerId} onValueChange={setReviewerId}>
              <SelectTrigger aria-label="审核人">
                <SelectValue placeholder="选择审核人" />
              </SelectTrigger>
              <SelectContent>
                {humanReviewers.map((member) => (
                  <SelectItem key={member.id} value={member.principal_id}>
                    {member.display_name_snapshot || member.principal_id} ·{" "}
                    {member.project_role}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Label>
        </details>

        {error ? <p className="text-sm text-destructive">{error}</p> : null}

        <div className="flex justify-end">
          <Button disabled={isSubmitting} onClick={handleSubmit} type="button">
            <SendHorizontal />
            发起任务
          </Button>
        </div>
      </div>
    </LiquidCard>
  );
}

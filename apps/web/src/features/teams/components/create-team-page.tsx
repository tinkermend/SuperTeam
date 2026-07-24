import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { BadgeCheck } from "lucide-react";
import type { TeamOverview } from "@/lib/api/teams";
import { createTeam } from "@/lib/api/teams";
import { Button, Callout, Stepper } from "@/components/superteam";

import { CreateTeamConfigurationCanvas } from "./create-team-configuration-canvas";
import { CreateTeamStepReview } from "./create-team-step-review";
import {
  type CreateTeamDraft,
  emptyCreateTeamDraft,
  toCreateTeamInput
} from "./create-team-draft";

export type CreateTeamCreatedHandler = (
  overview: TeamOverview,
  options: { goToConstitution: boolean },
) => void;

type CreateTeamViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  onCancel?: () => void;
  onCreated?: CreateTeamCreatedHandler;
  showHeading?: boolean;
};

const STEPS = [
  { id: "configure", label: "配置团队" },
  { id: "review", label: "确认并创建" },
] as const;

export function CreateTeamView({
  apiBaseUrl,
  fetcher,
  onCancel,
  onCreated,
  showHeading = true
}: CreateTeamViewProps) {
  const queryClient = useQueryClient();
  /** 0-based step index（与 Soft-Flat Stepper 对齐） */
  const [stepIndex, setStepIndex] = useState(0);
  const [draft, setDraft] = useState<CreateTeamDraft>(emptyCreateTeamDraft);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [goToConstitution, setGoToConstitution] = useState(false);

  const createMutation = useMutation({
    mutationFn: () =>
      createTeam({ baseUrl: apiBaseUrl, fetcher }, toCreateTeamInput(draft)),
    onSuccess: (overview) => {
      void queryClient.invalidateQueries({ queryKey: ["team-summaries"] });
      onCreated?.(overview, { goToConstitution });
    }
});

  function validateBasics(): Record<string, string> {
    const nextErrors: Record<string, string> = {};
    if (!draft.name.trim()) nextErrors.name = "团队名称不能为空";
    if (draft.description.trim().length > 280) {
      nextErrors.description = "团队说明不能超过 280 个字符";
    }
    const slug = draft.slug.trim();
    if (!slug) {
      nextErrors.slug = "团队标识不能为空";
    } else if (slug.length < 3 || slug.length > 64) {
      nextErrors.slug = "团队标识需为 3-64 个字符";
    } else if (!/^[a-z][a-z0-9-]*[a-z0-9]$/.test(slug)) {
      nextErrors.slug =
        "团队标识需以小写字母开头，仅含小写字母、数字和中划线，且以字母或数字结尾";
    }
    if (draft.owners.length === 0) nextErrors.owner = "请至少选择一位负责人";
    return nextErrors;
  }

  function validateStep(index: number): boolean {
    const nextErrors = index === 0 ? validateBasics() : {};
    setErrors(nextErrors);
    return Object.keys(nextErrors).length === 0;
  }

  function handleNext() {
    if (validateStep(stepIndex)) {
      setStepIndex((c) => Math.min(c + 1, STEPS.length - 1));
    }
  }

  function handlePrev() {
    setStepIndex((c) => Math.max(c - 1, 0));
  }

  /** Stepper 仅允许回到已完成步骤（组件侧限制）；此处同步 index。 */
  function handleStepChange(index: number) {
    if (index < stepIndex) {
      setStepIndex(index);
    }
  }

  function handleSubmit() {
    if (validateStep(0)) {
      createMutation.mutate();
    }
  }

  const submitError =
    createMutation.error instanceof Error ? createMutation.error.message : undefined;
  const nextStepLabel = STEPS[stepIndex + 1]?.label;
  const isLastStep = stepIndex >= STEPS.length - 1;

  return (
    <div
      className="flex min-h-0 w-full min-w-0 flex-1 flex-col gap-4"
      data-testid="create-team-view"
    >
      <div
        className={
          showHeading
            ? "flex shrink-0 flex-wrap items-center justify-between gap-3"
            : "flex shrink-0 flex-wrap items-center justify-start gap-3"
        }
      >
        {showHeading ? (
          <div className="min-w-0">
            <p className="text-xs font-medium text-ink-3">团队管理 › 新建团队</p>
            <h1 className="mt-2 text-2xl font-extrabold tracking-tight text-ink">
              新建团队
            </h1>
          </div>
        ) : (
          <span className="sr-only">新建团队步骤</span>
        )}

        <Stepper
          className="shrink-0"
          current={stepIndex}
          onStepChange={handleStepChange}
          steps={STEPS.map((step) => ({ id: step.id, label: step.label }))}
        />
      </div>

      <div
        className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto pb-4"
        data-testid="create-team-scroll-region"
      >
        {submitError ? (
          <Callout tone="danger" title="创建失败" description={submitError} />
        ) : null}

        {stepIndex === 0 ? (
          <CreateTeamConfigurationCanvas
            apiBaseUrl={apiBaseUrl}
            draft={draft}
            errors={errors}
            fetcher={fetcher}
            onChange={setDraft}
          />
        ) : null}

        {stepIndex === 1 ? (
          <CreateTeamStepReview
            draft={draft}
            goToConstitution={goToConstitution}
            setGoToConstitution={setGoToConstitution}
          />
        ) : null}
      </div>

      <div
        className="sticky bottom-0 z-10 flex shrink-0 items-center justify-end gap-3 border-t border-line px-4 py-4 shadow-[0_-12px_24px_rgba(15,23,42,0.06)] sm:px-6"
        data-testid="create-team-actions"
      >
        {onCancel && stepIndex === 0 ? (
          <Button onClick={onCancel} type="button" variant="outline">
            取消
          </Button>
        ) : null}
        {stepIndex > 0 ? (
          <Button onClick={handlePrev} type="button" variant="outline">
            上一步
          </Button>
        ) : null}
        {!isLastStep ? (
          <Button onClick={handleNext} type="button">
            下一步: {nextStepLabel}
          </Button>
        ) : (
          <Button
            disabled={createMutation.isPending}
            onClick={handleSubmit}
            type="button"
          >
            <BadgeCheck data-icon="inline-start" />
            确认并创建
          </Button>
        )}
      </div>
    </div>
  );
}

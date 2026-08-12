import { useEffect, useMemo, useState } from "react";
import { ArrowLeft, ArrowRight, Check, X } from "lucide-react";
import type { UserSummary, UserProjectTeamScope } from "@/lib/api";
import type { CreateProjectInput } from "@/lib/api/projects";
import { cn } from "@/lib/utils";
import { Button } from "@/components/superteam";
import {
  activeSelectableTeams,
  buildProjectCreateInput,
  emptyProjectCreateDraft,
  projectCreateSteps,
  projectCreateValidation,
  type ProjectCreateDraft,
  type ProjectCreateStep
} from "./create-project-draft";
import { ProjectBasicsStep } from "./project-basics-step";
import { ProjectDigitalEmployeesStep } from "./project-digital-employees-step";
import { ProjectHumanOwnersStep } from "./project-human-owners-step";
import { ProjectPolicyStep } from "./project-policy-step";
import { ProjectReviewPanel } from "./project-review-panel";
import { ProjectRuntimeNodesStep } from "./project-runtime-nodes-step";

type CreateProjectShellProps = {
  apiBaseUrl: string;
  availableTeams?: UserProjectTeamScope[];
  currentUser?: UserSummary;
  currentUserError?: string;
  fetcher?: typeof fetch;
  isCurrentUserLoading?: boolean;
  isSubmitting?: boolean;
  isTeamsLoading?: boolean;
  onCancel: () => void;
  showHeading?: boolean;
  onSubmit: (input: CreateProjectInput) => void;
  submitError?: string;
  teamsError?: string;
};

export function CreateProjectShell({
  apiBaseUrl,
  availableTeams,
  currentUser,
  currentUserError,
  fetcher,
  isCurrentUserLoading,
  isSubmitting,
  isTeamsLoading,
  onCancel,
  showHeading = true,
  onSubmit,
  submitError,
  teamsError
}: CreateProjectShellProps) {
  const selectableTeams = useMemo(() => activeSelectableTeams(availableTeams), [availableTeams]);
  const [draft, setDraft] = useState<ProjectCreateDraft>(emptyProjectCreateDraft);
  const [activeStep, setActiveStep] = useState<ProjectCreateStep>("basics");
  const [localError, setLocalError] = useState("");
  const [basicsAttempted, setBasicsAttempted] = useState(false);
  const validation = projectCreateValidation(draft, currentUser?.id, selectableTeams);
  const activeIndex = projectCreateSteps.findIndex((step) => step.id === activeStep);
  const isAuthorizationLoading = Boolean(isCurrentUserLoading || isTeamsLoading);
  const authorizationError = currentUserError || teamsError;

  useEffect(() => {
    setDraft((current) => {
      const selectableTeamIds = new Set(selectableTeams.map((scope) => scope.team_id));
      const sourceTeamIds = current.sourceTeamIds.filter((teamId) => selectableTeamIds.has(teamId));
      const selectedDigitalEmployees = current.selectedDigitalEmployees.filter(
        (employee) => !employee.team_id || sourceTeamIds.includes(employee.team_id),
      );

      if (
        arraysEqual(current.sourceTeamIds, sourceTeamIds) &&
        selectedDigitalEmployees.length === current.selectedDigitalEmployees.length
      ) {
        return current;
      }
      return { ...current, sourceTeamIds, selectedDigitalEmployees };
    });
  }, [selectableTeams]);

  useEffect(() => {
    if (typeof window === "undefined") return;
    function onKey(event: KeyboardEvent) {
      if (event.key === "Escape") onCancel();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onCancel]);

  function goNext() {
    if (activeStep === "basics" && !validation.basics) {
      setBasicsAttempted(true);
      setLocalError(
        validation.nameError ??
          validation.directoryError ??
          validation.repoError ??
          (!draft.goal.trim() ? "请补齐项目目标" : "请补齐基础信息"),
      );
      return;
    }
    setLocalError("");
    if (activeIndex < projectCreateSteps.length - 1) {
      setActiveStep(projectCreateSteps[activeIndex + 1].id);
    }
  }

  function goBack() {
    if (activeIndex > 0) {
      setActiveStep(projectCreateSteps[activeIndex - 1].id);
    }
  }

  function submit() {
    if (!currentUser) {
      setLocalError("当前用户未加载，无法创建项目");
      return;
    }
    if (!validation.basics) {
      setBasicsAttempted(true);
      setLocalError(
        validation.nameError ??
          validation.directoryError ??
          validation.repoError ??
          "请补齐项目名称和目标",
      );
      setActiveStep("basics");
      return;
    }
    if (!validation.teamAuthorized) {
      setLocalError("请选择已授权的数字员工来源团队");
      setActiveStep("digitalEmployees");
      return;
    }
    if (!validation.runtimeNodes) {
      setLocalError("请至少选择一个可运行节点");
      setActiveStep("runtimeNodes");
      return;
    }
    if (!validation.attachProbe) {
      setLocalError("认领已有目录前请先探测目标目录，并确认这就是要认领的目录");
      setActiveStep("runtimeNodes");
      return;
    }
    setLocalError("");
    onSubmit(buildProjectCreateInput(draft, currentUser));
  }

  const canSubmit =
    validation.basics &&
    validation.currentUser &&
    validation.teamAuthorized &&
    validation.runtimeNodes &&
    validation.attachProbe &&
    !authorizationError &&
    !isAuthorizationLoading;

  return (
    <div
      aria-label={showHeading ? undefined : "新建项目"}
      aria-labelledby={showHeading ? "project-create-title" : undefined}
      className="min-h-[calc(100svh-7rem)] overflow-hidden rounded-[16px] border border-line bg-background shadow-sm"
      data-variant="compact-control-surface"
      data-testid="project-create-page"
    >
      <div className="flex min-h-[calc(100svh-7rem)] flex-col">
        <header className="border-b border-line bg-card px-4 py-4 lg:px-6">
          {showHeading ? (
            <div className="flex items-start justify-between gap-4">
              <div>
                <p className="text-[12px] font-bold text-brand">项目管理 / 新建项目</p>
                <h2 className="mt-1 text-xl font-extrabold tracking-tight text-ink" id="project-create-title">新建项目</h2>
                <p className="mt-1 text-[13px] text-ink-2">建立项目事实容器，配置负责人、团队、数字员工池与协调策略。</p>
              </div>
              <Button aria-label="关闭新建项目" className="size-9 rounded-[10px]" onClick={onCancel} size="icon" type="button" variant="ghost">
                <X className="size-5" />
              </Button>
            </div>
          ) : null}
          <nav
            aria-label="新建项目步骤"
            className={cn("flex flex-wrap items-center gap-2", showHeading && "mt-4")}
          >
            {projectCreateSteps.map((step, index) => {
              const active = step.id === activeStep;
              const done = index < activeIndex;
              return (
                <button
                  className={cn(
                    "flex min-w-0 items-center gap-2 rounded-[9px] border px-2.5 py-1.5 text-[12px] font-bold transition",
                    active ? "border-brand/30 bg-brand-soft text-brand" : "border-line bg-card text-ink-2 hover:bg-card-soft",
                  )}
                  key={step.id}
                  onClick={() => setActiveStep(step.id)}
                  type="button"
                >
                  <span
                    className={cn(
                      "grid size-6 shrink-0 place-items-center rounded-[7px] border text-[11px]",
                      active && "border-brand bg-brand text-white",
                      done && "border-ok bg-ok text-white",
                      !active && !done && "border-line-strong bg-card text-ink-2",
                    )}
                  >
                    {done ? <Check className="size-4" /> : index + 1}
                  </span>
                  <span className="truncate">{step.label}</span>
                </button>
              );
            })}
          </nav>
        </header>

        <main className="grid min-h-0 flex-1 grid-cols-1 gap-4 overflow-y-auto px-4 py-4 lg:grid-cols-[minmax(0,1fr)_400px] lg:px-6">
          <section className="min-w-0 rounded-[14px] border border-line bg-card p-5 shadow-sm">
            <div className="mb-4 border-b border-line pb-3">
              <p className="text-base font-extrabold text-ink">新建项目工作台</p>
              <p className="mt-1 text-[12px] leading-5 text-ink-3">
                按步骤补齐项目事实、责任人、来源团队和治理策略。
              </p>
            </div>
            {authorizationError ? (
              <div className="mb-4 rounded-[10px] border border-danger/20 bg-danger-soft px-3 py-2 text-sm text-danger">
                {currentUserError ? "加载当前用户失败" : "加载可选团队失败"}
              </div>
            ) : null}
            {activeStep === "basics" ? (
              <ProjectBasicsStep
                directoryError={validation.directoryError}
                draft={draft}
                nameError={validation.nameError}
                onChange={setDraft}
                repoError={validation.repoError}
                showNameError={basicsAttempted}
              />
            ) : activeStep === "owners" ? (
              <ProjectHumanOwnersStep
                apiBaseUrl={apiBaseUrl}
                currentUser={currentUser}
                draft={draft}
                fetcher={fetcher}
                onChange={setDraft}
              />
            ) : activeStep === "digitalEmployees" ? (
              <ProjectDigitalEmployeesStep
                apiBaseUrl={apiBaseUrl}
                draft={draft}
                fetcher={fetcher}
                onChange={setDraft}
                selectableTeams={selectableTeams}
              />
            ) : activeStep === "runtimeNodes" ? (
              <ProjectRuntimeNodesStep
                apiBaseUrl={apiBaseUrl}
                draft={draft}
                fetcher={fetcher}
                onChange={setDraft}
              />
            ) : activeStep === "policies" ? (
              <ProjectPolicyStep draft={draft} onChange={setDraft} />
            ) : (
              <div className="rounded-[10px] border border-dashed border-line-strong bg-card-soft p-6 text-sm text-ink-2">
                {projectCreateSteps.find((step) => step.id === activeStep)?.label} 步骤将在后续任务中接入。
              </div>
            )}
          </section>

          <ProjectReviewPanel
            apiBaseUrl={apiBaseUrl}
            currentUser={currentUser}
            draft={draft}
            fetcher={fetcher}
            selectableTeams={selectableTeams}
          />
        </main>

        <footer className="flex flex-col gap-3 border-t border-line bg-card px-4 py-3 sm:flex-row sm:items-center sm:justify-between lg:px-6">
          <Button onClick={onCancel} type="button" variant="ghost">
            返回项目列表
          </Button>
          <div className="flex items-center gap-3">
            {(localError || submitError) ? <p className="text-sm text-danger">{localError || submitError}</p> : null}
            <Button disabled={activeIndex === 0} onClick={goBack} type="button" variant="outline">
              <ArrowLeft className="mr-2 size-4" />
              上一步
            </Button>
            {activeIndex === projectCreateSteps.length - 1 ? (
              <Button disabled={isSubmitting || !canSubmit} onClick={submit} type="button">
                创建项目
              </Button>
            ) : (
              <Button onClick={goNext} type="button">
                下一步
                <ArrowRight className="ml-2 size-4" />
              </Button>
            )}
          </div>
        </footer>
      </div>
    </div>
  );
}

function arraysEqual(left: string[], right: string[]) {
  return left.length === right.length && left.every((item, index) => item === right[index]);
}

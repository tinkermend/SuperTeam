import { useEffect, useMemo, useState } from "react";
import { ArrowLeft, ArrowRight, Check, X } from "lucide-react";
import type { UserSummary, UserProjectTeamScope } from "@/lib/api";
import type { CreateProjectInput } from "@/lib/api/projects";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import {
  activeSelectableTeams,
  buildProjectCreateInput,
  emptyProjectCreateDraft,
  projectCreateSteps,
  projectCreateValidation,
  type ProjectCreateDraft,
  type ProjectCreateStep,
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
  teamsError,
}: CreateProjectShellProps) {
  const selectableTeams = useMemo(() => activeSelectableTeams(availableTeams), [availableTeams]);
  const [draft, setDraft] = useState<ProjectCreateDraft>(emptyProjectCreateDraft);
  const [activeStep, setActiveStep] = useState<ProjectCreateStep>("basics");
  const [localError, setLocalError] = useState("");
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
      setLocalError("请补齐项目名称和目标");
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
    setLocalError("");
    onSubmit(buildProjectCreateInput(draft, currentUser));
  }

  const canSubmit =
    validation.basics &&
    validation.currentUser &&
    validation.teamAuthorized &&
    validation.runtimeNodes &&
    !authorizationError &&
    !isAuthorizationLoading;

  return (
    <div
      aria-label={showHeading ? undefined : "新建项目"}
      aria-labelledby={showHeading ? "project-create-title" : undefined}
      className="min-h-[calc(100svh-7rem)] overflow-hidden rounded-[16px] border border-v3-line bg-v3-bg shadow-sm"
      data-variant="compact-control-surface"
      data-testid="project-create-page"
    >
      <div className="flex min-h-[calc(100svh-7rem)] flex-col">
        <header className="border-b border-v3-line bg-v3-card px-4 py-4 lg:px-6">
          {showHeading ? (
            <div className="flex items-start justify-between gap-4">
              <div>
                <p className="text-[12px] font-bold text-v3-brand">项目管理 / 新建项目</p>
                <h2 className="mt-1 text-xl font-extrabold tracking-tight text-v3-ink" id="project-create-title">新建项目</h2>
                <p className="mt-1 text-[13px] text-v3-ink-2">建立项目事实容器，配置负责人、团队、数字员工池与策略预设。</p>
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
                    active ? "border-v3-brand/30 bg-v3-brand-soft text-v3-brand" : "border-v3-line bg-v3-card text-v3-ink-2 hover:bg-v3-card-soft",
                  )}
                  key={step.id}
                  onClick={() => setActiveStep(step.id)}
                  type="button"
                >
                  <span
                    className={cn(
                      "grid size-6 shrink-0 place-items-center rounded-[7px] border text-[11px]",
                      active && "border-v3-brand bg-v3-brand text-white",
                      done && "border-v3-ok bg-v3-ok text-white",
                      !active && !done && "border-v3-line-strong bg-v3-card text-v3-ink-2",
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
          <section className="min-w-0 rounded-[14px] border border-v3-line bg-v3-card p-5 shadow-sm">
            <div className="mb-4 border-b border-v3-line pb-3">
              <p className="text-base font-extrabold text-v3-ink">新建项目工作台</p>
              <p className="mt-1 text-[12px] leading-5 text-v3-ink-3">
                按步骤补齐项目事实、责任人、来源团队和治理策略。
              </p>
            </div>
            {authorizationError ? (
              <div className="mb-4 rounded-[10px] border border-v3-danger/20 bg-v3-danger-soft px-3 py-2 text-sm text-v3-danger">
                {currentUserError ? "加载当前用户失败" : "加载可选团队失败"}
              </div>
            ) : null}
            {activeStep === "basics" ? (
              <ProjectBasicsStep draft={draft} onChange={setDraft} />
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
              <div className="rounded-[10px] border border-dashed border-v3-line-strong bg-v3-card-soft p-6 text-sm text-v3-ink-2">
                {projectCreateSteps.find((step) => step.id === activeStep)?.label} 步骤将在后续任务中接入。
              </div>
            )}
          </section>

          <ProjectReviewPanel currentUser={currentUser} draft={draft} selectableTeams={selectableTeams} />
        </main>

        <footer className="flex flex-col gap-3 border-t border-v3-line bg-v3-card px-4 py-3 sm:flex-row sm:items-center sm:justify-between lg:px-6">
          <Button onClick={onCancel} type="button" variant="ghost">
            返回项目列表
          </Button>
          <div className="flex items-center gap-3">
            {(localError || submitError) ? <p className="text-sm text-v3-danger">{localError || submitError}</p> : null}
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

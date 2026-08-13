import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  PROJECT_DIRECTORY_NAME_MAX,
  PROJECT_DISPLAY_NAME_MAX,
  directoryNameHintFromGitURL,
  validateDisplayProjectName,
  validateProjectDirectoryName,
  type ProjectCreateDraft,
  type ProjectSourceKind
} from "./create-project-draft";

type ProjectBasicsStepProps = {
  draft: ProjectCreateDraft;
  directoryError?: string | null;
  nameError?: string | null;
  onChange: (draft: ProjectCreateDraft) => void;
  repoError?: string | null;
  showNameError?: boolean;
};

export function ProjectBasicsStep({
  draft,
  directoryError,
  nameError,
  onChange,
  repoError,
  showNameError = false
}: ProjectBasicsStepProps) {
  const liveNameError =
    draft.name.length > 0 ? validateDisplayProjectName(draft.name) : null;
  const displayedNameError = showNameError
    ? (nameError ?? liveNameError)
    : liveNameError;

  const gitHint = directoryNameHintFromGitURL(draft.repoUrl);
  const liveDirectoryError =
    draft.sourceKind === "directory" && draft.directoryName.length > 0
      ? validateProjectDirectoryName(draft.directoryName)
      : draft.sourceKind === "git" && draft.directoryName.length > 0
        ? validateProjectDirectoryName(draft.directoryName)
        : null;
  const displayedDirectoryError = showNameError
    ? (directoryError ?? liveDirectoryError)
    : liveDirectoryError;
  const displayedRepoError = showNameError ? repoError : null;

  return (
    <div className="grid gap-5">
      <div className="grid gap-2">
        <Label htmlFor="project-create-name">项目名称 *</Label>
        <Input
          aria-invalid={Boolean(displayedNameError)}
          id="project-create-name"
          maxLength={PROJECT_DISPLAY_NAME_MAX}
          onChange={(event) => onChange({ ...draft, name: event.target.value })}
          placeholder="例如：客户接入试点"
          value={draft.name}
        />
        <p className="text-xs text-ink-3">展示用名称，允许中文；与磁盘目录名分离。</p>
        {displayedNameError ? (
          <p className="text-xs text-danger" role="alert">
            {displayedNameError}
          </p>
        ) : null}
      </div>

      <div className="grid gap-3 rounded-[12px] border border-line bg-card-soft/60 p-4">
        <div className="grid gap-1">
          <Label>源码来源 *</Label>
          <p className="text-xs text-ink-3">
            Git：填写仓库地址，目录名默认由 URL 推导；非 Git：手填空目录名；认领已有目录：只填目录名，须先在节点上存在该目录。
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {(
            [
              { id: "directory", label: "非 Git（空目录）" },
              { id: "git", label: "Git 仓库" },
              { id: "attach", label: "认领已有目录" },
            ] as Array<{ id: ProjectSourceKind; label: string }>
          ).map((option) => {
            const active = draft.sourceKind === option.id;
            return (
              <button
                key={option.id}
                className={
                  active
                    ? "rounded-[10px] border border-brand bg-brand-soft px-3 py-1.5 text-sm font-semibold text-brand"
                    : "rounded-[10px] border border-line bg-card px-3 py-1.5 text-sm text-ink-2"
                }
                onClick={() =>
                  onChange({
                    ...draft,
                    sourceKind: option.id,
                    ...(option.id !== "git"
                      ? { repoUrl: "", repoDefaultBranch: "main" }
                      : {})
})
                }
                type="button"
              >
                {option.label}
              </button>
            );
          })}
        </div>
        {draft.sourceKind === "attach" ? (
          <p className="text-xs text-ink-3">
            认领路径：目录须已在所选 Runtime 节点的工作区根下存在；平台不创建、不填充、不改 git 状态。提交前请在「可运行节点」中选定主节点。
          </p>
        ) : null}

        {draft.sourceKind === "git" ? (
          <div className="grid gap-3">
            <div className="grid gap-2">
              <Label htmlFor="project-create-repo-url">仓库 URL *</Label>
              <Input
                aria-invalid={Boolean(displayedRepoError)}
                id="project-create-repo-url"
                onChange={(event) =>
                  onChange({ ...draft, repoUrl: event.target.value })
                }
                placeholder="https://github.com/org/repo.git 或 git@host:org/repo.git"
                value={draft.repoUrl}
              />
              {displayedRepoError ? (
                <p className="text-xs text-danger" role="alert">
                  {displayedRepoError}
                </p>
              ) : null}
            </div>
            <div className="grid gap-2">
              <Label htmlFor="project-create-repo-branch">默认分支</Label>
              <Input
                id="project-create-repo-branch"
                onChange={(event) =>
                  onChange({ ...draft, repoDefaultBranch: event.target.value })
                }
                placeholder="main"
                value={draft.repoDefaultBranch}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="project-create-directory-name">项目目录名（可选）</Label>
              <Input
                aria-invalid={Boolean(displayedDirectoryError)}
                id="project-create-directory-name"
                maxLength={PROJECT_DIRECTORY_NAME_MAX}
                onChange={(event) =>
                  onChange({ ...draft, directoryName: event.target.value })
                }
                placeholder={gitHint ?? "留空则由仓库名推导"}
                value={draft.directoryName}
              />
              <p className="text-xs text-ink-3">
                {gitHint
                  ? `未填写时将使用：${gitHint}`
                  : "无法从 URL 推导时请手填 ASCII 目录名。"}
              </p>
              {displayedDirectoryError ? (
                <p className="text-xs text-danger" role="alert">
                  {displayedDirectoryError}
                </p>
              ) : null}
            </div>
          </div>
        ) : (
          <div className="grid gap-2">
            <Label htmlFor="project-create-directory-name">项目目录名 *</Label>
            <Input
              aria-invalid={Boolean(displayedDirectoryError)}
              id="project-create-directory-name"
              maxLength={PROJECT_DIRECTORY_NAME_MAX}
              onChange={(event) =>
                onChange({ ...draft, directoryName: event.target.value })
              }
              placeholder="customer-onboarding"
              value={draft.directoryName}
            />
            <p className="text-xs text-ink-3">
              Runtime 工作区根下的文件夹名（全局唯一，创建后不可改）。仅 ASCII 字母/数字/点/下划线/连字符。
            </p>
            {displayedDirectoryError ? (
              <p className="text-xs text-danger" role="alert">
                {displayedDirectoryError}
              </p>
            ) : null}
          </div>
        )}
      </div>

      <div className="grid gap-2">
        <Label htmlFor="project-create-goal">项目目标 *</Label>
        <Textarea
          id="project-create-goal"
          onChange={(event) => onChange({ ...draft, goal: event.target.value })}
          placeholder="描述项目背景、预期产出与成功标准，便于对齐与评估。"
          value={draft.goal}
        />
      </div>

      <div className="grid gap-2">
        <Label htmlFor="project-create-description">描述</Label>
        <Textarea
          id="project-create-description"
          onChange={(event) => onChange({ ...draft, description: event.target.value })}
          placeholder="可选：背景、边界、风险、验收说明。"
          value={draft.description}
        />
      </div>
    </div>
  );
}

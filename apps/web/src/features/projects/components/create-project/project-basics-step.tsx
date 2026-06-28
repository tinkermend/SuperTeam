import type { UserSummary, UserProjectTeamScope } from "@/lib/api";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { UserIdentity } from "@/components/superteam/user-identity";
import type { ProjectCreateDraft } from "./create-project-draft";

type ProjectBasicsStepProps = {
  currentUser?: UserSummary;
  draft: ProjectCreateDraft;
  isAuthorizationLoading: boolean;
  onChange: (draft: ProjectCreateDraft) => void;
  selectableTeams: UserProjectTeamScope[];
};

export function ProjectBasicsStep({
  currentUser,
  draft,
  isAuthorizationLoading,
  onChange,
  selectableTeams,
}: ProjectBasicsStepProps) {
  return (
    <div className="grid gap-5">
      <div className="grid gap-2">
        <Label htmlFor="project-create-name">项目名称 *</Label>
        <Input
          id="project-create-name"
          maxLength={60}
          onChange={(event) => onChange({ ...draft, name: event.target.value })}
          placeholder="客户接入验收"
          value={draft.name}
        />
        <p className="text-xs text-v3-ink-3">建议使用清晰明确的业务闭环名称，2-60 个字符。</p>
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

      <div className="grid gap-2">
        <Label htmlFor="project-create-team">授权团队 *</Label>
        <select
          aria-label="授权团队"
          className="h-10 w-full rounded-xl border border-v3-line-strong bg-v3-card px-3 text-sm text-v3-ink outline-none transition focus-visible:border-v3-brand focus-visible:ring-4 focus-visible:ring-v3-brand/10 disabled:cursor-not-allowed disabled:opacity-60"
          disabled={isAuthorizationLoading || selectableTeams.length === 0}
          id="project-create-team"
          onChange={(event) => onChange({ ...draft, teamId: event.target.value, selectedDigitalEmployees: [] })}
          value={draft.teamId}
        >
          {selectableTeams.length === 0 ? <option value="">暂无可选团队</option> : null}
          {selectableTeams.map((scope) => (
            <option key={scope.id} value={scope.team_id}>
              {scope.team.name}
            </option>
          ))}
        </select>
        <p className="text-xs text-v3-ink-3">只展示当前用户被授权用于创建项目的团队。</p>
      </div>

      <div className="grid gap-2">
        <Label>固定负责人（人类）</Label>
        <div className="rounded-xl border border-v3-line bg-v3-card-soft px-3 py-2">
          {currentUser ? (
            <UserIdentity showSecondary user={currentUser} />
          ) : (
            <p className="text-sm text-v3-ink-3">正在加载当前用户...</p>
          )}
        </div>
        <p className="text-xs text-v3-ink-3">项目最终责任人固定为当前创建人，创建后可在项目配置中调整。</p>
      </div>
    </div>
  );
}

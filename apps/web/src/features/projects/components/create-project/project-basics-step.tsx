import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import type { ProjectCreateDraft } from "./create-project-draft";

type ProjectBasicsStepProps = {
  draft: ProjectCreateDraft;
  onChange: (draft: ProjectCreateDraft) => void;
};

export function ProjectBasicsStep({ draft, onChange }: ProjectBasicsStepProps) {
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
    </div>
  );
}

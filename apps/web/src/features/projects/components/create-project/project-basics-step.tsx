import { useQuery } from "@tanstack/react-query";
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
import { listScenarioTemplates } from "@/lib/api/scenario-templates";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import type { ProjectCreateDraft } from "./create-project-draft";

type ProjectBasicsStepProps = {
  draft: ProjectCreateDraft;
  onChange: (draft: ProjectCreateDraft) => void;
};

const NO_TEMPLATE_VALUE = "__none__";

export function ProjectBasicsStep({ draft, onChange }: ProjectBasicsStepProps) {
  const apiBaseUrl = resolveControlPlaneUrl();
  const templates = useQuery({
    queryKey: ["scenario-templates"],
    queryFn: () => listScenarioTemplates({ baseUrl: apiBaseUrl }),
  });
  const templateOptions = (templates.data ?? []).filter(
    (template) => template.status === "active",
  );

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
        <Label htmlFor="project-create-scenario-template">场景模板</Label>
        <Select
          value={draft.scenarioTemplateKey || NO_TEMPLATE_VALUE}
          onValueChange={(value) =>
            onChange({
              ...draft,
              scenarioTemplateKey: value === NO_TEMPLATE_VALUE ? "" : value,
            })
          }
        >
          <SelectTrigger id="project-create-scenario-template" className="w-full">
            <SelectValue placeholder="不绑定（通用）" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={NO_TEMPLATE_VALUE}>不绑定（通用）</SelectItem>
            {templateOptions.map((template) => (
              <SelectItem key={template.template_key} value={template.template_key}>
                {template.name}（{template.template_key}）
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <p className="text-xs text-v3-ink-3">
          绑定后，规划将按模板的分解骨架与交接契约实例化；不绑定则按通用方式规划。
        </p>
      </div>
    </div>
  );
}

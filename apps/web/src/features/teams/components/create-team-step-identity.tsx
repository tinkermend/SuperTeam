import { ShieldCheck } from "lucide-react";
import { TeamIconPicker } from "@/components/superteam/team-icon-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";
import { type CreateTeamDraft, inferTeamDisplay, slugify } from "./create-team-draft";

const THEME_COLORS = [
  { value: "blue", class: "bg-blue-500", label: "蓝" },
  { value: "cyan", class: "bg-cyan-500", label: "青" },
  { value: "neutral", class: "bg-slate-500", label: "灰" },
  { value: "teal", class: "bg-teal-500", label: "青绿" },
  { value: "violet", class: "bg-violet-500", label: "紫" },
] as const;

export function CreateTeamStepIdentity({
  draft,
  errors,
  onChange
}: {
  draft: CreateTeamDraft;
  errors: Record<string, string>;
  onChange: (draft: CreateTeamDraft) => void;
}) {
  function updateName(name: string) {
    onChange({
      ...draft,
      display: draft.displayTouched
        ? draft.display
        : inferTeamDisplay(`${name} ${draft.slug}`),
      name,
      slug: draft.slugTouched ? draft.slug : slugify(name)
});
  }

  function updateSlug(rawSlug: string) {
    const slug = rawSlug.toLowerCase().replace(/[^a-z0-9-]/g, "");
    onChange({
      ...draft,
      display: draft.displayTouched
        ? draft.display
        : inferTeamDisplay(`${draft.name} ${slug}`),
      slug,
      slugTouched: true
});
  }

  function updateIcon(iconKey: string) {
    onChange({
      ...draft,
      display: { ...draft.display, icon_key: iconKey },
      displayTouched: true
});
  }

  function updateColor(color: typeof draft.display.color_tone) {
    onChange({
      ...draft,
      display: { ...draft.display, color_tone: color },
      displayTouched: true
});
  }

  return (
    <section className="rounded-[22px] border border-line bg-card shadow-card">
      <header className="flex items-center gap-3 border-b border-line px-5 py-3.5 sm:px-6">
        <span className="flex size-8 items-center justify-center rounded-[10px] bg-brand-soft text-brand">
          <ShieldCheck className="size-4" />
        </span>
        <div className="min-w-0">
          <h2 className="text-sm font-semibold text-ink">团队身份</h2>
          <p className="text-xs text-ink-3">名称、标识与配色</p>
        </div>
      </header>

      <div className="grid gap-4 p-5 sm:p-6 lg:grid-cols-2">
        <div className="grid gap-2">
          <Label htmlFor="team-name">团队名称</Label>
          <Input
            id="team-name"
            onChange={(event) => updateName(event.target.value)}
            placeholder="例如：安全响应组"
            value={draft.name}
          />
          {errors.name ? (
            <span className="text-sm text-destructive">{errors.name}</span>
          ) : (
            <span className="text-xs text-ink-3">用于全站展示，可随时修改。</span>
          )}
        </div>

        <div className="grid gap-1.5">
          <Label className="text-xs font-medium text-ink-3" htmlFor="team-slug">
            团队标识 slug
          </Label>
          <div className="flex items-center gap-2 rounded-[12px] border border-line bg-card-soft px-2.5 py-1">
            <span className="select-none font-mono text-xs text-ink-3">/teams/</span>
            <Input
              className="h-8 border-0 bg-transparent px-0 font-mono text-sm shadow-none focus-visible:ring-0"
              id="team-slug"
              onChange={(event) => updateSlug(event.target.value)}
              placeholder="team-slug"
              value={draft.slug}
            />
          </div>
          {errors.slug ? (
            <span className="text-sm text-destructive">{errors.slug}</span>
          ) : (
            <span className="text-xs text-ink-3">默认按名称生成，可手动修改。</span>
          )}
        </div>

        <div className="grid gap-2">
          <Label>
            团队主题色 <span className="text-destructive">*</span>
          </Label>
          <div className="flex flex-wrap items-center gap-2.5">
            {THEME_COLORS.map((color) => {
              const isSelected = draft.display.color_tone === color.value;
              return (
                <button
                  aria-label={`主题色 ${color.label}`}
                  aria-pressed={isSelected}
                  className={cn(
                    "flex size-8 items-center justify-center rounded-[10px] transition",
                    color.class,
                    isSelected
                      ? "ring-2 ring-brand ring-offset-2 ring-offset-card"
                      : "opacity-75 hover:opacity-100",
                  )}
                  key={color.value}
                  onClick={() => updateColor(color.value)}
                  type="button"
                >
                  {isSelected ? <span className="size-1.5 rounded-full bg-white" /> : null}
                </button>
              );
            })}
          </div>
        </div>

        <div className="grid gap-2">
          <Label>
            团队图标 <span className="text-destructive">*</span>
          </Label>
          <TeamIconPicker
            colorTone={draft.display.color_tone}
            onSelect={updateIcon}
            value={draft.display.icon_key}
          />
        </div>
      </div>
    </section>
  );
}

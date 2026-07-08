import { ShieldCheck } from "lucide-react";
import { TeamIconPicker } from "@/components/superteam/team-icon-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";
import { type CreateTeamDraft, inferTeamDisplay, slugify } from "./create-team-draft";

const THEME_COLORS = [
  { value: "blue", class: "bg-blue-500" },
  { value: "cyan", class: "bg-cyan-500" },
  { value: "neutral", class: "bg-slate-500" },
  { value: "teal", class: "bg-teal-500" },
  { value: "violet", class: "bg-violet-500" },
] as const;

export function CreateTeamStepIdentity({
  draft,
  errors,
  onChange,
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
      slug: draft.slugTouched ? draft.slug : slugify(name),
    });
  }

  function updateSlug(rawSlug: string) {
    // 实时规整为后端可接受的字符集：小写字母、数字、中划线；剔除大写/下划线/空格等。
    const slug = rawSlug.toLowerCase().replace(/[^a-z0-9-]/g, "");
    onChange({
      ...draft,
      display: draft.displayTouched
        ? draft.display
        : inferTeamDisplay(`${draft.name} ${slug}`),
      slug,
      slugTouched: true,
    });
  }

  function updateIcon(iconKey: string) {
    onChange({
      ...draft,
      display: { ...draft.display, icon_key: iconKey },
      displayTouched: true,
    });
  }

  function updateColor(color: typeof draft.display.color_tone) {
    onChange({
      ...draft,
      display: { ...draft.display, color_tone: color },
      displayTouched: true,
    });
  }

  return (
    <div className="flex min-w-0 flex-col gap-5">
      <section className="rounded-xl border bg-card shadow-sm">
        <header className="flex items-center gap-3 border-b px-5 py-4">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary/10 text-primary">
            <ShieldCheck className="size-4" />
          </span>
          <div>
            <h2 className="text-sm font-semibold">团队身份</h2>
            <p className="text-xs text-muted-foreground">
              名称、标识与配色，用于全站识别这个团队。
            </p>
          </div>
        </header>
        <div className="flex flex-col gap-5 p-5">
          <div className="grid gap-2">
            <Label htmlFor="team-name">团队名称</Label>
            <Input
              id="team-name"
              onChange={(event) => updateName(event.target.value)}
              value={draft.name}
            />
            {errors.name ? (
              <span className="text-sm text-destructive">{errors.name}</span>
            ) : (
              <span className="text-xs text-muted-foreground">
                用于全站展示，可随时修改。
              </span>
            )}
          </div>

          <div className="grid gap-1.5">
            <Label
              className="text-xs font-medium text-muted-foreground"
              htmlFor="team-slug"
            >
              团队标识 slug
            </Label>
            <div className="flex items-center gap-2 rounded-lg border bg-muted/30 px-2.5 py-1">
              <span className="select-none font-mono text-xs text-muted-foreground">
                /teams/
              </span>
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
              <span className="text-xs text-muted-foreground">
                用于网址与接口标识，默认按名称生成，可手动修改。
              </span>
            )}
          </div>

          <div className="grid gap-2">
            <Label>团队图标 <span className="text-destructive">*</span></Label>
            <div className="text-xs text-muted-foreground mb-1">
              选择一个图标作为团队标识
            </div>
            <TeamIconPicker
              colorTone={draft.display.color_tone}
              onSelect={updateIcon}
              value={draft.display.icon_key}
            />
          </div>

          <div className="grid gap-2 border-t pt-5">
            <Label>团队主题色 <span className="text-destructive">*</span></Label>
            <div className="text-xs text-muted-foreground mb-2">
              选择团队的主题色，用于图标背景与界面点缀
            </div>
            <div className="flex items-center gap-3">
              {THEME_COLORS.map((color) => {
                const isSelected = draft.display.color_tone === color.value;
                return (
                  <button
                    key={color.value}
                    type="button"
                    onClick={() => updateColor(color.value)}
                    className={cn(
                      "flex size-8 items-center justify-center rounded-lg transition-all",
                      color.class,
                      isSelected ? "ring-2 ring-primary ring-offset-2 scale-110" : "hover:scale-105 opacity-80 hover:opacity-100"
                    )}
                  >
                    {isSelected && (
                      <div className="size-2 rounded-full bg-white shadow-sm" />
                    )}
                  </button>
                );
              })}
            </div>
          </div>
        </div>
      </section>
    </div>
  );
}

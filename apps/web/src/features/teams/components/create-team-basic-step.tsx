import { TeamIconTile } from "@/components/superteam/team-icon-tile";
import { UserSearchSelect } from "@/components/superteam/user-search-select";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";
import type { CreateTeamDraft, TeamDisplayDraft } from "./create-team-drawer";

type CreateTeamBasicStepProps = {
  apiBaseUrl: string;
  errors: Record<string, string>;
  fetcher?: typeof fetch;
  onChange: (draft: CreateTeamDraft) => void;
  value: CreateTeamDraft;
};

const iconOptions = [
  { color_tone: "cyan", icon_key: "ops", label: "选择运维团队图标" },
  { color_tone: "blue", icon_key: "dev", label: "选择研发团队图标" },
  { color_tone: "violet", icon_key: "qa", label: "选择测试团队图标" },
  { color_tone: "teal", icon_key: "security", label: "选择安全团队图标" },
  { color_tone: "neutral", icon_key: "default", label: "选择默认团队图标" },
] as const;

export function CreateTeamBasicStep({
  apiBaseUrl,
  errors,
  fetcher,
  onChange,
  value,
}: CreateTeamBasicStepProps) {
  function updateName(name: string) {
    onChange({
      ...value,
      display:
        value.display.icon_key === "default"
          ? inferDisplay(`${name} ${value.slug}`)
          : value.display,
      name,
    });
  }

  function updateSlug(slug: string) {
    onChange({
      ...value,
      display:
        value.display.icon_key === "default"
          ? inferDisplay(`${value.name} ${slug}`)
          : value.display,
      slug,
    });
  }

  function updateDisplay(display: TeamDisplayDraft) {
    onChange({ ...value, display });
  }

  return (
    <div className="flex flex-col gap-5">
      <div className="grid gap-2">
        <Label htmlFor="team-name">团队名称</Label>
        <Input
          id="team-name"
          value={value.name}
          onChange={(event) => updateName(event.target.value)}
        />
        {errors.name ? (
          <span className="text-sm text-destructive">{errors.name}</span>
        ) : null}
      </div>
      <div className="grid gap-2">
        <Label htmlFor="team-slug">团队标识 slug</Label>
        <Input
          id="team-slug"
          value={value.slug}
          onChange={(event) => updateSlug(event.target.value)}
        />
        {errors.slug ? (
          <span className="text-sm text-destructive">{errors.slug}</span>
        ) : null}
      </div>
      <div className="grid gap-2">
        <Label>团队图标</Label>
        <div className="grid grid-cols-5 gap-2">
          {iconOptions.map((option) => {
            const selected = option.icon_key === value.display.icon_key;

            return (
              <Button
                aria-label={option.label}
                aria-pressed={selected}
                className={cn("h-12 justify-center", selected ? "border-primary ring-2 ring-ring/30" : "")}
                key={option.icon_key}
                onClick={() =>
                  updateDisplay({
                    color_tone: option.color_tone,
                    icon_key: option.icon_key,
                  })
                }
                type="button"
                variant="outline"
              >
                <TeamIconTile
                  metadata={{
                    display: {
                      color_tone: option.color_tone,
                      icon_key: option.icon_key,
                    },
                  }}
                />
              </Button>
            );
          })}
        </div>
      </div>
      <div className="grid gap-2">
        <Label>负责人</Label>
        <UserSearchSelect
          apiBaseUrl={apiBaseUrl}
          fetcher={fetcher}
          onSelect={(owner) => onChange({ ...value, owner })}
          placeholder="负责人"
          value={value.owner}
        />
        {errors.owner ? (
          <span className="text-sm text-destructive">{errors.owner}</span>
        ) : null}
      </div>
    </div>
  );
}

function inferDisplay(value: string): TeamDisplayDraft {
  const normalized = value.toLowerCase();
  if (normalized.includes("ops") || normalized.includes("运维")) return { color_tone: "cyan", icon_key: "ops" };
  if (normalized.includes("dev") || normalized.includes("研发")) return { color_tone: "blue", icon_key: "dev" };
  if (normalized.includes("qa") || normalized.includes("测试")) return { color_tone: "violet", icon_key: "qa" };
  if (normalized.includes("security") || normalized.includes("安全")) return { color_tone: "teal", icon_key: "security" };
  return { color_tone: "neutral", icon_key: "default" };
}

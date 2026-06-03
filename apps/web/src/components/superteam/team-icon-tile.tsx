import type { ComponentType, SVGProps } from "react";
import { Code2, FlaskConical, ServerCog, Shield, UsersRound } from "lucide-react";
import { cn } from "@/lib/utils";

const iconLabels = {
  default: "默认团队图标",
  dev: "研发团队图标",
  ops: "运维团队图标",
  qa: "测试团队图标",
  security: "安全团队图标",
} as const;

const toneClasses = {
  blue: "border-blue-200 bg-blue-50 text-blue-600",
  cyan: "border-cyan-200 bg-cyan-50 text-cyan-700",
  neutral: "border-slate-200 bg-slate-50 text-slate-600",
  teal: "border-teal-200 bg-teal-50 text-teal-700",
  violet: "border-violet-200 bg-violet-50 text-violet-600",
} as const;

const iconComponents = {
  default: UsersRound,
  dev: Code2,
  ops: ServerCog,
  qa: FlaskConical,
  security: Shield,
} as const satisfies Record<keyof typeof iconLabels, ComponentType<SVGProps<SVGSVGElement>>>;

type TeamIconKey = keyof typeof iconLabels;
type TeamTone = keyof typeof toneClasses;

export type TeamDisplayMetadata = {
  display?: {
    color_tone?: string;
    icon_key?: string;
  };
};

export function getTeamDisplayConfig(metadata: TeamDisplayMetadata) {
  const iconKey = metadata.display?.icon_key;
  const tone = metadata.display?.color_tone;
  const resolvedIconKey = iconKey && iconKey in iconLabels ? (iconKey as TeamIconKey) : "default";
  const resolvedTone = tone && tone in toneClasses ? (tone as TeamTone) : "neutral";

  return {
    Icon: iconComponents[resolvedIconKey],
    iconKey: resolvedIconKey,
    label: iconLabels[resolvedIconKey],
    tone: resolvedTone,
    toneClassName: toneClasses[resolvedTone],
  };
}

type TeamIconTileProps = {
  className?: string;
  metadata: TeamDisplayMetadata;
};

export function TeamIconTile({ className, metadata }: TeamIconTileProps) {
  const config = getTeamDisplayConfig(metadata);
  const Icon = config.Icon;

  return (
    <div
      aria-label={config.label}
      className={cn(
        "flex size-9 shrink-0 items-center justify-center rounded-md border [&_svg]:size-4",
        config.toneClassName,
        className,
      )}
      role="img"
    >
      <Icon aria-hidden="true" />
    </div>
  );
}

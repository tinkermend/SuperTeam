import type { ComponentType, SVGProps } from "react";
import { Code2, FlaskConical, ServerCog, Shield, UsersRound } from "lucide-react";
import { DynamicIcon, iconNames } from "lucide-react/dynamic";
import { cn } from "@/lib/utils";

const lucideIconNameSet = new Set<string>(iconNames as readonly string[]);

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
  const resolvedTone = tone && tone in toneClasses ? (tone as TeamTone) : "neutral";

  // 旧版固定键：保持静态组件与既有行为。
  if (iconKey && iconKey in iconLabels) {
    const legacyKey = iconKey as TeamIconKey;
    return {
      Icon: iconComponents[legacyKey],
      dynamicName: undefined as string | undefined,
      iconKey: legacyKey,
      label: iconLabels[legacyKey],
      tone: resolvedTone,
      toneClassName: toneClasses[resolvedTone],
    };
  }

  // 任意有效的 lucide 图标名（kebab-case）：动态渲染。
  if (iconKey && lucideIconNameSet.has(iconKey)) {
    return {
      Icon: iconComponents.default,
      dynamicName: iconKey,
      iconKey,
      label: iconKey,
      tone: resolvedTone,
      toneClassName: toneClasses[resolvedTone],
    };
  }

  // 未知值：回退默认图标。
  return {
    Icon: iconComponents.default,
    dynamicName: undefined as string | undefined,
    iconKey: "default" as const,
    label: iconLabels.default,
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
      {config.dynamicName ? (
        <DynamicIcon
          aria-hidden="true"
          name={config.dynamicName as (typeof iconNames)[number]}
        />
      ) : (
        <Icon aria-hidden="true" />
      )}
    </div>
  );
}

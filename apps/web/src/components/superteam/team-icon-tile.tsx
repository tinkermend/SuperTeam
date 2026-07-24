import { cn } from "@/lib/utils";
import {
  DEFAULT_TEAM_ROLE_ICON_KEY,
  getTeamRoleIcon
} from "./team-role-icon-catalog";

const toneClasses = {
  blue: "border-blue-200 bg-blue-50 text-blue-600",
  cyan: "border-cyan-200 bg-cyan-50 text-cyan-700",
  neutral: "border-slate-200 bg-slate-50 text-slate-600",
  teal: "border-teal-200 bg-teal-50 text-teal-700",
  violet: "border-violet-200 bg-violet-50 text-violet-600"
} as const;

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
  const roleIcon =
    getTeamRoleIcon(iconKey) ?? getTeamRoleIcon(DEFAULT_TEAM_ROLE_ICON_KEY)!;

  // 旧的 Lucide 键和未知值统一回退为本地通用团队插画，避免继续渲染旧图标。
  return {
    iconKey: roleIcon.key,
    imageSrc: roleIcon.src,
    label: roleIcon.label,
    tone: resolvedTone,
    toneClassName: toneClasses[resolvedTone]
};
}

type TeamIconTileProps = {
  className?: string;
  metadata: TeamDisplayMetadata;
};

export function TeamIconTile({ className, metadata }: TeamIconTileProps) {
  const config = getTeamDisplayConfig(metadata);

  return (
    <div
      aria-label={config.label}
      className={cn(
        "flex size-9 shrink-0 items-center justify-center overflow-hidden rounded-md border",
        config.toneClassName,
        className,
      )}
      role="img"
    >
      <img
        alt=""
        className="h-[76%] w-[76%] object-contain"
        decoding="async"
        height={32}
        src={config.imageSrc}
        width={32}
      />
    </div>
  );
}

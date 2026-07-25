import { useEffect, useMemo, useState } from "react";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { avatarAssetById } from "@/lib/avatar-assets";
import { setCurrentUserAvatarSvg } from "@/lib/api/auth";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { cn } from "@/lib/utils";

export type UserAvatarDescriptor = {
  options?: Record<string, unknown>;
  provider: "dicebear";
  seed?: string;
  style: "adventurer";
  /** 预渲染的头像 data-URI（P1-D 2b）；非空时直接渲染，不加载 dicebear。 */
  svg?: string;
};

export type UserIdentityData = {
  avatar?: UserAvatarDescriptor;
  avatar_asset_id?: string | null;
  display_name?: string | null;
  email?: string | null;
  id: string;
  status: "active" | "disabled" | string;
  username?: string;
};

export function getUserIdentityLabel(user: UserIdentityData) {
  const displayName = user.display_name?.trim();
  const username = user.username?.trim();
  const email = user.email?.trim();
  const id = user.id.trim();
  const primary = displayName || username || email || id;
  const secondary = email || (username && username !== primary ? username : undefined) || (id !== primary ? id : undefined) || id;

  return {
    initials: primary.trim().slice(0, 1).toUpperCase() || "?",
    primary,
    secondary
};
}

// 懒加载 dicebear 生成头像 data-URI（P1-D 2b）。dicebear 只在此动态 import，落入独立
// chunk、不进入口包；仅当后端没有预渲染 SVG 时才走这条兜底。
export async function generateUserAvatarDataUri(
  avatar: UserAvatarDescriptor | null | undefined,
  username: string,
): Promise<string> {
  if (!avatar || avatar.provider !== "dicebear" || avatar.style !== "adventurer") {
    return "";
  }
  const [{ createAvatar }, adventurer] = await Promise.all([
    import("@dicebear/core"),
    import("@dicebear/adventurer"),
  ]);
  const options = avatar.options ?? {};
  return createAvatar(adventurer, {
    backgroundColor: ["eef8f4", "e6fbf5", "dbeafe"],
    radius: 50,
    seed: avatar.seed || `user:${username}`,
    size: 96,
    ...options,
  }).toDataUri();
}

type UserIdentityAvatarProps = {
  className?: string;
  user: UserIdentityData;
  /** 当此头像属于当前登录用户时置 true：兜底生成后自愈写回后端，下次直接读、永不再生成。 */
  selfHeal?: boolean;
};

export function UserIdentityAvatar({ className, selfHeal = false, user }: UserIdentityAvatarProps) {
  const label = getUserIdentityLabel(user);

  // 同步可得的头像源（零 dicebear）：图片资源缩略图，或后端预渲染的 SVG。
  const storedSrc = useMemo(() => {
    const avatarAsset = avatarAssetById(user.avatar_asset_id);
    if (avatarAsset?.thumbnail_url) {
      return avatarAsset.thumbnail_url;
    }
    return user.avatar?.svg?.trim() || "";
  }, [user.avatar?.svg, user.avatar_asset_id]);

  // 无预渲染 SVG 时才懒加载 dicebear 兜底生成；本人则自愈写回。
  const [generatedSrc, setGeneratedSrc] = useState("");
  useEffect(() => {
    if (storedSrc || !user.avatar) {
      return;
    }
    let cancelled = false;
    void generateUserAvatarDataUri(user.avatar, user.username || label.primary).then((dataUri) => {
      if (cancelled || !dataUri) {
        return;
      }
      setGeneratedSrc(dataUri);
      if (selfHeal) {
        // 自愈：仅当前用户写自己；失败静默（下次仍会兜底生成）。
        void setCurrentUserAvatarSvg({ baseUrl: resolveControlPlaneUrl() }, dataUri).catch(() => {});
      }
    });
    return () => {
      cancelled = true;
    };
  }, [storedSrc, user.avatar, user.username, label.primary, selfHeal]);

  const avatarSrc = storedSrc || generatedSrc;

  return (
    <Avatar className={cn("size-9 border border-border bg-background", className)}>
      {avatarSrc ? <AvatarImage src={avatarSrc} alt={`${label.primary} 的头像`} /> : null}
      <AvatarFallback className="text-xs font-medium">{label.initials}</AvatarFallback>
    </Avatar>
  );
}

type UserIdentityProps = {
  className?: string;
  size?: "sm" | "md";
  showSecondary?: boolean;
  user: UserIdentityData;
  /** 传 true 表示此身份是当前登录用户：兜底生成头像后自愈写回后端（P1-D 2b）。 */
  selfHeal?: boolean;
};

export function UserIdentity({ className, selfHeal = false, size = "md", showSecondary = false, user }: UserIdentityProps) {
  const label = getUserIdentityLabel(user);
  const isSmall = size === "sm";

  return (
    <div className={cn("flex min-w-0 items-center text-left", isSmall ? "gap-2" : "gap-3", className)} data-size={size}>
      <UserIdentityAvatar className={isSmall ? "size-7" : "size-9"} selfHeal={selfHeal} user={user} />
      <div className="min-w-0 flex-1 overflow-hidden text-left">
        <div className={cn("truncate font-medium", isSmall ? "text-xs" : "text-sm")}>{label.primary}</div>
        {showSecondary ? (
          <div className={cn("truncate text-muted-foreground", isSmall ? "text-[11px]" : "text-xs")}>{label.secondary}</div>
        ) : null}
      </div>
    </div>
  );
}

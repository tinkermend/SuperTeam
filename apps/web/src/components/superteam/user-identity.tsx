import { useMemo } from "react";
import { createAvatar } from "@dicebear/core";
import * as adventurer from "@dicebear/adventurer";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { cn } from "@/lib/utils";

export type UserAvatarDescriptor = {
  options?: Record<string, unknown>;
  provider: "dicebear";
  seed?: string;
  style: "adventurer";
};

export type UserIdentityData = {
  avatar?: UserAvatarDescriptor;
  display_name?: string;
  email?: string;
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
    secondary,
  };
}

export function buildUserAvatarDataUri(avatar: UserAvatarDescriptor | null | undefined, username: string) {
  if (!avatar || avatar.provider !== "dicebear" || avatar.style !== "adventurer") {
    return "";
  }

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
};

export function UserIdentityAvatar({ className, user }: UserIdentityAvatarProps) {
  const label = getUserIdentityLabel(user);
  const avatarSrc = useMemo(
    () => (user.avatar ? buildUserAvatarDataUri(user.avatar, user.username || label.primary) : ""),
    [label.primary, user.avatar, user.username],
  );

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
};

export function UserIdentity({ className, size = "md", showSecondary = false, user }: UserIdentityProps) {
  const label = getUserIdentityLabel(user);
  const isSmall = size === "sm";

  return (
    <div className={cn("flex min-w-0 items-center", isSmall ? "gap-2" : "gap-3", className)} data-size={size}>
      <UserIdentityAvatar className={isSmall ? "size-7" : "size-9"} user={user} />
      <div className="min-w-0">
        <div className={cn("truncate font-medium", isSmall ? "text-xs" : "text-sm")}>{label.primary}</div>
        {showSecondary ? (
          <div className={cn("truncate text-muted-foreground", isSmall ? "text-[11px]" : "text-xs")}>{label.secondary}</div>
        ) : null}
      </div>
    </div>
  );
}

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

export function buildUserAvatarDataUri(avatar: UserAvatarDescriptor, username: string) {
  if (avatar.provider !== "dicebear" || avatar.style !== "adventurer") {
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
  showSecondary?: boolean;
  user: UserIdentityData;
};

export function UserIdentity({ className, showSecondary = false, user }: UserIdentityProps) {
  const label = getUserIdentityLabel(user);

  return (
    <div className={cn("flex min-w-0 items-center gap-3", className)}>
      <UserIdentityAvatar user={user} />
      <div className="min-w-0">
        <div className="truncate text-sm font-medium">{label.primary}</div>
        {showSecondary ? <div className="truncate text-xs text-muted-foreground">{label.secondary}</div> : null}
      </div>
    </div>
  );
}

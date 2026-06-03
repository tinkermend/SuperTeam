import { useMemo } from "react";
import { createAvatar } from "@dicebear/core";
import * as adventurer from "@dicebear/adventurer";
import type { UserAvatar as UserAvatarConfig } from "@/lib/api";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";

type UserAvatarProps = {
  avatar: UserAvatarConfig;
  username: string;
};

export function UserAvatar({ avatar, username }: UserAvatarProps) {
  const src = useMemo(() => buildUserAvatarDataUri(avatar, username), [avatar, username]);
  const fallback = username.trim().slice(0, 1).toUpperCase() || "?";

  return (
    <Avatar className="size-10 border border-border bg-background">
      <AvatarImage src={src} alt={`${username} 的头像`} />
      <AvatarFallback className="text-xs font-medium">{fallback}</AvatarFallback>
    </Avatar>
  );
}

export function buildUserAvatarDataUri(avatar: UserAvatarConfig, username: string) {
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

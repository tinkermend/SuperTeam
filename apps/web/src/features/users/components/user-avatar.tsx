import type { UserAvatar as UserAvatarConfig } from "@/lib/api";
import { UserIdentityAvatar, buildUserAvatarDataUri } from "@/components/superteam/user-identity";

type UserAvatarProps = {
  avatar: UserAvatarConfig;
  username: string;
};

export function UserAvatar({ avatar, username }: UserAvatarProps) {
  return (
    <UserIdentityAvatar
      className="size-10"
      user={{ avatar, id: username, status: "active", username }}
    />
  );
}

export { buildUserAvatarDataUri };

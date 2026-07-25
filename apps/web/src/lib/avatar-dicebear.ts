import { createAvatar } from "@dicebear/core";
import * as adventurer from "@dicebear/adventurer";
import type { UserAvatarDescriptor } from "@/components/superteam/user-identity";

/**
 * 同步用 dicebear 生成头像 data-URI。
 *
 * 只给「头像选择器」这类需要即时预览多个预设的场景用（features/users 懒路由 chunk）。
 * dicebear 因此落在该路由 chunk、不进入口包。展示路径（侧栏/列表/详情，入口级）不引用
 * 本模块——那条路径优先读后端预渲染 SVG，缺失时才 user-identity 里动态 import dicebear
 * 兜底（P1-D 2b）。
 */
export function buildUserAvatarDataUri(
  avatar: UserAvatarDescriptor | null | undefined,
  username: string,
): string {
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

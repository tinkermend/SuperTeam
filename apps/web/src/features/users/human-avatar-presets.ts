import type { UserAvatar } from "@/lib/api";

export type HumanAvatarPreset = {
  avatar: UserAvatar;
  id: string;
  label: string;
};

const seeds = [
  "user:admin",
  "user:operator",
  "user:zhoumin",
  "user:zhaoqiang",
  "user:wangyi",
  "user:lina",
  "user:chenhao",
  "user:sunyan",
  "user:liwei",
  "user:zhangmin",
  "user:helen",
  "user:reviewer",
];

export const HUMAN_AVATAR_PRESETS: HumanAvatarPreset[] = seeds.map((seed, index) => ({
  avatar: {
    provider: "dicebear",
    seed,
    style: "adventurer",
  },
  id: `human-${String(index + 1).padStart(2, "0")}`,
  label: `人类头像 ${String(index + 1).padStart(2, "0")}`,
}));

import type { DigitalEmployeeAvatarAsset } from "@/lib/api/employees";

export const BUILT_IN_AVATAR_ASSETS: DigitalEmployeeAvatarAsset[] = [
  avatarAsset("engineer-m-01", "工程师头像 M01", "male", "24"),
  avatarAsset("engineer-m-02", "工程师头像 M02", "male", "31"),
  avatarAsset("engineer-m-03", "工程师头像 M03", "male", "28"),
  avatarAsset("engineer-m-04", "工程师头像 M04", "male", "38"),
  avatarAsset("engineer-m-05", "工程师头像 M05", "male", "35"),
  avatarAsset("engineer-m-06", "工程师头像 M06", "male", "29"),
  avatarAsset("engineer-m-07", "工程师头像 M07", "male", "22"),
  avatarAsset("engineer-m-08", "工程师头像 M08", "male", "33"),
  avatarAsset("engineer-m-09", "工程师头像 M09", "male", "27"),
  avatarAsset("engineer-m-10", "工程师头像 M10", "male", "40"),
  avatarAsset("engineer-m-11", "工程师头像 M11", "male", "24"),
  avatarAsset("engineer-m-12", "工程师头像 M12", "male", "33"),
  avatarAsset("engineer-m-13", "工程师头像 M13", "male", "39"),
  avatarAsset("engineer-m-14", "工程师头像 M14", "male", "29"),
  avatarAsset("engineer-m-15", "工程师头像 M15", "male", "21"),
  avatarAsset("engineer-m-16", "工程师头像 M16", "male", "36"),
  avatarAsset("engineer-m-17", "工程师头像 M17", "male", "27"),
  avatarAsset("engineer-m-18", "工程师头像 M18", "male", "31"),
  avatarAsset("engineer-f-01", "工程师头像 F01", "female", "23"),
  avatarAsset("engineer-f-02", "工程师头像 F02", "female", "30"),
  avatarAsset("engineer-f-03", "工程师头像 F03", "female", "27"),
  avatarAsset("engineer-f-04", "工程师头像 F04", "female", "34"),
  avatarAsset("engineer-f-05", "工程师头像 F05", "female", "37"),
  avatarAsset("engineer-f-06", "工程师头像 F06", "female", "32"),
  avatarAsset("engineer-f-07", "工程师头像 F07", "female", "21"),
  avatarAsset("engineer-f-08", "工程师头像 F08", "female", "39"),
  avatarAsset("engineer-f-09", "工程师头像 F09", "female", "26"),
  avatarAsset("engineer-f-10", "工程师头像 F10", "female", "29"),
  avatarAsset("engineer-f-11", "工程师头像 F11", "female", "25"),
  avatarAsset("engineer-f-12", "工程师头像 F12", "female", "38"),
  avatarAsset("engineer-f-13", "工程师头像 F13", "female", "30"),
  avatarAsset("engineer-f-14", "工程师头像 F14", "female", "22"),
];

export function avatarAssetById(id: string | null | undefined) {
  const normalized = id?.trim().toLowerCase();
  return BUILT_IN_AVATAR_ASSETS.find((asset) => asset.id === normalized);
}

function avatarAsset(id: string, label: string, gender: string, ageRange: string): DigitalEmployeeAvatarAsset {
  return {
    id,
    label,
    gender,
    age_range: ageRange,
    style: "photorealistic_2d",
    image_url: `/images/digital-employee-avatars/${id}.webp`,
    thumbnail_url: `/images/digital-employee-avatars/${id}-256.webp`,
    source: "ai_generated_internal_pack",
    license: "internal_product_asset",
    status: "active",
  };
}

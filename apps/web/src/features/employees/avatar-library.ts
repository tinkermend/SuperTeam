import type { DigitalEmployeeAvatarAsset, DigitalEmployee, DigitalEmployeeOverviewItem } from "@/lib/api/employees";

export const DIGITAL_EMPLOYEE_AVATAR_ASSETS: DigitalEmployeeAvatarAsset[] = [
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
];

export function employeeAvatarAsset(employee: Pick<DigitalEmployee, "id" | "metadata">): DigitalEmployeeAvatarAsset {
  return avatarAssetFromMetadata(employee.metadata) ?? stableAvatarAsset(employee.id);
}

export function overviewAvatarAsset(item: DigitalEmployeeOverviewItem): DigitalEmployeeAvatarAsset {
  return item.identity_summary.avatar_asset ?? stableAvatarAsset(item.identity_summary.id);
}

export function stableAvatarAsset(seed: string): DigitalEmployeeAvatarAsset {
  const normalized = seed.trim();
  let hash = 0;
  for (const char of normalized) {
    hash = (hash * 31 + char.charCodeAt(0)) >>> 0;
  }
  return DIGITAL_EMPLOYEE_AVATAR_ASSETS[hash % DIGITAL_EMPLOYEE_AVATAR_ASSETS.length] ?? DIGITAL_EMPLOYEE_AVATAR_ASSETS[0];
}

function avatarAssetFromMetadata(metadata: DigitalEmployee["metadata"]): DigitalEmployeeAvatarAsset | undefined {
  const avatar = metadata?.avatar;
  if (!isAvatarRecord(avatar)) {
    return undefined;
  }
  const id = stringValue(avatar.id);
  const fromLibrary = DIGITAL_EMPLOYEE_AVATAR_ASSETS.find((asset) => asset.id === id);
  if (fromLibrary) {
    return fromLibrary;
  }
  const imageURL = stringValue(avatar.image_url);
  const thumbnailURL = stringValue(avatar.thumbnail_url);
  if (!id || !thumbnailURL) {
    return undefined;
  }
  return {
    id,
    label: stringValue(avatar.label) || id,
    gender: stringValue(avatar.gender),
    age_range: stringValue(avatar.age_range),
    style: stringValue(avatar.style) || "photorealistic_2d",
    image_url: imageURL || thumbnailURL,
    thumbnail_url: thumbnailURL,
    source: stringValue(avatar.source),
    license: stringValue(avatar.license),
    status: stringValue(avatar.status) || "active",
  };
}

function isAvatarRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

function stringValue(value: unknown) {
  return typeof value === "string" ? value : "";
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

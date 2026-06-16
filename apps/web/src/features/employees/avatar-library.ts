import type { DigitalEmployeeAvatarAsset, DigitalEmployee, DigitalEmployeeOverviewItem } from "@/lib/api/employees";
import { avatarAssetById, BUILT_IN_AVATAR_ASSETS } from "@/lib/avatar-assets";

export const DIGITAL_EMPLOYEE_AVATAR_ASSETS = BUILT_IN_AVATAR_ASSETS;

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
  return BUILT_IN_AVATAR_ASSETS[hash % BUILT_IN_AVATAR_ASSETS.length] ?? BUILT_IN_AVATAR_ASSETS[0];
}

function avatarAssetFromMetadata(metadata: DigitalEmployee["metadata"]): DigitalEmployeeAvatarAsset | undefined {
  const avatar = metadata?.avatar;
  if (!isAvatarRecord(avatar)) {
    return undefined;
  }
  const id = stringValue(avatar.id);
  const fromLibrary = avatarAssetById(id);
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

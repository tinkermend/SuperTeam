import type { ApiClientOptions } from "./client";
import { deleteJsonWithResponse, getJson, putJson } from "./client";

export type SystemConfigItem = {
  key: string;
  /** 领域标签，前端按此分 tab；未知 domain 落"其他"。 */
  domain: string;
  label: string;
  description: string;
  /** bytes | duration_seconds | int | string（服务端注册表校验，非封闭枚举）。 */
  value_type: string;
  /** 数值型默认值；string 型为 0。 */
  default_value: number;
  /** 数值型生效值；string 型为 0。 */
  effective_value: number;
  /** string 型默认文本。 */
  default_string_value?: string;
  /** string 型生效文本。 */
  effective_string_value?: string;
  /** string 型最大字符数。 */
  max_string_length?: number;
  is_overridden: boolean;
  min_value: number;
  max_value: number;
  updated_at?: string;
  updated_by_name?: string;
};

export type SystemConfigListResponse = {
  items: SystemConfigItem[];
};

export type UpdateSystemConfigInput =
  | { value: number; string_value?: never }
  | { string_value: string; value?: never };

export function listSystemConfigs(options: ApiClientOptions) {
  return getJson<SystemConfigListResponse>(options, "/api/v1/system-configs", "system-configs");
}

export function updateSystemConfig(options: ApiClientOptions, key: string, input: UpdateSystemConfigInput) {
  return putJson<SystemConfigItem>(
    options,
    `/api/v1/system-configs/${encodeURIComponent(key)}`,
    input,
    "system-config-update",
  );
}

export function resetSystemConfig(options: ApiClientOptions, key: string) {
  return deleteJsonWithResponse<SystemConfigItem>(
    options,
    `/api/v1/system-configs/${encodeURIComponent(key)}`,
    "system-config-reset",
  );
}

export function isStringConfig(item: Pick<SystemConfigItem, "value_type">): boolean {
  return item.value_type === "string";
}

/** 高危配置 key：改动影响存量路径且不自动迁移。 */
export const HIGH_DANGER_SYSTEM_CONFIG_KEYS = new Set(["runtime.workspace_base_dir"]);

export function isHighDangerConfig(item: Pick<SystemConfigItem, "key">): boolean {
  return HIGH_DANGER_SYSTEM_CONFIG_KEYS.has(item.key);
}

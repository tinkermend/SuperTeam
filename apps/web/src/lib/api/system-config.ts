import type { ApiClientOptions } from "./client";
import { deleteJsonWithResponse, getJson, putJson } from "./client";

export type SystemConfigItem = {
  key: string;
  /** 领域标签，前端按此分 tab；未知 domain 落"其他"。 */
  domain: string;
  label: string;
  description: string;
  /** bytes | duration_seconds（服务端注册表校验，非封闭枚举）。 */
  value_type: string;
  default_value: number;
  effective_value: number;
  is_overridden: boolean;
  min_value: number;
  max_value: number;
  updated_at?: string;
  updated_by_name?: string;
};

export type SystemConfigListResponse = {
  items: SystemConfigItem[];
};

export function listSystemConfigs(options: ApiClientOptions) {
  return getJson<SystemConfigListResponse>(options, "/api/v1/system-configs", "system-configs");
}

export function updateSystemConfig(options: ApiClientOptions, key: string, value: number) {
  return putJson<SystemConfigItem>(
    options,
    `/api/v1/system-configs/${encodeURIComponent(key)}`,
    { value },
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

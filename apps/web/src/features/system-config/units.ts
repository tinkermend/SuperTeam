import type { SystemConfigItem } from "@/lib/api/system-config";

/** 编辑单位：按 value_type 与默认值量级选定，输入与边界提示均以该单位表达。 */
export type ConfigUnit = {
  label: string;
  factor: number;
};

const MIB = 1024 * 1024;

export function unitFor(item: SystemConfigItem): ConfigUnit {
  if (item.value_type === "bytes") {
    return { label: "MiB", factor: MIB };
  }
  if (item.value_type === "duration_seconds") {
    if (item.default_value % 3600 === 0 && item.min_value % 3600 === 0) {
      return { label: "小时", factor: 3600 };
    }
    if (item.default_value % 60 === 0 && item.min_value % 60 === 0) {
      return { label: "分钟", factor: 60 };
    }
    return { label: "秒", factor: 1 };
  }
  return { label: "", factor: 1 };
}

/** 人类可读展示：按值本身选最合适单位（与编辑单位无关）。 */
export function formatConfigValue(item: SystemConfigItem, rawValue: number): string {
  if (item.value_type === "bytes") {
    if (rawValue % MIB === 0) return `${rawValue / MIB} MiB`;
    return `${(rawValue / MIB).toFixed(1)} MiB`;
  }
  if (item.value_type === "duration_seconds") {
    if (rawValue % 3600 === 0) return `${rawValue / 3600} 小时`;
    if (rawValue % 60 === 0) return `${rawValue / 60} 分钟`;
    return `${rawValue} 秒`;
  }
  return String(rawValue);
}

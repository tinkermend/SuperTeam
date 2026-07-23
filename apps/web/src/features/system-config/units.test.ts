import { describe, expect, it } from "vitest";
import type { SystemConfigItem } from "@/lib/api/system-config";
import {
  displayDefaultValue,
  displayEffectiveValue,
  formatConfigValue,
  unitFor,
} from "./units";

const MIB = 1024 * 1024;

function item(overrides: Partial<SystemConfigItem>): SystemConfigItem {
  return {
    key: "test.key",
    domain: "artifact",
    label: "测试",
    description: "",
    value_type: "bytes",
    default_value: 10 * MIB,
    effective_value: 10 * MIB,
    is_overridden: false,
    min_value: MIB,
    max_value: 10 * MIB,
    ...overrides,
  };
}

describe("unitFor", () => {
  it("bytes 用 MiB", () => {
    expect(unitFor(item({}))).toEqual({ label: "MiB", factor: MIB });
  });

  it("整小时的 duration 用小时", () => {
    const ttl = item({
      value_type: "duration_seconds",
      default_value: 12 * 3600,
      min_value: 3600,
      max_value: 7 * 24 * 3600,
    });
    expect(unitFor(ttl)).toEqual({ label: "小时", factor: 3600 });
  });

  it("整分钟但非整小时的 duration 用分钟", () => {
    const ttl = item({
      value_type: "duration_seconds",
      default_value: 15 * 60,
      min_value: 60,
      max_value: 3600,
    });
    expect(unitFor(ttl)).toEqual({ label: "分钟", factor: 60 });
  });

  it("int 纯计数型免单位换算(P2 注册表新增类型)", () => {
    const count = item({
      value_type: "int",
      default_value: 20,
      min_value: 1,
      max_value: 100,
    });
    expect(unitFor(count)).toEqual({ label: "", factor: 1 });
  });

  it("string 型免单位", () => {
    const path = item({
      value_type: "string",
      default_value: 0,
      effective_value: 0,
      default_string_value: ".superteam/workspaces",
      effective_string_value: ".superteam/workspaces",
      max_string_length: 512,
      min_value: 0,
      max_value: 0,
    });
    expect(unitFor(path)).toEqual({ label: "", factor: 1 });
  });
});

describe("formatConfigValue", () => {
  it("按值本身选最合适单位", () => {
    const ttl = item({ value_type: "duration_seconds" });
    expect(formatConfigValue(ttl, 43200)).toBe("12 小时");
    expect(formatConfigValue(ttl, 900)).toBe("15 分钟");
    expect(formatConfigValue(ttl, 90)).toBe("90 秒");
    expect(formatConfigValue(item({}), 10 * MIB)).toBe("10 MiB");
    expect(formatConfigValue(item({}), 1.5 * MIB)).toBe("1.5 MiB");
  });

  it("int 纯计数型原样展示", () => {
    const count = item({ value_type: "int", default_value: 20 });
    expect(formatConfigValue(count, 20)).toBe("20");
  });

  it("string 型展示文本路径", () => {
    const path = item({
      key: "runtime.workspace_base_dir",
      value_type: "string",
      default_value: 0,
      effective_value: 0,
      default_string_value: ".superteam/workspaces",
      effective_string_value: "/data/workspaces",
      max_string_length: 512,
      min_value: 0,
      max_value: 0,
    });
    expect(displayEffectiveValue(path)).toBe("/data/workspaces");
    expect(displayDefaultValue(path)).toBe(".superteam/workspaces");
  });
});

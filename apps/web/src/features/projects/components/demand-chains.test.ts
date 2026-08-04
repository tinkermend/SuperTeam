import { describe, expect, it } from "vitest";

import type { ProjectDemand } from "@/lib/api/projects";

import { findChainOf, foldDemandChains } from "./demand-chains";

function demand(id: string, continuesDemandId?: string): ProjectDemand {
  return {
    attachments: [],
    coordination_mode: "plan",
    continues_demand_id: continuesDemandId,
    id,
    project_id: "project-1",
    reviewer: null,
    source_refs: {},
    source_type: "manual",
    status: "completed",
    submitted_by_user_id: "user-1",
    tenant_id: "tenant-1",
    title: `需求 ${id}`
  };
}

describe("foldDemandChains", () => {
  it("把接续单折进同一条链，列表行代表链上最新一单", () => {
    const chains = foldDemandChains([demand("a"), demand("b", "a"), demand("c", "b")]);

    expect(chains).toHaveLength(1);
    expect(chains[0].members.map((item) => item.id)).toEqual(["a", "b", "c"]);
    expect(chains[0].latest.id).toBe("c");
  });

  it("互不相干的单各占一条链", () => {
    const chains = foldDemandChains([demand("a"), demand("x"), demand("b", "a")]);

    expect(chains).toHaveLength(2);
    expect(chains.map((chain) => chain.latest.id).sort()).toEqual(["b", "x"]);
  });

  it("前序不在当前页时按链头处理，不挂到看不见的父节点上", () => {
    // 分页导致父单 "a" 不在列表里：b 必须自成一链，否则整条链渲染不出来。
    const chains = foldDemandChains([demand("b", "a")]);

    expect(chains).toHaveLength(1);
    expect(chains[0].latest.id).toBe("b");
  });

  it("畸形数据成环也能停机", () => {
    const looped = [demand("a", "b"), demand("b", "a")];

    // 两者互指 → 都不是链头 → 结果为空但**不挂死**，这是本用例的重点。
    expect(foldDemandChains(looped)).toEqual([]);
  });

  it("findChainOf 能按链内任一成员定位所属链", () => {
    const chains = foldDemandChains([demand("a"), demand("b", "a"), demand("x")]);

    expect(findChainOf(chains, "a")?.latest.id).toBe("b");
    expect(findChainOf(chains, "b")?.latest.id).toBe("b");
    expect(findChainOf(chains, "x")?.latest.id).toBe("x");
    expect(findChainOf(chains, undefined)).toBeUndefined();
  });
});

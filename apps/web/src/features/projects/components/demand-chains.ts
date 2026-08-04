import type { ProjectDemand } from "@/lib/api/projects";

export type DemandChain = {
  /** 链上最新一单——列表行代表它（人关心的是"这条线现在怎么样了"）。 */
  latest: ProjectDemand;
  /** 链成员，链头在前。 */
  members: ProjectDemand[];
};

/**
 * 把需求列表按接续血缘折叠成链（spec 2026-08-01 §8.2）。
 *
 * 不折叠的话，每接续一次就在需求列表里多出一行看似无关的单——那正是
 * 「一单的身份是链而不是行」这条不变量要防的事。
 *
 * 纯投影：只用列表里已有的 continues_demand_id，不发请求。父列表分页导致
 * 前序单不在当前页时，该单按链头处理（宁可少折一层，也不凭空引用看不见的行）。
 */
export function foldDemandChains(demands: ProjectDemand[]): DemandChain[] {
  const byID = new Map(demands.map((demand) => [demand.id, demand]));
  const childrenByParent = new Map<string, ProjectDemand[]>();
  const heads: ProjectDemand[] = [];

  for (const demand of demands) {
    const parentID = demand.continues_demand_id;
    // 前序不在本页 → 当链头，避免挂到一个渲染不出来的父节点上。
    if (!parentID || !byID.has(parentID)) {
      heads.push(demand);
      continue;
    }
    const siblings = childrenByParent.get(parentID) ?? [];
    siblings.push(demand);
    childrenByParent.set(parentID, siblings);
  }

  return heads.map((head) => {
    const members: ProjectDemand[] = [];
    const visited = new Set<string>();
    // 广度优先展开后代；visited 兜住畸形数据成环（服务端不可能写出环，
    // 但这里是纯前端投影，宁可自带停机保证）。
    let frontier = [head];
    while (frontier.length > 0) {
      const next: ProjectDemand[] = [];
      for (const demand of frontier) {
        if (visited.has(demand.id)) continue;
        visited.add(demand.id);
        members.push(demand);
        next.push(...(childrenByParent.get(demand.id) ?? []));
      }
      frontier = next;
    }
    return { latest: members[members.length - 1] ?? head, members };
  });
}

/** 某个 demand 落在哪条链上（用于保持选中项可见）。 */
export function findChainOf(chains: DemandChain[], demandId?: string) {
  if (!demandId) return undefined;
  return chains.find((chain) => chain.members.some((member) => member.id === demandId));
}

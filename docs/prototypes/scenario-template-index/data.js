const TEMPLATES = [
  {
    key: "generic",
    name: "通用兜底",
    desc: "未匹配专用场景时的 generic 行为。",
    status: "active",
    updated: "10 天前",
    roles: [],
    serial: true,
    groups: [],
    exits: [],
    deepest: ""
  },
  {
    key: "incident_response",
    name: "故障处置",
    desc: "告警接入到恢复与复盘的运维处置骨架。",
    status: "active",
    updated: "2 天前",
    roles: ["值班", "诊断", "变更", "独立验证"],
    serial: true,
    groups: [
      [{ title: "接入", role: "值班", exit: true }],
      [{ title: "定界", role: "诊断", exit: true }],
      [{ title: "根因", role: "诊断", exit: false }],
      [{ title: "恢复", role: "变更", exit: true }],
      [{ title: "复盘", role: "独立验证", exit: true }]
    ],
    exits: ["接入即止", "定界可交", "恢复完成", "复盘关闭"],
    deepest: "复盘关闭"
  },
  {
    key: "ops_analysis",
    name: "运维分析",
    desc: "从采集到复核签发的分析链。",
    status: "active",
    updated: "6 天前",
    roles: ["分析", "复核"],
    serial: true,
    groups: [
      [{ title: "采集", role: "分析", exit: false }],
      [{ title: "研判", role: "分析", exit: true }],
      [{ title: "复核", role: "复核", exit: true }]
    ],
    exits: ["研判结论", "复核签发"],
    deepest: "复核签发"
  },
  {
    key: "software_delivery",
    name: "软件交付",
    desc: "开发后审查与测试并行，再发布。列表按 depends_on 预览，不拉成假直线。",
    status: "active",
    updated: "昨天",
    roles: ["开发", "审查", "测试"],
    serial: false,
    groups: [
      [{ title: "开发", role: "开发", exit: false }],
      [
        { title: "审查", role: "审查", exit: true },
        { title: "测试", role: "测试", exit: true }
      ],
      [{ title: "发布", role: "开发", exit: true }]
    ],
    exits: ["审查通过", "测试通过", "已发布"],
    deepest: "已发布"
  },
  {
    key: "change_review",
    name: "变更评审",
    desc: "已停用。新规划不再匹配此骨架。",
    status: "disabled",
    updated: "30 天前",
    roles: ["提议", "批准"],
    serial: true,
    groups: [
      [{ title: "提案", role: "提议", exit: false }],
      [{ title: "批准", role: "批准", exit: true }]
    ],
    exits: ["批准落地"],
    deepest: "批准落地"
  },
  {
    key: "ops_review",
    name: "运维评审",
    desc: "停用中的评审骨架。",
    status: "disabled",
    updated: "18 天前",
    roles: ["提交", "评审"],
    serial: true,
    groups: [
      [{ title: "提交", role: "提交", exit: false }],
      [{ title: "评审", role: "评审", exit: true }]
    ],
    exits: ["评审结论"],
    deepest: "评审结论"
  }
];

function nodeHtml(n) {
  return `<span class="step ${n.exit ? "exit" : ""}">${n.exit ? `<span class="exit-dot" title="可收口"></span>` : ""}<span class="step-title">${n.title}</span><span class="role">${n.role}</span></span>`;
}

function chainHtml(t) {
  if (!t.groups.length) {
    return `<p class="generic">无骨架 · generic 行为</p>`;
  }
  const parts = [];
  t.groups.forEach((group, i) => {
    if (i) parts.push(`<span class="arrow">→</span>`);
    if (group.length === 1) {
      parts.push(nodeHtml(group[0]));
    } else {
      parts.push(`<span class="par">${group.map((n, j) => `${j ? `<span class="par-mark">∥</span>` : ""}${nodeHtml(n)}`).join("")}</span>`);
    }
  });
  return `<div class="chain">${parts.join("")}</div>`;
}

import type { RoleVocabularyEntry } from "@/lib/api/casting";

export type NodeVerify = "none" | "independent" | "human";

export type ComposerNode = {
  id: string;
  step: string;
  title: string;
  roleKey: string;
  dependsOn: string[];
  required: boolean;
  verify: NodeVerify;
  independentRoleKey: string;
  produceName: string;
  exit: boolean;
  exitLabel: string;
};

export type ComposerDraft = {
  nodes: ComposerNode[];
};

export type ComposerIssue = {
  message: string;
};

type SpecRole = { key?: string; title?: string; required_capabilities?: string[] };
type SpecProduce = { name?: string; kind?: string };
type SpecStep = {
  step?: string;
  title?: string;
  role?: string;
  depends_on?: string[];
  produces_defaults?: SpecProduce[];
  required_inputs_defaults?: string[];
};
type SpecExit = { deliverable?: string; label?: string };
type SpecConstraint = {
  kind?: string;
  roles?: string[];
  step?: string;
  target?: string;
  when?: { exit_at_or_beyond?: string };
};

let nodeSeq = 0;

export function newComposerNode(index: number, previousStep?: string): ComposerNode {
  nodeSeq += 1;
  const step = `step_${index}`;
  return {
    id: `node_${nodeSeq}`,
    step,
    title: "",
    roleKey: "",
    dependsOn: previousStep ? [previousStep] : [],
    required: true,
    verify: "none",
    independentRoleKey: "",
    produceName: `${step}_outcome`,
    exit: true,
    exitLabel: "",
  };
}

export function emptyComposerDraft(): ComposerDraft {
  return { nodes: [newComposerNode(1)] };
}

export const TEMPLATE_KEY_PATTERN = /^[A-Za-z][A-Za-z0-9_]{1,63}$/;

export function isValidTemplateKey(key: string): boolean {
  return TEMPLATE_KEY_PATTERN.test(key.trim());
}

export function suggestTemplateKey(name: string): string {
  const ascii = name
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "");
  return ascii.length >= 2 && TEMPLATE_KEY_PATTERN.test(ascii) ? ascii.slice(0, 64) : "";
}

export function nodeDisplayTitle(
  node: ComposerNode,
  vocabulary: RoleVocabularyEntry[],
): string {
  const titleOf = (key: string) =>
    vocabulary.find((entry) => entry.role_key === key)?.title || key;
  return node.title.trim() || titleOf(node.roleKey) || "未命名站";
}

export function exitDisplayLabel(
  node: ComposerNode,
  vocabulary: RoleVocabularyEntry[],
): string {
  return node.exitLabel.trim() || nodeDisplayTitle(node, vocabulary);
}

export function composerSummary(
  draft: ComposerDraft,
  vocabulary: RoleVocabularyEntry[],
): string {
  const named = draft.nodes.filter((node) => node.roleKey);
  if (named.length === 0) {
    return "还没有关键节点。添加节点并选择谁来接收，即可约束这类活必须经过哪些席位。";
  }
  const chain = named.map((node) => nodeDisplayTitle(node, vocabulary)).join(" → ");
  const parts = [`本场景要求：${chain} 必须串上`];
  for (const node of named) {
    const label = nodeDisplayTitle(node, vocabulary);
    if (node.verify === "independent" && node.independentRoleKey) {
      const other =
        vocabulary.find((entry) => entry.role_key === node.independentRoleKey)?.title ||
        node.independentRoleKey;
      parts.push(`${label}须由「${other}」独立验证，与「${vocabulary.find((entry) => entry.role_key === node.roleKey)?.title || node.roleKey}」不可同一人`);
    }
    if (node.verify === "human") {
      parts.push(`${label}必须人类确认`);
    }
  }
  parts.push("每步具体做法由执行模型决定，但不能跳过这些门");
  return parts.join("。") + "。";
}

function lastExitIndex(nodes: ComposerNode[]): number {
  let last = -1;
  nodes.forEach((node, index) => {
    if (node.exit) last = index;
  });
  return last;
}

function produceOf(node: ComposerNode, originalByStep: Map<string, SpecStep>): string {
  return (
    (originalByStep.get(node.step)?.produces_defaults ?? []).map((item) => item.name).find(Boolean) ||
    node.produceName ||
    `${node.step}_outcome`
  );
}

function nearestExitProduce(
  nodes: ComposerNode[],
  fromIndex: number,
  originalByStep: Map<string, SpecStep>,
): string | null {
  for (let index = fromIndex; index < nodes.length; index += 1) {
    const node = nodes[index];
    if (node?.exit) {
      return produceOf(node, originalByStep);
    }
  }
  return null;
}

export function validateComposerDraft(draft: ComposerDraft): ComposerIssue[] {
  const issues: ComposerIssue[] = [];
  if (draft.nodes.length === 0) {
    issues.push({ message: "至少需要一个关键节点" });
    return issues;
  }
  const steps = new Set<string>();
  for (const node of draft.nodes) {
    if (!node.roleKey.trim()) {
      issues.push({ message: `节点「${node.title.trim() || node.step}」还没有选择谁来接收` });
    }
    if (!node.step.trim()) {
      issues.push({ message: "节点缺少内部步骤标识" });
    } else if (steps.has(node.step)) {
      issues.push({ message: `步骤标识重复：${node.step}` });
    }
    steps.add(node.step);
    if (node.verify === "independent") {
      if (!node.independentRoleKey.trim()) {
        issues.push({ message: `节点「${node.title.trim() || node.step}」选择了独立验证，但未指定验证角色` });
      } else if (node.independentRoleKey === node.roleKey) {
        issues.push({ message: `节点「${node.title.trim() || node.step}」的验证角色不能与接收角色相同` });
      }
    }
  }
  if (!draft.nodes.some((node) => node.exit)) {
    issues.push({ message: "至少需要一个可作出口的站，规划才能选择收口档位" });
  }
  const lastExit = lastExitIndex(draft.nodes);
  draft.nodes.forEach((node, index) => {
    if (node.required && lastExit >= 0 && index > lastExit) {
      issues.push({
        message: `站「${node.title.trim() || node.step}」在所有出口之后，任何深度都不会执行`,
      });
    }
  });
  return issues;
}

export function specFromComposer(
  draft: ComposerDraft,
  vocabulary: RoleVocabularyEntry[],
  original: Record<string, unknown> = {},
): Record<string, unknown> {
  const titleOf = (key: string) =>
    vocabulary.find((entry) => entry.role_key === key)?.title || key;

  const nodes = draft.nodes.filter((node) => node.roleKey.trim());
  const rolesByKey = new Map<string, SpecRole>();
  for (const node of nodes) {
    if (!rolesByKey.has(node.roleKey)) {
      rolesByKey.set(node.roleKey, {
        key: node.roleKey,
        title: titleOf(node.roleKey),
        required_capabilities: [],
      });
    }
    if (node.verify === "independent" && node.independentRoleKey) {
      if (!rolesByKey.has(node.independentRoleKey)) {
        rolesByKey.set(node.independentRoleKey, {
          key: node.independentRoleKey,
          title: titleOf(node.independentRoleKey),
          required_capabilities: [],
        });
      }
    }
  }

  const originalRoles = asObjectArray(original.roles) as SpecRole[];
  for (const role of originalRoles) {
    const key = String(role.key ?? "").trim();
    if (!key || !rolesByKey.has(key)) continue;
    const current = rolesByKey.get(key)!;
    if (Array.isArray(role.required_capabilities) && role.required_capabilities.length) {
      current.required_capabilities = role.required_capabilities;
    }
    if (role.title && !vocabulary.some((entry) => entry.role_key === key)) {
      current.title = role.title;
    }
  }

  const originalSteps = asObjectArray(original.skeleton) as SpecStep[];
  const originalByStep = new Map(
    originalSteps
      .filter((step) => typeof step.step === "string" && step.step.trim())
      .map((step) => [String(step.step), step]),
  );

  const skeleton: SpecStep[] = nodes.map((node, index) => {
    const prev = originalByStep.get(node.step);
    const produces =
      prev?.produces_defaults && prev.produces_defaults.length > 0
        ? prev.produces_defaults
        : [{ name: node.produceName || `${node.step}_outcome`, kind: "conclusion" }];
    const step: SpecStep = {
      step: node.step,
      title: node.title.trim() || undefined,
      role: node.roleKey,
      produces_defaults: produces,
    };
    if (index > 0) {
      step.depends_on = [nodes[index - 1]!.step];
    }
    if (prev?.required_inputs_defaults && prev.required_inputs_defaults.length > 0) {
      step.required_inputs_defaults = prev.required_inputs_defaults;
    }
    return step;
  });

  const exits: SpecExit[] = nodes
    .filter((node) => node.exit)
    .map((node) => ({
      deliverable: produceOf(node, originalByStep),
      label: exitDisplayLabel(node, vocabulary),
    }));

  const constraints: SpecConstraint[] = [];
  nodes.forEach((node, index) => {
    const whenExit = nearestExitProduce(nodes, index, originalByStep);
    if (!whenExit) return;
    const when = { exit_at_or_beyond: whenExit };
    if (node.required) {
      constraints.push({ kind: "stage_required", step: node.step, when });
    }
    if (node.verify === "independent" && node.independentRoleKey) {
      constraints.push({
        kind: "role_independence",
        roles: [node.roleKey, node.independentRoleKey],
        when,
      });
    }
    if (node.verify === "human") {
      constraints.push({ kind: "human_gate", target: node.step, when });
    }
  });

  const criteria = nodes.map((node, index) => ({
    statement: `${nodeDisplayTitle(node, vocabulary)}按节点约束完成并留痕`,
    applies_from_exit: nearestExitProduce(nodes, index, originalByStep) || produceOf(node, originalByStep),
  }));

  const preserved: Record<string, unknown> = {};
  for (const key of [
    "collapse_rules",
    "autonomy_default",
    "autonomy_ceiling",
    "budget_profile",
    "feasibility_thresholds",
  ]) {
    if (key in original) {
      preserved[key] = original[key];
    }
  }

  return {
    ...preserved,
    spec_version: 2,
    roles: [...rolesByKey.values()],
    skeleton,
    exits,
    constraints,
    collapse_rules: Array.isArray(original.collapse_rules) ? original.collapse_rules : [],
    default_acceptance_criteria:
      Array.isArray(original.default_acceptance_criteria) && original.default_acceptance_criteria.length > 0
        ? original.default_acceptance_criteria
        : criteria,
  };
}

export function composerFromSpec(spec: Record<string, unknown>): ComposerDraft {
  const steps = asObjectArray(spec.skeleton) as SpecStep[];
  const exits = asObjectArray(spec.exits) as SpecExit[];
  const constraints = asObjectArray(spec.constraints) as SpecConstraint[];
  if (steps.length === 0) {
    return emptyComposerDraft();
  }

  const nodes: ComposerNode[] = steps.map((step, index) => {
    const stepKey = String(step.step ?? `step_${index + 1}`);
    const produceName = step.produces_defaults?.[0]?.name || `${stepKey}_outcome`;
    const exit = exits.find((item) => item.deliverable === produceName);
    const required = constraints.some(
      (constraint) => constraint.kind === "stage_required" && constraint.step === stepKey,
    );
    const human = constraints.some(
      (constraint) => constraint.kind === "human_gate" && constraint.target === stepKey,
    );
    const independence = constraints.find(
      (constraint) =>
        constraint.kind === "role_independence" &&
        Array.isArray(constraint.roles) &&
        constraint.roles.includes(String(step.role ?? "")),
    );
    const otherRole = independence?.roles?.find((role) => role !== step.role) ?? "";
    const title = String(step.title ?? "").trim();
    return {
      id: `parsed_${index}_${stepKey}`,
      step: stepKey,
      title: title || "",
      roleKey: String(step.role ?? ""),
      dependsOn: Array.isArray(step.depends_on) ? step.depends_on.map(String) : [],
      required: required || constraints.length === 0,
      verify: human ? "human" : otherRole ? "independent" : "none",
      independentRoleKey: otherRole,
      produceName,
      exit: Boolean(exit),
      exitLabel: exit && exit.label !== title ? String(exit.label ?? "") : "",
    };
  });

  return { nodes };
}

export function isSerialSkeleton(spec: Record<string, unknown>): boolean {
  const steps = asObjectArray(spec.skeleton) as SpecStep[];
  if (steps.length === 0) return true;
  for (let index = 0; index < steps.length; index += 1) {
    const step = steps[index];
    const deps = Array.isArray(step?.depends_on) ? step.depends_on.map(String).filter(Boolean) : [];
    if (index === 0) {
      if (deps.length > 0) return false;
      continue;
    }
    const previous = String(steps[index - 1]?.step ?? "");
    if (deps.length !== 1 || deps[0] !== previous) return false;
  }
  return true;
}

export function exitPreviewRows(
  draft: ComposerDraft,
  vocabulary: RoleVocabularyEntry[],
): Array<{
  index: number;
  label: string;
  station: string;
  path: string;
  humanGates: number;
}> {
  return draft.nodes
    .map((node, index) => ({ node, index }))
    .filter((item) => item.node.exit)
    .map((item, exitIndex) => {
      const passed = draft.nodes.slice(0, item.index + 1);
      return {
        index: exitIndex + 1,
        label: exitDisplayLabel(item.node, vocabulary),
        station: nodeDisplayTitle(item.node, vocabulary),
        path: passed.map((node) => nodeDisplayTitle(node, vocabulary)).join(" → "),
        humanGates: passed.filter((node) => node.verify === "human").length,
      };
    });
}

function asObjectArray(value: unknown): Record<string, unknown>[] {
  if (!Array.isArray(value)) return [];
  return value.filter(
    (item): item is Record<string, unknown> => typeof item === "object" && item !== null,
  );
}

import { describe, expect, it } from "vitest";
import type { RoleVocabularyEntry } from "@/lib/api/casting";
import {
  composerFromSpec,
  composerSummary,
  emptyComposerDraft,
  isSerialSkeleton,
  isValidTemplateKey,
  specFromComposer,
  suggestTemplateKey,
  validateComposerDraft,
  type ComposerDraft,
  type ComposerNode,
} from "./spec-composer";

const vocabulary: RoleVocabularyEntry[] = [
  {
    id: "1",
    tenant_id: "t",
    role_key: "developer",
    title: "开发",
    description: "",
    status: "active",
    created_at: "",
    updated_at: "",
  },
  {
    id: "2",
    tenant_id: "t",
    role_key: "reviewer",
    title: "审查",
    description: "",
    status: "active",
    created_at: "",
    updated_at: "",
  },
  {
    id: "3",
    tenant_id: "t",
    role_key: "tester",
    title: "测试",
    description: "",
    status: "active",
    created_at: "",
    updated_at: "",
  },
];

function node(partial: Partial<ComposerNode> & Pick<ComposerNode, "id" | "step" | "roleKey">): ComposerNode {
  return {
    title: "",
    dependsOn: [],
    required: true,
    verify: "none",
    independentRoleKey: "",
    produceName: `${partial.step}_outcome`,
    exit: true,
    exitLabel: "",
    ...partial,
  };
}

function deliveryDraft(): ComposerDraft {
  return {
    nodes: [
      node({
        id: "a",
        step: "develop",
        title: "开发",
        roleKey: "developer",
        produceName: "head_commit",
        exitLabel: "交付分支",
      }),
      node({
        id: "b",
        step: "review",
        title: "审查",
        roleKey: "reviewer",
        dependsOn: ["develop"],
        verify: "independent",
        independentRoleKey: "developer",
        produceName: "review_verdict",
        exitLabel: "审查通过",
      }),
      node({
        id: "c",
        step: "release",
        title: "发布",
        roleKey: "developer",
        dependsOn: ["review"],
        verify: "human",
        produceName: "release_record",
        exitLabel: "发布上线",
      }),
    ],
  };
}

describe("spec-composer", () => {
  it("suggests an ascii template key from the display name", () => {
    expect(suggestTemplateKey("Ops Review")).toBe("ops_review");
    expect(suggestTemplateKey("故障分析")).toBe("");
    expect(isValidTemplateKey("ops_review")).toBe(true);
    expect(isValidTemplateKey("运维评审")).toBe(false);
    expect(isValidTemplateKey("1ops")).toBe(false);
  });

  it("rejects nodes without a receiving role", () => {
    const issues = validateComposerDraft(emptyComposerDraft());
    expect(issues.some((issue) => issue.message.includes("谁来接收"))).toBe(true);
  });

  it("rejects drafts with no exits", () => {
    const draft = deliveryDraft();
    draft.nodes.forEach((item) => {
      item.exit = false;
    });
    const issues = validateComposerDraft(draft);
    expect(issues.some((issue) => issue.message.includes("可作出口"))).toBe(true);
  });

  it("rejects required stations after the last exit", () => {
    const draft = deliveryDraft();
    draft.nodes[0]!.exit = true;
    draft.nodes[1]!.exit = false;
    draft.nodes[2]!.exit = false;
    draft.nodes[2]!.required = true;
    const issues = validateComposerDraft(draft);
    expect(issues.some((issue) => issue.message.includes("所有出口之后"))).toBe(true);
  });

  it("builds a v2 spec with chain, independence, and human gate", () => {
    const spec = specFromComposer(deliveryDraft(), vocabulary);
    expect(spec.spec_version).toBe(2);
    expect(spec.roles).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ key: "developer", title: "开发" }),
        expect.objectContaining({ key: "reviewer", title: "审查" }),
      ]),
    );
    expect(spec.skeleton).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ step: "develop", role: "developer" }),
        expect.objectContaining({ step: "review", role: "reviewer", depends_on: ["develop"] }),
      ]),
    );
    expect(spec.constraints).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ kind: "stage_required", step: "review" }),
        expect.objectContaining({
          kind: "role_independence",
          roles: expect.arrayContaining(["developer", "reviewer"]),
        }),
        expect.objectContaining({ kind: "human_gate", target: "release" }),
      ]),
    );
  });

  it("only emits exits for checked stations and uses custom labels", () => {
    const draft = deliveryDraft();
    draft.nodes[1]!.exit = false;
    const spec = specFromComposer(draft, vocabulary);
    expect(spec.exits).toEqual([
      { deliverable: "head_commit", label: "交付分支" },
      { deliverable: "release_record", label: "发布上线" },
    ]);
    const constraints = spec.constraints as { when?: { exit_at_or_beyond?: string }; step?: string }[];
    expect(constraints.find((item) => item.step === "review")?.when?.exit_at_or_beyond).toBe(
      "release_record",
    );
  });

  it("rewrites depends_on into a serial chain", () => {
    const draft = deliveryDraft();
    draft.nodes[2]!.dependsOn = ["develop"];
    const spec = specFromComposer(draft, vocabulary);
    const skeleton = spec.skeleton as { step: string; depends_on?: string[] }[];
    expect(skeleton.find((step) => step.step === "release")?.depends_on).toEqual(["review"]);
  });

  it("detects parallel software-delivery skeletons as non-serial", () => {
    expect(
      isSerialSkeleton({
        skeleton: [
          { step: "develop" },
          { step: "review", depends_on: ["develop"] },
          { step: "test", depends_on: ["develop"] },
          { step: "release", depends_on: ["review", "test"] },
        ],
      }),
    ).toBe(false);
    expect(
      isSerialSkeleton({
        skeleton: [
          { step: "collect" },
          { step: "analyze", depends_on: ["collect"] },
        ],
      }),
    ).toBe(true);
  });

  it("preserves original produces_defaults on matching steps", () => {
    const original = {
      skeleton: [
        {
          step: "develop",
          role: "developer",
          produces_defaults: [
            { name: "branch_ref", kind: "branch_ref" },
            { name: "head_commit", kind: "git_commit" },
          ],
        },
      ],
    };
    const spec = specFromComposer(deliveryDraft(), vocabulary, original);
    const develop = (spec.skeleton as { step: string; produces_defaults: unknown[] }[]).find(
      (step) => step.step === "develop",
    );
    expect(develop?.produces_defaults).toEqual([
      { name: "branch_ref", kind: "branch_ref" },
      { name: "head_commit", kind: "git_commit" },
    ]);
    expect(spec.exits).toEqual(
      expect.arrayContaining([expect.objectContaining({ deliverable: "branch_ref" })]),
    );
  });

  it("round-trips a parsed skeleton back to the same roles and gates", () => {
    const spec = specFromComposer(deliveryDraft(), vocabulary);
    const parsed = composerFromSpec(spec);
    expect(parsed.nodes.map((item) => item.roleKey)).toEqual(["developer", "reviewer", "developer"]);
    expect(parsed.nodes[1]?.verify).toBe("independent");
    expect(parsed.nodes[2]?.verify).toBe("human");
    expect(parsed.nodes.every((item) => item.exit)).toBe(true);
    const again = specFromComposer(parsed, vocabulary, spec);
    expect(again.roles).toEqual(spec.roles);
    expect(again.exits).toEqual(spec.exits);
  });

  it("summarizes the playbook in human language", () => {
    const text = composerSummary(deliveryDraft(), vocabulary);
    expect(text).toContain("开发 → 审查 → 发布");
    expect(text).toContain("独立验证");
    expect(text).toContain("人类确认");
  });
});

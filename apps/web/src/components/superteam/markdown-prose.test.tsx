import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import { MarkdownProse } from "./markdown-prose";
import { splitMarkdownBlocks } from "./split-markdown-blocks";

describe("splitMarkdownBlocks", () => {
  it("keeps an unclosed fence as the last unfrozen block", () => {
    const blocks = splitMarkdownBlocks("hello\n\n```ts\nconst x = 1");
    expect(blocks.at(-1)?.frozen).toBe(false);
    expect(blocks.at(-1)?.text).toContain("```ts");
  });
});

describe("MarkdownProse", () => {
  it("renders gfm tables, emphasis, and fenced code separately from inline code", async () => {
    const screen = await render(
      <MarkdownProse>{[
        "## 标题",
        "",
        "结论是 **必须加粗**，模型是 `claude`。",
        "",
        "| a | b |",
        "| --- | --- |",
        "| 1 | 2 |",
        "",
        "```ts",
        "const ok = true;",
        "```",
      ].join("\n")}</MarkdownProse>,
    );
    await expect.element(screen.getByRole("heading", { name: "标题" })).toBeVisible();
    await expect.element(screen.getByText("必须加粗")).toBeVisible();
    expect(document.querySelector("table")).toBeTruthy();
    expect(document.querySelector("pre code")).toBeTruthy();
    await expect.element(screen.getByRole("button", { name: "复制" })).toBeVisible();
    expect(document.querySelector("pre")?.className).toContain("whitespace-pre-wrap");
    expect(document.querySelector("pre")?.className).toContain("break-all");
  });

  it("wraps long fenced json instead of requiring horizontal scroll", async () => {
    const longLine = `"summary": "${"验收结论".repeat(40)}"`;
    const screen = await render(
      <MarkdownProse>{["```json", `{${longLine}}`, "```"].join("\n")}</MarkdownProse>,
    );
    await expect.element(screen.getByText(/验收结论/)).toBeVisible();
    const pre = document.querySelector("pre");
    expect(pre?.className).toMatch(/whitespace-pre-wrap/);
    expect(pre?.className).not.toMatch(/overflow-x-auto/);
  });
});

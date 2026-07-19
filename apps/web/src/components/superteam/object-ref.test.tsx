import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { ObjectRef } from "./object-ref";

const UUID = "4be304f7-c073-4282-8c9d-d7805eae9746";

describe("ObjectRef", () => {
  it("shows name as primary text with a shortened copyable id chip", async () => {
    const screen = await render(<ObjectRef name="目录投影残债E2E锚点" id={UUID} />);

    await expect.element(screen.getByText("目录投影残债E2E锚点")).toBeInTheDocument();
    await expect.element(screen.getByText("4be304f7…")).toBeInTheDocument();
    await expect
      .element(screen.getByRole("button", { name: `复制标识符 ${UUID}` }))
      .toBeInTheDocument();
  });

  it("falls back to the full id when name is missing", async () => {
    const screen = await render(<ObjectRef id={UUID} />);

    await expect.element(screen.getByText(UUID)).toBeInTheDocument();
  });

  it("renders plain name without a chip when id is missing", async () => {
    const screen = await render(<ObjectRef name="仅有名称" />);

    await expect.element(screen.getByText("仅有名称")).toBeInTheDocument();
    expect(screen.container.querySelector("button")).toBeNull();
  });

  it("copies the full id on chip click", async () => {
    const writeText = vi.spyOn(navigator.clipboard, "writeText").mockResolvedValue();
    const screen = await render(<ObjectRef name="目录投影残债E2E锚点" id={UUID} />);

    await userEvent.click(screen.getByRole("button", { name: `复制标识符 ${UUID}` }));

    expect(writeText).toHaveBeenCalledWith(UUID);
    writeText.mockRestore();
  });
});

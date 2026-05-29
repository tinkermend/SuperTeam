import { describe, expect, it, vi } from "vitest";

vi.mock("next/font/google", () => ({
  Geist: () => ({ variable: "__font_sans" }),
}));

describe("RootLayout", () => {
  it("suppresses root hydration warnings caused by browser-added html attributes", async () => {
    const { default: RootLayout } = await import("./layout");
    const layout = RootLayout({ children: <main>content</main> });

    expect(layout.props.suppressHydrationWarning).toBe(true);
  });
});

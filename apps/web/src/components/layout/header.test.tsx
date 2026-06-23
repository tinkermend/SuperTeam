import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import { SidebarProvider } from "@/components/ui/sidebar";
import { Header } from "./header";
import "@/styles/index.css";

describe("Header", () => {
  it("uses the v3 shell header surface and trigger button", async () => {
    const screen = await render(
      <SidebarProvider>
        <Header>
          <button type="button">Search</button>
        </Header>
      </SidebarProvider>,
    );

    const header = screen.getByRole("banner").element() as HTMLElement;
    const trigger = document.querySelector('[data-slot="sidebar-trigger"]');

    expect(header.dataset.slot).toBe("v3-shell-header");
    expect(header.className).toContain("bg-v3-card");
    expect(trigger).toBeInstanceOf(HTMLElement);
    expect((trigger as HTMLElement).className).toContain("bg-v3-card-soft");
    expect((trigger as HTMLElement).className).toContain("text-v3-ink-2");
  });
});

import { describe, expect, it, vi } from "vitest";
import { applyPromptTemplate, listPromptTemplates } from "./prompt-templates";

const templates = [
  {
    id: "1",
    tenant_id: "t1",
    title: "Test",
    content: "Content",
    category_code: "CAT",
    scope: "GLOBAL" as const,
    creator_id: "c1",
    use_count: 0,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
  },
];

describe("prompt-templates API", () => {
  it("should list prompt templates", async () => {
    const fetcher = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => templates,
    } as unknown as Response);

    const result = await listPromptTemplates({ baseUrl: "http://api.example.com", fetcher });

    expect(fetcher).toHaveBeenCalledWith(
      "http://api.example.com/api/v1/templates",
      expect.objectContaining({ method: "GET" }),
    );
    expect(result).toEqual(templates);
  });

  it("should apply prompt template", async () => {
    const fetcher = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({}),
    } as unknown as Response);

    await applyPromptTemplate({ baseUrl: "http://api.example.com", fetcher }, "123");

    expect(fetcher).toHaveBeenCalledWith(
      "http://api.example.com/api/v1/templates/123/apply",
      expect.objectContaining({ method: "POST" }),
    );
  });
});

import { describe, expect, it, vi } from "vitest";
import { createTask, listTasks } from "./tasks";

describe("listTasks", () => {
  it("calls the tasks endpoint and parses JSON", async () => {
    const tasks = [
      {
        id: "task-1",
        title: "Analyze requirements",
        status: "pending",
        provider_type: "codex",
      },
    ];
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(tasks), {
        status: 200,
        headers: {
          "content-type": "application/json",
        },
      }),
    );

    await expect(
      listTasks({
        baseUrl: "http://control-plane.local/",
        fetcher,
      }),
    ).resolves.toEqual(tasks);

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/v1/tasks", {
      headers: {
        accept: "application/json",
      },
      method: "GET",
    });
  });
});

describe("createTask", () => {
  it("posts JSON to the tasks endpoint and parses JSON", async () => {
    const input = {
      title: "Implement foundation boundary",
      description: "Prepare real data access",
      provider_type: "codex",
      risk_level: "medium",
    };
    const createdTask = {
      id: "task-2",
      status: "pending",
      ...input,
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(createdTask), {
        status: 201,
        headers: {
          "content-type": "application/json",
        },
      }),
    );

    await expect(
      createTask(input, {
        baseUrl: "http://control-plane.local/",
        fetcher,
      }),
    ).resolves.toEqual(createdTask);

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/v1/tasks", {
      body: JSON.stringify(input),
      headers: {
        accept: "application/json",
        "content-type": "application/json",
      },
      method: "POST",
    });
  });
});

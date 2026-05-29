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

  it("throws when the tasks endpoint returns a non-ok response", async () => {
    const fetcher = vi.fn(async () => new Response("", { status: 500 }));

    await expect(
      listTasks({
        baseUrl: "http://control-plane.local",
        fetcher,
      }),
    ).rejects.toThrow("tasks request failed with status 500");

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
      params: {
        issue: "foundation-hardening",
      },
      priority: 3,
      target_node_id: "node-1",
      workspace_path: "/workspace/superteam",
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
      createTask(
        {
          baseUrl: "http://control-plane.local/",
          fetcher,
        },
        input,
      ),
    ).resolves.toEqual(createdTask);

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/v1/tasks", {
      body: JSON.stringify(input),
      headers: {
        accept: "application/json",
        "content-type": "application/json",
      },
      method: "POST",
    });
    expect(JSON.parse(fetcher.mock.calls[0]?.[1]?.body as string)).toEqual({
      title: "Implement foundation boundary",
      description: "Prepare real data access",
      provider_type: "codex",
      params: {
        issue: "foundation-hardening",
      },
      priority: 3,
      target_node_id: "node-1",
      workspace_path: "/workspace/superteam",
    });
  });

  it("throws when the create task endpoint returns a non-ok response", async () => {
    const fetcher = vi.fn(async () => new Response("invalid", { status: 400 }));

    await expect(
      createTask(
        {
          baseUrl: "http://control-plane.local",
          fetcher,
        },
        {
          title: "Invalid task",
          provider_type: "codex",
          params: {},
        },
      ),
    ).rejects.toThrow("tasks request failed with status 400");

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/v1/tasks", {
      body: JSON.stringify({
        title: "Invalid task",
        provider_type: "codex",
        params: {},
      }),
      headers: {
        accept: "application/json",
        "content-type": "application/json",
      },
      method: "POST",
    });
  });
});

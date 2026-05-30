import { describe, expect, it, vi } from "vitest";
import type { TaskResponse, TaskStatus } from "./tasks";
import { cancelTask, createTask, getTask, listTasks, updateTaskStatus } from "./tasks";

describe("listTasks", () => {
  it("calls the tasks endpoint and parses JSON", async () => {
    const status: TaskStatus = "pending";
    const tasks: TaskResponse[] = [
      {
        id: 1,
        title: "Analyze requirements",
        status,
        provider_type: "codex",
        priority: 2,
        description: "Clarify initial scope",
        target_node_id: "node-1",
        assigned_node_id: "node-1",
        workspace_path: "/workspace/superteam",
        params: {
          source: "console",
        },
        created_at: "2026-05-29T00:00:00Z",
        updated_at: "2026-05-29T00:01:00Z",
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

describe("getTask", () => {
  it("calls the task detail endpoint and parses JSON", async () => {
    const task: TaskResponse = {
      id: 42,
      title: "Inspect foundation task",
      provider_type: "codex",
      status: "running",
      priority: 1,
      params: {},
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(task), {
        status: 200,
        headers: {
          "content-type": "application/json",
        },
      }),
    );

    await expect(
      getTask({
        baseUrl: "http://control-plane.local/",
        fetcher,
        taskId: 42,
      }),
    ).resolves.toEqual(task);

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/v1/tasks/42", {
      headers: {
        accept: "application/json",
      },
      method: "GET",
    });
  });
});

describe("updateTaskStatus", () => {
  it("puts JSON to the task status endpoint and parses JSON", async () => {
    const task: TaskResponse = {
      id: 42,
      title: "Inspect foundation task",
      provider_type: "codex",
      status: "completed",
      priority: 1,
      params: {},
    };
    const input = {
      status: "completed" as const,
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(task), {
        status: 200,
        headers: {
          "content-type": "application/json",
        },
      }),
    );

    await expect(
      updateTaskStatus(
        {
          baseUrl: "http://control-plane.local/",
          fetcher,
          taskId: 42,
        },
        input,
      ),
    ).resolves.toEqual(task);

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/v1/tasks/42/status", {
      body: JSON.stringify(input),
      headers: {
        accept: "application/json",
        "content-type": "application/json",
      },
      method: "PUT",
    });
  });
});

describe("cancelTask", () => {
  it("posts to the task cancel endpoint and parses JSON", async () => {
    const task: TaskResponse = {
      id: 42,
      title: "Inspect foundation task",
      provider_type: "codex",
      status: "cancelled",
      priority: 1,
      params: {},
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(task), {
        status: 200,
        headers: {
          "content-type": "application/json",
        },
      }),
    );

    await expect(
      cancelTask({
        baseUrl: "http://control-plane.local/",
        fetcher,
        taskId: 42,
      }),
    ).resolves.toEqual(task);

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/v1/tasks/42/cancel", {
      headers: {
        accept: "application/json",
      },
      method: "POST",
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
    const status: TaskStatus = "pending";
    const createdTask: TaskResponse = {
      id: 2,
      status,
      assigned_node_id: "node-1",
      created_at: "2026-05-29T00:02:00Z",
      updated_at: "2026-05-29T00:02:00Z",
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
    expect(fetcher).toHaveBeenCalledTimes(1);
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

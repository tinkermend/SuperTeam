import { describe, expect, it, vi } from "vitest";
import type { TaskResponse, TaskStatus } from "./tasks";
import { cancelTask, createTask, getTask, listTasks, updateTaskStatus } from "./tasks";

const taskId = "11111111-1111-4111-8111-111111111111";
const tenantId = "00000000-0000-4000-8000-000000000001";
const teamId = "00000000-0000-4000-8000-000000000101";
const creatorId = "22222222-2222-4222-8222-222222222222";

describe("listTasks", () => {
  it("calls the tasks endpoint and parses JSON", async () => {
    const status: TaskStatus = "pending";
    const tasks: TaskResponse[] = [
      {
        id: taskId,
        tenant_id: tenantId,
        team_id: teamId,
        title: "Analyze requirements",
        status,
        provider_type: "codex",
        priority: 2,
        description: "Clarify initial scope",
        creator_id: creatorId,
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
      credentials: "include",
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
      credentials: "include",
      headers: {
        accept: "application/json",
      },
      method: "GET",
    });
  });

  it("adds pagination query parameters in a deterministic order", async () => {
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify([]), {
        status: 200,
        headers: {
          "content-type": "application/json",
        },
      }),
    );

    await expect(
      listTasks({
        baseUrl: "http://control-plane.local/root/",
        fetcher,
        limit: 50,
        offset: 100,
      }),
    ).resolves.toEqual([]);

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/root/api/v1/tasks?limit=50&offset=100", {
      credentials: "include",
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
      id: taskId,
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
      getTask(
        {
          baseUrl: "http://control-plane.local/",
          fetcher,
        },
        taskId,
      ),
    ).resolves.toEqual(task);

    expect(fetcher).toHaveBeenCalledWith(`http://control-plane.local/api/v1/tasks/${taskId}`, {
      credentials: "include",
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
      id: taskId,
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
        },
        taskId,
        input,
      ),
    ).resolves.toEqual(task);

    expect(fetcher).toHaveBeenCalledWith(`http://control-plane.local/api/v1/tasks/${taskId}/status`, {
      body: JSON.stringify(input),
      credentials: "include",
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
      id: taskId,
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
      cancelTask(
        {
          baseUrl: "http://control-plane.local/",
          fetcher,
        },
        taskId,
      ),
    ).resolves.toEqual(task);

    expect(fetcher).toHaveBeenCalledWith(`http://control-plane.local/api/v1/tasks/${taskId}/cancel`, {
      credentials: "include",
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
      id: taskId,
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
      credentials: "include",
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
      credentials: "include",
      headers: {
        accept: "application/json",
        "content-type": "application/json",
      },
      method: "POST",
    });
  });
});

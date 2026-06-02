import { describe, expect, it, vi } from "vitest";
import { createDigitalEmployee, getDigitalEmployeeExecutionInstance, listDigitalEmployees } from "./employees";

describe("digital employee API", () => {
  it("lists digital employees with cookie credentials", async () => {
    const employees = [
      {
        id: "11111111-1111-4111-8111-111111111111",
        name: "需求分析员工",
        role: "requirements_analyst",
        status: "draft",
      },
    ];
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(employees), {
        headers: { "content-type": "application/json" },
        status: 200,
      }),
    );

    await expect(
      listDigitalEmployees({
        baseUrl: "http://control-plane.local",
        fetcher,
      }),
    ).resolves.toEqual(employees);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/digital-employees",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });

  it("creates digital employee with cookie credentials", async () => {
    const employee = {
      id: "11111111-1111-4111-8111-111111111111",
      name: "需求分析员工",
      role: "requirements_analyst",
      status: "draft",
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(employee), {
        headers: { "content-type": "application/json" },
        status: 201,
      }),
    );

    await expect(
      createDigitalEmployee(
        {
          baseUrl: "http://control-plane.local",
          fetcher,
        },
        {
          name: "需求分析员工",
          role: "requirements_analyst",
        },
      ),
    ).resolves.toEqual(employee);

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/v1/digital-employees", {
      body: JSON.stringify({
        name: "需求分析员工",
        role: "requirements_analyst",
      }),
      credentials: "include",
      headers: { accept: "application/json", "content-type": "application/json" },
      method: "POST",
    });
  });

  it("gets execution instance with encoded employee id and cookie credentials", async () => {
    const instance = {
      id: "22222222-2222-4222-8222-222222222222",
      digital_employee_id: "11111111-1111-4111-8111-111111111111",
      runtime_node_id: "33333333-3333-4333-8333-333333333333",
      provider_type: "codex",
      status: "ready",
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(instance), {
        headers: { "content-type": "application/json" },
        status: 200,
      }),
    );

    await expect(
      getDigitalEmployeeExecutionInstance(
        {
          baseUrl: "http://control-plane.local",
          fetcher,
        },
        "employee 1/primary",
      ),
    ).resolves.toEqual(instance);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/digital-employees/employee%201%2Fprimary/execution-instance",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });
});

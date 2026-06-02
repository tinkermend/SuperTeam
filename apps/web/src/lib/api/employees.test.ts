import { describe, expect, it, vi } from "vitest";
import { createDigitalEmployee, listDigitalEmployees } from "./employees";

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
});

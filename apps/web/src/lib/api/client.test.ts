import { describe, expect, it } from "vitest";
import { ApiRequestError, parseJson } from "./client";

describe("ApiRequestError", () => {
  it("keeps parsed JSON error payload", async () => {
    const payload = {
      code: "digital_employee_delete_blocked",
      message: "该数字员工仍有排队或执行中的工作，停止或完成后再删除。",
      blockers: [{ type: "run", id: "run-1", status: "running", title: "运行中" }],
    };

    await expect(
      parseJson(
        new Response(JSON.stringify(payload), {
          status: 409,
          headers: { "content-type": "application/json" },
        }),
        "delete digital employee",
      ),
    ).rejects.toMatchObject({
      status: 409,
      code: "digital_employee_delete_blocked",
      detail: payload.message,
      payload,
    } satisfies Partial<ApiRequestError>);
  });
});

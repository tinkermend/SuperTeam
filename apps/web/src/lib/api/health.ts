import type { ApiClientOptions } from "./client";
import { parseJson } from "./client";

export type HealthResponse = {
  status: "ok";
  service: "control-plane";
};

export async function getHealth(options: ApiClientOptions): Promise<HealthResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(new URL("/health", options.baseUrl).toString(), {
    headers: {
      accept: "application/json",
    },
  });

  return parseJson<HealthResponse>(response, "health");
}

import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";

export type HealthResponse = {
  status: "ok";
  service: "control-plane";
};

export async function getHealth(options: ApiClientOptions): Promise<HealthResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/health"), {
    headers: {
      accept: "application/json",
    },
  });

  return parseJson<HealthResponse>(response, "health");
}

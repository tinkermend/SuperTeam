export type HealthResponse = {
  status: "ok";
  service: "control-plane";
};

export type ApiClientOptions = {
  baseUrl: string;
  fetcher?: typeof fetch;
};

export async function getHealth(options: ApiClientOptions): Promise<HealthResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(new URL("/health", options.baseUrl).toString(), {
    headers: {
      accept: "application/json",
    },
  });

  if (!response.ok) {
    throw new Error(`health request failed with status ${response.status}`);
  }

  return (await response.json()) as HealthResponse;
}


export type ApiClientOptions = {
  baseUrl: string;
  fetcher?: typeof fetch;
};

export class ApiRequestError extends Error {
  readonly status: number;

  constructor(resource: string, status: number) {
    super(`${resource} request failed with status ${status}`);
    this.name = "ApiRequestError";
    this.status = status;
  }
}

export function buildApiUrl(baseUrl: string, path: string): string {
  return `${baseUrl.replace(/\/+$/, "")}${path}`;
}

export async function parseJson<T>(response: Response, resource: string): Promise<T> {
  if (!response.ok) {
    throw new ApiRequestError(resource, response.status);
  }

  return (await response.json()) as T;
}

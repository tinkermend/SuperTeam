export type ApiClientOptions = {
  baseUrl: string;
  fetcher?: typeof fetch;
};

export class ApiRequestError extends Error {
  readonly status: number;

  constructor(resource: string, status: number, detail?: string) {
    super(`${resource} request failed with status ${status}${detail ? `: ${detail}` : ""}`);
    this.name = "ApiRequestError";
    this.status = status;
  }
}

export function buildApiUrl(baseUrl: string, path: string): string {
  return `${baseUrl.replace(/\/+$/, "")}${path}`;
}

export async function parseJson<T>(response: Response, resource: string): Promise<T> {
  if (!response.ok) {
    throw new ApiRequestError(resource, response.status, await readErrorDetail(response));
  }

  return (await response.json()) as T;
}

export async function getJson<T>(
  options: ApiClientOptions,
  path: string,
  resource: string,
): Promise<T> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    credentials: "include",
    headers: { accept: "application/json" },
    method: "GET",
  });

  return parseJson<T>(response, resource);
}

export async function postJson<T>(
  options: ApiClientOptions,
  path: string,
  input: unknown,
  resource: string,
): Promise<T> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "POST",
  });

  return parseJson<T>(response, resource);
}

export async function postJsonWithoutBody<T>(
  options: ApiClientOptions,
  path: string,
  resource: string,
): Promise<T> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    credentials: "include",
    headers: { accept: "application/json" },
    method: "POST",
  });

  return parseJson<T>(response, resource);
}

export async function putJson<T>(
  options: ApiClientOptions,
  path: string,
  input: unknown,
  resource: string,
): Promise<T> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "PUT",
  });

  return parseJson<T>(response, resource);
}

export async function patchJson<T>(
  options: ApiClientOptions,
  path: string,
  input: unknown,
  resource: string,
): Promise<T> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "PATCH",
  });

  return parseJson<T>(response, resource);
}

export async function deleteJson(
  options: ApiClientOptions,
  path: string,
  resource: string,
): Promise<void> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    credentials: "include",
    headers: { accept: "application/json" },
    method: "DELETE",
  });

  if (!response.ok) {
    await parseJson<unknown>(response, resource);
  }
}

async function readErrorDetail(response: Response): Promise<string | undefined> {
  const contentType = response.headers.get("content-type") ?? "";
  const body = await response.text();

  if (!body) {
    return undefined;
  }

  if (contentType.includes("application/json")) {
    try {
      const parsed = JSON.parse(body) as { error?: unknown; message?: unknown };
      if (typeof parsed.error === "string" && parsed.error) {
        return parsed.error;
      }
      if (typeof parsed.message === "string" && parsed.message) {
        return parsed.message;
      }
    } catch {
      return body;
    }
  }

  return body;
}

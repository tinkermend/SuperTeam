export type ApiClientOptions = {
  baseUrl: string;
  fetcher?: typeof fetch;
};

export class ApiRequestError extends Error {
  readonly status: number;
  readonly code?: string;
  readonly detail?: string;

  constructor(resource: string, status: number, detail?: string, code?: string) {
    super(`${resource} request failed with status ${status}${detail ? `: ${detail}` : ""}`);
    this.name = "ApiRequestError";
    this.status = status;
    this.code = code;
    this.detail = detail;
  }
}

export function buildApiUrl(baseUrl: string, path: string): string {
  return `${baseUrl.replace(/\/+$/, "")}${path}`;
}

export async function parseJson<T>(response: Response, resource: string): Promise<T> {
  if (!response.ok) {
    const errorDetail = await readErrorDetail(response);
    throw new ApiRequestError(resource, response.status, errorDetail.detail, errorDetail.code);
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

export async function deleteJsonWithResponse<T>(
  options: ApiClientOptions,
  path: string,
  resource: string,
): Promise<T> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    credentials: "include",
    headers: { accept: "application/json" },
    method: "DELETE",
  });

  return parseJson<T>(response, resource);
}

type ParsedErrorDetail = {
  detail?: string;
  code?: string;
};

async function readErrorDetail(response: Response): Promise<ParsedErrorDetail> {
  const contentType = response.headers.get("content-type") ?? "";
  const body = await response.text();

  if (!body) {
    return {};
  }

  if (contentType.includes("application/json")) {
    try {
      const parsed = JSON.parse(body) as { code?: unknown; error?: unknown; message?: unknown };
      const detail =
        typeof parsed.message === "string" && parsed.message
          ? parsed.message
          : typeof parsed.error === "string" && parsed.error
            ? parsed.error
            : body;
      return {
        detail,
        code: typeof parsed.code === "string" && parsed.code ? parsed.code : undefined,
      };
    } catch {
      return { detail: body };
    }
  }

  return { detail: body };
}

import { ApiRequestError } from "@/lib/api";

export function shouldRetryQuery(failureCount: number, error: unknown) {
  if (error instanceof ApiRequestError && error.status >= 400 && error.status < 500) {
    return false;
  }
  return failureCount < 2;
}

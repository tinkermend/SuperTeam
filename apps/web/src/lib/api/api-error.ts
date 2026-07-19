import { ApiRequestError } from "./client";

// apiErrorMessage 取后端权威中文提示：结构化 coded error（后端 apierror 包输出
// {code, message}）的 message 直接展示，否则回退给定 fallback。取代前端按英文
// 错误文本关键词匹配的脆弱做法——用户可读文本由后端单一源提供。
export function apiErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof ApiRequestError && error.code && error.detail) {
    return error.detail;
  }
  return fallback;
}

// apiErrorCode 取稳定的机器可读错误码，供需要分支逻辑的调用方使用（如聚焦某字段）。
export function apiErrorCode(error: unknown): string | undefined {
  return error instanceof ApiRequestError ? error.code : undefined;
}

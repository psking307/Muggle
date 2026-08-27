import axios from "axios";

// ApiProblem 把 Axios 的技术错误转换成页面可以理解的有限状态。
export type ApiProblem =
  | { kind: "not-found"; message: string }
  | { kind: "bad-request"; message: string }
  | { kind: "conflict"; message: string }
  | { kind: "server"; message: string }
  | { kind: "network"; message: string }
  | { kind: "timeout"; message: string }
  | { kind: "unknown"; message: string };

// apiErrorCode 从错误中提取后端返回的业务错误码（error.code）。
//
// 后端统一错误格式为 { error: { code, message, request_id } }。
// 页面在需要区分具体冲突（如 slug_taken 与 post_version_conflict）时，
// 用它拿到精确的错误码，而不是只依赖笼统的 HTTP 状态。
export function apiErrorCode(error: unknown): string | undefined {
  if (!axios.isAxiosError(error)) {
    return undefined;
  }
  const data = error.response?.data as { error?: { code?: string } } | undefined;
  return data?.error?.code;
}

export function describeApiProblem(error: unknown): ApiProblem {
  if (!axios.isAxiosError(error)) {
    return {
      kind: "unknown",
      message: "发生了未知错误，请稍后重试。",
    };
  }

  if (error.code === "ECONNABORTED") {
    return {
      kind: "timeout",
      message: "等待服务器响应超时，请稍后重试。",
    };
  }

  // 没有 response 表示请求甚至没有得到 HTTP 响应，通常是断网或 API 未启动。
  if (!error.response) {
    return {
      kind: "network",
      message: "无法连接 API，请确认后端服务是否正在运行。",
    };
  }

  if (error.response.status === 404) {
    return {
      kind: "not-found",
      message: "文章不存在或尚未发布。",
    };
  }

  if (error.response.status === 400) {
    return {
      kind: "bad-request",
      message: "请求参数不合法，请检查后重试。",
    };
  }

  // 409 表示版本冲突或 slug 冲突（乐观锁 / 唯一索引），提示用户刷新后重试。
  if (error.response.status === 409) {
    return {
      kind: "conflict",
      message: "文章已被其他操作修改或 slug 已占用，请刷新后重试。",
    };
  }

  if (error.response.status >= 500) {
    return {
      kind: "server",
      message: "服务器暂时无法读取文章，请稍后重试。",
    };
  }

  return {
    kind: "unknown",
    message: `API 返回了 HTTP ${error.response.status}。`,
  };
}

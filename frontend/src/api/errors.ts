import axios from "axios";

// ApiProblem 把 Axios 的技术错误转换成页面可以理解的有限状态。
export type ApiProblem =
  | { kind: "not-found"; message: string }
  | { kind: "bad-request"; message: string }
  | { kind: "server"; message: string }
  | { kind: "network"; message: string }
  | { kind: "timeout"; message: string }
  | { kind: "unknown"; message: string };

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

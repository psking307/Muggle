import axios from "axios";

// 扩展 Axios 请求配置：增加项目自定义开关 skipAuthRefresh。
// 登录、刷新、退出接口自己就是认证流程的一部分，它们的 401 不应触发
// “自动刷新会话并重试”的响应拦截器，否则可能死循环。
declare module "axios" {
  export interface AxiosRequestConfig {
    skipAuthRefresh?: boolean;
    // 响应拦截器重放请求前会打上标记，防止刷新后仍然 401 时无限循环。
    _retried?: boolean;
  }
}

export const api = axios.create({
  baseURL: "/api/v1",
  timeout: 5_000,
  headers: {
    Accept: "application/json",
  },
});

// 认证拦截器注册在 stores/authStore.ts 中（而非本文件）：
// 拦截器需要读写登录状态，而 Store 又依赖 api 发请求，
// 把两者分开可以避免“client -> store -> client”的循环 import。

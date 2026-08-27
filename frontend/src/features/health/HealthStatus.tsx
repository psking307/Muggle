import { Alert, Button, Card, Flex, Spin, Tag, Typography } from "antd";
import axios from "axios";
import { useCallback, useEffect, useState } from "react";

import { api } from "../../api/client";

interface HealthResponse {
  status: string;
}

type HealthState =
  | { kind: "loading" }
  | { kind: "healthy" }
  | { kind: "unreachable"; message: string };

function healthResponseState(response: HealthResponse): HealthState {
  if (response.status === "ok") {
    return { kind: "healthy" };
  }

  return {
    kind: "unreachable",
    message: "API 可以访问，但返回了无法识别的健康状态。",
  };
}

function describeError(error: unknown): string {
  if (!axios.isAxiosError(error)) {
    return "发生了未知错误，请稍后重试。";
  }

  if (error.code === "ECONNABORTED") {
    return "等待 API 响应超时，请确认后端是否正在运行。";
  }

  if (!error.response) {
    return "无法连接 API。请先运行 make dev-api，再点击重新检查。";
  }

  return `API 返回了 HTTP ${error.response.status}，请查看后端日志。`;
}

export function HealthStatus() {
  const [state, setState] = useState<HealthState>({ kind: "loading" });

  const checkHealth = useCallback(async (signal?: AbortSignal) => {
    try {
      const response = await api.get<HealthResponse>("/health/live", {
        signal,
      });

      setState(healthResponseState(response.data));
    } catch (error) {
      if (signal?.aborted) {
        return;
      }

      setState({
        kind: "unreachable",
        message: describeError(error),
      });
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();

    void api
      .get<HealthResponse>("/health/live", {
        signal: controller.signal,
      })
      .then((response) => {
        setState(healthResponseState(response.data));
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) {
          return;
        }

        setState({
          kind: "unreachable",
          message: describeError(error),
        });
      });

    return () => {
      controller.abort();
    };
  }, []);

  return (
    <Card className="w-full max-w-2xl shadow-xl shadow-slate-200/70">
      <Flex vertical gap={24}>
        <Flex justify="space-between" align="center" gap={16} wrap>
          <div>
            <Typography.Text type="secondary">
              GET /api/v1/health/live
            </Typography.Text>
            <Typography.Title level={2} className="!mb-0 !mt-2">
              API 连接状态
            </Typography.Title>
          </div>
          <Tag color={state.kind === "healthy" ? "success" : "default"}>
            {state.kind === "healthy" ? "ONLINE" : "CHECKING"}
          </Tag>
        </Flex>

        {state.kind === "loading" && (
          <Flex vertical align="center" gap={16} className="py-10">
            <Spin size="large" />
            <Typography.Text type="secondary">
              正在检查 Gin API……
            </Typography.Text>
          </Flex>
        )}

        {state.kind === "healthy" && (
          <Alert
            showIcon
            type="success"
            title="API 运行正常"
            description="前端已通过 Vite 代理成功连接到 Gin 健康检查接口。"
          />
        )}

        {state.kind === "unreachable" && (
          <Alert
            showIcon
            type="error"
            title="API 当前不可达"
            description={state.message}
          />
        )}

        <Button
          type="primary"
          size="large"
          loading={state.kind === "loading"}
          onClick={() => {
            setState({ kind: "loading" });
            void checkHealth();
          }}
        >
          重新检查
        </Button>
      </Flex>
    </Card>
  );
}

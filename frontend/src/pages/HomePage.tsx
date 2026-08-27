import { Typography } from "antd";

import { HealthStatus } from "../features/health/HealthStatus";

export function HomePage() {
  return (
    <main className="min-h-screen bg-[radial-gradient(circle_at_top_left,_#dbeafe,_#f8fafc_48%,_#e2e8f0)] px-6 py-16">
      <section className="mx-auto flex max-w-5xl flex-col items-center gap-10">
        <header className="max-w-2xl text-center">
          <Typography.Text className="!font-semibold !uppercase !tracking-[0.28em] !text-blue-600">
            Tiny Blog Lab · Phase 2
          </Typography.Text>
          <Typography.Title className="!mb-4 !mt-4 !text-4xl sm:!text-5xl">
            Muggle API 状态
          </Typography.Title>
          <Typography.Paragraph className="!m-0 !text-base !leading-7 !text-slate-600">
            这个辅助页面用于检查 React、Axios、Vite Proxy、Gin Router 和 API 存活状态。
          </Typography.Paragraph>
        </header>

        <HealthStatus />
      </section>
    </main>
  );
}

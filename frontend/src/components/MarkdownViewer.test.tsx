import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { MarkdownViewer } from "./MarkdownViewer";

// 本测试使用真实的 ByteMD Viewer 验证 Markdown 安全清洗（设计文档 9.5）：
// 原始 HTML 被禁用、危险协议被拦截，脚本/样式/事件属性不会执行。
// 页面测试（PostDetailPage 等）为了聚焦状态处理会 mock 掉本组件，
// 所以安全行为在这里单独覆盖。
//
// 注意：多行 Markdown 用模板字符串书写，换行是真实换行符，避免转义歧义。

describe("MarkdownViewer（ByteMD 安全清洗）", () => {
  it("把普通 Markdown 渲染为 HTML", async () => {
    const { container } = render(
      <MarkdownViewer
        value={`# 标题

正文段落`}
      />,
    );

    // ByteMD 渲染是异步的（组件挂载后处理 Markdown），用 findBy* 等待。
    expect(
      await screen.findByRole("heading", { name: "标题" }),
    ).toBeInTheDocument();
    expect(container.textContent).toContain("正文段落");
  });

  it("禁用原始 HTML：<script> 不会成为可执行元素", async () => {
    const { container } = render(
      <MarkdownViewer
        value={`<script>alert("xss")</script>

正常文本`}
      />,
    );

    expect(await screen.findByText("正常文本")).toBeInTheDocument();
    // 原始 <script> 节点应被丢弃，最终 DOM 中不存在 script 元素。
    expect(container.querySelector("script")).toBeNull();
  });

  it("禁用原始 HTML：<div> 及其内容不会被渲染", async () => {
    const { container } = render(
      <MarkdownViewer
        value={`<div>我是原始 HTML</div>

## 正常标题`}
      />,
    );

    expect(
      await screen.findByRole("heading", { name: "正常标题" }),
    ).toBeInTheDocument();
    // 原始 <div> 被丢弃，其内部文本不应出现在最终输出里。
    expect(container.textContent).not.toContain("我是原始 HTML");
  });

  it("过滤 javascript: 协议链接", async () => {
    const { container } = render(
      <MarkdownViewer value="[点我](javascript:alert('xss'))" />,
    );

    await screen.findByText("点我");
    const link = container.querySelector("a");
    // 危险协议被协议白名单拦截，href 会被剥离；用 ?? "" 兜底空值再判断。
    expect(link?.getAttribute("href") ?? "").not.toContain("javascript:");
  });
});

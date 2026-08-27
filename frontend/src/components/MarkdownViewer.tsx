import { Viewer } from "@bytemd/react";

import {
  markdownPlugins,
  markdownRemarkRehype,
  sanitizeSchema,
} from "./markdown";

// MarkdownViewer 用与编辑器完全相同的安全策略，把 Markdown 渲染为 HTML。
//
// 公开文章详情页也使用它，因此访客看到的正文同样经过清洗：
// 原始 HTML 被禁用、危险协议被拦截，脚本/样式/事件属性不会执行。
interface MarkdownViewerProps {
  // value 是要渲染的 Markdown 文本。
  value: string;
}

export function MarkdownViewer({ value }: MarkdownViewerProps) {
  return (
    <Viewer
      value={value ?? ""}
      plugins={markdownPlugins}
      sanitize={sanitizeSchema}
      remarkRehype={markdownRemarkRehype}
    />
  );
}

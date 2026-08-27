import { Editor } from "@bytemd/react";

import {
  markdownLocale,
  markdownPlugins,
  markdownRemarkRehype,
  sanitizeSchema,
} from "./markdown";

// MarkdownEditor 封装 ByteMD 编辑器，作为受控组件供 antd 表单使用。
//
// 对外只暴露 value 和 onChange 两个接口，把 ByteMD 的插件、中文文案、
// 原始 HTML 禁用与协议白名单等安全配置全部收在内部。这样页面无需了解
// ByteMD 的具体用法，也能保证“编辑预览”与“公开阅读”使用同一套安全规则。
//
// 两个属性都声明为可选：当 MarkdownEditor 作为 antd Form.Item 的子组件时，
// antd 会在运行时注入 value 和 onChange；此时 JSX 里写 <MarkdownEditor />
// 并不显式传参，因此类型上把它们标成可选更贴合实际用法。
interface MarkdownEditorProps {
  // value 是当前 Markdown 文本（受控组件）。
  value?: string;
  // onChange 在用户编辑时回调，参数是最新的 Markdown 文本。
  onChange?: (value: string) => void;
}

export function MarkdownEditor({ value, onChange }: MarkdownEditorProps) {
  return (
    <Editor
      // antd Form 在字段尚未赋值时会给 value 传入 undefined，
      // 这里兜底为空字符串，避免 ByteMD 收到 undefined。
      value={value ?? ""}
      onChange={onChange}
      plugins={markdownPlugins}
      locale={markdownLocale}
      sanitize={sanitizeSchema}
      remarkRehype={markdownRemarkRehype}
      placeholder="在这里书写 Markdown 正文…"
    />
  );
}

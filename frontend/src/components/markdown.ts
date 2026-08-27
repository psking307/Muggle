// 本模块集中管理 ByteMD 的插件与 Markdown 安全策略，
// 供 MarkdownEditor（编辑）和 MarkdownViewer（预览/公开阅读）共用，
// 保证两条渲染路径使用完全一致的安全规则（设计文档 9.5）。

import gfm from "@bytemd/plugin-gfm";
import gfmLocale from "@bytemd/plugin-gfm/locales/zh_Hans.json";
import highlight from "@bytemd/plugin-highlight";
import type { BytemdLocale, BytemdPlugin } from "bytemd";
import type { Schema } from "hast-util-sanitize";

// 编辑器工具栏/帮助面板的中文文案（来自 ByteMD 官方 locale）。
// 直接导入官方 JSON，避免手写一长串翻译，也与 ByteMD 官方文案保持一致。
import editorLocale from "bytemd/locales/zh_Hans.json";

// markdownPlugins 是编辑器和预览器共用的插件列表：
//   - gfm：支持表格、删除线、任务列表等 GitHub Flavored Markdown 扩展；
//   - highlight：代码块语法高亮（高亮主题 CSS 在入口 main.tsx 里统一引入）。
export const markdownPlugins: BytemdPlugin[] = [
  gfm({ locale: gfmLocale }),
  highlight(),
];

// markdownLocale 是编辑器工具栏的中文文案。
export const markdownLocale: Partial<BytemdLocale> = editorLocale;

// markdownRemarkRehype 关闭 remark-rehype 的 allowDangerousHtml。
//
// ByteMD 默认会把这个选项置为 true，即把 Markdown 里手写的原始 HTML
// 直接注入最终 HTML，再靠 sanitize 清洗。这里显式关闭它：原始 HTML 节点
// 会在转换阶段就被丢弃，做到“默认禁用 Markdown 原始 HTML”（设计文档 9.5）。
// 因此用户写的 <script>、<style>、<img onerror=...> 等都不会被当作 HTML 渲染。
export const markdownRemarkRehype = { allowDangerousHtml: false };

// sanitizeSchema 在 ByteMD 默认的 GitHub 清洗规则基础上，进一步显式收紧
// 链接与图片的协议白名单：只允许 http / https / mailto 以及相对路径，
// 拒绝 javascript:、data: 等可能被用来注入脚本的危险协议。
//
// 说明：ByteMD 默认已使用 hast-util-sanitize 的 GitHub schema 做安全清洗
// （脚本、样式、事件属性都会被剥离），这里把协议白名单写得更明确、更严格，
// 作为“链接协议允许列表”的第二道防线，防止 Markdown 链接 [x](javascript:...) 注入。
export function sanitizeSchema(schema: Schema): Schema {
  return {
    ...schema,
    protocols: {
      // 保留 ByteMD 默认允许的其它协议，再显式覆盖 href / src。
      ...(schema.protocols ?? {}),
      href: ["http", "https", "mailto"],
      src: ["http", "https"],
    },
  };
}

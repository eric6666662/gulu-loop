import MarkdownIt from 'markdown-it'
import type { RenderRule } from 'markdown-it/lib/renderer.mjs'


/**
 * Markdown 渲染（02-前端设计.md §2.1）：markdown-it 负责结构，shiki 负责代码高亮。
 *
 * 为什么是 async：shiki 的 highlighter 初始化是异步的（且体积大，只在第一次渲染时加载）。
 * 组件里用 `await renderMarkdown(...)` 后 `v-html`。
 *
 * 安全：`html: false` —— 原始 HTML 一律转义（消息内容来自模型，不能当 HTML 执行）。
 */

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

/** 把 shiki 的 token 拼成带内联颜色的 <span>，行间用 \n */
function tokensToHtml(lines: { content: string; color?: string; fontStyle?: number }[][]): string {
  return lines
    .map((line) =>
      line
        .map((t) => {
          const style = t.color ? `color:${t.color}` : ''
          const italic = t.fontStyle === 1 || t.fontStyle === 3 ? 'font-style:italic' : ''
          const styleAttr = [style, italic].filter(Boolean).join(';')
          return styleAttr ? `<span style="${styleAttr}">${escapeHtml(t.content)}</span>` : escapeHtml(t.content)
        })
        .join(''),
    )
    .join('\n')
}

/** 代码块容器：与 02-ui原型.html 的 @component CodeBlock 同构（复制按钮 + pre > code） */
function codeBlockHtml(lang: string, highlighted: string): string {
  return (
    `<div class="group/code relative rounded-md border border-border bg-surface-alt">` +
    `<button type="button" data-copy-code aria-label="复制代码" ` +
    `class="absolute top-2 right-2 flex size-7 items-center justify-center rounded-sm bg-surface-alt text-ink-muted opacity-0 transition-opacity duration-150 hover:bg-surface hover:text-ink focus-visible:opacity-100 group-hover/code:opacity-100">` +
    `<svg class="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect width="14" height="14" x="8" y="8" rx="2" ry="2" /><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2" /></svg>` +
    `</button>` +
    `<pre class="scroll-thin overflow-x-auto p-3 font-mono text-meta leading-6 text-ink" data-lang="${escapeHtml(lang)}">` +
    `<code>${highlighted}</code></pre></div>`
  )
}

function createRenderer(): MarkdownIt {
  const md = new MarkdownIt({ html: false, linkify: true, breaks: false })

  // 行内 code：用原型里的 class
  md.renderer.rules.code_inline = ((tokens, idx) => {
    const content = tokens[idx].content
    return `<code class="rounded-xs bg-surface-alt px-1 py-1 font-mono text-meta text-ink">${escapeHtml(content)}</code>`
  }) as RenderRule

  // 外链：新标签打开 + noopener
  const defaultLinkOpen: RenderRule =
    md.renderer.rules.link_open ??
    ((tokens, idx, options, _env, self) => self.renderToken(tokens, idx, options))
  md.renderer.rules.link_open = ((tokens, idx, options, env, self) => {
    tokens[idx].attrSet('target', '_blank')
    tokens[idx].attrSet('rel', 'noopener noreferrer')
    return defaultLinkOpen(tokens, idx, options, env, self)
  }) as RenderRule

  return md
}

const md = createRenderer()

/** 把消息正文渲染成 HTML（异步：代码高亮需要先加载 shiki） */
export async function renderMarkdown(text: string): Promise<string> {
  if (!text) return ''

  // 先同步渲染，代码块留占位，最后统一替换成高亮结果
  const blocks: { lang: string; code: string; key: string }[] = []
  md.renderer.rules.fence = ((tokens, idx) => {
    const token = tokens[idx]
    const lang = (token.info || '').trim().split(/\s+/)[0] || 'text'
    const key = `\u0000CODEBLOCK${blocks.length}\u0000`
    blocks.push({ lang, code: token.content, key })
    return `<p>${key}</p>`
  }) as RenderRule

  let html = md.render(text)

  if (blocks.length > 0) {
    const { getHighlighter, HIGHLIGHT_THEME } = await import('./highlighter')
    const hl = await getHighlighter()
    for (const b of blocks) {
      let inner: string
      try {
        inner = tokensToHtml(
          hl.codeToTokensBase(b.code.replace(/\n$/, ''), { lang: b.lang, theme: HIGHLIGHT_THEME }),
        )
      } catch {
        // 未知语言：退化成纯文本（不崩）
        inner = escapeHtml(b.code.replace(/\n$/, ''))
      }
      html = html.replace(`<p>${b.key}</p>`, codeBlockHtml(b.lang, inner))
    }
  }

  return html
}

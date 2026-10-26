import { describe, expect, it } from 'vitest'
import { renderMarkdown } from './markdown'

describe('lib/markdown', () => {
  it('代码块：包一层可复制容器，pre > code，且有语法高亮色', async () => {
    const html = await renderMarkdown('```go\npackage main\n```')

    expect(html).toContain('data-copy-code')
    expect(html).toMatch(/<pre[^>]*><code/)
    expect(html).toMatch(/style="color:/) // shiki 高亮后的 token 带颜色
    expect(html).toContain('package')
  })

  it('未知语言不崩，仍渲染成代码块', async () => {
    const html = await renderMarkdown('```nope\nx = 1\n```')

    expect(html).toMatch(/<pre[^>]*><code/)
    expect(html).toContain('x = 1')
  })

  it('行内 code 带原型里的 class', async () => {
    const html = await renderMarkdown('用 `[]int` 和 `[]string`')

    expect(html).toContain('<code class="rounded-xs bg-surface-alt')
    expect(html).toContain('[]int')
  })

  it('HTML 被转义（防 XSS）', async () => {
    const html = await renderMarkdown('<script>alert(1)</script>')

    expect(html).not.toContain('<script>')
    expect(html).toContain('&lt;script&gt;')
  })

  it('外链带 target=_blank 与 rel=noopener', async () => {
    const html = await renderMarkdown('见 [文档](https://example.com/doc)')

    expect(html).toContain('href="https://example.com/doc"')
    expect(html).toContain('rel="noopener noreferrer"')
    expect(html).toContain('target="_blank"')
  })

  it('段落与列表按原型排版', async () => {
    const html = await renderMarkdown('第一段\n\n- 甲\n- 乙')

    expect(html).toContain('<p>')
    expect(html).toContain('<ul>')
    expect(html).toContain('<li>甲</li>')
  })
})

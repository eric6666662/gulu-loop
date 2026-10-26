import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

// 直接用 fs 读源文件：vitest 会把 CSS 导入 stub 成空串，`?raw` 拿不到内容
const css = readFileSync(resolve(process.cwd(), 'src/styles/main.css'), 'utf8')

/**
 * 布局壳的回归防线（2026-10-26）：
 * body 有 h-dvh，但 Vue 挂载点 #app 没有高度时，内层 `h-full` 会解析成内容高度，
 * 整页塌成内容高度 —— 侧栏底部按钮就不再固定在页面底部（无会话时最明显）。
 *
 * happy-dom 不做布局计算，所以这里锁"样式规则存在"这条可自动化的部分，
 * 真实高度由浏览器冒烟验证（见 03-计划.md）。
 */
describe('布局壳', () => {
  it('html / body / #app 三者都要有高度，否则内层 h-full 没有参照', () => {
    expect(css).toMatch(/html[^{]*\{[^}]*height:\s*100%/s)
    expect(css).toMatch(/#app\s*\{[^}]*height:\s*100%/s)
  })
})

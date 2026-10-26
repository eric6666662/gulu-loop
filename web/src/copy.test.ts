import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

/**
 * 文案纪律（docs/设计稿.md §8 禁用术语）：界面上不许出现这些词。
 * 扫描 src 下的 .vue / .ts（排除测试文件），发现即失败。
 */
const BANNED = ['暂无数据', '操作成功', '系统繁忙', '抱歉', '很遗憾', '加载中...']

function walk(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) walk(p, out)
    else if (/\.(vue|ts)$/.test(name) && !/\.test\.ts$/.test(name) && name !== 'copy.test.ts') out.push(p)
  }
  return out
}

describe('文案纪律', () => {
  it('源码里不出现禁用术语', () => {
    const root = resolve(process.cwd(), 'src')
    const hits: string[] = []
    for (const file of walk(root)) {
      // 只查"会被用户看到"的部分：先剥掉注释（注释里出现反例词是正常的）
      const text = readFileSync(file, 'utf8')
        .replace(/<!--[\s\S]*?-->/g, '')
        .replace(/\/\*[\s\S]*?\*\//g, '')
        .replace(/^\s*\/\/.*$/gm, '')
      for (const word of BANNED) {
        if (text.includes(word)) hits.push(`${file.replace(root, 'src')}: ${word}`)
      }
    }
    expect(hits).toEqual([])
  })
})

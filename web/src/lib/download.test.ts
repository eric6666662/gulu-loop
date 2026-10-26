import { afterEach, describe, expect, it, vi } from 'vitest'
import { downloadText } from './download'

describe('lib/download', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('用 Blob + createObjectURL 触发下载，并在点击后释放 URL', () => {
    const createObjectURL = vi.fn(() => 'blob:mock')
    const revokeObjectURL = vi.fn()
    vi.stubGlobal('URL', { ...URL, createObjectURL, revokeObjectURL })
    const click = vi.fn()
    const origCreate = document.createElement.bind(document)
    vi.spyOn(document, 'createElement').mockImplementation((tag: string) => {
      const el = origCreate(tag) as HTMLElement
      if (tag === 'a') el.click = click
      return el
    })

    downloadText('会话.md', '# 标题\n\n内容')

    expect(createObjectURL).toHaveBeenCalledTimes(1)
    const blob = (createObjectURL.mock.calls[0] as unknown as [Blob])[0]
    expect(blob).toBeInstanceOf(Blob)
    expect(blob.type).toContain('markdown')
    expect(click).toHaveBeenCalledTimes(1)
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:mock')
  })
})

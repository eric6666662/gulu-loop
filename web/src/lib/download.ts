/**
 * 触发浏览器下载一段文本（导出 Markdown 用）。
 *
 * 后端返回的是 JSON（`{filename, content}`，02-后端设计.md §1.4），
 * 所以由前端造 Blob + createObjectURL —— 中文文件名跨浏览器更稳。
 */
export function downloadText(filename: string, content: string): void {
  const blob = new Blob([content], { type: 'text/markdown;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.style.display = 'none'
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

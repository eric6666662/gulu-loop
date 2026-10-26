import { createHighlighterCore, type HighlighterCore } from 'shiki/core'
import { createJavaScriptRegexEngine } from 'shiki/engine/javascript'
import themeGithubLight from '@shikijs/themes/github-light'
import langGo from '@shikijs/langs/go'
import langTypeScript from '@shikijs/langs/typescript'
import langJavaScript from '@shikijs/langs/javascript'
import langTsx from '@shikijs/langs/tsx'
import langVue from '@shikijs/langs/vue'
import langJson from '@shikijs/langs/json'
import langBash from '@shikijs/langs/bash'
import langPython from '@shikijs/langs/python'
import langSql from '@shikijs/langs/sql'
import langYaml from '@shikijs/langs/yaml'
import langMarkdown from '@shikijs/langs/markdown'
import langHtml from '@shikijs/langs/html'
import langCss from '@shikijs/langs/css'

/**
 * 代码高亮（单独成模块，由 `lib/markdown` 动态 import —— 不进主 chunk）。
 *
 * 只装用得上的语言：shiki 的 `createHighlighter` 会把 300+ 语言全打进 dist（9.9MB），
 * 而这些文件最终会被 go:embed 塞进二进制。这里显式引入 + JS 正则引擎（不拉 oniguruma wasm）。
 */
const THEME = 'github-light'

const LANGS = [
  langGo,
  langTypeScript,
  langJavaScript,
  langTsx,
  langVue,
  langJson,
  langBash,
  langPython,
  langSql,
  langYaml,
  langMarkdown,
  langHtml,
  langCss,
]

let highlighterPromise: Promise<HighlighterCore> | null = null

export function getHighlighter(): Promise<HighlighterCore> {
  if (!highlighterPromise) {
    highlighterPromise = createHighlighterCore({
      themes: [themeGithubLight],
      langs: LANGS,
      engine: createJavaScriptRegexEngine(),
    })
  }
  return highlighterPromise
}

export const HIGHLIGHT_THEME = THEME

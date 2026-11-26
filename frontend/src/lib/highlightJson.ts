// JSON 语法高亮（浅色调色板）：先 HTML 转义再包 span，输出可安全用于 v-html。
// design.md §4.3：key / 字符串 / 数字 / 布尔 / null 分色；非法 JSON 由调用方回退纯文本。

const ESC: Record<string, string> = { '&': '&amp;', '<': '&lt;', '>': '&gt;' }

function escapeHtml(s: string): string {
  return s.replace(/[&<>]/g, (c) => ESC[c])
}

const TOKEN =
  /("(?:\\u[0-9a-fA-F]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(?:true|false|null)\b|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g

export function highlightJson(src: string): string {
  return escapeHtml(src).replace(TOKEN, (m) => {
    let cls = 'j-num'
    if (m.startsWith('"')) {
      cls = /:\s*$/.test(m) ? 'j-key' : 'j-str'
    } else if (m === 'true' || m === 'false') {
      cls = 'j-bool'
    } else if (m === 'null') {
      cls = 'j-null'
    }
    return `<span class="${cls}">${m}</span>`
  })
}

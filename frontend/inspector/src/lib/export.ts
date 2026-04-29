import type { Chunk, ToolCallChunk } from './types'
import { fmt } from './fmt'

export interface SessionMeta {
  sessionId: string
  projectName: string
  started: string
  ended: string
}

function renderToolCall(tc: ToolCallChunk): string {
  const inputStr = fmt.htmlSafe(JSON.stringify(tc.input, null, 2))
  const outputStr = fmt.htmlSafe(tc.output ?? '')
  const errorBadge = tc.isError ? '<span class="error-badge">⚠ Error</span> ' : ''
  return `<div class="tool-call">
  <div class="tool-name">⚙ ${fmt.htmlSafe(tc.name)}</div>
  <pre class="tool-pre">${inputStr}</pre>
  <div class="tool-output-label">${errorBadge}Output</div>
  <pre class="tool-pre">${outputStr}</pre>
</div>`
}

function renderChunk(chunk: Chunk): string {
  const ts = chunk.timestamp.slice(11, 19)
  switch (chunk.type) {
    case 'user':
      return `<div class="turn user-turn">
  <div class="turn-header"><span class="badge">you</span><span class="ts">${ts}</span></div>
  <div class="turn-text">${fmt.htmlSafe(chunk.text ?? '')}</div>
</div>`

    case 'ai': {
      const thinking = chunk.thinking
        ? `<details class="thinking"><summary>Thinking</summary><pre>${fmt.htmlSafe(chunk.thinking)}</pre></details>`
        : ''
      const tools = (chunk.toolCalls ?? []).map(renderToolCall).join('\n')
      return `<div class="turn ai-turn">
  <div class="turn-header"><span class="badge ai-badge">claude</span><span class="ts">${ts}</span></div>
  ${thinking}
  ${tools}
  <div class="token-row">${fmt.tok(chunk.inputTokens)} in · ${fmt.tok(chunk.outputTokens)} out${chunk.cacheRead ? ` · ${fmt.tok(chunk.cacheRead)} cache` : ''}</div>
</div>`
    }

    case 'compact':
      return `<div class="compact-boundary">⚡ Context compacted — ${fmt.tok(chunk.tokensBefore)} → ${fmt.tok(chunk.tokensAfter)} tokens</div>`

    case 'system':
      return `<div class="turn system-turn"><span class="system-label">system</span> ${fmt.htmlSafe(chunk.text ?? '')}</div>`

    default:
      return ''
  }
}

const CSS = `
*{box-sizing:border-box;margin:0;padding:0}
:root{--bg:#fff;--text:#1a1a1a;--muted:#666;--border:#e0e0e0;--panel:#f5f5f5;--accent:#7c3aed;--mono:'Menlo','Consolas',monospace}
@media(prefers-color-scheme:dark){:root{--bg:#1a1a1a;--text:#e0e0e0;--muted:#999;--border:#333;--panel:#252525;--accent:#9d72ff}}
body{background:var(--bg);color:var(--text);font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;font-size:13px;line-height:1.5;padding:16px}
header{max-width:860px;margin:0 auto 24px;padding-bottom:12px;border-bottom:2px solid var(--accent)}
h1{font-size:18px;font-weight:600;margin-bottom:6px}
.meta{display:flex;gap:16px;font-size:11px;font-family:var(--mono);color:var(--muted)}
main{max-width:860px;margin:0 auto}
.turn{padding:12px 0;border-bottom:1px solid var(--border)}
.turn-header{display:flex;align-items:center;gap:8px;margin-bottom:8px}
.badge{font-size:10px;font-family:var(--mono);background:var(--panel);border:1px solid var(--border);border-radius:3px;padding:2px 6px}
.ai-badge{background:rgba(124,58,237,.15);border-color:var(--accent);color:var(--accent)}
.ts{font-size:10px;font-family:var(--mono);color:var(--muted)}
.turn-text{white-space:pre-wrap;word-break:break-word;font-size:13px;line-height:1.6}
.thinking{margin:8px 0;border:1px solid var(--border);border-radius:4px;padding:6px 10px;font-size:11px;color:var(--muted)}
.thinking summary{cursor:pointer;font-family:var(--mono)}
.thinking pre{margin-top:6px;white-space:pre-wrap;font-size:11px}
.tool-call{margin:6px 0;border:1px solid var(--border);border-radius:4px;overflow:hidden;font-size:11px}
.tool-name{padding:4px 8px;font-family:var(--mono);font-size:11px;background:var(--panel);color:var(--muted);border-bottom:1px solid var(--border)}
.tool-pre{padding:8px;background:var(--bg);font-family:var(--mono);font-size:11px;white-space:pre-wrap;word-break:break-word;max-height:300px;overflow:auto}
.tool-output-label{padding:2px 8px;font-size:10px;font-family:var(--mono);color:var(--muted);background:var(--panel);border-top:1px solid var(--border);border-bottom:1px solid var(--border)}
.error-badge{color:#e53e3e}
.token-row{margin-top:6px;font-size:10px;font-family:var(--mono);color:var(--muted)}
.compact-boundary{padding:10px 0;font-size:11px;font-family:var(--mono);color:var(--accent);text-align:center;border-bottom:1px solid var(--border)}
.system-turn{padding:6px 0;border-bottom:1px solid var(--border);font-size:12px;color:var(--muted)}
.system-label{font-family:var(--mono);font-size:10px;margin-right:8px}
`

export function generateSessionHTML(chunks: Chunk[], meta: SessionMeta): string {
  const totalIn = chunks.reduce((s, c) => s + (c.inputTokens ?? 0), 0)
  const totalOut = chunks.reduce((s, c) => s + (c.outputTokens ?? 0), 0)
  const title = fmt.htmlSafe(meta.projectName || meta.sessionId.slice(0, 8))
  const body = chunks.map(renderChunk).join('\n')

  return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>${title} — TokenTally Session</title>
  <style>${CSS}</style>
</head>
<body>
  <header>
    <h1>${title}</h1>
    <div class="meta">
      <span>${fmt.htmlSafe(meta.sessionId.slice(0, 8))}</span>
      <span>${fmt.ts(meta.started) || '—'} → ${fmt.ts(meta.ended) || '—'}</span>
      <span>${fmt.tok(totalIn)} in · ${fmt.tok(totalOut)} out</span>
    </div>
  </header>
  <main>
${body}
  </main>
</body>
</html>`
}

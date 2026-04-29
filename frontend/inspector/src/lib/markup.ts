import { marked } from 'marked'
import DOMPurify from 'dompurify'

const OPEN_RE  = /^<[a-z][a-z0-9_-]*(?:\s[^>]*)?>$/
const CLOSE_RE = /^<\/[a-z][a-z0-9_-]*>$/
const CAPS_RE  = /^<\/?[A-Z][A-Z0-9_-]*(?:\s[^>]*)?>$/

const SANITIZE_CFG: DOMPurify.Config = {
  ALLOWED_TAGS: [
    'p', 'br', 'hr', 'em', 'strong', 'b', 'i',
    'h1', 'h2', 'h3', 'h4', 'h5', 'h6',
    'ul', 'ol', 'li', 'code', 'pre', 'blockquote', 'a', 'span', 'div',
  ],
  ALLOWED_ATTR: ['class', 'href'],
  ALLOW_DATA_ATTR: false,
}

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;')
}

// Converts raw text into safe HTML:
//  - ALL_CAPS tags on their own line → muted pill
//  - lowercase <tag>…</tag> blocks → .sys-block wrapper (dimmed) with pills
export function preprocessText(text: string): string {
  const lines = text.split('\n')
  const out: string[] = []
  let inBlock = false

  for (const line of lines) {
    const t = line.trim()
    if (CAPS_RE.test(t)) {
      out.push(`<span class="sys-tag">${escapeHtml(t)}</span>`)
    } else if (OPEN_RE.test(t)) {
      out.push(inBlock
        ? `<span class="sys-tag">${escapeHtml(t)}</span>`
        : `<div class="sys-block"><span class="sys-tag">${escapeHtml(t)}</span>`)
      inBlock = true
    } else if (CLOSE_RE.test(t) && inBlock) {
      out.push(`<span class="sys-tag">${escapeHtml(t)}</span></div>`)
      inBlock = false
    } else {
      out.push(line)
    }
  }
  if (inBlock) out.push('</div>')
  return out.join('\n')
}

// Ensures lowercase XML tags land on their own lines before preprocessing.
function spaceXmlTags(text: string): string {
  if (!/<[a-z_][a-z_0-9]*[^>]*>/.test(text)) return text
  return text
    .replace(/(<[a-z_][a-z_0-9]*(?:\s[^>]*)?>)/g, '\n$1\n')
    .replace(/(<\/[a-z_][a-z_0-9]*>)/g, '\n$1\n')
    .replace(/\n{3,}/g, '\n\n')
    .trim()
}

// Full pipeline: space tags → preprocess pills/blocks → marked → DOMPurify.
export function renderMarkdown(text: string): string {
  if (!text) return ''
  const raw = marked.parse(preprocessText(spaceXmlTags(text)), { async: false }) as string
  return DOMPurify.sanitize(raw, SANITIZE_CFG) as string
}

// Extract only the user-typed text from a prompt, removing injected XML blocks
// (tag + all content between open and close) and standalone ALL_CAPS tags.
// Falls back to tag-stripped content if nothing user-typed is found.
export function stripTagsForPreview(text: string): string {
  if (!text) return ''

  // Remove complete <lowercase-tag>…</lowercase-tag> blocks including their content
  const withoutBlocks = text
    .replace(/<[a-z][a-z0-9_-]*(?:\s[^>]*)?>([\s\S]*?)<\/[a-z][a-z0-9_-]*>/g, '')

  // Remove any remaining standalone tags (open-only or ALL_CAPS system tags)
  const cleaned = withoutBlocks
    .replace(/<\/?[A-Za-z][A-Za-z0-9_-]*(?:\s[^>]*)?>/g, '')
    .replace(/\s+/g, ' ')
    .trim()

  if (cleaned) return cleaned

  // Fallback: nothing user-typed found — strip tags but keep content text
  return text
    .replace(/<\/?[A-Za-z][A-Za-z0-9_-]*(?:\s[^>]*)?>/g, '')
    .replace(/\s+/g, ' ')
    .trim()
}

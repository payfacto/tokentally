export const SECONDS_PER_DAY = 86_400

export const RANGES = [
  { key: '7d',  label: '7d',  days: 7 },
  { key: '30d', label: '30d', days: 30 },
  { key: '90d', label: '90d', days: 90 },
  { key: 'all', label: 'All', days: null as number | null },
]

export function sinceIso(range: { days: number | null }): string | null {
  if (!range.days) return null
  return new Date(Date.now() - range.days * SECONDS_PER_DAY * 1000).toISOString()
}

export function withSince(url: string, since: string | null): string {
  if (!since) return url
  return url + (url.includes('?') ? '&' : '?') + 'since=' + encodeURIComponent(since)
}

function App() {
  return window.go.app.App
}

type QS = Record<string, string>

const apiMap: Record<string, (qs: QS) => Promise<unknown>> = {
  '/api/overview': (qs) => App().GetOverview(qs.since || '', qs.until || ''),
  '/api/prompts':        (qs) => App().GetPrompts(parseInt(qs.limit || '50', 10), qs.sort || 'tokens'),
  '/api/prompts/search': (qs) => App().SearchPrompts(qs.q || '', qs.types || 'user', qs.from || '', qs.to || ''),
  '/api/projects': (qs) => App().GetProjects(qs.since || '', qs.until || ''),
  '/api/sessions': (qs) => App().GetSessions(parseInt(qs.limit || '200', 10), qs.since || '', qs.until || ''),
  '/api/tools':    (qs) => App().GetTools(qs.since || '', qs.until || ''),
  '/api/daily':    (qs) => App().GetDaily(qs.since || '', qs.until || ''),
  '/api/by-model': (qs) => App().GetByModel(qs.since || '', qs.until || ''),
  '/api/skills':   (qs) => App().GetSkills(qs.since || '', qs.until || ''),
  '/api/tips':     (_)  => App().GetTips(),
  '/api/plan':     (_)  => App().GetPlan(),
  '/api/scan':     (_)  => App().ScanNow(),
}

export async function api<T = unknown>(path: string, opts?: { method: string; body: string }): Promise<T> {
  const [base, search] = path.split('?')
  const qs = Object.fromEntries(new URLSearchParams(search || ''))

  if (base.startsWith('/api/sessions/')) {
    const sid = base.split('/').pop() || ''
    return (await App().GetSessionChunks(decodeURIComponent(sid))) as unknown as T
  }

  if (opts?.method === 'POST') {
    const body = JSON.parse(opts.body || '{}')
    if (base === '/api/tips/dismiss') { await App().DismissTip(body.key || ''); return undefined as unknown as T }
    if (base === '/api/plan') { await App().SetPlan(body.plan || ''); return undefined as unknown as T }
  }

  const handler = apiMap[base]
  if (!handler) throw new Error(`No binding for ${base}`)
  return (await handler(qs)) as T
}

declare global {
  interface Window {
    go: {
      app: {
        App: {
          GetOverview(since: string, until: string): Promise<unknown>
          GetPrompts(limit: number, sort: string): Promise<unknown>
          GetProjects(since: string, until: string): Promise<unknown>
          GetSessions(limit: number, since: string, until: string): Promise<unknown>
          GetSessionChunks(sessionId: string): Promise<unknown>
          GetTools(since: string, until: string): Promise<unknown>
          GetDaily(since: string, until: string): Promise<unknown>
          GetByModel(since: string, until: string): Promise<unknown>
          GetSkills(since: string, until: string): Promise<unknown>
          GetTips(): Promise<unknown>
          GetPlan(): Promise<unknown>
          SetPlan(plan: string): Promise<unknown>
          DismissTip(key: string): Promise<unknown>
          ScanNow(): Promise<unknown>
          SaveHTMLExport(html: string): Promise<string>
        }
      }
    }
    runtime: {
      EventsOn(event: string, cb: (...args: unknown[]) => void): void
      EventsOff(event: string): void
    }
  }
}

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
  '/api/prompts':  (qs) => App().GetPrompts(parseInt(qs.limit || '50', 10), qs.sort || 'tokens'),
  '/api/projects': (qs) => App().GetProjects(qs.since || '', qs.until || ''),
  '/api/sessions': (qs) => App().GetSessions(parseInt(qs.limit || '20', 10), qs.since || '', qs.until || ''),
  '/api/tools':    (qs) => App().GetTools(qs.since || '', qs.until || ''),
  '/api/daily':    (qs) => App().GetDaily(qs.since || '', qs.until || ''),
  '/api/by-model': (qs) => App().GetByModel(qs.since || '', qs.until || ''),
  '/api/skills':   (qs) => App().GetSkills(qs.since || '', qs.until || ''),
  '/api/tips':     (_)  => App().GetTips(),
  '/api/plan':     (_)  => App().GetPlan(),
  '/api/scan':     (_)  => App().ScanNow(),
}

export async function api(path: string, opts?: { method: string; body: string }): Promise<unknown> {
  const [base, search] = path.split('?')
  const qs = Object.fromEntries(new URLSearchParams(search || ''))

  if (base.startsWith('/api/sessions/')) {
    const sid = base.split('/').pop() || ''
    return App().GetSessionChunks(decodeURIComponent(sid))
  }

  if (opts?.method === 'POST') {
    const body = JSON.parse(opts.body || '{}')
    if (base === '/api/tips/dismiss') return App().DismissTip(body.key || '')
    if (base === '/api/plan') return App().SetPlan(body.plan || '')
  }

  const handler = apiMap[base]
  if (!handler) throw new Error(`No binding for ${base}`)
  return handler(qs)
}

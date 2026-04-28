const COMPACT = new Intl.NumberFormat('en', { notation: 'compact', maximumFractionDigits: 1 })

const CURRENCY_SYMBOLS: Record<string, string> = {
  USD: '$', CAD: 'CA$', EUR: '€', GBP: '£', AUD: 'A$',
  NZD: 'NZ$', CHF: 'CHF ', JPY: '¥', MXN: 'MX$', BRL: 'R$',
}

export const SESSION_ID_PREFIX = 8

export const fmt = {
  int:      (n: number | null | undefined): string => (n ?? 0).toLocaleString(),
  compact:  (n: number | null | undefined): string => COMPACT.format(n ?? 0),
  usd:      (n: number | null | undefined): string => n == null ? '—' : '$' + Number(n).toFixed(2),
  usd4:     (n: number | null | undefined): string => n == null ? '—' : '$' + Number(n).toFixed(4),
  money:    (n: number | null | undefined, currency = 'USD', exchangeRate = 1.0): string => {
    if (n == null) return '—'
    const sym = CURRENCY_SYMBOLS[currency] || (currency + ' ')
    return sym + (Number(n) * exchangeRate).toFixed(2)
  },
  money4:   (n: number | null | undefined, currency = 'USD', exchangeRate = 1.0): string => {
    if (n == null) return '—'
    const sym = CURRENCY_SYMBOLS[currency] || (currency + ' ')
    return sym + (Number(n) * exchangeRate).toFixed(4)
  },
  pct:      (n: number | null | undefined): string => n == null ? '—' : (n * 100).toFixed(0) + '%',
  short:    (s: string | null | undefined, n = 80): string =>
    s == null ? '' : (s.length > n ? s.slice(0, n - 1) + '…' : s),
  htmlSafe: (s: string | null | undefined): string =>
    (s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c] ?? c)),
  modelClass: (m: string | null | undefined): string => {
    const s = (m || '').toLowerCase()
    if (s.includes('opus'))   return 'opus'
    if (s.includes('sonnet')) return 'sonnet'
    if (s.includes('haiku'))  return 'haiku'
    return ''
  },
  modelShort: (m: string | null | undefined): string => (m || '').replace('claude-', ''),
  ts: (t: string | null | undefined): string => (t || '').slice(0, 16).replace('T', ' '),
}

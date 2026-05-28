export const KIND_LABELS: Record<string, string> = {
  'retry-churn':       'Retry churn',
  'tool-cascade':      'Tool cascade',
  'looping':           'Looping',
  'output-waste':      'Output waste',
  'overpowered-model': 'Overpowered model',
  'wasteful-thinking': 'Wasteful thinking',
}

export function sevClass(rank: number): string {
  return rank >= 3 ? 'sev-high' : rank === 2 ? 'sev-med' : 'sev-low'
}

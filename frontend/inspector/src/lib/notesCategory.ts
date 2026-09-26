export const ALL_CATEGORY = '__all__'

export interface CategoryChip {
  label: string
  count: number
}

export function buildCategoryChips(groups: { label: string; files: unknown[] }[]): CategoryChip[] {
  const total = groups.reduce((sum, g) => sum + g.files.length, 0)
  return [{ label: ALL_CATEGORY, count: total }, ...groups.map((g) => ({ label: g.label, count: g.files.length }))]
}

export function filterGroupsByCategory<T extends { label: string }>(groups: T[], selected: string): T[] {
  if (selected === ALL_CATEGORY) return groups
  return groups.filter((g) => g.label === selected)
}

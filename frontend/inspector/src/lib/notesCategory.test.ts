import { describe, expect, it } from 'vitest'
import { ALL_CATEGORY, buildCategoryChips, filterGroupsByCategory } from './notesCategory'

interface Group {
  label: string
  files: unknown[]
}

describe('buildCategoryChips', () => {
  it('puts an All chip first with the total file count across all groups', () => {
    const groups: Group[] = [
      { label: 'Handoffs', files: [{}, {}] },
      { label: 'AFK Notes', files: [{}] },
    ]

    const chips = buildCategoryChips(groups)

    expect(chips[0]).toEqual({ label: ALL_CATEGORY, count: 3 })
  })

  it('adds one chip per group with that group\'s own file count', () => {
    const groups: Group[] = [
      { label: 'Handoffs', files: [{}, {}] },
      { label: 'AFK Notes', files: [{}] },
    ]

    const chips = buildCategoryChips(groups)

    expect(chips.slice(1)).toEqual([
      { label: 'Handoffs', count: 2 },
      { label: 'AFK Notes', count: 1 },
    ])
  })
})

describe('filterGroupsByCategory', () => {
  const groups: Group[] = [
    { label: 'Handoffs', files: [{}, {}] },
    { label: 'AFK Notes', files: [{}] },
  ]

  it('returns every group unchanged when ALL_CATEGORY is selected', () => {
    expect(filterGroupsByCategory(groups, ALL_CATEGORY)).toEqual(groups)
  })

  it('returns only the group matching the selected label', () => {
    expect(filterGroupsByCategory(groups, 'AFK Notes')).toEqual([groups[1]])
  })

  it('returns an empty array when the selected label matches no group', () => {
    expect(filterGroupsByCategory(groups, 'Deleted Folder')).toEqual([])
  })
})

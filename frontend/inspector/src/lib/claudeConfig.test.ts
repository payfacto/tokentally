import { describe, it, expect } from 'vitest'
import {
  formatBytes, sortProjects, isValidJson, parseArgs, parseEnv, buildMcpServerConfirmMessage,
  type ClaudeJSONProjectStat,
} from './claudeConfig'

describe('formatBytes', () => {
  it('formats sub-1KB sizes as bytes', () => {
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(512)).toBe('512 B')
    expect(formatBytes(1023)).toBe('1023 B')
  })

  it('formats KB/MB/GB with one decimal', () => {
    expect(formatBytes(1024)).toBe('1.0 KB')
    expect(formatBytes(1536)).toBe('1.5 KB')
    expect(formatBytes(1024 * 1024)).toBe('1.0 MB')
    expect(formatBytes(1024 * 1024 * 1024)).toBe('1.0 GB')
  })

  it('falls back to TB for very large sizes', () => {
    expect(formatBytes(1024 * 1024 * 1024 * 1024)).toBe('1.0 TB')
  })
})

describe('sortProjects', () => {
  const projects: ClaudeJSONProjectStat[] = [
    { path: '/b', size_bytes: 200, message_count: 5 },
    { path: '/a', size_bytes: 500, message_count: 1 },
    { path: '/c', size_bytes: 100, message_count: 10 },
  ]

  it('sorts by path ascending regardless of the descending flag', () => {
    const sorted = sortProjects(projects, 'path', true)
    expect(sorted.map(p => p.path)).toEqual(['/a', '/b', '/c'])
  })

  it('sorts by size_bytes descending by default (largest first)', () => {
    const sorted = sortProjects(projects, 'size_bytes', true)
    expect(sorted.map(p => p.path)).toEqual(['/a', '/b', '/c'])
  })

  it('sorts by size_bytes ascending when descending=false', () => {
    const sorted = sortProjects(projects, 'size_bytes', false)
    expect(sorted.map(p => p.path)).toEqual(['/c', '/b', '/a'])
  })

  it('sorts by message_count', () => {
    const sorted = sortProjects(projects, 'message_count', true)
    expect(sorted.map(p => p.path)).toEqual(['/c', '/b', '/a'])
  })

  it('does not mutate the input array', () => {
    const copy = [...projects]
    sortProjects(projects, 'size_bytes', true)
    expect(projects).toEqual(copy)
  })
})

describe('isValidJson', () => {
  it('accepts well-formed JSON', () => {
    expect(isValidJson('{}')).toBe(true)
    expect(isValidJson('{"a":[1,2,3]}')).toBe(true)
    expect(isValidJson('[]')).toBe(true)
  })

  it('rejects malformed JSON', () => {
    expect(isValidJson('{not valid')).toBe(false)
    expect(isValidJson('')).toBe(false)
    expect(isValidJson('{"a":1,}')).toBe(false)
  })
})

describe('parseArgs', () => {
  it('splits on whitespace', () => {
    expect(parseArgs('-y some-package')).toEqual(['-y', 'some-package'])
  })

  it('collapses repeated whitespace and trims', () => {
    expect(parseArgs('  -y   some-package  ')).toEqual(['-y', 'some-package'])
  })

  it('returns an empty array for blank input', () => {
    expect(parseArgs('')).toEqual([])
    expect(parseArgs('   ')).toEqual([])
  })
})

describe('parseEnv', () => {
  it('parses one KEY=value pair per line', () => {
    expect(parseEnv('A=1\nB=2')).toEqual({ A: '1', B: '2' })
  })

  it('keeps everything after the first "=" as the value', () => {
    expect(parseEnv('URL=https://example.com?a=1')).toEqual({ URL: 'https://example.com?a=1' })
  })

  it('skips blank lines and lines with no "="', () => {
    expect(parseEnv('A=1\n\nnotapair\nB=2')).toEqual({ A: '1', B: '2' })
  })

  it('returns an empty object for blank input', () => {
    expect(parseEnv('')).toEqual({})
  })
})

describe('buildMcpServerConfirmMessage', () => {
  it('warns about replacement when a server with the same name exists', () => {
    const msg = buildMcpServerConfirmMessage('github', 'npx', '~/.claude.json', true)
    expect(msg).toContain('already exists')
    expect(msg).toContain('github')
    expect(msg).toContain('~/.claude.json')
  })

  it('describes the command that will run when adding a new server', () => {
    const msg = buildMcpServerConfirmMessage('github', 'npx', '~/.claude.json', false)
    expect(msg).toContain('npx')
    expect(msg).toContain('github')
    expect(msg).not.toContain('already exists')
  })
})

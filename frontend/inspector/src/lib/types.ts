export interface ToolCallChunk {
  id: string
  name: string
  input: Record<string, unknown>
  output?: string
  isError: boolean
  durationMs?: number
  subagentId?: string
  subagentName?: string
}

export interface ContextAttrib {
  toolOutput: number
  thinking: number
  userText: number
}

export interface Chunk {
  type: 'user' | 'ai' | 'compact' | 'system'
  timestamp: string
  text?: string
  thinking?: string
  toolCalls?: ToolCallChunk[]
  inputTokens?: number
  outputTokens?: number
  cacheRead?: number
  contextAttrib?: ContextAttrib
  tokensBefore?: number
  tokensAfter?: number
}

export interface Session {
  session_id: string
  project_slug: string
  project_name: string
  started: string
  ended: string
  turns: number
  tokens: number
}

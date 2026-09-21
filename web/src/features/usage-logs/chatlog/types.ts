/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
export type ChatlogRole = 'system' | 'user' | 'assistant' | 'tool'

export type ChatlogToolCall = {
  id?: string
  name: string
  arguments: string
}

export type ChatlogMessage = {
  role: ChatlogRole
  content: string
  reasoning?: string
  tool_calls?: ChatlogToolCall[]
  tool_call_id?: string
  /** URLs or data URIs. */
  images?: string[]
  source: 'request' | 'response'
}

export type ChatlogProtocol =
  | 'openai.chat'
  | 'openai.completions'
  | 'openai.responses'
  | 'anthropic.messages'
  | 'gemini.generate'

export type ChatlogRecord = {
  request_id: string
  /** Unix seconds. */
  created_at: number
  protocol: ChatlogProtocol
  path: string
  stream: boolean
  status: number
  latency_ms: number
  user_id: number
  username: string
  token_id: number
  token_name: string
  group: string
  /** Only returned by the admin endpoint. */
  channel_id?: number
  model: string
  truncated: boolean
  messages: ChatlogMessage[]
  tools?: unknown[]
  request: unknown
  response: unknown
  /** Raw response text, present when the response could not be reassembled. */
  response_raw?: string
}

export type ChatlogResponse = {
  success: boolean
  message: string
  /** `null` means no conversation was recorded for the request. */
  data: ChatlogRecord | null
}

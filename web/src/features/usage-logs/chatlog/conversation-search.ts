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
import type { ChatlogMessage } from './types'

/** Every text of a message that the viewer can show, in display order. */
export function searchableTexts(message: ChatlogMessage): string[] {
  const texts = [message.content ?? '', message.reasoning ?? '']
  if (message.tool_call_id) texts.push(message.tool_call_id)
  for (const toolCall of message.tool_calls ?? []) {
    texts.push(toolCall.name, toolCall.arguments)
  }
  return texts
}

/** Case-insensitive; an empty query matches everything. */
export function messageMatches(
  message: ChatlogMessage,
  query: string
): boolean {
  const needle = query.trim().toLowerCase()
  if (!needle) return true
  return searchableTexts(message).some((text) =>
    text.toLowerCase().includes(needle)
  )
}

export interface TextSegment {
  text: string
  isMatch: boolean
}

/** Splits text around every case-insensitive occurrence of the query. */
export function splitMatches(text: string, query: string): TextSegment[] {
  const needle = query.trim().toLowerCase()
  if (!needle) return [{ text, isMatch: false }]
  const haystack = text.toLowerCase()
  const segments: TextSegment[] = []
  let cursor = 0
  let found = haystack.indexOf(needle)
  while (found !== -1) {
    if (found > cursor) {
      segments.push({ text: text.slice(cursor, found), isMatch: false })
    }
    cursor = found + needle.length
    segments.push({ text: text.slice(found, cursor), isMatch: true })
    found = haystack.indexOf(needle, cursor)
  }
  if (cursor < text.length) {
    segments.push({ text: text.slice(cursor), isMatch: false })
  }
  return segments
}

/** Position of the first occurrence, or -1. */
export function firstMatchIndex(text: string, query: string): number {
  const needle = query.trim().toLowerCase()
  return needle ? text.toLowerCase().indexOf(needle) : -1
}

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

/** Conversations up to this size are rendered in full. */
export const INITIAL_MESSAGE_LIMIT = 20
/** How many hidden messages one click on "show earlier" reveals. */
export const EARLIER_MESSAGES_STEP = 50

/**
 * Index of the first message rendered when the dialog opens. Agent requests
 * replay their whole history, so a long conversation opens at its latest turn
 * (from the last user message on), and never with more than
 * INITIAL_MESSAGE_LIMIT messages; the rest is revealed on demand.
 */
export function latestTurnStart(messages: ChatlogMessage[]): number {
  if (messages.length <= INITIAL_MESSAGE_LIMIT) return 0
  let lastUser = -1
  for (const [index, message] of messages.entries()) {
    if (message.role === 'user' && message.source === 'request') {
      lastUser = index
    }
  }
  return Math.max(lastUser, messages.length - INITIAL_MESSAGE_LIMIT, 0)
}

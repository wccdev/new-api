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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test, vi } from 'vitest'

import { ConversationButton } from '../conversation-button'
import { LONG_TEXT_LIMIT } from '../conversation-long-text'
import {
  EARLIER_MESSAGES_STEP,
  INITIAL_MESSAGE_LIMIT,
  latestTurnStart,
} from '../conversation-window'
import type { ChatlogMessage, ChatlogRecord } from '../types'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: { get } }))

function recordWith(messages: ChatlogMessage[]): ChatlogRecord {
  return {
    request_id: 'req-long',
    created_at: 1_700_000_000,
    protocol: 'openai.chat',
    path: '/v1/chat/completions',
    stream: true,
    status: 200,
    latency_ms: 10,
    user_id: 7,
    username: 'alice',
    token_id: 3,
    token_name: 'dev-token',
    group: 'default',
    model: 'gpt-test',
    truncated: false,
    messages,
    request: { model: 'gpt-test' },
    response: { id: 'chatcmpl-1' },
  }
}

/** system, then user/assistant pairs, a final user question and the response. */
function agentHistory(pairs: number): ChatlogMessage[] {
  const messages: ChatlogMessage[] = [
    { role: 'system', content: 'system prompt', source: 'request' },
  ]
  for (let turn = 1; turn <= pairs; turn++) {
    messages.push(
      { role: 'user', content: `question ${turn}`, source: 'request' },
      { role: 'assistant', content: `answer ${turn}`, source: 'request' }
    )
  }
  messages.push(
    { role: 'user', content: 'latest question', source: 'request' },
    { role: 'assistant', content: 'latest answer', source: 'response' }
  )
  return messages
}

async function openDialog(record: ChatlogRecord) {
  get.mockResolvedValue({ data: { success: true, message: '', data: record } })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <ConversationButton requestId='req-long' logType={2} isAdmin />
    </QueryClientProvider>
  )
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'View conversation' }))
  return { user, dialog: await screen.findByRole('dialog') }
}

beforeEach(() => get.mockReset())

test.each([
  { name: 'an empty conversation', messages: [], expected: 0 },
  { name: 'a short conversation', messages: agentHistory(3), expected: 0 },
  {
    name: 'a long history ending in a short turn',
    messages: agentHistory(40),
    // 1 system + 80 history messages precede the latest user question.
    expected: 81,
  },
])('$name starts at message $expected', ({ messages, expected }) => {
  expect(latestTurnStart(messages)).toBe(expected)
})

test('a latest turn longer than the limit only keeps its newest messages', () => {
  const toolLoop: ChatlogMessage[] = [
    { role: 'user', content: 'do the task', source: 'request' },
  ]
  for (let step = 0; step < 60; step++) {
    toolLoop.push({
      role: 'tool',
      content: `result ${step}`,
      source: 'request',
    })
  }

  expect(latestTurnStart(toolLoop)).toBe(
    toolLoop.length - INITIAL_MESSAGE_LIMIT
  )
})

test('a long agent history opens at the latest turn and reveals earlier messages on demand', async () => {
  const { user, dialog } = await openDialog(recordWith(agentHistory(40)))

  expect(await within(dialog).findByText('latest question')).toBeVisible()
  expect(within(dialog).getByText('latest answer')).toBeVisible()
  expect(within(dialog).queryByText('answer 40')).not.toBeInTheDocument()
  expect(within(dialog).getAllByRole('article')).toHaveLength(2)

  await user.click(
    within(dialog).getByRole('button', {
      name: `Show ${EARLIER_MESSAGES_STEP} earlier messages (81 hidden)`,
    })
  )

  expect(within(dialog).getByText('answer 40')).toBeVisible()
  expect(within(dialog).queryByText('system prompt')).not.toBeInTheDocument()
  expect(within(dialog).getAllByRole('article')).toHaveLength(
    2 + EARLIER_MESSAGES_STEP
  )

  await user.click(
    within(dialog).getByRole('button', {
      name: 'Show 31 earlier messages (31 hidden)',
    })
  )

  expect(within(dialog).getByText('system prompt')).toBeVisible()
  expect(
    within(dialog).queryByRole('button', { name: /earlier messages/ })
  ).not.toBeInTheDocument()
})

test('a message over the limit shows its head until it is expanded', async () => {
  const head = 'H'.repeat(LONG_TEXT_LIMIT)
  const { user, dialog } = await openDialog(
    recordWith([
      { role: 'user', content: `${head}TAIL-MARKER`, source: 'request' },
      { role: 'assistant', content: 'short answer', source: 'response' },
    ])
  )

  const expand = await within(dialog).findByRole('button', {
    name: /^Show all \(2,011 characters\)$/,
  })
  expect(expand).toHaveAttribute('aria-expanded', 'false')
  expect(within(dialog).queryByText(/TAIL-MARKER/)).not.toBeInTheDocument()
  expect(within(dialog).getByText(`${head}…`)).toBeVisible()

  await user.click(expand)

  expect(within(dialog).getByText(/TAIL-MARKER/)).toBeVisible()
  const collapse = within(dialog).getByRole('button', { name: 'Collapse' })
  expect(collapse).toHaveAttribute('aria-expanded', 'true')

  await user.click(collapse)

  expect(within(dialog).queryByText(/TAIL-MARKER/)).not.toBeInTheDocument()
})

test('the raw tab offers the request and response as downloads', async () => {
  const { user, dialog } = await openDialog(recordWith(agentHistory(1)))

  await user.click(await within(dialog).findByRole('tab', { name: 'Raw JSON' }))

  expect(
    within(dialog).getByRole('button', { name: 'Download request JSON' })
  ).toBeVisible()
  expect(
    within(dialog).getByRole('button', { name: 'Download response JSON' })
  ).toBeVisible()
})

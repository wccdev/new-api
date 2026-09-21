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
import { messageMatches, splitMatches } from '../conversation-search'
import type { ChatlogMessage, ChatlogRecord } from '../types'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: { get } }))

function recordWith(messages: ChatlogMessage[]): ChatlogRecord {
  return {
    request_id: 'req-search',
    created_at: 1_700_000_000,
    protocol: 'openai.chat',
    path: '/v1/chat/completions',
    stream: false,
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
    request: {},
    response: {},
  }
}

/** 60 filler turns push the first messages out of the initially shown window. */
function longHistory(): ChatlogMessage[] {
  const messages: ChatlogMessage[] = [
    {
      role: 'system',
      content: 'Deploy with Kubernetes only.',
      source: 'request',
    },
  ]
  for (let turn = 1; turn <= 60; turn++) {
    messages.push(
      { role: 'user', content: `filler question ${turn}`, source: 'request' },
      { role: 'assistant', content: `filler answer ${turn}`, source: 'request' }
    )
  }
  messages.push(
    { role: 'user', content: 'latest question', source: 'request' },
    {
      role: 'assistant',
      content: 'Calling a tool.',
      tool_calls: [
        {
          id: 'call-9',
          name: 'run_shell',
          arguments: '{"cmd":"kubectl get pods"}',
        },
      ],
      source: 'response',
    }
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
      <ConversationButton requestId='req-search' logType={2} isAdmin />
    </QueryClientProvider>
  )
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'View conversation' }))
  const dialog = await screen.findByRole('dialog')
  const search = await within(dialog).findByRole('textbox', {
    name: 'Search in conversation',
  })
  return { user, dialog, search }
}

beforeEach(() => get.mockReset())

test.each([
  {
    name: 'an empty query leaves the text whole',
    text: 'Hello',
    query: '  ',
    expected: [{ text: 'Hello', isMatch: false }],
  },
  {
    name: 'matches ignore case and keep the original casing',
    text: 'Kube kube KUBE!',
    query: 'kube',
    expected: [
      { text: 'Kube', isMatch: true },
      { text: ' ', isMatch: false },
      { text: 'kube', isMatch: true },
      { text: ' ', isMatch: false },
      { text: 'KUBE', isMatch: true },
      { text: '!', isMatch: false },
    ],
  },
  {
    name: 'no occurrence yields a single plain segment',
    text: 'Hello',
    query: 'xyz',
    expected: [{ text: 'Hello', isMatch: false }],
  },
])('splitMatches: $name', ({ text, query, expected }) => {
  expect(splitMatches(text, query)).toEqual(expected)
})

test('a message matches on content, reasoning, tool name, tool arguments and tool call id', () => {
  const message: ChatlogMessage = {
    role: 'assistant',
    content: 'Visible answer',
    reasoning: 'hidden thought',
    tool_call_id: 'call-77',
    tool_calls: [{ name: 'run_shell', arguments: '{"cmd":"ls"}' }],
    source: 'response',
  }

  for (const query of [
    'VISIBLE',
    'thought',
    'run_sh',
    '"cmd"',
    'call-77',
    '',
  ]) {
    expect(messageMatches(message, query)).toBe(true)
  }
  expect(messageMatches(message, 'absent')).toBe(false)
})

test('searching finds messages hidden in the earlier history and marks the matches', async () => {
  const { user, dialog, search } = await openDialog(recordWith(longHistory()))
  expect(within(dialog).queryByText(/Deploy with/)).not.toBeInTheDocument()

  await user.type(search, 'kubernetes')

  const status = await within(dialog).findByRole('status')
  expect(status).toHaveTextContent('1 of 123 messages')
  const articles = within(dialog).getAllByRole('article')
  expect(articles).toHaveLength(1)
  expect(within(articles[0]).getByText('System')).toBeVisible()
  expect(within(articles[0]).getByText('#1')).toBeVisible()
  const mark = within(articles[0]).getByText('Kubernetes')
  expect(mark.tagName).toBe('MARK')

  await user.click(within(dialog).getByRole('button', { name: 'Clear search' }))

  expect(within(dialog).queryByRole('status')).not.toBeInTheDocument()
  expect(within(dialog).getByText('latest question')).toBeVisible()
  expect(within(dialog).queryByText(/Deploy with/)).not.toBeInTheDocument()
})

test('a match inside tool call arguments opens that tool call', async () => {
  const { user, dialog, search } = await openDialog(recordWith(longHistory()))

  await user.type(search, 'kubectl')

  const toolCall = await within(dialog).findByRole('button', {
    name: /run_shell/,
  })
  expect(toolCall).toHaveAttribute('aria-expanded', 'true')
  expect(within(dialog).getByText('kubectl').tagName).toBe('MARK')
})

test('a match deep inside a long message is shown without expanding it', async () => {
  const padding = 'x'.repeat(LONG_TEXT_LIMIT * 2)
  const { user, dialog, search } = await openDialog(
    recordWith([
      {
        role: 'user',
        content: `${padding} NEEDLE ${padding}`,
        source: 'request',
      },
      { role: 'assistant', content: 'ok', source: 'response' },
    ])
  )

  await user.type(search, 'needle')

  expect((await within(dialog).findByText('NEEDLE')).tagName).toBe('MARK')
  expect(
    within(dialog).getByRole('button', { name: /^Show all/ })
  ).toHaveAttribute('aria-expanded', 'false')
})

test('the role filter narrows the list and a query without matches says so', async () => {
  const { user, dialog, search } = await openDialog(
    recordWith([
      { role: 'system', content: 'sys', source: 'request' },
      { role: 'user', content: 'hello', source: 'request' },
      {
        role: 'tool',
        content: 'tool output',
        tool_call_id: 'c1',
        source: 'request',
      },
      { role: 'assistant', content: 'bye', source: 'response' },
    ])
  )
  const filter = within(dialog).getByRole('group', { name: 'Filter by role' })

  await user.click(within(filter).getByRole('button', { name: /^Tool/ }))

  expect(within(dialog).getAllByRole('article')).toHaveLength(1)
  expect(within(dialog).getByText('tool output')).toBeVisible()
  expect(within(dialog).getByRole('status')).toHaveTextContent(
    '1 of 4 messages'
  )

  await user.type(search, 'nothing-like-this')

  expect(await within(dialog).findByText('No results found')).toBeVisible()
  expect(within(dialog).queryByRole('article')).not.toBeInTheDocument()
})

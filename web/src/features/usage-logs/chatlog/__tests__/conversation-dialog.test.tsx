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
import i18next from 'i18next'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { ConversationButton } from '../conversation-button'
import type { ChatlogRecord } from '../types'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: { get } }))

const record: ChatlogRecord = {
  request_id: 'req-1',
  created_at: 1_700_000_000,
  protocol: 'openai.chat',
  path: '/v1/chat/completions',
  stream: true,
  status: 200,
  latency_ms: 1234,
  user_id: 7,
  username: 'alice',
  token_id: 3,
  token_name: 'dev-token',
  group: 'default',
  channel_id: 5,
  model: 'gpt-test',
  truncated: true,
  messages: [
    { role: 'system', content: 'You are terse.', source: 'request' },
    { role: 'user', content: 'What is the weather?', source: 'request' },
    {
      role: 'assistant',
      content: 'Checking the forecast.',
      reasoning: 'The user wants weather data.',
      tool_calls: [
        { id: 'call-1', name: 'get_weather', arguments: '{"city":"Paris"}' },
      ],
      source: 'response',
    },
  ],
  request: { model: 'gpt-test', stream: true },
  response: { id: 'chatcmpl-1' },
}

function renderButton(props: {
  isAdmin: boolean
  requestId?: string
  logType?: number
}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ConversationButton
        requestId={'requestId' in props ? props.requestId : 'req-1'}
        logType={props.logType ?? 2}
        isAdmin={props.isAdmin}
      />
    </QueryClientProvider>
  )
}

async function openConversation() {
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'View conversation' }))
  return { user, dialog: await screen.findByRole('dialog') }
}

beforeEach(() => get.mockReset())
afterEach(async () => {
  await i18next.changeLanguage('en')
})

test('opening the conversation shows request and response messages by role', async () => {
  get.mockResolvedValue({ data: { success: true, message: '', data: record } })
  renderButton({ isAdmin: true })

  const { dialog } = await openConversation()

  const system = (await within(dialog).findByText('You are terse.')).closest(
    'article'
  )
  const assistant = (
    await within(dialog).findByText('Checking the forecast.')
  ).closest('article')
  expect(within(system as HTMLElement).getByText('System')).toBeVisible()
  expect(system).toHaveAttribute('data-source', 'request')
  expect(within(assistant as HTMLElement).getByText('Assistant')).toBeVisible()
  expect(assistant).toHaveAttribute('data-source', 'response')
  expect(within(dialog).getByText('What is the weather?')).toBeVisible()
  expect(within(dialog).getByText('gpt-test')).toBeVisible()
  expect(within(dialog).getByText('alice')).toBeVisible()
  expect(within(dialog).getByText('Truncated')).toBeVisible()
})

test('reasoning and tool calls stay collapsed until they are toggled', async () => {
  get.mockResolvedValue({ data: { success: true, message: '', data: record } })
  renderButton({ isAdmin: true })

  const { user, dialog } = await openConversation()
  const reasoning = await within(dialog).findByRole('button', {
    name: 'Reasoning',
  })
  const toolCall = within(dialog).getByRole('button', { name: /get_weather/ })

  expect(reasoning).toHaveAttribute('aria-expanded', 'false')
  expect(
    within(dialog).queryByText('The user wants weather data.')
  ).not.toBeInTheDocument()

  await user.click(reasoning)
  await user.click(toolCall)

  expect(reasoning).toHaveAttribute('aria-expanded', 'true')
  expect(within(dialog).getByText('The user wants weather data.')).toBeVisible()
  expect(within(dialog).getByText(/"city": "Paris"/)).toBeVisible()
})

test('the raw JSON tab shows the recorded request and response', async () => {
  get.mockResolvedValue({ data: { success: true, message: '', data: record } })
  renderButton({ isAdmin: true })

  const { user, dialog } = await openConversation()
  await user.click(await within(dialog).findByRole('tab', { name: 'Raw JSON' }))

  expect(within(dialog).getByText(/"model": "gpt-test"/)).toBeVisible()
  expect(within(dialog).getByText(/"id": "chatcmpl-1"/)).toBeVisible()
})

test('a request without a record shows the empty state instead of an error', async () => {
  get.mockResolvedValue({ data: { success: true, message: '', data: null } })
  renderButton({ isAdmin: true })

  const { dialog } = await openConversation()

  expect(
    await within(dialog).findByText('No conversation record for this request')
  ).toBeVisible()
  expect(
    within(dialog).queryByText('Failed to load the conversation record')
  ).not.toBeInTheDocument()
})

test('a failed business response shows the server reason with a retry action', async () => {
  get.mockResolvedValue({
    data: { success: false, message: 'record storage offline', data: null },
  })
  renderButton({ isAdmin: true })

  const { dialog } = await openConversation()

  expect(
    await within(dialog).findByText('Failed to load the conversation record')
  ).toBeVisible()
  expect(within(dialog).getByText('record storage offline')).toBeVisible()
  expect(within(dialog).getByRole('button', { name: 'Retry' })).toBeVisible()
})

test.each([
  { isAdmin: true, url: '/api/chatlog/req%2F1' },
  { isAdmin: false, url: '/api/chatlog/self/req%2F1' },
])(
  'isAdmin=$isAdmin loads the record from $url and only after opening',
  async ({ isAdmin, url }) => {
    get.mockResolvedValue({ data: { success: true, message: '', data: null } })
    renderButton({ isAdmin, requestId: 'req/1' })
    expect(get).not.toHaveBeenCalled()

    const { dialog } = await openConversation()
    await within(dialog).findByText('No conversation record for this request')

    expect(get).toHaveBeenCalledTimes(1)
    expect(get).toHaveBeenCalledWith(url)
  }
)

test('non-admin viewers do not see the username of the record', async () => {
  get.mockResolvedValue({ data: { success: true, message: '', data: record } })
  renderButton({ isAdmin: false })

  const { dialog } = await openConversation()
  await within(dialog).findByText('dev-token')

  expect(within(dialog).queryByText('alice')).not.toBeInTheDocument()
})

test.each([
  { name: 'a top-up log', logType: 1, requestId: 'req-1' },
  { name: 'a relay log without a request id', logType: 2, requestId: '' },
])('$name has no conversation button', ({ logType, requestId }) => {
  renderButton({ isAdmin: true, logType, requestId })

  expect(
    screen.queryByRole('button', { name: 'View conversation' })
  ).not.toBeInTheDocument()
})

test('an error log offers the conversation button', () => {
  renderButton({ isAdmin: true, logType: 5 })

  expect(
    screen.getByRole('button', { name: 'View conversation' })
  ).toBeVisible()
})

test('switching to Simplified Chinese uses the feature-local translations', async () => {
  await i18next.changeLanguage('zhCN')
  renderButton({ isAdmin: true })

  expect(screen.getByRole('button', { name: '查看对话' })).toBeVisible()
})

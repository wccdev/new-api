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
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { markManagedKey } from '../../gcwork-managed'
import { apiKeySchema } from '../../types'
import { ManagedKeyActions } from '../gcwork-managed-key'

const key = (id: number) =>
  apiKeySchema.parse({
    id,
    name: `key-${id}`,
    key: 'demo********1234',
    status: 1,
    remain_quota: 0,
    used_quota: 0,
    unlimited_quota: true,
    expired_time: -1,
    created_time: 0,
    accessed_time: 0,
    model_limits_enabled: false,
  })
const listing = () => ({
  success: true,
  data: { items: [key(1), key(2)], total: 2, page: 1, page_size: 10 },
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

it('flags only the token GC Work manages', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { token_id: 2 } },
  })
  const { data } = await markManagedKey(listing())
  expect(data?.items.map((item) => item.app_managed)).toEqual([false, true])
})

it('leaves the listing usable when the flag is unavailable', async () => {
  vi.spyOn(api, 'get').mockRejectedValue(new Error('offline'))
  const { data } = await markManagedKey(listing())
  expect(data?.items.map((item) => item.id)).toEqual([1, 2])
})

it('offers no working action on a managed key', () => {
  render(<ManagedKeyActions />)
  for (const button of screen.getAllByRole('button')) {
    expect(button.hasAttribute('disabled')).toBe(true)
  }
})

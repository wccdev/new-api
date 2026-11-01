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
import { api } from '@/lib/api'

import type { GetApiKeysResponse } from './types'

/**
 * Flag the token GC Work manages for the signed-in user. The server enforces the
 * protection, so an unavailable flag only leaves the row looking editable.
 */
export async function markManagedKey(
  response: GetApiKeysResponse
): Promise<GetApiKeysResponse> {
  const items = response.data?.items
  if (!response.data || !items) return response
  try {
    const res = await api.get('/api/gcwork/self/managed-token')
    const tokenId: unknown = res.data?.data?.token_id
    return {
      ...response,
      data: {
        ...response.data,
        items: items.map((item) => ({
          ...item,
          app_managed: item.id === tokenId,
        })),
      },
    }
  } catch {
    return response
  }
}

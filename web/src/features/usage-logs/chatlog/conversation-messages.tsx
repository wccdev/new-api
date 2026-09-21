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
import { Search, X } from 'lucide-react'
import { useDeferredValue, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from '@/components/ui/input-group'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

import { ConversationMessage } from './conversation-message'
import { messageMatches } from './conversation-search'
import { EARLIER_MESSAGES_STEP, latestTurnStart } from './conversation-window'
import './i18n'
import { withOccurrenceKeys } from './occurrence-keys'
import type { ChatlogMessage, ChatlogRole } from './types'

const ROLE_FILTERS: { role: ChatlogRole; labelKey: string }[] = [
  { role: 'system', labelKey: 'System' },
  { role: 'user', labelKey: 'User' },
  { role: 'assistant', labelKey: 'Assistant' },
  { role: 'tool', labelKey: 'Tool' },
]
const ALL_ROLES = 'all'

interface NumberedMessage {
  key: string
  number: number
  message: ChatlogMessage
}

/**
 * Renders the tail of a message list and reveals the rest in steps, so the DOM
 * stays small for agent histories. It is remounted whenever the filter changes,
 * which resets how much has been revealed.
 */
function MessageWindow(props: {
  messages: NumberedMessage[]
  initialStart: number
  query: string
}) {
  const { t } = useTranslation()
  const [start, setStart] = useState(props.initialStart)
  const revealCount = Math.min(EARLIER_MESSAGES_STEP, start)

  return (
    <div className='min-w-0 space-y-2'>
      {start > 0 && (
        <Button
          type='button'
          variant='outline'
          size='sm'
          className='w-full border-dashed'
          onClick={() => setStart(start - revealCount)}
        >
          {t('Show {{shown}} earlier messages ({{hidden}} hidden)', {
            shown: revealCount,
            hidden: start,
          })}
        </Button>
      )}
      {props.messages.slice(start).map((entry) => (
        <ConversationMessage
          key={entry.key}
          message={entry.message}
          number={entry.number}
          query={props.query}
        />
      ))}
    </div>
  )
}

export function ConversationMessages(props: { messages: ChatlogMessage[] }) {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const [role, setRole] = useState<string>(ALL_ROLES)
  // Typing stays responsive while a large history is being filtered.
  const query = useDeferredValue(search)

  const numbered = useMemo<NumberedMessage[]>(
    () =>
      withOccurrenceKeys(
        props.messages,
        (message) =>
          `${message.role}:${message.source}:${(message.content ?? '').slice(0, 48)}`
      ).map((entry, index) => ({
        key: entry.key,
        number: index + 1,
        message: entry.item,
      })),
    [props.messages]
  )
  const roleCounts = useMemo(() => {
    const counts = new Map<string, number>()
    for (const message of props.messages) {
      counts.set(message.role, (counts.get(message.role) ?? 0) + 1)
    }
    return counts
  }, [props.messages])
  const isFiltering = query.trim() !== '' || role !== ALL_ROLES
  const visible = useMemo(
    () =>
      isFiltering
        ? numbered.filter(
            (entry) =>
              (role === ALL_ROLES || entry.message.role === role) &&
              messageMatches(entry.message, query)
          )
        : numbered,
    [numbered, isFiltering, role, query]
  )
  // Unfiltered, a long history opens at its latest turn; filtered, at the
  // newest matches.
  const initialStart = isFiltering
    ? Math.max(visible.length - EARLIER_MESSAGES_STEP, 0)
    : latestTurnStart(props.messages)

  return (
    <div className='min-w-0 space-y-2'>
      <div className='bg-popover sticky top-0 z-10 -mx-1 flex flex-wrap items-center gap-2 px-1 py-1.5'>
        <InputGroup className='h-8 min-w-40 flex-1'>
          <InputGroupAddon>
            <Search aria-hidden='true' />
          </InputGroupAddon>
          <InputGroupInput
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder={t('Search in conversation')}
            aria-label={t('Search in conversation')}
          />
          {search && (
            <InputGroupAddon align='inline-end'>
              <InputGroupButton
                size='icon-xs'
                aria-label={t('Clear search')}
                onClick={() => setSearch('')}
              >
                <X aria-hidden='true' />
              </InputGroupButton>
            </InputGroupAddon>
          )}
        </InputGroup>
        <ToggleGroup
          value={[role]}
          onValueChange={(values) => setRole(values[0] ?? ALL_ROLES)}
          variant='outline'
          size='sm'
          aria-label={t('Filter by role')}
        >
          <ToggleGroupItem value={ALL_ROLES}>
            {t('All')}
            <span className='text-muted-foreground tabular-nums'>
              {props.messages.length}
            </span>
          </ToggleGroupItem>
          {ROLE_FILTERS.filter((filter) => roleCounts.has(filter.role)).map(
            (filter) => (
              <ToggleGroupItem key={filter.role} value={filter.role}>
                {t(filter.labelKey)}
                <span className='text-muted-foreground tabular-nums'>
                  {roleCounts.get(filter.role)}
                </span>
              </ToggleGroupItem>
            )
          )}
        </ToggleGroup>
      </div>

      {isFiltering && (
        <p className='text-muted-foreground text-xs' role='status'>
          {t('{{matched}} of {{total}} messages', {
            matched: visible.length,
            total: props.messages.length,
          })}
        </p>
      )}
      {props.messages.length === 0 && (
        <p className='text-muted-foreground py-6 text-center text-sm'>
          {t('No parsed messages. Check the Raw JSON tab.')}
        </p>
      )}
      {isFiltering && visible.length === 0 && (
        <p className='text-muted-foreground py-6 text-center text-sm'>
          {t('No results found')}
        </p>
      )}
      <MessageWindow
        key={`${role}|${query}`}
        messages={visible}
        initialStart={initialStart}
        query={query}
      />
    </div>
  )
}

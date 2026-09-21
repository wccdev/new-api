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
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import './i18n'

/** Messages longer than this open collapsed to keep long agent contexts light. */
export const LONG_TEXT_LIMIT = 2000
const EXCERPT_LEAD = 200

interface LongTextProps {
  text: string
  /** Renders the visible text; receives the head while collapsed. */
  children: (text: string, isComplete: boolean) => ReactNode
  limit?: number
  /** While collapsed, show the text around this position instead of its head. */
  focusIndex?: number
}

/**
 * Renders only the head of a long text until the viewer asks for all of it, so
 * a multi-megabyte prompt never reaches the DOM unless it is wanted.
 */
export function LongText(props: LongTextProps) {
  const { t, i18n } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const limit = props.limit ?? LONG_TEXT_LIMIT

  if (props.text.length <= limit) return <>{props.children(props.text, true)}</>

  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const focus = props.focusIndex ?? -1
  // Keep some context before a search hit that lies beyond the head.
  const excerptStart = focus > limit / 2 ? focus - EXCERPT_LEAD : 0
  const excerptEnd = Math.min(excerptStart + limit, props.text.length)
  const excerpt = [
    excerptStart > 0 ? '…' : '',
    props.text.slice(excerptStart, excerptEnd),
    excerptEnd < props.text.length ? '…' : '',
  ].join('')
  return (
    <div className='min-w-0 space-y-1'>
      {expanded
        ? props.children(props.text, true)
        : props.children(excerpt, false)}
      <Button
        type='button'
        variant='link'
        size='xs'
        className='h-auto px-0'
        aria-expanded={expanded}
        onClick={() => setExpanded(!expanded)}
      >
        {expanded
          ? t('Collapse')
          : t('Show all ({{total}} characters)', {
              total: formatNumber(props.text.length, locale),
            })}
      </Button>
    </div>
  )
}

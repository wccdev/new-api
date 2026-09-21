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
import {
  Clock,
  KeyRound,
  Timer,
  User,
  Waypoints,
  type LucideIcon,
} from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber, formatTimestamp } from '@/lib/format'

import type { ChatlogRecord } from './types'
import './i18n'

function MetaItem(props: {
  icon: LucideIcon
  label: string
  children: ReactNode
}) {
  const Icon = props.icon
  return (
    <div className='flex min-w-0 items-center gap-1.5'>
      <Icon
        aria-hidden='true'
        className='text-muted-foreground/70 size-3.5 shrink-0'
      />
      <dt className='text-muted-foreground shrink-0'>{props.label}</dt>
      <dd className='min-w-0 font-medium [overflow-wrap:anywhere]'>
        {props.children}
      </dd>
    </div>
  )
}

export function ConversationMeta(props: {
  record: ChatlogRecord
  isAdmin: boolean
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const record = props.record
  const isSuccess = record.status >= 200 && record.status < 300

  return (
    <div className='bg-muted/40 space-y-2.5 rounded-lg border p-3'>
      <div className='flex flex-wrap items-center gap-x-2 gap-y-1.5'>
        <span className='min-w-0 font-mono text-sm font-semibold [overflow-wrap:anywhere]'>
          {record.model}
        </span>
        <StatusBadge
          label={String(record.status)}
          variant={isSuccess ? 'success' : 'danger'}
          size='sm'
          copyable={false}
          aria-label={`${t('Status')}: ${record.status}`}
        />
        <StatusBadge
          label={record.stream ? t('Stream') : t('Non-stream')}
          variant='neutral'
          size='sm'
          copyable={false}
        />
        {record.truncated && (
          <StatusBadge
            label={t('Truncated')}
            variant='warning'
            size='sm'
            copyable={false}
            title={t('This record exceeded the size limit and was truncated')}
          />
        )}
      </div>
      <dl className='flex flex-wrap gap-x-5 gap-y-1.5 text-xs'>
        <MetaItem icon={Waypoints} label={t('Protocol')}>
          <span className='font-mono'>{record.protocol}</span>
        </MetaItem>
        <MetaItem icon={Clock} label={t('Time')}>
          {formatTimestamp(record.created_at)}
        </MetaItem>
        <MetaItem icon={Timer} label={t('Latency')}>
          {formatNumber(record.latency_ms, locale)} ms
        </MetaItem>
        <MetaItem icon={KeyRound} label={t('Token')}>
          {record.token_name || '-'}
        </MetaItem>
        {props.isAdmin && (
          <MetaItem icon={User} label={t('Username')}>
            {record.username || '-'}
          </MetaItem>
        )}
      </dl>
    </div>
  )
}

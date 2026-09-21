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
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'

import type { ChatlogRecord } from './types'
import './i18n'

function RawSection(props: { title: string; copyLabel: string; text: string }) {
  return (
    <section className='min-w-0 space-y-1.5'>
      <div className='flex items-center justify-between gap-2'>
        <h3 className='text-sm font-medium'>{props.title}</h3>
        <CopyButton
          value={props.text}
          className='size-6'
          iconClassName='size-3'
          aria-label={props.copyLabel}
        />
      </div>
      <pre className='bg-muted max-h-[50dvh] overflow-auto rounded-md border p-3 font-mono text-xs [overflow-wrap:anywhere] whitespace-pre-wrap'>
        {props.text}
      </pre>
    </section>
  )
}

export function ConversationRaw(props: { record: ChatlogRecord }) {
  const { t } = useTranslation()
  const request = props.record.request
  const response = props.record.response
  const responseRaw = props.record.response_raw

  const requestText = useMemo(
    () => JSON.stringify(request ?? null, null, 2),
    [request]
  )
  // A response that could not be reassembled is only available as raw text.
  const responseText = useMemo(
    () =>
      response == null && responseRaw
        ? responseRaw
        : JSON.stringify(response ?? null, null, 2),
    [response, responseRaw]
  )

  return (
    <div className='space-y-4'>
      <RawSection
        title={t('Request')}
        copyLabel={t('Copy request JSON')}
        text={requestText}
      />
      <RawSection
        title={t('Response')}
        copyLabel={t('Copy response JSON')}
        text={responseText}
      />
    </div>
  )
}

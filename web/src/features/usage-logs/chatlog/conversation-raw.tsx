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
import { Download } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Button } from '@/components/ui/button'

import { LongText } from './conversation-long-text'
import type { ChatlogRecord } from './types'
import './i18n'

/** A raw body above this size is rendered head-first; the rest is on demand. */
const RAW_RENDER_LIMIT = 200_000

function downloadText(fileName: string, text: string): void {
  const url = URL.createObjectURL(
    new Blob([text], { type: 'application/json' })
  )
  const link = document.createElement('a')
  link.href = url
  link.download = fileName
  link.click()
  URL.revokeObjectURL(url)
}

function RawSection(props: {
  title: string
  copyLabel: string
  downloadLabel: string
  fileName: string
  text: string
}) {
  return (
    <section className='min-w-0 space-y-1.5'>
      <div className='flex items-center justify-between gap-2'>
        <h3 className='text-sm font-medium'>{props.title}</h3>
        <div className='flex items-center gap-1'>
          <Button
            type='button'
            variant='ghost'
            size='icon-xs'
            aria-label={props.downloadLabel}
            onClick={() => downloadText(props.fileName, props.text)}
          >
            <Download aria-hidden='true' className='size-3' />
          </Button>
          <CopyButton
            value={props.text}
            className='size-6'
            iconClassName='size-3'
            aria-label={props.copyLabel}
          />
        </div>
      </div>
      <LongText text={props.text} limit={RAW_RENDER_LIMIT}>
        {(text) => (
          <pre className='bg-muted max-h-[50dvh] overflow-auto rounded-md border p-3 font-mono text-xs [overflow-wrap:anywhere] whitespace-pre-wrap'>
            {text}
          </pre>
        )}
      </LongText>
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
        downloadLabel={t('Download request JSON')}
        fileName={`${props.record.request_id}-request.json`}
        text={requestText}
      />
      <RawSection
        title={t('Response')}
        copyLabel={t('Copy response JSON')}
        downloadLabel={t('Download response JSON')}
        fileName={`${props.record.request_id}-response.json`}
        text={responseText}
      />
    </div>
  )
}

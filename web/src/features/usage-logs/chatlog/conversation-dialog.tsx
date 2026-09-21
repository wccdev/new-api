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
import { useQuery } from '@tanstack/react-query'
import { MessagesSquare } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  getServerErrorMessage,
  requireServerSuccess,
} from '@/lib/server-error-message'

import { getChatlogRecord } from './api'
import { ConversationMessage } from './conversation-message'
import { ConversationMeta } from './conversation-meta'
import { ConversationRaw } from './conversation-raw'
import { withOccurrenceKeys } from './occurrence-keys'
import './i18n'
import type { ChatlogRecord } from './types'

interface ConversationDialogProps {
  requestId: string
  isAdmin: boolean
  open: boolean
  onOpenChange: (open: boolean) => void
}

function ConversationRecord(props: {
  record: ChatlogRecord
  isAdmin: boolean
}) {
  const { t } = useTranslation()
  const messages = props.record.messages ?? []

  return (
    <div className='min-w-0 space-y-3'>
      <ConversationMeta record={props.record} isAdmin={props.isAdmin} />
      <Tabs defaultValue='conversation' className='min-w-0'>
        <TabsList>
          <TabsTrigger value='conversation'>{t('Conversation')}</TabsTrigger>
          <TabsTrigger value='raw'>{t('Raw JSON')}</TabsTrigger>
        </TabsList>
        <TabsContent value='conversation' className='min-w-0 space-y-2'>
          {messages.length === 0 && (
            <p className='text-muted-foreground py-6 text-center text-sm'>
              {t('No parsed messages. Check the Raw JSON tab.')}
            </p>
          )}
          {withOccurrenceKeys(
            messages,
            (message) =>
              `${message.role}:${message.source}:${(message.content ?? '').slice(0, 48)}`
          ).map((message) => (
            <ConversationMessage key={message.key} message={message.item} />
          ))}
        </TabsContent>
        <TabsContent value='raw' className='min-w-0'>
          <ConversationRaw record={props.record} />
        </TabsContent>
      </Tabs>
    </div>
  )
}

export function ConversationDialog(props: ConversationDialogProps) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['chatlog', props.isAdmin ? 'admin' : 'self', props.requestId],
    queryFn: async () =>
      requireServerSuccess(
        await getChatlogRecord(props.requestId, props.isAdmin)
      ),
    enabled: props.open,
    staleTime: 60_000,
    // The failure is shown inside the dialog instead of a toast.
    meta: { errorToast: false },
  })
  const record = query.data?.data ?? null

  let body = <LoadingState />
  if (query.isError) {
    body = (
      <ErrorState
        title={t('Failed to load the conversation record')}
        description={getServerErrorMessage(query.error)}
        onRetry={() => void query.refetch()}
      />
    )
  } else if (record) {
    body = <ConversationRecord record={record} isAdmin={props.isAdmin} />
  } else if (query.isSuccess) {
    body = (
      <EmptyState
        icon={MessagesSquare}
        title={t('No conversation record for this request')}
        description={t(
          'Conversation recording may be disabled, or the record has expired.'
        )}
      />
    )
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Conversation')}
      description={t('View the full conversation recorded for this request')}
      contentClassName='min-w-0 overflow-hidden max-sm:max-h-(--dialog-available-height) max-sm:w-[calc(100vw-1.5rem)] max-sm:max-w-[calc(100vw-1.5rem)] max-sm:p-4 sm:max-w-3xl lg:max-w-4xl'
      descriptionClassName='sr-only'
      contentHeight='min(76dvh, 760px)'
      bodyClassName='pr-2 sm:pr-4'
    >
      {body}
    </Dialog>
  )
}

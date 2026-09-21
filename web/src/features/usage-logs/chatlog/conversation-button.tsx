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
import { BubbleChatIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import { LOG_TYPE_ENUM } from '../constants'
import { ConversationDialog } from './conversation-dialog'
import './i18n'

interface ConversationButtonProps {
  requestId?: string
  logType: number
  isAdmin: boolean
}

export function ConversationButton(props: ConversationButtonProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  // Rows stay cheap: the dialog and its query mount on first open, then stay
  // mounted so the close animation can finish.
  const [mounted, setMounted] = useState(false)

  // Conversations are only recorded for relayed requests.
  const isRelayLog =
    props.logType === LOG_TYPE_ENUM.CONSUME ||
    props.logType === LOG_TYPE_ENUM.ERROR
  if (!props.requestId || !isRelayLog) return null

  return (
    <>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              type='button'
              variant='ghost'
              size='icon-sm'
              className='text-foreground/70 hover:text-foreground'
              aria-label={t('View conversation')}
              onClick={() => {
                setMounted(true)
                setOpen(true)
              }}
            />
          }
        >
          <HugeiconsIcon
            icon={BubbleChatIcon}
            strokeWidth={2}
            aria-hidden='true'
          />
        </TooltipTrigger>
        <TooltipContent>{t('View conversation')}</TooltipContent>
      </Tooltip>
      {mounted && (
        <ConversationDialog
          requestId={props.requestId}
          isAdmin={props.isAdmin}
          open={open}
          onOpenChange={setOpen}
        />
      )}
    </>
  )
}

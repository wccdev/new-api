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
import { ChevronRight } from 'lucide-react'
import { lazy, Suspense, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { StatusBadge, type StatusBadgeProps } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { cn } from '@/lib/utils'

import { ImageDialog } from '../components/dialogs/image-dialog'
import { LongText } from './conversation-long-text'
import { withOccurrenceKeys } from './occurrence-keys'
import './i18n'
import type { ChatlogMessage, ChatlogRole, ChatlogToolCall } from './types'

const AssistantMarkdown = lazy(() =>
  import('@/components/ui/markdown').then((module) => ({
    default: module.Markdown,
  }))
)

const ROLE_CONFIG: Record<
  ChatlogRole,
  { labelKey: string; variant: StatusBadgeProps['variant'] }
> = {
  system: { labelKey: 'System', variant: 'neutral' },
  user: { labelKey: 'User', variant: 'info' },
  assistant: { labelKey: 'Assistant', variant: 'success' },
  tool: { labelKey: 'Tool', variant: 'warning' },
}

const plainTextClassName =
  'text-sm leading-relaxed whitespace-pre-wrap [overflow-wrap:anywhere]'
const codeBlockClassName =
  'bg-muted max-h-80 overflow-auto rounded-md border p-2 font-mono text-xs whitespace-pre-wrap [overflow-wrap:anywhere]'

function formatToolArguments(value: string): string {
  try {
    return JSON.stringify(JSON.parse(value), null, 2)
  } catch {
    return value
  }
}

function CollapsibleSection(props: { title: ReactNode; children: ReactNode }) {
  return (
    <Collapsible className='min-w-0'>
      <CollapsibleTrigger className='group/section text-muted-foreground hover:text-foreground focus-visible:ring-ring/50 flex max-w-full items-center gap-1 rounded-sm text-xs font-medium outline-none focus-visible:ring-[3px]'>
        <ChevronRight
          aria-hidden='true'
          className='size-3.5 shrink-0 transition-transform group-data-[panel-open]/section:rotate-90'
        />
        <span className='min-w-0 truncate'>{props.title}</span>
      </CollapsibleTrigger>
      <CollapsibleContent className='pt-1.5'>
        {props.children}
      </CollapsibleContent>
    </Collapsible>
  )
}

function ToolCallSection(props: { toolCall: ChatlogToolCall }) {
  const { t } = useTranslation()
  return (
    <CollapsibleSection
      title={
        <>
          {t('Tool call')}
          <span className='text-foreground ml-1.5 font-mono'>
            {props.toolCall.name}
          </span>
        </>
      }
    >
      <div className='space-y-1'>
        {props.toolCall.id && (
          <p className='text-muted-foreground font-mono text-xs break-all'>
            {props.toolCall.id}
          </p>
        )}
        <LongText text={formatToolArguments(props.toolCall.arguments)}>
          {(text) => <pre className={codeBlockClassName}>{text}</pre>}
        </LongText>
      </div>
    </CollapsibleSection>
  )
}

function MessageImage(props: { src: string; index: number }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const alt = t('Conversation image {{index}}', { index: props.index })
  const image = (
    <img
      src={props.src}
      alt={alt}
      loading='lazy'
      className='size-20 rounded-md border object-cover'
    />
  )
  // ImageDialog prints the image URL, which is unusable for inline data URIs.
  if (!/^https?:\/\//i.test(props.src)) return image

  return (
    <>
      <Button
        type='button'
        variant='ghost'
        className='size-20 p-0'
        aria-label={t('View image {{index}}', { index: props.index })}
        onClick={() => setOpen(true)}
      >
        {image}
      </Button>
      <ImageDialog imageUrl={props.src} open={open} onOpenChange={setOpen} />
    </>
  )
}

function isDisplayableImage(src: string): boolean {
  return /^(https?:\/\/|data:image\/)/i.test(src)
}

export function ConversationMessage(props: { message: ChatlogMessage }) {
  const { t } = useTranslation()
  const message = props.message
  const role = ROLE_CONFIG[message.role] ?? ROLE_CONFIG.user
  const isResponse = message.source === 'response'
  const images = (message.images ?? []).filter(isDisplayableImage)

  return (
    <article
      data-source={message.source}
      className={cn(
        'min-w-0 space-y-2 rounded-lg border p-3',
        isResponse ? 'border-primary/40 bg-primary/5' : 'bg-card'
      )}
    >
      <header className='flex items-center gap-1.5'>
        <StatusBadge
          label={t(role.labelKey)}
          variant={role.variant}
          size='sm'
          copyable={false}
        />
        {isResponse && (
          <StatusBadge
            label={t('Response')}
            variant='neutral'
            size='sm'
            copyable={false}
          />
        )}
        {message.content && (
          <CopyButton
            value={message.content}
            className='ml-auto size-6'
            iconClassName='size-3'
            aria-label={t('Copy message')}
          />
        )}
      </header>

      {message.tool_call_id && (
        <p className='text-muted-foreground text-xs'>
          {t('Tool call ID')}:{' '}
          <span className='font-mono break-all'>{message.tool_call_id}</span>
        </p>
      )}

      {message.reasoning && (
        <CollapsibleSection title={t('Reasoning')}>
          <LongText text={message.reasoning}>
            {(text) => (
              <p className={cn(plainTextClassName, 'text-muted-foreground')}>
                {text}
              </p>
            )}
          </LongText>
        </CollapsibleSection>
      )}

      {message.content && (
        <LongText text={message.content}>
          {(text, isComplete) =>
            // Markdown is only worth its cost for a complete assistant answer;
            // a cut-off head could also end inside a code fence.
            message.role === 'assistant' && isComplete ? (
              <Suspense fallback={<p className={plainTextClassName}>{text}</p>}>
                <AssistantMarkdown breaks>{text}</AssistantMarkdown>
              </Suspense>
            ) : (
              <p className={plainTextClassName}>{text}</p>
            )
          }
        </LongText>
      )}

      {images.length > 0 && (
        <div className='flex flex-wrap gap-2'>
          {withOccurrenceKeys(images, (src) => src.slice(0, 96)).map(
            (image, index) => (
              <MessageImage
                key={image.key}
                src={image.item}
                index={index + 1}
              />
            )
          )}
        </div>
      )}

      {withOccurrenceKeys(
        message.tool_calls ?? [],
        (toolCall) => toolCall.id ?? toolCall.name
      ).map((toolCall) => (
        <ToolCallSection key={toolCall.key} toolCall={toolCall.item} />
      ))}
    </article>
  )
}

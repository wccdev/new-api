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
import { Highlighted } from './conversation-highlight'
import { LongText } from './conversation-long-text'
import { firstMatchIndex } from './conversation-search'
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
  { labelKey: string; variant: StatusBadgeProps['variant']; accent: string }
> = {
  system: {
    labelKey: 'System',
    variant: 'neutral',
    accent: 'border-l-slate-400/70',
  },
  user: { labelKey: 'User', variant: 'info', accent: 'border-l-sky-500/70' },
  assistant: {
    labelKey: 'Assistant',
    variant: 'success',
    accent: 'border-l-emerald-500/70',
  },
  tool: {
    labelKey: 'Tool',
    variant: 'warning',
    accent: 'border-l-amber-500/70',
  },
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

function CollapsibleSection(props: {
  title: ReactNode
  defaultOpen?: boolean
  children: ReactNode
}) {
  return (
    <Collapsible className='min-w-0' defaultOpen={props.defaultOpen}>
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

function ToolCallSection(props: { toolCall: ChatlogToolCall; query: string }) {
  const { t } = useTranslation()
  const argumentsText = formatToolArguments(props.toolCall.arguments)
  const hit =
    firstMatchIndex(props.toolCall.name, props.query) >= 0 ||
    firstMatchIndex(argumentsText, props.query) >= 0
  return (
    <CollapsibleSection
      defaultOpen={hit}
      title={
        <>
          {t('Tool call')}
          <span className='text-foreground ml-1.5 font-mono'>
            <Highlighted text={props.toolCall.name} query={props.query} />
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
        <LongText
          text={argumentsText}
          focusIndex={firstMatchIndex(argumentsText, props.query)}
        >
          {(text) => (
            <pre className={codeBlockClassName}>
              <Highlighted text={text} query={props.query} />
            </pre>
          )}
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

export function ConversationMessage(props: {
  message: ChatlogMessage
  /** 1-based position in the whole conversation. */
  number: number
  query?: string
}) {
  const { t } = useTranslation()
  const message = props.message
  const query = props.query ?? ''
  const isSearching = query.trim() !== ''
  const role = ROLE_CONFIG[message.role] ?? ROLE_CONFIG.user
  const isResponse = message.source === 'response'
  const images = (message.images ?? []).filter(isDisplayableImage)

  return (
    <article
      data-source={message.source}
      className={cn(
        'min-w-0 space-y-2 rounded-lg border border-l-[3px] p-3 shadow-xs',
        role.accent,
        isResponse ? 'bg-primary/5' : 'bg-card'
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
        <span className='text-muted-foreground/70 font-mono text-[11px] tabular-nums'>
          #{props.number}
        </span>
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
          <span className='font-mono break-all'>
            <Highlighted text={message.tool_call_id} query={query} />
          </span>
        </p>
      )}

      {message.reasoning && (
        <CollapsibleSection
          title={t('Reasoning')}
          defaultOpen={firstMatchIndex(message.reasoning, query) >= 0}
        >
          <LongText
            text={message.reasoning}
            focusIndex={firstMatchIndex(message.reasoning, query)}
          >
            {(text) => (
              <p className={cn(plainTextClassName, 'text-muted-foreground')}>
                <Highlighted text={text} query={query} />
              </p>
            )}
          </LongText>
        </CollapsibleSection>
      )}

      {message.content && (
        <LongText
          text={message.content}
          focusIndex={firstMatchIndex(message.content, query)}
        >
          {(text, isComplete) =>
            // Markdown is only worth its cost for a complete assistant answer;
            // a cut-off head could also end inside a code fence. Search results
            // stay plain text so that matches can be marked.
            message.role === 'assistant' && isComplete && !isSearching ? (
              <Suspense fallback={<p className={plainTextClassName}>{text}</p>}>
                <AssistantMarkdown breaks>{text}</AssistantMarkdown>
              </Suspense>
            ) : (
              // Tool results are mostly terminal output and file contents.
              <p
                className={cn(
                  plainTextClassName,
                  message.role === 'tool' && 'font-mono text-xs'
                )}
              >
                <Highlighted text={text} query={query} />
              </p>
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
        <ToolCallSection
          key={toolCall.key}
          toolCall={toolCall.item}
          query={query}
        />
      ))}
    </article>
  )
}

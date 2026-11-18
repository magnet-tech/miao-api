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
  AlignCenter,
  AlignLeft,
  Bold,
  Braces,
  ImagePlus,
  Italic,
  Link2,
  List,
  ListOrdered,
  Underline,
} from 'lucide-react'
import { useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { HtmlContent } from '@/components/html-content'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

const MAX_IMAGE_BYTES = 5 * 1024 * 1024
const IMAGE_TYPES = ['image/png', 'image/jpeg', 'image/gif', 'image/webp']

export const EMAIL_BODY_CLASSES =
  'text-sm [&_a]:text-primary [&_a]:underline [&_h2]:mb-2 [&_h2]:text-lg [&_h2]:font-semibold [&_img]:max-w-full [&_ol]:list-decimal [&_ol]:pl-6 [&_p]:my-2 [&_ul]:list-disc [&_ul]:pl-6'

type RichEmailEditorProps = {
  /** Content set when the editor mounts; remount with a new key to replace it. */
  initialHtml: string
  onChange: (html: string) => void
  ariaLabel: string
  /** Placeholders offered in an "Insert variable" menu, e.g. display_name. */
  variables?: { key: string; label: string }[]
  allowImages?: boolean
}

/** A small formatting editor for email bodies. */
export function RichEmailEditor(props: RichEmailEditorProps) {
  const { t } = useTranslation()
  const editorRef = useRef<HTMLDivElement | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)
  const savedRange = useRef<Range | null>(null)

  const emit = () => props.onChange(editorRef.current?.innerHTML ?? '')

  const restoreSelection = () => {
    editorRef.current?.focus()
    const range = savedRange.current
    const selection = window.getSelection()
    if (range && selection) {
      selection.removeAllRanges()
      selection.addRange(range)
    }
  }

  const exec = (command: string, value?: string) => {
    restoreSelection()
    document.execCommand(command, false, value)
    emit()
  }

  const insertLink = () => {
    const url = window.prompt(t('Link address (https://...)'))
    if (url && /^https?:\/\//.test(url)) exec('createLink', url)
  }

  const insertImage = (file: File | undefined) => {
    if (!file) return
    if (!IMAGE_TYPES.includes(file.type)) {
      toast.error(t('Only PNG, JPEG, GIF and WebP images can be inserted.'))
      return
    }
    if (file.size > MAX_IMAGE_BYTES) {
      toast.error(t('Images can be at most 5 MB in total.'))
      return
    }
    const reader = new FileReader()
    reader.addEventListener('load', () =>
      exec('insertImage', String(reader.result))
    )
    reader.readAsDataURL(file)
  }

  const tools = [
    { icon: Bold, label: t('Bold'), run: () => exec('bold') },
    { icon: Italic, label: t('Italic'), run: () => exec('italic') },
    { icon: Underline, label: t('Underline'), run: () => exec('underline') },
    {
      icon: List,
      label: t('Bulleted list'),
      run: () => exec('insertUnorderedList'),
    },
    {
      icon: ListOrdered,
      label: t('Numbered list'),
      run: () => exec('insertOrderedList'),
    },
    { icon: AlignLeft, label: t('Align left'), run: () => exec('justifyLeft') },
    {
      icon: AlignCenter,
      label: t('Align center'),
      run: () => exec('justifyCenter'),
    },
    { icon: Link2, label: t('Insert link'), run: insertLink },
  ]

  return (
    <div className='rounded-lg border'>
      <div className='flex flex-wrap items-center gap-0.5 border-b p-1'>
        {tools.map((tool) => (
          <Button
            key={tool.label}
            type='button'
            variant='ghost'
            size='icon-sm'
            aria-label={tool.label}
            title={tool.label}
            onMouseDown={(event) => event.preventDefault()}
            onClick={tool.run}
          >
            <tool.icon />
          </Button>
        ))}
        {props.allowImages ? (
          <>
            <Button
              type='button'
              variant='ghost'
              size='icon-sm'
              aria-label={t('Insert image')}
              title={t('Insert image')}
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => fileRef.current?.click()}
            >
              <ImagePlus />
            </Button>
            <input
              ref={fileRef}
              type='file'
              accept={IMAGE_TYPES.join(',')}
              className='hidden'
              onChange={(event) => {
                insertImage(event.target.files?.[0])
                event.target.value = ''
              }}
            />
          </>
        ) : null}
        {props.variables && props.variables.length > 0 ? (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  type='button'
                  variant='ghost'
                  size='sm'
                  className='ml-auto'
                />
              }
            >
              <Braces />
              {t('Insert variable')}
            </DropdownMenuTrigger>
            <DropdownMenuContent align='end' className='w-56'>
              {props.variables.map((variable) => (
                <DropdownMenuItem
                  key={variable.key}
                  onClick={() => exec('insertText', `{{${variable.key}}}`)}
                >
                  <code className='text-xs'>{`{{${variable.key}}}`}</code>
                  <span className='text-muted-foreground ml-auto text-xs'>
                    {variable.label}
                  </span>
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        ) : null}
      </div>
      <div
        ref={(node) => {
          editorRef.current = node
          if (node && !node.innerHTML) node.innerHTML = props.initialHtml
        }}
        role='textbox'
        aria-multiline='true'
        aria-label={props.ariaLabel}
        contentEditable
        suppressContentEditableWarning
        onInput={emit}
        onBlur={() => {
          const selection = window.getSelection()
          if (
            selection &&
            selection.rangeCount > 0 &&
            editorRef.current?.contains(selection.anchorNode)
          ) {
            savedRange.current = selection.getRangeAt(0).cloneRange()
          }
        }}
        className={`min-h-64 p-3 outline-none ${EMAIL_BODY_CLASSES}`}
      />
    </div>
  )
}

/** How an email looks to the recipient: header lines and the sanitized body. */
export function EmailPreview(props: {
  from: string
  to: string
  subject: string
  html: string
}) {
  const { t } = useTranslation()
  return (
    <div className='bg-muted/40 rounded-lg border p-3'>
      <div className='bg-background rounded-md border'>
        <div className='space-y-0.5 border-b px-4 py-3 text-xs'>
          <div className='text-muted-foreground'>
            {t('From')}: {props.from}
          </div>
          <div className='text-muted-foreground'>
            {t('To')}: {props.to}
          </div>
          <div className='text-sm font-semibold'>
            {props.subject || t('(no subject)')}
          </div>
        </div>
        <HtmlContent
          content={props.html}
          className={`px-4 py-3 ${EMAIL_BODY_CLASSES}`}
        />
      </div>
    </div>
  )
}

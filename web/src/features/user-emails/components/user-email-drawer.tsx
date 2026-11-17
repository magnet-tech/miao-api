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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { History, Mail } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  SideDrawerSection,
  SideDrawerSectionHeader,
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type {
  RequestVerificationOptions,
  SecurityProof,
} from '@/features/auth/secure-verification/types'
import { formatTimestamp } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { AuthOperationError } from '@/lib/secure-verification'

import {
  getEmailHistory,
  getEmailTemplates,
  prepareDraft,
  renderTemplate,
  sendTestEmail,
  sendUserEmail,
  type EmailTemplateKind,
} from '../api'
import { EmailPreview, RichEmailEditor } from './rich-email-editor'

type UserEmailDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  user: { id: number; username: string; display_name?: string; email?: string }
  requestVerification: (
    options: RequestVerificationOptions
  ) => Promise<SecurityProof | null>
}

export function UserEmailDrawer(props: UserEmailDrawerProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [template, setTemplate] = useState<EmailTemplateKind>('blank')
  const [subject, setSubject] = useState('')
  const [html, setHtml] = useState('')
  const [editorKey, setEditorKey] = useState(0)
  const [tab, setTab] = useState('edit')
  const [sending, setSending] = useState(false)
  const name = props.user.display_name || props.user.username

  const templates = useQuery({
    queryKey: ['email-templates'],
    queryFn: async () => (await getEmailTemplates()).data ?? null,
    enabled: props.open,
  })
  const history = useQuery({
    queryKey: ['user-email-history', props.user.id],
    queryFn: async () => (await getEmailHistory(props.user.id)).data ?? [],
    enabled: props.open,
  })
  const siteName = templates.data?.site_name ?? 'New API'

  const applyTemplate = (kind: EmailTemplateKind) => {
    let next = {
      subject: '',
      html: `<p>${t('Hi {{name}},', { name })}</p><p></p>`,
    }
    if (kind !== 'blank' && templates.data) {
      next = renderTemplate(templates.data.templates[kind], {
        display_name: name,
        username: props.user.username,
        email: props.user.email ?? '',
        site_name: siteName,
        site_url: templates.data.site_url,
      })
    }
    setTemplate(kind)
    setSubject(next.subject)
    setHtml(next.html)
    setEditorKey((key) => key + 1)
  }

  useEffect(() => {
    if (props.open) {
      applyTemplate('blank')
      setTab('edit')
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [props.open])

  const send = async () => {
    const draft = prepareDraft(subject, html)
    if (!draft.subject) {
      toast.error(t('Enter a subject.'))
      return
    }
    setSending(true)
    try {
      const proof = await props.requestVerification({
        scope: 'admin.user.email',
        context: { user_id: props.user.id, email: props.user.email ?? '' },
        title: t('Verify to send email'),
        description: t(
          'Confirm your identity before sending this email to {{email}}.',
          { email: props.user.email }
        ),
      })
      if (!proof) return
      const result = await sendUserEmail(
        props.user.id,
        draft,
        proof.proof_token
      )
      if (!result.success) {
        handleServerError(result, t('Failed to send email'))
        return
      }
      toast.success(t('Email sent to {{email}}', { email: props.user.email }))
      await queryClient.invalidateQueries({
        queryKey: ['user-email-history', props.user.id],
      })
      props.onOpenChange(false)
    } catch (error) {
      handleServerError(
        AuthOperationError.from(error),
        t('Failed to send email')
      )
    } finally {
      setSending(false)
    }
  }

  const sendToSelf = async () => {
    const result = await sendTestEmail(prepareDraft(subject, html))
    if (!result.success) {
      handleServerError(result, t('Failed to send email'))
      return
    }
    toast.success(
      t('Test email sent to {{email}}', {
        email: result.data?.email || t('your address'),
      })
    )
  }

  const templateItems = [
    { value: 'blank', label: t('Blank') },
    { value: 'suspended', label: t('Account suspended notice') },
    { value: 'restored', label: t('Account restored notice') },
  ]

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-[720px]')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>{t('Send email')}</SheetTitle>
          <SheetDescription>
            {t('To {{name}} <{{email}}>', {
              name,
              email: props.user.email,
              interpolation: { escapeValue: false },
            })}
          </SheetDescription>
        </SheetHeader>

        <div className={sideDrawerFormClassName()}>
          <SideDrawerSection>
            <SideDrawerSectionHeader
              icon={<Mail />}
              iconTone='primary'
              title={t('Compose')}
              description={t(
                'Notice templates are edited in System Settings › Site & Branding › Email Templates.'
              )}
            />
            <div className='grid gap-3 sm:grid-cols-[200px_1fr]'>
              <div className='space-y-1.5'>
                <Label>{t('Template')}</Label>
                <Select
                  items={templateItems}
                  value={template}
                  onValueChange={(value) =>
                    value && applyTemplate(value as EmailTemplateKind)
                  }
                >
                  <SelectTrigger className='w-full'>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      {templateItems.map((item) => (
                        <SelectItem key={item.value} value={item.value}>
                          {item.label}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </div>
              <div className='space-y-1.5'>
                <Label htmlFor='email-subject'>{t('Subject')}</Label>
                <Input
                  id='email-subject'
                  value={subject}
                  maxLength={200}
                  onChange={(event) => setSubject(event.target.value)}
                />
              </div>
            </div>

            <Tabs value={tab} onValueChange={(value) => setTab(String(value))}>
              <TabsList>
                <TabsTrigger value='edit'>{t('Edit')}</TabsTrigger>
                <TabsTrigger value='preview'>{t('Preview')}</TabsTrigger>
              </TabsList>
              <TabsContent value='edit' keepMounted>
                <RichEmailEditor
                  key={editorKey}
                  initialHtml={html}
                  onChange={setHtml}
                  ariaLabel={t('Email body')}
                  allowImages
                />
              </TabsContent>
              <TabsContent value='preview'>
                <EmailPreview
                  from={siteName}
                  to={props.user.email ?? ''}
                  subject={subject}
                  html={html}
                />
              </TabsContent>
            </Tabs>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Up to 5 images, 5 MB in total. Formatting outside the supported set is removed before sending.'
              )}
            </p>
          </SideDrawerSection>

          <SideDrawerSection>
            <SideDrawerSectionHeader
              icon={<History />}
              iconTone='neutral'
              title={t('Sent emails')}
            />
            {history.data && history.data.length > 0 ? (
              <div className='divide-y rounded-lg border'>
                {history.data.map((item) => (
                  <div
                    key={`${item.time}-${item.operator}-${item.subject}`}
                    className='flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2 text-sm'
                  >
                    <span className='text-muted-foreground w-36 shrink-0 text-xs tabular-nums'>
                      {formatTimestamp(item.time)}
                    </span>
                    <span className='min-w-0 flex-1 truncate'>
                      {item.subject}
                    </span>
                    <span className='text-muted-foreground text-xs'>
                      {item.operator}
                    </span>
                    <StatusBadge
                      label={item.success ? t('Sent') : t('Failed')}
                      variant={item.success ? 'success' : 'danger'}
                      size='sm'
                    />
                  </div>
                ))}
              </div>
            ) : (
              <p className='text-muted-foreground text-sm'>
                {t('No emails have been sent to this user.')}
              </p>
            )}
          </SideDrawerSection>
        </div>

        <SheetFooter className={sideDrawerFooterClassName()}>
          <SheetClose render={<Button variant='outline' />}>
            {t('Close')}
          </SheetClose>
          <Button variant='outline' onClick={sendToSelf} disabled={sending}>
            {t('Send to myself')}
          </Button>
          <Button onClick={send} disabled={sending || !props.user.email}>
            {sending ? t('Sending...') : t('Send')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}

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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { handleServerError } from '@/lib/handle-server-error'

import {
  getEmailTemplates,
  renderTemplate,
  sendAccountNotice,
  type NoticeKind,
} from '../api'
import { EmailPreview } from './rich-email-editor'

type AccountNoticeDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  kind: NoticeKind
  user: { id: number; username: string; display_name?: string; email?: string }
  /** Returned by the disable/enable action that the administrator verified. */
  noticeToken: string
}

/**
 * Offered right after an administrator disables or enables a user. The action
 * itself was verified, so sending the notice needs no second verification.
 */
export function AccountNoticeDialog(props: AccountNoticeDialogProps) {
  const { t } = useTranslation()
  const [sending, setSending] = useState(false)
  const templates = useQuery({
    queryKey: ['email-templates'],
    queryFn: async () => (await getEmailTemplates()).data ?? null,
    enabled: props.open,
  })

  const notice = templates.data
    ? renderTemplate(templates.data.templates[props.kind], {
        display_name: props.user.display_name || props.user.username,
        username: props.user.username,
        email: props.user.email ?? '',
        site_name: templates.data.site_name,
        site_url: templates.data.site_url,
      })
    : null

  const send = async () => {
    setSending(true)
    try {
      const result = await sendAccountNotice(
        props.user.id,
        props.kind,
        props.noticeToken
      )
      if (!result.success) {
        handleServerError(result, t('Failed to send email'))
        return
      }
      toast.success(t('Email sent to {{email}}', { email: props.user.email }))
      props.onOpenChange(false)
    } catch (error) {
      handleServerError(error, t('Failed to send email'))
    } finally {
      setSending(false)
    }
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={
        props.kind === 'suspended'
          ? t('Notify the user of the suspension?')
          : t('Notify the user that the account is restored?')
      }
      description={t(
        'This email goes to {{email}}. You can also skip it. The template is set in System Settings › Site & Branding › Email Templates.',
        { email: props.user.email }
      )}
      contentClassName='sm:max-w-xl'
      footer={
        <>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t("Don't send")}
          </Button>
          <Button onClick={send} disabled={sending || !notice}>
            {sending ? t('Sending...') : t('Send notice')}
          </Button>
        </>
      }
    >
      {notice ? (
        <EmailPreview
          from={templates.data?.site_name ?? ''}
          to={props.user.email ?? ''}
          subject={notice.subject}
          html={notice.html}
        />
      ) : (
        <Skeleton className='h-48 w-full' />
      )}
    </Dialog>
  )
}

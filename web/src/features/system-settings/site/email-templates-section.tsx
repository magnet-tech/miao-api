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
import { RotateCcw, Send } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  getEmailTemplates,
  prepareDraft,
  renderTemplate,
  sendTestEmail,
  updateEmailTemplates,
  type NoticeKind,
  type NoticeTemplates,
} from '@/features/user-emails/api'
import {
  EmailPreview,
  RichEmailEditor,
} from '@/features/user-emails/components/rich-email-editor'
import { handleServerError } from '@/lib/handle-server-error'

import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'

const SAMPLE_USER = {
  display_name: 'Alice Chen',
  username: 'alice',
  email: 'alice@example.com',
}

/** Root edits the suspension and restore notices offered after a user is
 * disabled or enabled; the server fills the variables when it sends them. */
export function EmailTemplatesSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [kind, setKind] = useState<NoticeKind>('suspended')
  const [draft, setDraft] = useState<NoticeTemplates | null>(null)
  const [editorVersion, setEditorVersion] = useState(0)
  const [saving, setSaving] = useState(false)

  const query = useQuery({
    queryKey: ['email-templates'],
    queryFn: async () => (await getEmailTemplates()).data ?? null,
  })
  const saved = query.data?.templates

  useEffect(() => {
    if (saved) {
      setDraft(structuredClone(saved))
      setEditorVersion((version) => version + 1)
    }
  }, [saved])

  const dirty =
    draft !== null && saved !== undefined
      ? JSON.stringify(draft) !== JSON.stringify(saved)
      : false

  const variables = [
    { key: 'display_name', label: t('Display name') },
    { key: 'username', label: t('Username') },
    { key: 'email', label: t('Email') },
    { key: 'site_name', label: t('Site name') },
    { key: 'site_url', label: t('Site address') },
  ]

  const updateDraft = (patch: Partial<NoticeTemplates[NoticeKind]>) =>
    setDraft((current) =>
      current ? { ...current, [kind]: { ...current[kind], ...patch } } : current
    )

  const save = async () => {
    if (!draft) return
    setSaving(true)
    try {
      const result = await updateEmailTemplates(draft)
      if (!result.success) {
        handleServerError(result, t('Failed to save email templates'))
        return
      }
      toast.success(t('Email templates saved'))
      await queryClient.invalidateQueries({ queryKey: ['email-templates'] })
    } catch (error) {
      handleServerError(error, t('Failed to save email templates'))
    } finally {
      setSaving(false)
    }
  }

  const discard = () => {
    if (!saved) return
    setDraft(structuredClone(saved))
    setEditorVersion((version) => version + 1)
  }

  const restoreDefault = () => {
    if (!query.data) return
    updateDraft(structuredClone(query.data.defaults[kind]))
    setEditorVersion((version) => version + 1)
  }

  if (!query.data || !draft) {
    return (
      <SettingsSection title={t('Email Templates')}>
        <Skeleton className='h-96 w-full' />
      </SettingsSection>
    )
  }

  const values = {
    ...SAMPLE_USER,
    site_name: query.data.site_name,
    site_url: query.data.site_url,
  }
  const preview = renderTemplate(draft[kind], values)

  const sendTest = async () => {
    const result = await sendTestEmail(
      prepareDraft(preview.subject, preview.html)
    )
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

  return (
    <SettingsSection title={t('Email Templates')}>
      <SettingsPageFormActions
        onSave={save}
        onReset={discard}
        isSaving={saving}
        isSaveDisabled={!dirty}
        isResetDisabled={!dirty}
        saveLabel='Save templates'
        resetLabel='Discard changes'
      />
      <div className='space-y-4'>
        <p className='text-muted-foreground text-sm'>
          {t(
            'After an administrator disables or enables a user who has an email address, the console offers to send one of these notices. Variables are filled in for each user when the email is sent.'
          )}
        </p>

        <Tabs
          value={kind}
          onValueChange={(value) => setKind(value as NoticeKind)}
        >
          <TabsList>
            <TabsTrigger value='suspended'>
              {t('Account suspended notice')}
            </TabsTrigger>
            <TabsTrigger value='restored'>
              {t('Account restored notice')}
            </TabsTrigger>
          </TabsList>
        </Tabs>

        <div className='grid gap-6 xl:grid-cols-2'>
          <div className='space-y-4'>
            <div className='space-y-1.5'>
              <Label htmlFor='template-subject'>{t('Subject')}</Label>
              <Input
                id='template-subject'
                value={draft[kind].subject}
                maxLength={200}
                onChange={(event) =>
                  updateDraft({ subject: event.target.value })
                }
              />
            </div>
            <div className='space-y-1.5'>
              <Label>{t('Body')}</Label>
              <RichEmailEditor
                key={`${kind}-${editorVersion}`}
                initialHtml={draft[kind].html}
                onChange={(html) => updateDraft({ html })}
                ariaLabel={t('Email body')}
                variables={variables}
              />
              <p className='text-muted-foreground text-xs'>
                {t('Available variables:')}{' '}
                {variables.map((variable) => (
                  <code
                    key={variable.key}
                    className='bg-muted mr-1 rounded px-1'
                  >
                    {`{{${variable.key}}}`}
                  </code>
                ))}
              </p>
            </div>
            <div className='flex flex-wrap gap-2'>
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={restoreDefault}
              >
                <RotateCcw />
                {t('Restore default')}
              </Button>
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={sendTest}
              >
                <Send />
                {t('Send a test to myself')}
              </Button>
            </div>
          </div>

          <div className='space-y-1.5'>
            <Label>{t('Preview with a sample user')}</Label>
            <EmailPreview
              from={query.data.site_name}
              to={SAMPLE_USER.email}
              subject={preview.subject}
              html={preview.html}
            />
          </div>
        </div>
      </div>
    </SettingsSection>
  )
}

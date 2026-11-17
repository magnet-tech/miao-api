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
import { useNavigate } from '@tanstack/react-router'
import { Loader2, MailCheck, MailPlus } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  InputOTP,
  InputOTPGroup,
  InputOTPSlot,
} from '@/components/ui/input-otp'
import { Label } from '@/components/ui/label'
import { logout } from '@/features/auth/api'
import { checkVerificationMethods } from '@/features/auth/secure-verification'
import { useCountdown } from '@/hooks/use-countdown'
import { getSelf } from '@/lib/api'
import { clearAuthenticatedClientState } from '@/lib/auth-session'
import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'

import {
  finishEmailVerification,
  resendEmailVerification,
  startEmailVerification,
  type EmailVerificationFlow,
} from '../api'
import { EmailBindDialog } from './dialogs/email-bind-dialog'

const ROOT_ROLE = 100

/**
 * Shown after sign-in to a user whose administrator requires a verified email.
 * It cannot be dismissed: the user verifies the current address, binds one, or
 * signs out. Accounts with a step-up method bind through the regular email
 * binding dialog; accounts without one bind a first address here directly.
 */
export function RequiredEmailDialog() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const user = useAuthStore((state) => state.auth.user)
  const setUser = useAuthStore((state) => state.auth.setUser)

  const required =
    user !== null &&
    user.role < ROOT_ROLE &&
    user.require_verified_email === true &&
    !(user.email && user.email_verified)

  const [bindOpen, setBindOpen] = useState(false)
  const [email, setEmail] = useState('')
  const [code, setCode] = useState('')
  const [flow, setFlow] = useState<EmailVerificationFlow | null>(null)
  const [busy, setBusy] = useState(false)
  const resend = useCountdown({ initialSeconds: 60 })

  const methods = useQuery({
    queryKey: ['required-email-methods', user?.id],
    queryFn: () => checkVerificationMethods('account.binding.bind'),
    enabled: required && !user?.email,
    meta: { errorToast: false },
  })
  const canStepUp =
    methods.data?.methods.some((method) => method.available) ?? true

  const resetResend = resend.reset
  useEffect(() => {
    if (!required) return
    setEmail('')
    setCode('')
    setFlow(null)
    resetResend()
  }, [required, user?.email, resetResend])

  if (!required || !user) return null

  const hasEmail = Boolean(user.email)

  const refreshSelf = async () => {
    const self = await getSelf()
    if (self?.success && self.data) setUser(self.data)
  }

  const signOut = async () => {
    try {
      await logout()
    } finally {
      clearAuthenticatedClientState(queryClient)
      void navigate({ to: '/sign-in', replace: true })
    }
  }

  const applyFlow = (next: EmailVerificationFlow) => {
    setFlow(next)
    setCode('')
    resend.start(Math.max(1, Math.ceil(next.resend_at - Date.now() / 1000)))
  }

  const sendCode = async () => {
    setBusy(true)
    try {
      if (flow) {
        applyFlow(await resendEmailVerification(flow.flow_token))
      } else {
        applyFlow(await startEmailVerification(hasEmail ? '' : email.trim()))
      }
      toast.success(t('Verification code sent! Please check your email.'))
    } catch (error) {
      handleServerError(error, t('Failed to send the code'))
    } finally {
      setBusy(false)
    }
  }

  const confirm = async () => {
    if (!flow) return
    setBusy(true)
    try {
      await finishEmailVerification(flow.flow_token, code)
      await refreshSelf()
      toast.success(t('Email verified. You can use the service now.'))
    } catch (error) {
      handleServerError(error, t('Verification failed'))
    } finally {
      setBusy(false)
    }
  }

  const bindsDirectly = !hasEmail && !canStepUp
  let title = t('Verify your email')
  let description = t(
    'Your administrator requires a verified email. Until then, requests made with your API keys are rejected.'
  )
  if (!hasEmail) {
    title = t('Bind an email')
    description = t(
      'Your administrator requires you to bind and verify an email before you continue.'
    )
  }

  let sendLabel = t('Send code')
  if (resend.isActive) {
    sendLabel = t('Resend in {{seconds}}s', { seconds: resend.secondsLeft })
  } else if (flow) {
    sendLabel = t('Resend')
  }

  return (
    <>
      <Dialog
        open={!bindOpen}
        onOpenChange={() => {
          // Required: outside clicks and Escape keep the dialog open.
        }}
        showCloseButton={false}
        contentClassName='sm:max-w-md'
        titleClassName='flex items-center gap-2'
        title={
          <>
            {hasEmail ? (
              <MailCheck className='size-5' />
            ) : (
              <MailPlus className='size-5' />
            )}
            {title}
          </>
        }
        description={description}
        footer={
          <>
            <Button variant='outline' onClick={signOut} disabled={busy}>
              {t('Cancel and sign out')}
            </Button>
            {hasEmail || bindsDirectly ? (
              <Button
                onClick={confirm}
                disabled={busy || !flow || code.length !== 6}
              >
                {busy && <Loader2 className='size-4 animate-spin' />}
                {hasEmail ? t('Verify') : t('Bind email')}
              </Button>
            ) : (
              <Button onClick={() => setBindOpen(true)}>
                {t('Bind email')}
              </Button>
            )}
          </>
        }
      >
        <div className='space-y-4 py-1'>
          {hasEmail || bindsDirectly ? (
            <div className='space-y-2'>
              <Label htmlFor='required-email'>
                {hasEmail ? t('Current email') : t('Email address')}
              </Label>
              <div className='flex gap-2'>
                <Input
                  id='required-email'
                  value={hasEmail ? (user.email ?? '') : email}
                  readOnly={hasEmail || flow !== null}
                  placeholder='you@example.com'
                  onChange={(event) => setEmail(event.target.value)}
                />
                <Button
                  variant='outline'
                  onClick={sendCode}
                  disabled={
                    busy || resend.isActive || (!hasEmail && !email.trim())
                  }
                >
                  {sendLabel}
                </Button>
              </div>
              {hasEmail ? (
                <button
                  type='button'
                  className='text-primary text-xs hover:underline'
                  onClick={() => setBindOpen(true)}
                >
                  {t('Not your address? Use another email')}
                </button>
              ) : null}
            </div>
          ) : (
            <p className='text-muted-foreground text-sm'>
              {t('You will confirm your identity before the code is sent.')}
            </p>
          )}

          {flow ? (
            <div className='space-y-2'>
              <Label>{t('Verification code')}</Label>
              <InputOTP
                maxLength={6}
                value={code}
                onChange={setCode}
                aria-label={t('Verification code')}
              >
                <InputOTPGroup>
                  {[0, 1, 2, 3, 4, 5].map((index) => (
                    <InputOTPSlot key={index} index={index} />
                  ))}
                </InputOTPGroup>
              </InputOTP>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'We sent a 6-digit code to {{email}}. It expires in 10 minutes.',
                  { email: flow.email }
                )}
              </p>
            </div>
          ) : null}
        </div>
      </Dialog>
      <EmailBindDialog
        open={bindOpen}
        onOpenChange={setBindOpen}
        currentEmail={user.email || undefined}
        onSuccess={() => {
          void refreshSelf()
        }}
      />
    </>
  )
}

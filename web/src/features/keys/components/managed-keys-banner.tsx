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
import { Link } from '@tanstack/react-router'
import { ArrowLeft, Lock, ShieldCheck } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { hasPermission } from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'

import { useApiKeys } from './api-keys-provider'

/**
 * Shown on the API Keys page while an administrator manages another user's
 * keys: read-only until the administrator verifies once, then a 30-minute
 * management session that also ends when the page is left.
 */
export function ManagedKeysBanner() {
  const { t } = useTranslation()
  const keys = useApiKeys()
  const currentUser = useAuthStore((state) => state.auth.user)
  const canManage = hasPermission(currentUser, 'user_token', 'manage')
  const expiresAt = keys.managementExpiresAt
  const [now, setNow] = useState(() => Math.floor(Date.now() / 1000))

  useEffect(() => {
    if (expiresAt === null) return
    setNow(Math.floor(Date.now() / 1000))
    const timer = setInterval(
      () => setNow(Math.floor(Date.now() / 1000)),
      15_000
    )
    return () => clearInterval(timer)
  }, [expiresAt])

  if (!keys.owner) return null
  const username = keys.owner.username
  const backToUsers = (
    <Button size='sm' variant='outline' render={<Link to='/users' />}>
      <ArrowLeft />
      {t('Back to users')}
    </Button>
  )

  if (expiresAt === null) {
    return (
      <div className='bg-muted/40 flex flex-wrap items-center gap-3 rounded-lg border px-4 py-3 text-sm'>
        <Lock
          className='text-muted-foreground size-5 shrink-0'
          aria-hidden='true'
        />
        <div className='min-w-0 flex-1'>
          <div className='font-medium'>
            {t('Managing the API keys of {{username}} · read-only', {
              username,
            })}
          </div>
          <div className='text-muted-foreground text-xs'>
            {canManage
              ? t(
                  'Verify once before creating, editing, enabling, disabling or deleting keys. You will not be asked again for 30 minutes.'
                )
              : t('You can view these keys but not change them.')}
          </div>
        </div>
        {backToUsers}
        {canManage ? (
          <Button size='sm' onClick={() => void keys.startManagement()}>
            {t('Verify identity')}
          </Button>
        ) : null}
      </div>
    )
  }

  const minutesLeft = Math.max(1, Math.ceil((expiresAt - now) / 60))
  return (
    <div className='border-success/40 bg-success/5 flex flex-wrap items-center gap-3 rounded-lg border px-4 py-3 text-sm'>
      <ShieldCheck
        className='text-success size-5 shrink-0'
        aria-hidden='true'
      />
      <div className='min-w-0 flex-1'>
        <div className='font-medium'>
          {t('Managing the API keys of {{username}} · verified', { username })}
        </div>
        <div className='text-muted-foreground text-xs'>
          {t('{{minutes}} minutes left. Leaving this page ends the session.', {
            minutes: minutesLeft,
          })}
        </div>
      </div>
      {backToUsers}
      <Button size='sm' variant='outline' onClick={keys.endManagement}>
        {t('End management')}
      </Button>
    </div>
  )
}

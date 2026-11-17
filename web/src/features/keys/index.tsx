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
import { getRouteApi } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { getUser } from '@/features/users/api'

import { ApiKeysDialogs } from './components/api-keys-dialogs'
import { ApiKeysPrimaryButtons } from './components/api-keys-primary-buttons'
import { ApiKeysProvider } from './components/api-keys-provider'
import { ApiKeysTable } from './components/api-keys-table'
import { ManagedKeysBanner } from './components/managed-keys-banner'

const route = getRouteApi('/_authenticated/keys/')

export function ApiKeys() {
  const { t } = useTranslation()
  // ?owner=ID: an administrator managing another user's keys.
  const ownerId = route.useSearch({ select: (search) => search.owner })
  const ownerQuery = useQuery({
    queryKey: ['managed-key-owner', ownerId],
    queryFn: async () => (await getUser(ownerId ?? 0)).data ?? null,
    enabled: ownerId !== undefined,
  })
  const ownerName = ownerQuery.data?.username ?? `#${ownerId}`
  const owner = useMemo(
    () => (ownerId === undefined ? null : { id: ownerId, username: ownerName }),
    [ownerId, ownerName]
  )

  return (
    <ApiKeysProvider key={ownerId ?? 'self'} owner={owner}>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>
          {owner
            ? t('API Keys of {{username}}', { username: owner.username })
            : t('API Keys')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <ApiKeysPrimaryButtons />
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='flex min-h-0 flex-1 flex-col gap-3'>
            <ManagedKeysBanner />
            <ApiKeysTable />
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <ApiKeysDialogs />
    </ApiKeysProvider>
  )
}

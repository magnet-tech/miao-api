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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore, type AuthUser } from '@/stores/auth-store'

import { createApiKey, getApiKeys, type TokenScope } from '../../api'
import { ApiKeysPrimaryButtons } from '../api-keys-primary-buttons'
import { ApiKeysProvider } from '../api-keys-provider'
import { ManagedKeysBanner } from '../managed-keys-banner'

afterEach(() => {
  useAuthStore.getState().auth.setUser(null)
  vi.restoreAllMocks()
})

describe('managed key requests', () => {
  it('send another user’s key changes to that user with the session', async () => {
    const post = vi
      .spyOn(api, 'post')
      .mockResolvedValue({ data: { success: true } })
    const get = vi
      .spyOn(api, 'get')
      .mockResolvedValue({ data: { success: true, data: { items: [] } } })
    const scope: TokenScope = { ownerId: 7, managementToken: 'session-1' }

    await createApiKey({ name: 'k' } as never, scope)
    await getApiKeys({ p: 2, size: 20 }, { ownerId: 7, managementToken: null })

    expect(post).toHaveBeenCalledWith(
      '/api/user/7/tokens/',
      { name: 'k' },
      { headers: { 'X-Key-Management-Session': 'session-1' } }
    )
    expect(get).toHaveBeenCalledWith('/api/user/7/tokens/?p=2&size=20')
  })

  it('leave requests for one’s own keys unchanged', async () => {
    const post = vi
      .spyOn(api, 'post')
      .mockResolvedValue({ data: { success: true } })

    await createApiKey({ name: 'k' } as never)

    expect(post).toHaveBeenCalledWith('/api/token/', { name: 'k' })
  })
})

async function renderManaged(user: AuthUser) {
  useAuthStore.getState().auth.setUser(user)
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: [] } })
  const root = createRootRoute({
    component: () => (
      <ApiKeysProvider owner={{ id: 7, username: 'managed' }}>
        <ManagedKeysBanner />
        <ApiKeysPrimaryButtons />
      </ApiKeysProvider>
    ),
  })
  const router = createRouter({
    routeTree: root,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await screen.findByText(/Managing the API keys of managed/)
}

describe('managing another user’s keys', () => {
  it('stays read-only until the administrator verifies', async () => {
    await renderManaged({ id: 1, username: 'root', role: 100 })

    expect(
      screen.getByRole('button', { name: 'Verify identity' })
    ).toBeEnabled()
    expect(
      screen.getByRole('button', { name: 'Create API Key' })
    ).toBeDisabled()
  })

  it('offers no verification without the permission to manage keys', async () => {
    await renderManaged({
      id: 2,
      username: 'viewer',
      role: 10,
      permissions: {
        admin_permissions: { user_token: { read: true, manage: false } },
      },
    })

    expect(
      screen.queryByRole('button', { name: 'Verify identity' })
    ).not.toBeInTheDocument()
    expect(
      screen.getByText('You can view these keys but not change them.')
    ).toBeVisible()
  })
})

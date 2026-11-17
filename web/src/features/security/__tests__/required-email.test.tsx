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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore, type AuthUser } from '@/stores/auth-store'

import { RequiredEmailDialog } from '../components/required-email-dialog'

const { navigate, logout } = vi.hoisted(() => ({
  navigate: vi.fn(),
  logout: vi.fn(),
}))
vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  useNavigate: () => navigate,
}))
vi.mock('@/features/auth/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/auth/api')>()),
  logout,
}))

let client: QueryClient
beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  logout.mockResolvedValue({ success: true })
})

afterEach(() => {
  client.clear()
  navigate.mockReset()
  logout.mockReset()
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset('idle')
})

function renderFor(user: AuthUser) {
  useAuthStore.getState().auth.setUser(user)
  return render(
    <QueryClientProvider client={client}>
      <RequiredEmailDialog />
    </QueryClientProvider>
  )
}

const unverified: AuthUser = {
  id: 7,
  username: 'bob',
  role: 1,
  email: 'bob@example.org',
  email_verified: false,
  require_verified_email: true,
}

it.each([
  ['the requirement is off', { ...unverified, require_verified_email: false }],
  ['the email is verified', { ...unverified, email_verified: true }],
  ['the user is root', { ...unverified, role: 100 }],
])('stays hidden when %s', (_, user) => {
  renderFor(user)
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})

it('cannot be dismissed with Escape and verifies the current address', async () => {
  const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
    if (url === '/api/user/self/email/verify/start') {
      return {
        data: {
          success: true,
          data: {
            flow_token: 'flow-1',
            email: 'bob@example.org',
            mode: 'current',
            expires_at: Date.now() / 1000 + 600,
            resend_at: Date.now() / 1000 + 60,
          },
        },
      }
    }
    return {
      data: {
        success: true,
        data: { email: 'bob@example.org', email_verified: true },
      },
    }
  })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { ...unverified, email_verified: true } },
  })
  const user = userEvent.setup()
  renderFor(unverified)

  const dialog = await screen.findByRole('dialog', {
    name: 'Verify your email',
  })
  await user.keyboard('{Escape}')
  expect(dialog).toBeVisible()
  expect(screen.getByRole('button', { name: 'Verify' })).toBeDisabled()

  await user.click(screen.getByRole('button', { name: 'Send code' }))
  await user.type(
    await screen.findByRole('textbox', { name: 'Verification code' }),
    '123456'
  )
  await user.click(screen.getByRole('button', { name: 'Verify' }))

  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
  expect(post).toHaveBeenCalledWith(
    '/api/user/self/email/verify',
    { flow_token: 'flow-1', code: '123456' },
    expect.anything()
  )
})

it('signs out when the user cancels', async () => {
  const user = userEvent.setup()
  renderFor(unverified)
  await user.click(
    await screen.findByRole('button', { name: 'Cancel and sign out' })
  )
  await waitFor(() => expect(logout).toHaveBeenCalledOnce())
  expect(navigate).toHaveBeenCalledWith({ to: '/sign-in', replace: true })
})

it('binds a first address directly when the account has no step-up method', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        scope: 'account.binding.bind',
        methods: [
          { method: 'password', available: false, reason: 'Not set up.' },
        ],
        oauth_providers: [],
        password_encryption_enabled: false,
      },
    },
  })
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: {
        flow_token: 'flow-2',
        email: 'carol@example.com',
        mode: 'required_bind',
        expires_at: Date.now() / 1000 + 600,
        resend_at: Date.now() / 1000 + 60,
      },
    },
  })
  const user = userEvent.setup()
  renderFor({ ...unverified, email: '' })

  await screen.findByRole('dialog', { name: 'Bind an email' })
  await user.type(
    await screen.findByRole('textbox', { name: 'Email address' }),
    'carol@example.com'
  )
  await user.click(screen.getByRole('button', { name: 'Send code' }))
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/api/user/self/email/verify/start',
      { email: 'carol@example.com' },
      expect.anything()
    )
  )
  expect(screen.getByText(/carol@example\.com\. It expires/)).toBeVisible()
})

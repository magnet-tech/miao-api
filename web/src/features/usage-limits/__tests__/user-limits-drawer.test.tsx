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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import type { UserLimits } from '../api'
import { UserLimitsDrawer } from '../components/user-limits-drawer'

const now = Math.floor(Date.now() / 1000)
const limits: UserLimits = {
  max_concurrency: 2,
  in_flight: 1,
  server_time_zone: 'UTC+08:00',
  rules: [
    {
      window_seconds: 86400,
      mode: 'calendar',
      calendar_period: 'day',
      quota_limit: 1000,
      enabled: true,
      models: [],
      state: 'active',
      used_quota: 400,
      window_start: now - 3600,
      reset_at: now + 3600,
    },
  ],
}

let client: QueryClient
beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/user/7/limits') {
      return { data: { success: true, data: limits } }
    }
    return { data: { success: true, data: [] } }
  })
})

afterEach(() => {
  client.clear()
  useAuthStore.getState().auth.setUser(null)
  vi.restoreAllMocks()
})

function renderDrawer() {
  render(
    <QueryClientProvider client={client}>
      <UserLimitsDrawer
        open
        onOpenChange={() => {}}
        user={{ id: 7, username: 'limited' }}
      />
    </QueryClientProvider>
  )
}

it('adds a rule to the draft and saves every rule when Save is pressed', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true, data: limits } })
  renderDrawer()

  await userEvent.click(
    await screen.findByRole('button', { name: 'Add rule (9 left)' })
  )
  await userEvent.type(screen.getByLabelText(/^Limit/), '2')
  await userEvent.click(screen.getByRole('button', { name: 'Add rule' }))
  await userEvent.click(screen.getByRole('button', { name: 'Save changes' }))

  await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
  const [url, body] = put.mock.calls[0] as [
    string,
    { max_concurrency: number; rules: UserLimits['rules'] },
  ]
  expect(url).toBe('/api/user/7/limits')
  expect(body.max_concurrency).toBe(2)
  expect(body.rules).toHaveLength(2)
  expect(body.rules[0]).toEqual({
    window_seconds: 86400,
    mode: 'calendar',
    calendar_period: 'day',
    quota_limit: 1000,
    enabled: true,
    models: [],
  })
  expect(body.rules[1]).toMatchObject({
    window_seconds: 18000,
    mode: 'window',
    calendar_period: '',
    enabled: true,
    models: [],
  })
  expect(body.rules[1].quota_limit).toBeGreaterThan(0)
})

it('keeps the limits read-only for an operator without the edit permission', async () => {
  useAuthStore.getState().auth.setUser({
    id: 2,
    username: 'viewer',
    role: 10,
    permissions: {
      admin_permissions: {
        user_limit: { read: true, write: false, reset: false },
      },
    },
  })
  renderDrawer()

  expect(
    await screen.findByRole('button', { name: 'Reset window' })
  ).toBeDisabled()
  expect(screen.getByLabelText('Maximum concurrency')).toBeDisabled()
  expect(screen.getByRole('switch', { name: 'Enable rule' })).toHaveAttribute(
    'aria-disabled',
    'true'
  )
  expect(
    screen.getByRole('button', { name: 'Add rule (9 left)' })
  ).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
})

it('ends a running window after the reset is confirmed', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true, data: limits } })
  renderDrawer()

  await userEvent.click(
    await screen.findByRole('button', { name: 'Reset window' })
  )
  const dialog = await screen.findByRole('alertdialog')
  await userEvent.click(
    within(dialog).getByRole('button', { name: 'Reset window' })
  )

  await waitFor(() =>
    expect(post).toHaveBeenCalledWith('/api/user/7/limits/reset', {
      window_seconds: 86400,
    })
  )
})

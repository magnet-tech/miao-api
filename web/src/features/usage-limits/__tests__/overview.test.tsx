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
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { UsageLimitsOverview } from '..'
import type { LimitOverviewItem } from '../api'

const item = (id: number, percent: number): LimitOverviewItem => ({
  id,
  username: `user-${id}`,
  display_name: '',
  status: 1,
  highest_percent: percent,
  max_concurrency: 0,
  in_flight: 0,
  server_time_zone: 'UTC+08:00',
  rules: [],
})

let client: QueryClient
beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        items: [item(3, 90), item(9, 10), item(5, 100)],
        counts: { all: 3, near: 1, exhausted: 1 },
      },
    },
  })
})

afterEach(() => {
  client.clear()
  useAuthStore.getState().auth.setUser(null)
  vi.restoreAllMocks()
})

function usernames() {
  return screen
    .getAllByRole('row')
    .slice(1)
    .map((row) => within(row).getByText(/^user-/).textContent)
}

it('lists users by ID, newest first, until another sort is chosen', async () => {
  render(
    <QueryClientProvider client={client}>
      <UsageLimitsOverview />
    </QueryClientProvider>
  )

  await screen.findByText('user-9')
  expect(usernames()).toEqual(['user-9', 'user-5', 'user-3'])

  await userEvent.click(screen.getByRole('button', { name: 'Usage by rule' }))
  expect(usernames()).toEqual(['user-5', 'user-3', 'user-9'])
})

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
import { toast } from 'sonner'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore, type AuthUser } from '@/stores/auth-store'

import { ModelReplacement } from '..'

const overview = {
  enabled: false,
  interval_minutes: 10,
  server_time_zone: 'UTC+08:00',
  next_run_at: null,
  last_run: null,
  active_run: null,
  counts: { healthy: 0, suspected: 0, failed: 0, not_tested: 1, in_scope: 1 },
  rounds: 3,
  models: [],
}

afterEach(() => {
  useAuthStore.getState().auth.setUser(null)
  vi.restoreAllMocks()
})

function renderPage(user: AuthUser) {
  useAuthStore.getState().auth.setUser(user)
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/model_replacement/overview') {
      return { data: { success: true, data: overview } }
    }
    if (url === '/api/model_replacement/runs') {
      return { data: { success: true, data: { runs: [], total: 0 } } }
    }
    return { data: { success: true, data: [] } }
  })
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <ModelReplacement
        tab='history'
        lastRuns={10}
        onTabChange={() => {}}
        onLastRunsChange={() => {}}
      />
    </QueryClientProvider>
  )
}

it('runs detection by hand while automatic detection is off', async () => {
  const success = vi.spyOn(toast, 'success')
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: {
        id: 'run',
        trigger: 'manual',
        channel_id: null,
        started_at: 1,
        done: 0,
        total: 3,
      },
    },
  })
  renderPage({ id: 1, username: 'root', role: 100 })

  const runNow = await screen.findByRole('button', { name: 'Run now' })
  await waitFor(() => expect(screen.getByText('Off')).toBeVisible())
  expect(runNow).toBeEnabled()
  await userEvent.click(runNow)

  await waitFor(() =>
    expect(post).toHaveBeenCalledWith('/api/model_replacement/run')
  )
  // The run response has no message; the toast must not fall back to the
  // shared "Started" key, which other pages translate as a start time.
  await waitFor(() => expect(success).toHaveBeenCalledWith('Detection started'))
})

it('offers no run without the permission to operate detection', async () => {
  renderPage({
    id: 2,
    username: 'viewer',
    role: 10,
    permissions: {
      admin_permissions: { model_replacement: { read: true, operate: false } },
    },
  })

  await screen.findByText('Model Replacement Detection')
  expect(
    screen.queryByRole('button', { name: 'Run now' })
  ).not.toBeInTheDocument()
})

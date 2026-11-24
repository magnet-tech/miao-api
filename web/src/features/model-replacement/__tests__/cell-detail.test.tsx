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
import { render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, it, vi } from 'vitest'

import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'

import { CellDetailPanel } from '../components/cell-detail-panel'
import type { Attempt, CellDetail } from '../types'

const failedAttempt = (round: number, error: string): Attempt => ({
  round,
  answer: '',
  answer_matched: false,
  response_model: '',
  response_model_ok: false,
  response_model_note: 'not_checked',
  matched: false,
  latency_ms: 0,
  tries: 1,
  error,
})

const detail: CellDetail = {
  run: {
    id: 'r1',
    trigger: 'manual',
    channel_id: null,
    started_at: 1_790_000_000,
    finished_at: 1_790_000_060,
    duration_ms: 60_000,
    counts: { passed: 0, replaced: 0, failed: 1 },
  },
  channel: {
    id: 1,
    name: 'Relay A',
    status: 'enabled',
    groups: ['default'],
    priority: 0,
    weight: 0,
    tag: '',
  },
  model: 'gpt-4o',
  verdict: 'failed',
  match_percent: 0,
  channel_verdict: 'failed',
  question: '1+1=?',
  expected_answer: '2',
  match_mode: 'exact',
  response_model_check: 'loose',
  attempts: [
    failedAttempt(1, 'upstream returned an empty answer'),
    failedAttempt(2, 'request timed out'),
    failedAttempt(3, 'dial tcp: connection refused'),
  ],
  actions: [],
  skipped_actions: false,
}

afterEach(() => {
  vi.restoreAllMocks()
})

it('shows the attempt errors the gateway writes in the reader language and upstream errors as written', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: detail },
  })
  const i18n = createInstance()
  await i18n.init({ lng: 'zh', resources: { zh } })
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <CellDetailPanel
          selection={{ runId: 'r1', channelId: 1, model: 'gpt-4o' }}
          retestBusy={false}
        />
      </QueryClientProvider>
    </I18nextProvider>
  )

  expect(await screen.findByText('上游返回了空答案')).toBeVisible()
  expect(screen.getByText('请求超时')).toBeVisible()
  expect(screen.getByText('dial tcp: connection refused')).toBeVisible()
})

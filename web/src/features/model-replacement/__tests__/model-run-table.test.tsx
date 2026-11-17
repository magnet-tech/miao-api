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
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'

import { ModelRunTable } from '../components/model-run-table'
import type { DetectionChannel, DetectionRun } from '../types'

const channel = (id: number, name: string): DetectionChannel => ({
  id,
  name,
  status: 'enabled',
  groups: ['default'],
  priority: 0,
  weight: 10,
  tag: '',
  in_scope: true,
  monitored_models: ['gpt-4o'],
  latest: null,
})

const run = (id: string, startedAt: number, cells: DetectionRun['cells']) =>
  ({
    id,
    trigger: 'schedule',
    channel_id: null,
    started_at: startedAt,
    finished_at: startedAt + 30,
    duration_ms: 30_000,
    counts: { passed: 0, replaced: 0, failed: 0 },
    cells,
    channel_results: [],
  }) satisfies DetectionRun

const runs = [
  run('r2', 1_800_000_600, [
    {
      channel_id: 1,
      model: 'gpt-4o',
      verdict: 'replaced',
      match_percent: 33,
      response_model_mismatch: true,
    },
  ]),
  run('r1', 1_800_000_000, [
    {
      channel_id: 1,
      model: 'gpt-4o',
      verdict: 'passed',
      match_percent: 100,
      response_model_mismatch: false,
    },
    {
      channel_id: 2,
      model: 'gpt-4o',
      verdict: 'failed',
      match_percent: 0,
      response_model_mismatch: false,
    },
  ]),
]

it('shows one row per channel and one column per run for the model', () => {
  render(
    <ModelRunTable
      model='gpt-4o'
      runs={runs}
      channels={[channel(1, 'Relay A'), channel(2, 'Relay B')]}
      selected={null}
      onSelect={() => {}}
    />
  )

  const table = screen.getByRole('table')
  expect(within(table).getByText('gpt-4o')).toBeVisible()
  const rows = within(table).getAllByRole('row').slice(1)
  expect(rows).toHaveLength(2)
  const relayA = within(rows[0]).getAllByRole('button')
  expect(relayA.map((cell) => cell.textContent)).toEqual(['33%', '100%'])
  const relayB = within(rows[1]).getAllByRole('button')
  expect(relayB[0]).toBeDisabled()
  expect(relayB[1]).toHaveTextContent('Failed')
})

it('selects the clicked run, channel and model for the detail panel', async () => {
  const onSelect = vi.fn()
  render(
    <ModelRunTable
      model='gpt-4o'
      runs={runs}
      channels={[channel(1, 'Relay A')]}
      selected={{ runId: 'r1', channelId: 1, model: 'gpt-4o' }}
      onSelect={onSelect}
    />
  )

  const cells = screen.getAllByRole('button', { pressed: false })
  await userEvent.click(cells[0])

  expect(onSelect).toHaveBeenCalledWith({
    runId: 'r2',
    channelId: 1,
    model: 'gpt-4o',
  })
  expect(screen.getByRole('button', { pressed: true })).toHaveTextContent(
    '100%'
  )
})

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
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { getSettings, updateSettings } from '../api'
import type { DetectionSettings } from '../types'

const stored: DetectionSettings = {
  enabled: false,
  interval_minutes: 10,
  interval_windows: [],
  retention_days: 7,
  rounds: 3,
  concurrency: 3,
  timeout_seconds: 60,
  threshold: 0,
  models: [],
  pass_actions: [{ type: 'add_group', value: 'vip', amount: 0 }],
  fail_actions: [
    { type: 'decrease_priority', value: '', amount: 10 },
    { type: 'set_tag', value: 'suspect', amount: 0 },
  ],
  request_failure_actions: [{ type: 'set_priority', value: '', amount: -5 }],
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('detection settings', () => {
  it('give the editor every action parameter as text', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { settings: stored, server_time_zone: 'UTC+08:00' },
      },
    })

    const settings = (await getSettings()).data?.settings

    expect(settings?.fail_actions).toEqual([
      { type: 'decrease_priority', value: '10' },
      { type: 'set_tag', value: 'suspect' },
    ])
    expect(settings?.request_failure_actions).toEqual([
      { type: 'set_priority', value: '-5' },
    ])
  })

  it('save priority and weight numbers as amounts', async () => {
    const put = vi.spyOn(api, 'put').mockResolvedValue({
      data: {
        success: true,
        data: { settings: stored, server_time_zone: 'UTC+08:00' },
      },
    })

    await updateSettings({
      ...stored,
      pass_actions: [{ type: 'add_group', value: ' vip ' }],
      fail_actions: [{ type: 'decrease_priority', value: '10' }],
      request_failure_actions: [{ type: 'set_weight', value: '0' }],
    })

    const body = put.mock.calls[0][1] as DetectionSettings
    expect(body.pass_actions).toEqual([
      { type: 'add_group', value: 'vip', amount: 0 },
    ])
    expect(body.fail_actions).toEqual([
      { type: 'decrease_priority', value: '', amount: 10 },
    ])
    expect(body.request_failure_actions).toEqual([
      { type: 'set_weight', value: '', amount: 0 },
    ])
  })
})

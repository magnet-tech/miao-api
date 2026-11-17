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
import type { TFunction } from 'i18next'
import { describe, expect, it } from 'vitest'

import type { UsageRule } from '../api'
import { formatDuration, ruleTitle, splitWindow } from '../lib/rules'

// Renders the English source text with its values, like i18next without
// translations.
const t = ((key: string, values?: Record<string, unknown>) =>
  key.replaceAll(/\{\{(\w+)\}\}/g, (_, name: string) =>
    String(values?.[name] ?? '')
  )) as unknown as TFunction

const rule = (overrides: Partial<UsageRule>): UsageRule => ({
  window_seconds: 18000,
  mode: 'window',
  calendar_period: '',
  quota_limit: 100,
  enabled: true,
  models: [],
  ...overrides,
})

describe('usage limit rule text', () => {
  it('rounds the remaining time up so it never suggests retrying early', () => {
    expect(formatDuration(t, 1)).toBe('1m')
    expect(formatDuration(t, 61)).toBe('2m')
    expect(formatDuration(t, 2 * 3600 + 14 * 60 + 1)).toBe('2h 15m')
    expect(formatDuration(t, 3 * 3600)).toBe('3h')
    expect(formatDuration(t, 86400 + 2 * 3600 + 1)).toBe('1d 3h')
    expect(formatDuration(t, 2 * 86400)).toBe('2d')
  })

  it('names a window by its largest whole unit', () => {
    expect(splitWindow(7 * 86400)).toEqual({ value: 7, unit: 'days' })
    expect(splitWindow(5 * 3600)).toEqual({ value: 5, unit: 'hours' })
    expect(splitWindow(90 * 60)).toEqual({ value: 90, unit: 'minutes' })
    expect(ruleTitle(t, rule({ window_seconds: 90 * 60 }))).toBe('90 minutes')
  })

  it('names a calendar rule by its period', () => {
    const weekly = rule({
      mode: 'calendar',
      calendar_period: 'week',
      window_seconds: 604800,
    })
    expect(ruleTitle(t, weekly)).toBe('Calendar week')
  })
})

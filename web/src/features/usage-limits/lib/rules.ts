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

import type { CalendarPeriod, UsageRule, UsageRuleView } from '../api'

export const CALENDAR_PERIOD_SECONDS: Record<CalendarPeriod, number> = {
  day: 86400,
  week: 604800,
  month: 2592000,
  quarter: 7776000,
  year: 31536000,
}

export const CALENDAR_PERIOD_LABELS: Record<CalendarPeriod, string> = {
  day: 'Calendar day',
  week: 'Calendar week',
  month: 'Calendar month',
  quarter: 'Calendar quarter',
  year: 'Calendar year',
}

export const WINDOW_PRESETS = [
  { seconds: 18000, label: '5 hours' },
  { seconds: 86400, label: '1 day' },
  { seconds: 604800, label: '7 days' },
  { seconds: 2592000, label: '30 days' },
] as const

export type WindowUnit = 'minutes' | 'hours' | 'days'

export const WINDOW_UNIT_SECONDS: Record<WindowUnit, number> = {
  minutes: 60,
  hours: 3600,
  days: 86400,
}

export const MAX_RULES = 10
export const MAX_WINDOW_SECONDS = 90 * 86400

/** A whole-unit split of a window, preferring the largest exact unit. */
export function splitWindow(seconds: number): {
  value: number
  unit: WindowUnit
} {
  if (seconds % 86400 === 0) return { value: seconds / 86400, unit: 'days' }
  if (seconds % 3600 === 0) return { value: seconds / 3600, unit: 'hours' }
  return { value: Math.round(seconds / 60), unit: 'minutes' }
}

export function windowLabel(t: TFunction, seconds: number): string {
  const { value, unit } = splitWindow(seconds)
  if (unit === 'days') return t('{{count}} days', { count: value })
  if (unit === 'hours') return t('{{count}} hours', { count: value })
  return t('{{count}} minutes', { count: value })
}

export function ruleTitle(t: TFunction, rule: UsageRule): string {
  if (rule.mode === 'calendar' && rule.calendar_period) {
    return t(CALENDAR_PERIOD_LABELS[rule.calendar_period])
  }
  return windowLabel(t, rule.window_seconds)
}

export function rulePercent(rule: UsageRuleView): number {
  if (rule.quota_limit <= 0) return 0
  return (rule.used_quota / rule.quota_limit) * 100
}

/** Remaining time until a reset, rounded up to minutes (hours beyond a day). */
export function formatDuration(t: TFunction, seconds: number): string {
  const minutes = Math.max(1, Math.ceil(seconds / 60))
  if (minutes >= 1440) {
    const hours = Math.ceil(minutes / 60)
    const days = Math.floor(hours / 24)
    const rest = hours % 24
    if (rest === 0) return t('{{days}}d', { days })
    return t('{{days}}d {{hours}}h', { days, hours: rest })
  }
  const hours = Math.floor(minutes / 60)
  const rest = minutes % 60
  if (hours === 0) return t('{{minutes}}m', { minutes: rest })
  if (rest === 0) return t('{{hours}}h', { hours })
  return t('{{hours}}h {{minutes}}m', { hours, minutes: rest })
}

export function progressTone(rule: UsageRuleView): string {
  const percent = rulePercent(rule)
  if (rule.state === 'exhausted' || percent >= 100) {
    return '[&_[data-slot=progress-indicator]]:bg-destructive'
  }
  if (percent >= 80) return '[&_[data-slot=progress-indicator]]:bg-warning'
  return ''
}

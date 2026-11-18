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
import type {
  ActionListKey,
  ActionType,
  ChannelStatus,
  MatchMode,
  ResponseModelCheck,
  Verdict,
} from '../types'

export const VERDICT_STYLES: Record<
  Verdict,
  {
    cell: string
    dot: string
    labelKey: string
    badge: 'success' | 'danger' | 'warning' | 'neutral'
  }
> = {
  passed: {
    cell: 'bg-success/85 text-white hover:bg-success',
    dot: 'bg-success',
    labelKey: 'Passed',
    badge: 'success',
  },
  replaced: {
    cell: 'bg-destructive/85 text-white hover:bg-destructive',
    dot: 'bg-destructive',
    labelKey: 'Suspected replacement',
    badge: 'danger',
  },
  failed: {
    cell: 'bg-warning/85 text-white hover:bg-warning',
    dot: 'bg-warning',
    labelKey: 'Request failed',
    badge: 'warning',
  },
  not_tested: {
    cell: 'bg-muted text-muted-foreground hover:bg-muted/80',
    dot: 'bg-muted-foreground/40',
    labelKey: 'Not tested',
    badge: 'neutral',
  },
}

export const VERDICT_ORDER: Verdict[] = [
  'passed',
  'replaced',
  'failed',
  'not_tested',
]

export const CHANNEL_STATUS: Record<
  ChannelStatus,
  { labelKey: string; badge: 'success' | 'warning' | 'neutral' }
> = {
  enabled: { labelKey: 'Enabled', badge: 'success' },
  auto_disabled: { labelKey: 'Auto disabled', badge: 'warning' },
  manual_disabled: { labelKey: 'Manually disabled', badge: 'neutral' },
}

export const MATCH_MODE_LABELS: Record<MatchMode, string> = {
  exact: 'Exact match',
  contains: 'Contains',
  regex: 'Regular expression',
}

export const RESPONSE_CHECK_LABELS: Record<ResponseModelCheck, string> = {
  off: 'Off',
  exact: 'Exact',
  loose: 'Ignore case and date suffix',
}

export type ActionValueKind =
  | 'none'
  | 'group'
  | 'tag'
  | 'optional_tag'
  | 'amount'
  | 'integer'
  | 'non_negative'

export const ACTION_DEFS: Record<
  ActionType,
  { labelKey: string; category: string; valueKind: ActionValueKind }
> = {
  add_group: {
    labelKey: 'Add to group',
    category: 'Groups',
    valueKind: 'group',
  },
  remove_group: {
    labelKey: 'Remove from group',
    category: 'Groups',
    valueKind: 'group',
  },
  increase_priority: {
    labelKey: 'Increase priority',
    category: 'Priority',
    valueKind: 'amount',
  },
  decrease_priority: {
    labelKey: 'Decrease priority',
    category: 'Priority',
    valueKind: 'amount',
  },
  set_priority: {
    labelKey: 'Set priority',
    category: 'Priority',
    valueKind: 'integer',
  },
  increase_weight: {
    labelKey: 'Increase weight',
    category: 'Weight',
    valueKind: 'amount',
  },
  decrease_weight: {
    labelKey: 'Decrease weight',
    category: 'Weight',
    valueKind: 'amount',
  },
  set_weight: {
    labelKey: 'Set weight',
    category: 'Weight',
    valueKind: 'non_negative',
  },
  set_tag: { labelKey: 'Set tag', category: 'Tag', valueKind: 'tag' },
  remove_tag: {
    labelKey: 'Remove tag',
    category: 'Tag',
    valueKind: 'optional_tag',
  },
  auto_disable: {
    labelKey: 'Disable automatically',
    category: 'Status',
    valueKind: 'none',
  },
  manual_disable: {
    labelKey: 'Disable manually',
    category: 'Status',
    valueKind: 'none',
  },
}

export const ACTION_CATEGORIES = [
  'Groups',
  'Priority',
  'Weight',
  'Tag',
  'Status',
]

export const ACTION_LISTS: {
  key: ActionListKey
  titleKey: string
  descriptionKey: string
  accent: string
}[] = [
  {
    key: 'pass_actions',
    titleKey: 'On pass',
    descriptionKey: 'Runs when every monitored model of the channel passes.',
    accent: 'border-t-success',
  },
  {
    key: 'fail_actions',
    titleKey: 'On suspected replacement',
    descriptionKey:
      'Runs when any model answers wrongly or returns another model.',
    accent: 'border-t-destructive',
  },
  {
    key: 'request_failure_actions',
    titleKey: 'On request failure',
    descriptionKey: 'Runs when a request still fails after its retries.',
    accent: 'border-t-warning',
  },
]

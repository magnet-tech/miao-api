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
import { useTranslation } from 'react-i18next'

import { Progress } from '@/components/ui/progress'
import { formatQuota, formatTimestamp } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { UsageRuleView } from '../api'
import { formatDuration, progressTone, rulePercent } from '../lib/rules'

/** Usage of one rule's current window: bar, used/limit and reset time. */
export function RuleUsage(props: { rule: UsageRuleView; className?: string }) {
  const { t } = useTranslation()
  const rule = props.rule
  const percent = rulePercent(rule)
  const remaining = rule.reset_at - Math.floor(Date.now() / 1000)

  let resetText = t('Starts with the next charged request')
  if (rule.state !== 'idle' || rule.mode === 'calendar') {
    resetText = t('Resets in {{duration}} · {{time}}', {
      duration: formatDuration(t, remaining),
      time: formatTimestamp(rule.reset_at),
    })
  }
  if (rule.state === 'exhausted') {
    resetText = t('Used up · available again at {{time}}', {
      time: formatTimestamp(rule.reset_at),
    })
  }

  return (
    <div className={cn('space-y-1.5', props.className)}>
      <Progress
        value={Math.min(100, percent)}
        className={cn('[&_[data-slot=progress-track]]:h-2', progressTone(rule))}
      />
      <div className='flex flex-wrap items-center justify-between gap-x-3 gap-y-0.5 text-xs'>
        <span className='text-muted-foreground tabular-nums'>
          {t('{{used}} of {{limit}} used', {
            used: formatQuota(rule.used_quota),
            limit: formatQuota(rule.quota_limit),
          })}
          {' · '}
          {Math.round(percent)}%
        </span>
        <span
          className={cn(
            'tabular-nums',
            rule.state === 'exhausted'
              ? 'text-destructive font-medium'
              : 'text-muted-foreground'
          )}
        >
          {resetText}
        </span>
      </div>
    </div>
  )
}

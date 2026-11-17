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
import { useQuery } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { MultiSelect } from '@/components/multi-select'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { getCurrencyLabel } from '@/lib/currency'
import { parseQuotaFromDollars, quotaUnitsToEditableAmount } from '@/lib/format'

import { getLimitModels, type CalendarPeriod, type UsageRule } from '../api'
import {
  CALENDAR_PERIOD_LABELS,
  CALENDAR_PERIOD_SECONDS,
  MAX_WINDOW_SECONDS,
  splitWindow,
  WINDOW_PRESETS,
  WINDOW_UNIT_SECONDS,
  type WindowUnit,
} from '../lib/rules'

type RuleEditorProps = {
  userId: number
  initial?: UsageRule
  usedWindows: number[]
  onCancel: () => void
  onApply: (rule: UsageRule) => void
}

function initialWindowChoice(rule?: UsageRule): string {
  if (!rule) return String(WINDOW_PRESETS[0].seconds)
  if (rule.mode === 'calendar') return 'calendar'
  if (WINDOW_PRESETS.some((preset) => preset.seconds === rule.window_seconds)) {
    return String(rule.window_seconds)
  }
  return 'custom'
}

/** Inline editor for one usage limit rule; changes apply to the drawer draft. */
export function RuleEditor(props: RuleEditorProps) {
  const { t } = useTranslation()
  const limitId = useId()
  const initial = props.initial
  const [choice, setChoice] = useState(initialWindowChoice(initial))
  const [period, setPeriod] = useState<CalendarPeriod>(
    initial?.calendar_period || 'day'
  )
  const custom = splitWindow(initial?.window_seconds ?? 7200)
  const [customValue, setCustomValue] = useState(String(custom.value))
  const [customUnit, setCustomUnit] = useState<WindowUnit>(custom.unit)
  const [amount, setAmount] = useState(
    initial ? String(quotaUnitsToEditableAmount(initial.quota_limit)) : ''
  )
  const [models, setModels] = useState<string[]>(initial?.models ?? [])
  const [error, setError] = useState('')

  const { data: modelGroups = [] } = useQuery({
    queryKey: ['usage-limit-models', props.userId],
    queryFn: async () => (await getLimitModels(props.userId)).data ?? [],
  })
  const modelOptions = modelGroups.flatMap((group) =>
    group.models.map((model) => ({
      label: model,
      value: model,
      hint: group.vendor || t('Other'),
    }))
  )

  const windowItems = [
    ...WINDOW_PRESETS.map((preset) => ({
      value: String(preset.seconds),
      label: t(preset.label),
    })),
    { value: 'custom', label: t('Custom') },
    { value: 'calendar', label: t('Calendar period') },
  ]
  const unitItems = [
    { value: 'minutes', label: t('Minutes') },
    { value: 'hours', label: t('Hours') },
    { value: 'days', label: t('Days') },
  ]
  const periodItems = (
    Object.keys(CALENDAR_PERIOD_LABELS) as CalendarPeriod[]
  ).map((key) => ({ value: key, label: t(CALENDAR_PERIOD_LABELS[key]) }))

  const apply = () => {
    let windowSeconds = Number(choice)
    let mode: UsageRule['mode'] = 'window'
    if (choice === 'calendar') {
      mode = 'calendar'
      windowSeconds = CALENDAR_PERIOD_SECONDS[period]
    } else if (choice === 'custom') {
      const value = Number(customValue)
      if (!Number.isInteger(value) || value < 1) {
        setError(t('Enter a whole number for the window.'))
        return
      }
      windowSeconds = value * WINDOW_UNIT_SECONDS[customUnit]
      if (windowSeconds > MAX_WINDOW_SECONDS) {
        setError(t('The window can be at most 90 days.'))
        return
      }
    }
    if (
      windowSeconds !== initial?.window_seconds &&
      props.usedWindows.includes(windowSeconds)
    ) {
      setError(t('Another rule already uses this time window.'))
      return
    }
    const quota = parseQuotaFromDollars(Number(amount))
    if (!Number.isFinite(Number(amount)) || quota < 1) {
      setError(t('Enter a limit greater than 0.'))
      return
    }
    props.onApply({
      window_seconds: windowSeconds,
      mode,
      calendar_period: mode === 'calendar' ? period : '',
      quota_limit: quota,
      enabled: initial?.enabled ?? true,
      models,
    })
  }

  return (
    <div className='bg-muted/30 space-y-4 rounded-lg border border-dashed p-3'>
      <div className='grid gap-3 sm:grid-cols-2'>
        <div className='space-y-1.5'>
          <Label>{t('Time window')}</Label>
          <Select
            items={windowItems}
            value={choice}
            onValueChange={(value) => value && setChoice(value)}
          >
            <SelectTrigger className='w-full'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                {windowItems.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>

        {choice === 'custom' ? (
          <div className='space-y-1.5'>
            <Label>{t('Custom window')}</Label>
            <div className='flex gap-2'>
              <Input
                inputMode='numeric'
                value={customValue}
                onChange={(event) => setCustomValue(event.target.value)}
                aria-label={t('Custom window')}
              />
              <Select
                items={unitItems}
                value={customUnit}
                onValueChange={(value) =>
                  value && setCustomUnit(value as WindowUnit)
                }
              >
                <SelectTrigger className='w-28'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    {unitItems.map((item) => (
                      <SelectItem key={item.value} value={item.value}>
                        {item.label}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
          </div>
        ) : null}

        {choice === 'calendar' ? (
          <div className='space-y-1.5'>
            <Label>{t('Period')}</Label>
            <Select
              items={periodItems}
              value={period}
              onValueChange={(value) =>
                value && setPeriod(value as CalendarPeriod)
              }
            >
              <SelectTrigger className='w-full'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {periodItems.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>
        ) : null}

        <div className='space-y-1.5'>
          <Label htmlFor={limitId}>
            {t('Limit')} ({getCurrencyLabel()})
          </Label>
          <Input
            id={limitId}
            type='number'
            step='any'
            min={0}
            value={amount}
            onChange={(event) => setAmount(event.target.value)}
            placeholder='20'
          />
        </div>
      </div>

      <div className='space-y-1.5'>
        <Label>{t('Models')}</Label>
        <MultiSelect
          options={modelOptions}
          selected={models}
          onChange={setModels}
          placeholder={t('All models')}
        />
        <p className='text-muted-foreground text-xs'>
          {t(
            'Leave empty to limit all models. Changing models or the mode starts a new window.'
          )}
        </p>
      </div>

      {error ? <p className='text-destructive text-xs'>{error}</p> : null}

      <div className='flex justify-end gap-2'>
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={props.onCancel}
        >
          {t('Cancel')}
        </Button>
        <Button type='button' size='sm' onClick={apply}>
          {initial ? t('Apply') : t('Add rule')}
        </Button>
      </div>
    </div>
  )
}

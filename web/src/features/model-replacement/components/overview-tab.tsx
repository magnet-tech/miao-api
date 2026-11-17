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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Progress } from '@/components/ui/progress'
import dayjs from '@/lib/dayjs'
import { requireServerSuccess } from '@/lib/server-error-message'
import { cn } from '@/lib/utils'

import { getChannels, getRuns, modelReplacementKeys } from '../api'
import { VERDICT_ORDER, VERDICT_STYLES } from '../lib/constants'
import type { DetectionOverview } from '../types'
import { CellDetailPanel } from './cell-detail-panel'
import { ModelRunTable, type CellSelection } from './model-run-table'

const MAX_RUNS = 48

function Kpi(props: {
  label: string
  value: string
  hint?: string
  tone?: string
}) {
  return (
    <Card className='gap-0 py-0'>
      <CardContent className='space-y-1 p-4'>
        <div className='text-muted-foreground text-xs'>{props.label}</div>
        <div className={cn('text-xl font-semibold tabular-nums', props.tone)}>
          {props.value}
        </div>
        {props.hint ? (
          <div className='text-muted-foreground text-xs'>{props.hint}</div>
        ) : null}
      </CardContent>
    </Card>
  )
}

function NextRunKpi(props: { overview: DetectionOverview | undefined }) {
  const { t } = useTranslation()
  const overview = props.overview
  if (overview?.active_run) {
    const active = overview.active_run
    const percent = active.total ? (active.done / active.total) * 100 : 0
    return (
      <Card className='gap-0 py-0'>
        <CardContent className='space-y-2 p-4'>
          <div className='text-muted-foreground text-xs'>
            {active.trigger === 'retest'
              ? t('Retesting channel #{{id}}', { id: active.channel_id })
              : t('Detection running')}
          </div>
          <Progress value={percent} />
          <div className='text-muted-foreground text-xs tabular-nums'>
            {t('{{done}} of {{total}} checks', {
              done: active.done,
              total: active.total,
            })}
          </div>
        </CardContent>
      </Card>
    )
  }
  if (!overview?.enabled || !overview.next_run_at) {
    return (
      <Kpi
        label={t('Next run')}
        value={t('Off')}
        hint={t('Automatic detection is turned off')}
      />
    )
  }
  const minutes = Math.max(
    0,
    Math.ceil((overview.next_run_at * 1000 - Date.now()) / 60_000)
  )
  return (
    <Kpi
      label={t('Next run')}
      value={t('In {{minutes}} min', { minutes })}
      hint={`${dayjs(overview.next_run_at * 1000).format('HH:mm')} · ${t('Every {{minutes}} min', { minutes: overview.interval_minutes })}`}
    />
  )
}

export function OverviewTab(props: {
  overview: DetectionOverview | undefined
  lastRuns: number
  onLastRunsChange: (value: number) => void
}) {
  const { t } = useTranslation()
  const [selection, setSelection] = useState<CellSelection | null>(null)
  const runsQuery = useQuery({
    queryKey: modelReplacementKeys.runs(MAX_RUNS + 4),
    queryFn: async () => requireServerSuccess(await getRuns(MAX_RUNS + 4)).data,
  })
  const channelsQuery = useQuery({
    queryKey: modelReplacementKeys.channels(),
    queryFn: async () => requireServerSuccess(await getChannels()).data ?? [],
  })
  const [draftLastRuns, setDraftLastRuns] = useState(String(props.lastRuns))

  const allRuns = runsQuery.data?.runs ?? []
  const runs = allRuns.slice(0, props.lastRuns).reverse()
  const channels = (channelsQuery.data ?? []).filter(
    (channel) => channel.in_scope || channel.latest
  )
  const models = props.overview?.models ?? []
  const counts = props.overview?.counts

  let effectiveSelection = selection
  if (!effectiveSelection && allRuns.length > 0) {
    const latest = allRuns.find((run) => run.trigger !== 'retest') ?? allRuns[0]
    const interesting =
      latest.cells.find((cell) => cell.verdict === 'replaced') ??
      latest.cells[0]
    if (interesting) {
      effectiveSelection = {
        runId: latest.id,
        channelId: interesting.channel_id,
        model: interesting.model,
      }
    }
  }

  const commitLastRuns = () => {
    const parsed = Number.parseInt(draftLastRuns, 10)
    const next = Number.isFinite(parsed)
      ? Math.min(MAX_RUNS, Math.max(1, parsed))
      : props.lastRuns
    setDraftLastRuns(String(next))
    props.onLastRunsChange(next)
  }

  return (
    <div className='space-y-4'>
      <div className='grid gap-4 sm:grid-cols-2 xl:grid-cols-4'>
        <Kpi
          label={t('Healthy channels')}
          value={counts ? `${counts.healthy} / ${counts.in_scope}` : '—'}
          hint={t('Enabled and passed the latest run')}
        />
        <Kpi
          label={t('Suspected replacement')}
          value={String(counts?.suspected ?? '—')}
          tone='text-destructive'
        />
        <Kpi
          label={t('Request failed')}
          value={String(counts?.failed ?? '—')}
          tone='text-warning'
        />
        <NextRunKpi overview={props.overview} />
      </div>

      <div className='flex flex-wrap items-center gap-x-4 gap-y-2'>
        <div className='flex items-center gap-2'>
          <Label htmlFor='mr-last-runs' className='whitespace-nowrap'>
            {t('Show the last')}
          </Label>
          <Input
            id='mr-last-runs'
            type='number'
            min={1}
            max={MAX_RUNS}
            inputMode='numeric'
            className='w-20 text-right tabular-nums'
            value={draftLastRuns}
            onChange={(event) => setDraftLastRuns(event.target.value)}
            onBlur={commitLastRuns}
            onKeyDown={(event) => {
              if (event.key === 'Enter') commitLastRuns()
            }}
          />
          <span className='text-sm'>{t('runs')}</span>
        </div>
        <div className='text-muted-foreground flex flex-wrap items-center gap-3 text-xs'>
          {VERDICT_ORDER.map((verdict) => (
            <span key={verdict} className='flex items-center gap-1.5'>
              <span
                className={cn('size-3 rounded-sm', VERDICT_STYLES[verdict].dot)}
              />
              {t(VERDICT_STYLES[verdict].labelKey)}
            </span>
          ))}
          <span>{t('* single channel retest')}</span>
          <span className='underline decoration-2 underline-offset-2'>
            {t('Underlined: returned another model')}
          </span>
        </div>
      </div>

      <div className='grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_400px]'>
        <div className='min-w-0 space-y-4'>
          {models.map((model) => (
            <ModelRunTable
              key={model}
              model={model}
              runs={runs}
              channels={channels}
              selected={effectiveSelection}
              onSelect={setSelection}
            />
          ))}
          {models.length === 0 ? (
            <Card className='gap-0 py-0'>
              <CardContent className='text-muted-foreground p-6 text-center text-sm'>
                {t('No monitored models yet. Add models in Settings.')}
              </CardContent>
            </Card>
          ) : null}
        </div>
        <div className='xl:sticky xl:top-4'>
          <CellDetailPanel
            selection={effectiveSelection}
            retestBusy={Boolean(props.overview?.active_run)}
          />
        </div>
      </div>
    </div>
  )
}

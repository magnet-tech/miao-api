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

import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import dayjs from '@/lib/dayjs'
import { requireServerSuccess } from '@/lib/server-error-message'
import { cn } from '@/lib/utils'

import { getChannels, getRuns, modelReplacementKeys } from '../api'
import { TRIGGER_LABELS, VERDICT_STYLES } from '../lib/constants'
import { describeAction } from '../lib/utils'
import type { DetectionRun } from '../types'

function formatDuration(
  ms: number,
  t: (key: string, options?: Record<string, unknown>) => string
) {
  const seconds = Math.round(ms / 1000)
  if (seconds < 60) return t('{{seconds}} s', { seconds })
  return t('{{minutes}} min {{seconds}} s', {
    minutes: Math.floor(seconds / 60),
    seconds: seconds % 60,
  })
}

function RunDialog(props: { run: DetectionRun | null; onClose: () => void }) {
  const { t } = useTranslation()
  const channelsQuery = useQuery({
    queryKey: modelReplacementKeys.channels(),
    queryFn: async () => requireServerSuccess(await getChannels()).data ?? [],
    enabled: props.run !== null,
  })
  const run = props.run
  const models = run ? [...new Set(run.cells.map((cell) => cell.model))] : []
  const channels = (channelsQuery.data ?? []).filter((channel) =>
    run?.channel_results.some((result) => result.channel_id === channel.id)
  )
  return (
    <Dialog
      open={run !== null}
      onOpenChange={(open) => !open && props.onClose()}
      title={
        run ? dayjs(run.started_at * 1000).format('YYYY-MM-DD HH:mm:ss') : ''
      }
      description={
        run
          ? `${t(TRIGGER_LABELS[run.trigger])} · ${formatDuration(run.duration_ms, t)}`
          : ''
      }
      contentClassName='sm:max-w-4xl'
      footer={
        <Button type='button' onClick={props.onClose}>
          {t('Close')}
        </Button>
      }
    >
      {run ? (
        <div className='overflow-x-auto'>
          <table className='w-full border-separate border-spacing-1 text-xs'>
            <thead>
              <tr>
                <th className='text-muted-foreground px-2 text-left font-normal'>
                  {t('Channel')}
                </th>
                {models.map((model) => (
                  <th
                    key={model}
                    className='text-muted-foreground px-1 font-normal'
                  >
                    <code>{model}</code>
                  </th>
                ))}
                <th className='text-muted-foreground px-2 text-left font-normal'>
                  {t('Actions')}
                </th>
              </tr>
            </thead>
            <tbody>
              {channels.map((channel) => {
                const result = run.channel_results.find(
                  (item) => item.channel_id === channel.id
                )
                return (
                  <tr key={channel.id}>
                    <th
                      scope='row'
                      className='px-2 text-left font-normal whitespace-nowrap'
                    >
                      {channel.name}{' '}
                      <span className='text-muted-foreground'>
                        #{channel.id}
                      </span>
                    </th>
                    {models.map((model) => {
                      const cell = run.cells.find(
                        (item) =>
                          item.channel_id === channel.id && item.model === model
                      )
                      const verdict = cell?.verdict ?? 'not_tested'
                      let text = '—'
                      if (cell && verdict === 'failed') text = t('Failed')
                      else if (cell) text = `${cell.match_percent}%`
                      return (
                        <td key={model}>
                          <div
                            className={cn(
                              'flex h-8 min-w-20 items-center justify-center rounded-md font-medium tabular-nums',
                              VERDICT_STYLES[verdict].cell
                            )}
                          >
                            {text}
                          </div>
                        </td>
                      )
                    })}
                    <td className='px-2'>
                      {result?.skipped_actions ? t('Recorded only') : null}
                      {!result?.skipped_actions
                        ? (result?.actions ?? [])
                            .filter((action) => action.changed)
                            .map((action) => describeAction(t, action))
                            .join(t(', ')) || '—'
                        : null}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      ) : null}
    </Dialog>
  )
}

export function HistoryTab() {
  const { t } = useTranslation()
  const [selected, setSelected] = useState<DetectionRun | null>(null)
  const query = useQuery({
    queryKey: modelReplacementKeys.runs(100),
    queryFn: async () => requireServerSuccess(await getRuns(100)).data,
  })
  const runs = query.data?.runs ?? []

  return (
    <div className='space-y-3'>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Records older than the retention period are removed automatically.'
        )}
      </p>
      <div className='overflow-hidden rounded-lg border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className='pl-4'>{t('Start time')}</TableHead>
              <TableHead>{t('Trigger')}</TableHead>
              <TableHead className='w-[35%]'>
                {t('Result distribution')}
              </TableHead>
              <TableHead>{t('Passed / Suspected / Failed')}</TableHead>
              <TableHead>{t('Duration')}</TableHead>
              <TableHead className='pr-4 text-right'>{t('Details')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {runs.map((run) => {
              const total =
                run.counts.passed + run.counts.replaced + run.counts.failed || 1
              return (
                <TableRow
                  key={run.id}
                  className='cursor-pointer'
                  onClick={() => setSelected(run)}
                >
                  <TableCell className='pl-4 tabular-nums'>
                    {dayjs(run.started_at * 1000).format('MM-DD HH:mm:ss')}
                  </TableCell>
                  <TableCell>
                    <StatusBadge
                      label={t(TRIGGER_LABELS[run.trigger])}
                      variant={run.trigger === 'schedule' ? 'neutral' : 'info'}
                      size='sm'
                    />
                  </TableCell>
                  <TableCell>
                    <div className='bg-muted flex h-2.5 w-full overflow-hidden rounded-full'>
                      <div
                        className='bg-success'
                        style={{
                          width: `${(run.counts.passed / total) * 100}%`,
                        }}
                      />
                      <div
                        className='bg-destructive'
                        style={{
                          width: `${(run.counts.replaced / total) * 100}%`,
                        }}
                      />
                      <div
                        className='bg-warning'
                        style={{
                          width: `${(run.counts.failed / total) * 100}%`,
                        }}
                      />
                    </div>
                  </TableCell>
                  <TableCell className='tabular-nums'>
                    <span className='text-success'>{run.counts.passed}</span>
                    {' / '}
                    <span className='text-destructive'>
                      {run.counts.replaced}
                    </span>
                    {' / '}
                    <span className='text-warning'>{run.counts.failed}</span>
                  </TableCell>
                  <TableCell className='text-muted-foreground'>
                    {formatDuration(run.duration_ms, t)}
                  </TableCell>
                  <TableCell className='pr-4 text-right'>
                    <Button
                      type='button'
                      variant='link'
                      size='sm'
                      className='h-auto p-0'
                      onClick={(event) => {
                        event.stopPropagation()
                        setSelected(run)
                      }}
                    >
                      {t('View')}
                    </Button>
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </div>
      <RunDialog run={selected} onClose={() => setSelected(null)} />
    </div>
  )
}

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

import { StatusBadge } from '@/components/status-badge'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import dayjs from '@/lib/dayjs'
import { cn } from '@/lib/utils'

import { CHANNEL_STATUS, VERDICT_STYLES } from '../lib/constants'
import type { DetectionChannel, DetectionRun, RunCell } from '../types'

export type CellSelection = {
  runId: string
  channelId: number
  model: string
}

type ModelRunTableProps = {
  model: string
  runs: DetectionRun[]
  channels: DetectionChannel[]
  selected: CellSelection | null
  onSelect: (selection: CellSelection) => void
}

function cellText(cell: RunCell | undefined, failedLabel: string): string {
  if (!cell || cell.verdict === 'not_tested') return '—'
  if (cell.verdict === 'failed') return failedLabel
  return `${cell.match_percent}%`
}

export function ModelRunTable(props: ModelRunTableProps) {
  const { t } = useTranslation()
  const rows = props.channels.filter(
    (channel) =>
      channel.monitored_models.includes(props.model) ||
      props.runs.some((run) =>
        run.cells.some(
          (cell) => cell.channel_id === channel.id && cell.model === props.model
        )
      )
  )
  const lookup = new Map<string, RunCell>()
  for (const run of props.runs) {
    for (const cell of run.cells) {
      if (cell.model === props.model) {
        lookup.set(`${run.id}|${cell.channel_id}`, cell)
      }
    }
  }

  return (
    <div className='bg-card overflow-hidden rounded-xl border'>
      <div className='overflow-x-auto'>
        <table className='w-max min-w-full border-separate border-spacing-1 p-2 text-xs'>
          <thead>
            <tr>
              <th
                scope='col'
                className='bg-card sticky left-0 z-10 min-w-36 px-2 py-1 text-left align-bottom'
              >
                <code className='text-foreground text-sm font-semibold'>
                  {props.model}
                </code>
              </th>
              {props.runs.map((run, index) => {
                const time = dayjs(run.started_at * 1000)
                const previous = props.runs[index - 1]
                const newDay =
                  !previous ||
                  !dayjs(previous.started_at * 1000).isSame(time, 'day')
                return (
                  <th
                    key={run.id}
                    scope='col'
                    className='text-muted-foreground min-w-11 px-0.5 py-1 text-center font-normal'
                  >
                    <Tooltip>
                      <TooltipTrigger
                        render={
                          <span className='inline-flex flex-col items-center leading-tight' />
                        }
                      >
                        <span className='text-[10px]'>
                          {newDay ? time.format('MM-DD') : ' '}
                        </span>
                        <span className='tabular-nums'>
                          {time.format('HH:mm')}
                          {run.trigger === 'retest' ? '*' : ''}
                        </span>
                      </TooltipTrigger>
                      <TooltipContent>
                        {time.format('YYYY-MM-DD HH:mm:ss')}
                        {run.trigger === 'retest'
                          ? ` · ${t('Single channel retest')}`
                          : ''}
                        {run.trigger === 'manual'
                          ? ` · ${t('Started manually')}`
                          : ''}
                      </TooltipContent>
                    </Tooltip>
                  </th>
                )
              })}
            </tr>
          </thead>
          <tbody>
            {rows.map((channel) => (
              <tr key={channel.id}>
                <th
                  scope='row'
                  className='bg-card sticky left-0 z-10 px-2 text-left font-normal'
                >
                  <div className='flex max-w-40 min-w-0 items-center gap-1.5'>
                    <span className='truncate text-sm' title={channel.name}>
                      {channel.name}
                    </span>
                    <span className='text-muted-foreground shrink-0'>
                      #{channel.id}
                    </span>
                  </div>
                  {channel.status !== 'enabled' ? (
                    <StatusBadge
                      className='-mt-0.5'
                      label={t(CHANNEL_STATUS[channel.status].labelKey)}
                      variant={CHANNEL_STATUS[channel.status].badge}
                      size='sm'
                    />
                  ) : null}
                </th>
                {props.runs.map((run) => {
                  const cell = lookup.get(`${run.id}|${channel.id}`)
                  const verdict = cell?.verdict ?? 'not_tested'
                  const active =
                    props.selected?.runId === run.id &&
                    props.selected.channelId === channel.id &&
                    props.selected.model === props.model
                  return (
                    <td key={run.id} className='p-0'>
                      <button
                        type='button'
                        disabled={!cell}
                        aria-pressed={active}
                        aria-label={`${channel.name} ${dayjs(run.started_at * 1000).format('HH:mm')} ${t(VERDICT_STYLES[verdict].labelKey)}`}
                        onClick={() =>
                          props.onSelect({
                            runId: run.id,
                            channelId: channel.id,
                            model: props.model,
                          })
                        }
                        className={cn(
                          'flex h-8 w-full min-w-11 items-center justify-center rounded-md font-medium tabular-nums transition-colors disabled:cursor-default',
                          VERDICT_STYLES[verdict].cell,
                          cell?.response_model_mismatch &&
                            verdict !== 'passed' &&
                            'underline decoration-2 underline-offset-2',
                          active && 'ring-foreground ring-2 ring-offset-1'
                        )}
                      >
                        {cellText(cell, t('Failed'))}
                      </button>
                    </td>
                  )
                })}
              </tr>
            ))}
            {rows.length === 0 ? (
              <tr>
                <td
                  colSpan={props.runs.length + 1}
                  className='text-muted-foreground px-2 py-6 text-center'
                >
                  {t('No channel serves this model.')}
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
    </div>
  )
}

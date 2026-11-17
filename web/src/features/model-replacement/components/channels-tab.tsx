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
import { ChevronDown, ChevronRight, RefreshCw } from 'lucide-react'
import { Fragment, useState } from 'react'
import { useTranslation } from 'react-i18next'

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

import { getChannels, modelReplacementKeys } from '../api'
import { CHANNEL_STATUS, VERDICT_STYLES } from '../lib/constants'
import { useRetestChannel } from '../lib/hooks'
import { describeAction, useModelReplacementAccess } from '../lib/utils'
import type { DetectionChannel, Verdict } from '../types'

type Filter = 'all' | Verdict

const FILTERS: { key: Filter; labelKey: string }[] = [
  { key: 'all', labelKey: 'All' },
  { key: 'passed', labelKey: 'Healthy' },
  { key: 'replaced', labelKey: 'Suspected replacement' },
  { key: 'failed', labelKey: 'Request failed' },
  { key: 'not_tested', labelKey: 'Not tested' },
]

function verdictOf(channel: DetectionChannel): Verdict {
  return channel.latest?.verdict ?? 'not_tested'
}

export function ChannelsTab(props: {
  activeChannelId: number | null
  busy: boolean
}) {
  const { t } = useTranslation()
  const access = useModelReplacementAccess()
  const retest = useRetestChannel()
  const [filter, setFilter] = useState<Filter>('all')
  const [expanded, setExpanded] = useState<number | null>(null)
  const query = useQuery({
    queryKey: modelReplacementKeys.channels(),
    queryFn: async () => requireServerSuccess(await getChannels()).data ?? [],
  })
  const channels = query.data ?? []
  const rows = channels.filter(
    (channel) => filter === 'all' || verdictOf(channel) === filter
  )

  return (
    <div className='space-y-3'>
      <div className='bg-muted flex w-fit flex-wrap rounded-lg p-0.5'>
        {FILTERS.map((item) => {
          const count =
            item.key === 'all'
              ? channels.length
              : channels.filter((channel) => verdictOf(channel) === item.key)
                  .length
          return (
            <button
              key={item.key}
              type='button'
              aria-pressed={filter === item.key}
              onClick={() => setFilter(item.key)}
              className={cn(
                'rounded-md px-3 py-1 text-xs font-medium transition-colors',
                filter === item.key
                  ? 'bg-background text-foreground shadow-sm'
                  : 'text-muted-foreground hover:text-foreground'
              )}
            >
              {t(item.labelKey)} {count}
            </button>
          )
        })}
      </div>

      <div className='overflow-hidden rounded-lg border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className='w-10' />
              <TableHead>{t('Channel')}</TableHead>
              <TableHead>{t('Groups')}</TableHead>
              <TableHead>{t('Latest result')}</TableHead>
              <TableHead>{t('Last run')}</TableHead>
              <TableHead className='text-right'>{t('Avg latency')}</TableHead>
              <TableHead>{t('Last actions')}</TableHead>
              <TableHead className='w-32 text-right'>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((channel) => {
              const verdict = verdictOf(channel)
              const open = expanded === channel.id
              const changedActions =
                channel.latest?.actions.filter((action) => action.changed) ?? []
              const retesting = props.activeChannelId === channel.id
              return (
                <Fragment key={channel.id}>
                  <TableRow>
                    <TableCell>
                      <Button
                        type='button'
                        variant='ghost'
                        size='icon-sm'
                        aria-expanded={open}
                        aria-label={open ? t('Collapse') : t('Expand')}
                        onClick={() => setExpanded(open ? null : channel.id)}
                      >
                        {open ? <ChevronDown /> : <ChevronRight />}
                      </Button>
                    </TableCell>
                    <TableCell>
                      <div className='font-medium'>{channel.name}</div>
                      <div className='text-muted-foreground flex items-center gap-1.5 text-xs'>
                        #{channel.id}
                        <StatusBadge
                          label={t(CHANNEL_STATUS[channel.status].labelKey)}
                          variant={CHANNEL_STATUS[channel.status].badge}
                          size='sm'
                        />
                        {!channel.in_scope ? (
                          <span>{t('Out of scope')}</span>
                        ) : null}
                      </div>
                    </TableCell>
                    <TableCell className='text-xs'>
                      {channel.groups.join(', ') || '—'}
                    </TableCell>
                    <TableCell>
                      <StatusBadge
                        label={t(VERDICT_STYLES[verdict].labelKey)}
                        variant={VERDICT_STYLES[verdict].badge}
                        size='sm'
                      />
                    </TableCell>
                    <TableCell className='text-muted-foreground text-xs tabular-nums'>
                      {channel.latest
                        ? dayjs(channel.latest.started_at * 1000).format(
                            'MM-DD HH:mm'
                          )
                        : '—'}
                    </TableCell>
                    <TableCell className='text-right tabular-nums'>
                      {channel.latest?.avg_latency_ms
                        ? `${channel.latest.avg_latency_ms} ms`
                        : '—'}
                    </TableCell>
                    <TableCell className='max-w-56 text-xs whitespace-normal'>
                      {changedActions.length > 0
                        ? changedActions
                            .map((action) => describeAction(t, action))
                            .join(t(', '))
                        : '—'}
                    </TableCell>
                    <TableCell className='text-right'>
                      {access.canOperate ? (
                        <Button
                          type='button'
                          size='sm'
                          variant='outline'
                          disabled={
                            props.busy || !channel.in_scope || retest.isPending
                          }
                          onClick={() => retest.mutate(channel.id)}
                        >
                          <RefreshCw
                            className={cn(retesting && 'animate-spin')}
                            aria-hidden='true'
                          />
                          {retesting ? t('Retesting...') : t('Retest')}
                        </Button>
                      ) : null}
                    </TableCell>
                  </TableRow>
                  {open ? (
                    <TableRow className='bg-muted/30 hover:bg-muted/30'>
                      <TableCell />
                      <TableCell colSpan={7}>
                        <div className='flex flex-wrap gap-2 py-1'>
                          {(channel.latest?.cells ?? []).map((cell) => (
                            <span
                              key={cell.model}
                              className='bg-background flex items-center gap-2 rounded-md border px-2 py-1 text-xs'
                            >
                              <span
                                className={cn(
                                  'size-2.5 rounded-full',
                                  VERDICT_STYLES[cell.verdict].dot
                                )}
                              />
                              <code>{cell.model}</code>
                              <span className='text-muted-foreground tabular-nums'>
                                {cell.verdict === 'failed'
                                  ? t('Failed')
                                  : `${cell.match_percent}%`}
                              </span>
                              {cell.response_model_mismatch ? (
                                <span className='text-destructive'>
                                  {t('Returned another model')}
                                </span>
                              ) : null}
                            </span>
                          ))}
                          {!channel.latest ? (
                            <span className='text-muted-foreground text-xs'>
                              {t('Not tested yet')}
                            </span>
                          ) : null}
                        </div>
                        <div className='text-muted-foreground mt-1 text-xs'>
                          {t(
                            'Priority {{priority}} · Weight {{weight}} · Tag {{tag}}',
                            {
                              priority: channel.priority,
                              weight: channel.weight,
                              tag: channel.tag || '—',
                            }
                          )}
                        </div>
                      </TableCell>
                    </TableRow>
                  ) : null}
                </Fragment>
              )
            })}
            {rows.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={8}
                  className='text-muted-foreground py-8 text-center'
                >
                  {t('No channels match this filter.')}
                </TableCell>
              </TableRow>
            ) : null}
          </TableBody>
        </Table>
      </div>
    </div>
  )
}

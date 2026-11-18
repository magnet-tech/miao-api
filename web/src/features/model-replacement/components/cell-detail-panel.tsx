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
import { FlaskConical, Loader2, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import dayjs from '@/lib/dayjs'
import {
  requireServerSuccess,
  translateServerText,
} from '@/lib/server-error-message'
import { cn } from '@/lib/utils'

import { getCandidates, getCellDetail, modelReplacementKeys } from '../api'
import {
  CHANNEL_STATUS,
  MATCH_MODE_LABELS,
  RESPONSE_CHECK_LABELS,
  VERDICT_STYLES,
} from '../lib/constants'
import { useRetestChannel } from '../lib/hooks'
import { describeAction, useModelReplacementAccess } from '../lib/utils'
import type { CellDetail } from '../types'
import type { CellSelection } from './model-run-table'
import { ResponseModelLine } from './response-model-line'
import { TrialRunPanel } from './trial-run-panel'

function TrialDialog(props: {
  detail: CellDetail
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const candidates = useQuery({
    queryKey: modelReplacementKeys.candidates(),
    queryFn: async () => requireServerSuccess(await getCandidates()).data ?? [],
    enabled: props.open,
  })
  const channels = candidates.data?.find(
    (item) => item.model === props.detail.model
  )?.channels ?? [props.detail.channel]
  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Trial run')}
      description={t(
        'Model {{model}} with the saved question and expected answer.',
        {
          model: props.detail.model,
        }
      )}
      contentClassName='sm:max-w-lg'
    >
      <TrialRunPanel
        key={props.detail.channel.id}
        config={{
          model: props.detail.model,
          question: props.detail.question,
          expected_answer: props.detail.expected_answer,
          match_mode: props.detail.match_mode,
          response_model_check: props.detail.response_model_check,
        }}
        channels={channels}
        defaultChannelId={props.detail.channel.id}
      />
    </Dialog>
  )
}

export function CellDetailPanel(props: {
  selection: CellSelection | null
  retestBusy: boolean
}) {
  const { t } = useTranslation()
  const access = useModelReplacementAccess()
  const retest = useRetestChannel()
  const [trialOpen, setTrialOpen] = useState(false)
  const selection = props.selection
  const query = useQuery({
    queryKey: selection
      ? modelReplacementKeys.cell(
          selection.runId,
          selection.channelId,
          selection.model
        )
      : [...modelReplacementKeys.all, 'cell', 'none'],
    queryFn: async () => {
      if (!selection) return null
      return (
        requireServerSuccess(
          await getCellDetail(
            selection.runId,
            selection.channelId,
            selection.model
          )
        ).data ?? null
      )
    },
    enabled: selection !== null,
  })

  if (!selection) {
    return (
      <Card className='gap-0 py-0'>
        <CardContent className='text-muted-foreground p-6 text-center text-sm'>
          {t('Select a cell to see the answers of that run.')}
        </CardContent>
      </Card>
    )
  }
  const detail = query.data
  if (!detail) {
    return (
      <Card className='gap-0 py-0'>
        <CardContent className='text-muted-foreground flex items-center justify-center gap-2 p-6 text-sm'>
          <Loader2 className='size-4 animate-spin' aria-hidden='true' />
          {t('Loading...')}
        </CardContent>
      </Card>
    )
  }
  const style = VERDICT_STYLES[detail.verdict]
  return (
    <Card className='gap-0 py-0'>
      <CardHeader className='border-b py-3'>
        <CardTitle className='flex flex-wrap items-center gap-2 text-sm'>
          <span>{detail.channel.name}</span>
          <span className='text-muted-foreground font-normal'>
            #{detail.channel.id}
          </span>
          {detail.channel.status !== 'enabled' ? (
            <StatusBadge
              label={t(CHANNEL_STATUS[detail.channel.status].labelKey)}
              variant={CHANNEL_STATUS[detail.channel.status].badge}
              size='sm'
            />
          ) : null}
          <StatusBadge
            className='ml-auto'
            label={t(style.labelKey)}
            variant={style.badge}
            size='sm'
          />
        </CardTitle>
        <div className='text-muted-foreground flex flex-wrap gap-x-3 gap-y-1 text-xs'>
          <code className='text-foreground'>{detail.model}</code>
          <span>
            {dayjs(detail.run.started_at * 1000).format('YYYY-MM-DD HH:mm:ss')}
          </span>
          <span>
            {t('Match rate')} {detail.match_percent}%
          </span>
        </div>
      </CardHeader>
      <CardContent className='space-y-3 p-4 text-sm'>
        <dl className='bg-muted/40 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 rounded-md p-3 text-xs'>
          <dt className='text-muted-foreground'>{t('Question')}</dt>
          <dd className='break-words'>{detail.question || '—'}</dd>
          <dt className='text-muted-foreground'>{t('Expected answer')}</dt>
          <dd>
            <code>{detail.expected_answer}</code>
            <span className='text-muted-foreground'>
              {' '}
              · {t(MATCH_MODE_LABELS[detail.match_mode])}
            </span>
          </dd>
          <dt className='text-muted-foreground'>{t('Response model check')}</dt>
          <dd>{t(RESPONSE_CHECK_LABELS[detail.response_model_check])}</dd>
        </dl>

        <ol className='space-y-2'>
          {detail.attempts.map((attempt) => (
            <li key={attempt.round} className='rounded-md border p-3'>
              <div className='flex items-center justify-between text-xs'>
                <span className='font-medium'>
                  {t('Round {{round}}', { round: attempt.round })}
                  {attempt.tries > 1 ? (
                    <span className='text-muted-foreground font-normal'>
                      {' '}
                      · {t('{{count}} tries', { count: attempt.tries })}
                    </span>
                  ) : null}
                </span>
                <span className='text-muted-foreground tabular-nums'>
                  {attempt.latency_ms ? `${attempt.latency_ms} ms` : '—'}
                </span>
              </div>
              {attempt.error ? (
                <p className='text-destructive mt-1 text-xs'>
                  {translateServerText(t, attempt.error)}
                </p>
              ) : (
                <div className='mt-1 space-y-0.5 text-xs'>
                  <div>
                    <span className='text-muted-foreground'>
                      {t('Answer')}:{' '}
                    </span>
                    <span
                      className={cn(
                        attempt.answer_matched
                          ? 'text-success'
                          : 'text-destructive'
                      )}
                    >
                      {attempt.answer}
                    </span>
                  </div>
                  <ResponseModelLine
                    model={detail.model}
                    responseModel={attempt.response_model}
                    note={attempt.response_model_note}
                  />
                </div>
              )}
            </li>
          ))}
        </ol>

        <div className='text-xs'>
          <div className='text-muted-foreground mb-1'>
            {t('Actions on this channel')}
          </div>
          {detail.skipped_actions ? (
            <p>
              {t('The channel was disabled, so the result was only recorded.')}
            </p>
          ) : null}
          {!detail.skipped_actions && detail.actions.length === 0 ? (
            <p>{t('None')}</p>
          ) : null}
          <div className='flex flex-wrap gap-1.5'>
            {detail.actions.map((action) => (
              <span
                key={`${action.type}:${action.value}:${action.amount ?? ''}`}
                className={cn(
                  'rounded border px-1.5 py-0.5',
                  action.changed
                    ? 'border-primary/40 bg-primary/5'
                    : 'text-muted-foreground'
                )}
              >
                {describeAction(t, action)}
                {action.changed ? '' : ` (${t('no change')})`}
              </span>
            ))}
          </div>
        </div>

        {access.canOperate ? (
          <div className='flex flex-wrap justify-end gap-2 border-t pt-3'>
            <Button
              type='button'
              size='sm'
              variant='outline'
              onClick={() => setTrialOpen(true)}
            >
              <FlaskConical aria-hidden='true' />
              {t('Trial run')}
            </Button>
            <Button
              type='button'
              size='sm'
              disabled={props.retestBusy || retest.isPending}
              onClick={() => retest.mutate(detail.channel.id)}
            >
              <RefreshCw aria-hidden='true' />
              {t('Retest channel')}
            </Button>
          </div>
        ) : null}
      </CardContent>
      <TrialDialog
        detail={detail}
        open={trialOpen}
        onOpenChange={setTrialOpen}
      />
    </Card>
  )
}

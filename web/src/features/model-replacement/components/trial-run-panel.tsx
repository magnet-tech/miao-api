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
import { useMutation } from '@tanstack/react-query'
import { FlaskConical, Loader2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { handleServerError } from '@/lib/handle-server-error'
import {
  requireServerSuccess,
  translateServerText,
} from '@/lib/server-error-message'

import { trialRun } from '../api'
import { CHANNEL_STATUS, VERDICT_STYLES } from '../lib/constants'
import type { ChannelInfo, MonitoredModel, TrialResult } from '../types'
import { ResponseModelLine } from './response-model-line'

type TrialRunPanelProps = {
  config: Pick<
    MonitoredModel,
    | 'model'
    | 'question'
    | 'expected_answer'
    | 'match_mode'
    | 'response_model_check'
  >
  channels: ChannelInfo[]
  defaultChannelId?: number
  disabled?: boolean
}

export function TrialRunPanel(props: TrialRunPanelProps) {
  const { t } = useTranslation()
  const [channelId, setChannelId] = useState<string>(
    String(props.defaultChannelId ?? props.channels[0]?.id ?? '')
  )
  const [result, setResult] = useState<TrialResult | null>(null)
  const mutation = useMutation({
    mutationFn: async () =>
      requireServerSuccess(
        await trialRun({
          channel_id: Number(channelId),
          model: props.config.model,
          question: props.config.question,
          expected_answer: props.config.expected_answer,
          match_mode: props.config.match_mode,
          response_model_check: props.config.response_model_check,
        })
      ),
    onSuccess: (response) => setResult(response.data ?? null),
    onError: (error) => handleServerError(error, t('Trial run failed')),
  })

  const items = props.channels.map((channel) => ({
    value: String(channel.id),
    label: `${channel.name} #${channel.id}`,
  }))

  return (
    <div className='space-y-3'>
      <div className='flex flex-wrap items-end gap-2'>
        <div className='min-w-48 flex-1 space-y-1.5'>
          <Label>{t('Channel')}</Label>
          <Select
            items={items}
            value={channelId}
            onValueChange={(value) => {
              if (value === null) return
              setChannelId(value)
              setResult(null)
            }}
          >
            <SelectTrigger className='w-full'>
              <SelectValue placeholder={t('Select a channel')} />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                {props.channels.map((channel) => (
                  <SelectItem key={channel.id} value={String(channel.id)}>
                    {channel.name} #{channel.id}
                    {channel.status !== 'enabled'
                      ? ` · ${t(CHANNEL_STATUS[channel.status].labelKey)}`
                      : ''}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>
        <Button
          type='button'
          variant='outline'
          disabled={props.disabled || !channelId || mutation.isPending}
          onClick={() => mutation.mutate()}
        >
          {mutation.isPending ? (
            <Loader2 className='animate-spin' aria-hidden='true' />
          ) : (
            <FlaskConical aria-hidden='true' />
          )}
          {t('Trial run')}
        </Button>
      </div>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Sends the question once with the settings shown here. Nothing is saved and no actions run.'
        )}
      </p>
      {result ? (
        <div className='bg-muted/40 space-y-2 rounded-lg border p-3 text-sm'>
          <div className='flex flex-wrap items-center gap-2'>
            <span className='font-medium'>{t('Result')}</span>
            <StatusBadge
              label={t(VERDICT_STYLES[result.verdict].labelKey)}
              variant={VERDICT_STYLES[result.verdict].badge}
              size='sm'
            />
            <span className='text-muted-foreground ml-auto text-xs tabular-nums'>
              {result.latency_ms} ms
            </span>
          </div>
          {result.error ? (
            <p className='text-destructive text-xs'>
              {translateServerText(t, result.error)}
            </p>
          ) : (
            <div className='space-y-1 text-xs'>
              <div>
                <span className='text-muted-foreground'>{t('Answer')}: </span>
                <span
                  className={
                    result.answer_matched ? 'text-success' : 'text-destructive'
                  }
                >
                  {result.answer}
                </span>
              </div>
              <ResponseModelLine
                model={result.model}
                responseModel={result.response_model}
                note={result.response_model_note}
              />
            </div>
          )}
        </div>
      ) : null}
    </div>
  )
}

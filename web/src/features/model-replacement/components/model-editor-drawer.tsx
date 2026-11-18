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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  SideDrawerSection,
  SideDrawerSectionHeader,
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Textarea } from '@/components/ui/textarea'

import {
  CHANNEL_STATUS,
  MATCH_MODE_LABELS,
  RESPONSE_CHECK_LABELS,
} from '../lib/constants'
import type {
  CandidateModel,
  MatchMode,
  MonitoredModel,
  ResponseModelCheck,
} from '../types'
import { TrialRunPanel } from './trial-run-panel'

const EMPTY_MODEL: MonitoredModel = {
  model: '',
  question: '',
  expected_answer: '',
  match_mode: 'exact',
  response_model_check: 'loose',
  retry_times: 2,
  excluded_channel_ids: [],
}

const RESPONSE_CHECK_HINTS: Record<ResponseModelCheck, string> = {
  off: 'Only the answer is checked.',
  exact: 'The model name returned by the channel must equal the test model.',
  loose: 'Case differences and date suffixes such as -2026-11-01 are accepted.',
}

type Errors = Partial<
  Record<'model' | 'question' | 'expected_answer' | 'retry_times', string>
>

function validate(draft: MonitoredModel, taken: string[]): Errors {
  const errors: Errors = {}
  if (!draft.model) errors.model = 'Select a model.'
  else if (taken.includes(draft.model)) {
    errors.model = 'Each model can be monitored only once.'
  }
  if (!draft.question.trim()) errors.question = 'Question is required.'
  if (!draft.expected_answer.trim()) {
    errors.expected_answer = 'Expected answer is required.'
  } else if (draft.match_mode === 'regex') {
    try {
      new RegExp(draft.expected_answer)
    } catch {
      errors.expected_answer =
        'The expected answer is not a valid regular expression.'
    }
  }
  if (
    !Number.isInteger(draft.retry_times) ||
    draft.retry_times < 0 ||
    draft.retry_times > 10
  ) {
    errors.retry_times = 'Retry times must be between 0 and 10.'
  }
  return errors
}

function FieldError(props: { message?: string }) {
  const { t } = useTranslation()
  if (!props.message) return null
  return <p className='text-destructive text-xs'>{t(props.message)}</p>
}

type ModelEditorDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  initial: MonitoredModel | null
  takenModels: string[]
  candidates: CandidateModel[]
  readOnly: boolean
  onApply: (config: MonitoredModel) => void
}

export function ModelEditorDrawer(props: ModelEditorDrawerProps) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState<MonitoredModel>(
    props.initial ?? EMPTY_MODEL
  )
  const [submitted, setSubmitted] = useState(false)
  const errors = validate(draft, props.takenModels)
  const channels =
    props.candidates.find((item) => item.model === draft.model)?.channels ?? []
  const modelItems = props.candidates.map((item) => ({
    value: item.model,
    label: item.model,
  }))
  const set = <K extends keyof MonitoredModel>(
    key: K,
    value: MonitoredModel[K]
  ) => setDraft((previous) => ({ ...previous, [key]: value }))

  const apply = () => {
    setSubmitted(true)
    if (Object.keys(errors).length > 0) return
    props.onApply({
      ...draft,
      question: draft.question.trim(),
      expected_answer: draft.expected_answer.trim(),
    })
  }

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-[640px]')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>
            {props.initial
              ? t('Edit monitored model')
              : t('Add monitored model')}
          </SheetTitle>
          <SheetDescription>
            {t('Changes take effect after you save the settings.')}
          </SheetDescription>
        </SheetHeader>
        <div className={sideDrawerFormClassName()}>
          <SideDrawerSection>
            <SideDrawerSectionHeader title={t('Question and answer')} />
            <div className='space-y-1.5'>
              <Label>{t('Model')}</Label>
              <Select
                items={modelItems}
                value={draft.model || null}
                disabled={props.readOnly}
                onValueChange={(value) => {
                  if (value === null) return
                  setDraft((previous) => ({
                    ...previous,
                    model: value,
                    excluded_channel_ids: [],
                  }))
                }}
              >
                <SelectTrigger
                  className='w-full'
                  aria-invalid={submitted && Boolean(errors.model)}
                >
                  <SelectValue placeholder={t('Select a model')} />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    {props.candidates.map((item) => (
                      <SelectItem key={item.model} value={item.model}>
                        {item.model}
                        <span className='text-muted-foreground ml-2 text-xs'>
                          {t('{{count}} channels', {
                            count: item.channels.length,
                          })}
                        </span>
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
              {submitted ? <FieldError message={errors.model} /> : null}
            </div>
            <div className='space-y-1.5'>
              <Label htmlFor='mr-question'>{t('Question')}</Label>
              <Textarea
                id='mr-question'
                rows={3}
                value={draft.question}
                disabled={props.readOnly}
                placeholder={t('A question with one short, predictable answer')}
                onChange={(event) => set('question', event.target.value)}
              />
              {submitted ? <FieldError message={errors.question} /> : null}
            </div>
            <div className='grid gap-3 sm:grid-cols-[1fr_180px]'>
              <div className='space-y-1.5'>
                <Label htmlFor='mr-expected'>{t('Expected answer')}</Label>
                <Input
                  id='mr-expected'
                  value={draft.expected_answer}
                  disabled={props.readOnly}
                  onChange={(event) =>
                    set('expected_answer', event.target.value)
                  }
                />
                {submitted ? (
                  <FieldError message={errors.expected_answer} />
                ) : null}
              </div>
              <div className='space-y-1.5'>
                <Label>{t('Match mode')}</Label>
                <Select
                  items={(Object.keys(MATCH_MODE_LABELS) as MatchMode[]).map(
                    (mode) => ({
                      value: mode,
                      label: t(MATCH_MODE_LABELS[mode]),
                    })
                  )}
                  value={draft.match_mode}
                  disabled={props.readOnly}
                  onValueChange={(value) =>
                    value && set('match_mode', value as MatchMode)
                  }
                >
                  <SelectTrigger className='w-full'>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      {(Object.keys(MATCH_MODE_LABELS) as MatchMode[]).map(
                        (mode) => (
                          <SelectItem key={mode} value={mode}>
                            {t(MATCH_MODE_LABELS[mode])}
                          </SelectItem>
                        )
                      )}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </div>
            </div>
            <div className='grid gap-3 sm:grid-cols-[1fr_180px]'>
              <div className='space-y-1.5'>
                <Label>{t('Response model check')}</Label>
                <Select
                  items={(
                    Object.keys(RESPONSE_CHECK_LABELS) as ResponseModelCheck[]
                  ).map((mode) => ({
                    value: mode,
                    label: t(RESPONSE_CHECK_LABELS[mode]),
                  }))}
                  value={draft.response_model_check}
                  disabled={props.readOnly}
                  onValueChange={(value) =>
                    value &&
                    set('response_model_check', value as ResponseModelCheck)
                  }
                >
                  <SelectTrigger className='w-full'>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      {(
                        Object.keys(
                          RESPONSE_CHECK_LABELS
                        ) as ResponseModelCheck[]
                      ).map((mode) => (
                        <SelectItem key={mode} value={mode}>
                          {t(RESPONSE_CHECK_LABELS[mode])}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <p className='text-muted-foreground text-xs'>
                  {t(RESPONSE_CHECK_HINTS[draft.response_model_check])}
                </p>
              </div>
              <div className='space-y-1.5'>
                <Label htmlFor='mr-retry'>{t('Retry times')}</Label>
                <Input
                  id='mr-retry'
                  inputMode='numeric'
                  className='text-right tabular-nums'
                  value={
                    Number.isNaN(draft.retry_times)
                      ? ''
                      : String(draft.retry_times)
                  }
                  disabled={props.readOnly}
                  onChange={(event) =>
                    set('retry_times', Number.parseInt(event.target.value, 10))
                  }
                />
                {submitted ? <FieldError message={errors.retry_times} /> : null}
              </div>
            </div>
          </SideDrawerSection>

          <SideDrawerSection>
            <SideDrawerSectionHeader
              title={t('Channels')}
              description={t(
                'Unchecked channels are excluded. Channels added later are included automatically. Disabled channels are only tested when you retest them.'
              )}
            />
            {channels.length === 0 ? (
              <p className='text-muted-foreground text-sm'>
                {t('Select a model to list the channels that serve it.')}
              </p>
            ) : (
              <ul className='grid gap-2 sm:grid-cols-2'>
                {channels.map((channel) => {
                  const checked = !draft.excluded_channel_ids.includes(
                    channel.id
                  )
                  const disabled = channel.status !== 'enabled'
                  return (
                    <li key={channel.id}>
                      <label
                        className={`flex items-center gap-2 rounded-md border px-3 py-2 text-sm ${disabled ? 'text-muted-foreground bg-muted/40' : ''}`}
                      >
                        <Checkbox
                          checked={checked}
                          disabled={props.readOnly}
                          onCheckedChange={(value) =>
                            set(
                              'excluded_channel_ids',
                              value
                                ? draft.excluded_channel_ids.filter(
                                    (id) => id !== channel.id
                                  )
                                : [...draft.excluded_channel_ids, channel.id]
                            )
                          }
                        />
                        <span className='min-w-0 flex-1 truncate'>
                          {channel.name}{' '}
                          <span className='text-muted-foreground'>
                            #{channel.id}
                          </span>
                        </span>
                        {disabled ? (
                          <StatusBadge
                            label={t(CHANNEL_STATUS[channel.status].labelKey)}
                            variant={CHANNEL_STATUS[channel.status].badge}
                            size='sm'
                          />
                        ) : null}
                      </label>
                    </li>
                  )
                })}
              </ul>
            )}
          </SideDrawerSection>

          <SideDrawerSection>
            <SideDrawerSectionHeader title={t('Trial run')} />
            {channels.length > 0 &&
            !errors.expected_answer &&
            draft.question.trim() ? (
              <TrialRunPanel
                key={draft.model}
                config={draft}
                channels={channels}
                disabled={props.readOnly}
              />
            ) : (
              <p className='text-muted-foreground text-sm'>
                {t(
                  'Fill in the model, question and expected answer to try them on a channel.'
                )}
              </p>
            )}
          </SideDrawerSection>
        </div>
        <SheetFooter className={sideDrawerFooterClassName()}>
          <SheetClose render={<Button variant='outline' />}>
            {t('Cancel')}
          </SheetClose>
          {!props.readOnly ? (
            <Button type='button' onClick={apply}>
              {props.initial ? t('Apply changes') : t('Add model')}
            </Button>
          ) : null}
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}

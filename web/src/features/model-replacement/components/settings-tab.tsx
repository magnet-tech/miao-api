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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil, Plus, Trash2, X } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  getCandidates,
  getSettings,
  modelReplacementKeys,
  updateSettings,
} from '../api'
import { MATCH_MODE_LABELS, RESPONSE_CHECK_LABELS } from '../lib/constants'
import { useStableKeys } from '../lib/stable-keys'
import { useModelReplacementAccess } from '../lib/utils'
import type { DetectionSettings, MonitoredModel } from '../types'
import { ActionRules } from './action-rules'
import { ModelEditorDrawer } from './model-editor-drawer'

function NumberField(props: {
  id: string
  label: string
  value: number
  unit: string
  disabled: boolean
  onChange: (value: number) => void
}) {
  return (
    <div className='space-y-1.5'>
      <Label
        htmlFor={props.id}
        className='text-muted-foreground text-xs font-normal'
      >
        {props.label}
      </Label>
      <div className='flex items-center gap-2'>
        <Input
          id={props.id}
          inputMode='numeric'
          className='text-right tabular-nums'
          value={Number.isNaN(props.value) ? '' : String(props.value)}
          disabled={props.disabled}
          onChange={(event) =>
            props.onChange(Number.parseInt(event.target.value, 10))
          }
        />
        <span className='text-muted-foreground w-10 shrink-0 text-sm'>
          {props.unit}
        </span>
      </div>
    </div>
  )
}

type EditorState = { index: number | null } | null

export function SettingsTab(props: { initialEdit?: string }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const access = useModelReplacementAccess()
  const readOnly = !access.canConfigure
  const settingsQuery = useQuery({
    queryKey: modelReplacementKeys.settings(),
    queryFn: async () => requireServerSuccess(await getSettings()).data,
  })
  const candidatesQuery = useQuery({
    queryKey: modelReplacementKeys.candidates(),
    queryFn: async () => requireServerSuccess(await getCandidates()).data ?? [],
  })
  const [draft, setDraft] = useState<DetectionSettings | null>(null)
  const [deleting, setDeleting] = useState<number | null>(null)
  const stableKeys = useStableKeys()
  const base = settingsQuery.data?.settings
  const current = draft ?? base
  const [editor, setEditor] = useState<EditorState>(() => {
    if (props.initialEdit === 'new') return { index: null }
    return null
  })
  const [pendingEdit, setPendingEdit] = useState(
    props.initialEdit && props.initialEdit !== 'new' ? props.initialEdit : ''
  )
  if (pendingEdit && current) {
    const index = current.models.findIndex((item) => item.model === pendingEdit)
    setPendingEdit('')
    if (index >= 0) setEditor({ index })
  }

  const save = useMutation({
    mutationFn: async (settings: DetectionSettings) =>
      requireServerSuccess(await updateSettings(settings)),
    onSuccess: () => {
      toast.success(t('Settings saved'))
      setDraft(null)
      void queryClient.invalidateQueries({ queryKey: modelReplacementKeys.all })
    },
    onError: (error) => handleServerError(error, t('Failed to save settings')),
  })

  if (!current) {
    return (
      <p className='text-muted-foreground p-6 text-sm'>{t('Loading...')}</p>
    )
  }
  const dirty = draft !== null && JSON.stringify(draft) !== JSON.stringify(base)
  const update = (patch: Partial<DetectionSettings>) =>
    setDraft({ ...current, ...patch })
  const candidates = candidatesQuery.data ?? []
  const editing = editor?.index != null ? current.models[editor.index] : null

  return (
    <div className='space-y-4 pb-20'>
      {readOnly ? (
        <p className='bg-muted text-muted-foreground rounded-md px-3 py-2 text-sm'>
          {t('You can view these settings but not change them.')}
        </p>
      ) : null}
      <div className='grid gap-4 lg:grid-cols-2'>
        <Card className='gap-0 py-0'>
          <CardHeader className='border-b py-3'>
            <CardTitle className='text-sm'>{t('Schedule')}</CardTitle>
            <CardDescription>
              {t('Time windows use the server time zone ({{zone}}).', {
                zone: settingsQuery.data?.server_time_zone ?? '',
              })}
            </CardDescription>
          </CardHeader>
          <CardContent className='space-y-4 p-4'>
            <label className='flex items-center justify-between gap-3 text-sm'>
              <span>{t('Automatic detection')}</span>
              <Switch
                checked={current.enabled}
                disabled={readOnly}
                onCheckedChange={(value) => update({ enabled: value })}
              />
            </label>
            <div className='grid grid-cols-2 gap-3'>
              <NumberField
                id='mr-interval'
                label={t('Default interval')}
                value={current.interval_minutes}
                unit={t('min')}
                disabled={readOnly}
                onChange={(value) => update({ interval_minutes: value })}
              />
              <NumberField
                id='mr-retention'
                label={t('Keep records for')}
                value={current.retention_days}
                unit={t('days')}
                disabled={readOnly}
                onChange={(value) => update({ retention_days: value })}
              />
            </div>
            <div className='space-y-2'>
              <div className='text-muted-foreground text-xs'>
                {t('Interval by time of day')}
              </div>
              {current.interval_windows.map((window, index) => {
                const windowKeys = stableKeys.keys(
                  'windows',
                  current.interval_windows.length
                )
                const patch = (next: Partial<typeof window>) =>
                  update({
                    interval_windows: current.interval_windows.map(
                      (item, position) =>
                        position === index ? { ...item, ...next } : item
                    ),
                  })
                return (
                  <div
                    key={windowKeys[index]}
                    className='flex flex-wrap items-center gap-2 text-sm'
                  >
                    <Input
                      aria-label={t('Start')}
                      value={window.start}
                      placeholder='HH:MM'
                      disabled={readOnly}
                      className='w-20 tabular-nums'
                      onChange={(event) => patch({ start: event.target.value })}
                    />
                    <span>–</span>
                    <Input
                      aria-label={t('End')}
                      value={window.end}
                      placeholder='HH:MM'
                      disabled={readOnly}
                      className='w-20 tabular-nums'
                      onChange={(event) => patch({ end: event.target.value })}
                    />
                    <span className='text-muted-foreground'>{t('every')}</span>
                    <Input
                      aria-label={t('Interval')}
                      inputMode='numeric'
                      value={
                        Number.isNaN(window.interval_minutes)
                          ? ''
                          : String(window.interval_minutes)
                      }
                      disabled={readOnly}
                      className='w-16 text-right tabular-nums'
                      onChange={(event) =>
                        patch({
                          interval_minutes: Number.parseInt(
                            event.target.value,
                            10
                          ),
                        })
                      }
                    />
                    <span className='text-muted-foreground'>{t('min')}</span>
                    {!readOnly ? (
                      <Button
                        type='button'
                        variant='ghost'
                        size='icon-sm'
                        aria-label={t('Remove time window')}
                        onClick={() => {
                          stableKeys.remove('windows', index)
                          update({
                            interval_windows: current.interval_windows.filter(
                              (_, position) => position !== index
                            ),
                          })
                        }}
                      >
                        <X />
                      </Button>
                    ) : null}
                  </div>
                )
              })}
              {current.interval_windows.length === 0 ? (
                <p className='text-muted-foreground text-xs'>
                  {t('The default interval applies all day.')}
                </p>
              ) : null}
              {!readOnly ? (
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() => {
                    stableKeys.add('windows')
                    update({
                      interval_windows: [
                        ...current.interval_windows,
                        { start: '09:00', end: '18:00', interval_minutes: 5 },
                      ],
                    })
                  }}
                >
                  <Plus aria-hidden='true' />
                  {t('Add time window')}
                </Button>
              ) : null}
            </div>
          </CardContent>
        </Card>

        <Card className='gap-0 py-0'>
          <CardHeader className='border-b py-3'>
            <CardTitle className='text-sm'>{t('Probe parameters')}</CardTitle>
          </CardHeader>
          <CardContent className='grid grid-cols-2 gap-3 p-4'>
            <NumberField
              id='mr-rounds'
              label={t('Rounds per run')}
              value={current.rounds}
              unit={t('rounds')}
              disabled={readOnly}
              onChange={(value) => update({ rounds: value })}
            />
            <NumberField
              id='mr-concurrency'
              label={t('Channels tested at once')}
              value={current.concurrency}
              unit=''
              disabled={readOnly}
              onChange={(value) => update({ concurrency: value })}
            />
            <NumberField
              id='mr-timeout'
              label={t('Request timeout')}
              value={current.timeout_seconds}
              unit={t('s')}
              disabled={readOnly}
              onChange={(value) => update({ timeout_seconds: value })}
            />
            <NumberField
              id='mr-threshold'
              label={t('Threshold')}
              value={current.threshold}
              unit='%'
              disabled={readOnly}
              onChange={(value) => update({ threshold: value })}
            />
            <p className='text-muted-foreground col-span-2 text-xs'>
              {t(
                'A model is a suspected replacement when the share of mismatched rounds is above the threshold. 0 means any mismatched round counts.'
              )}
            </p>
          </CardContent>
        </Card>
      </div>

      <Card className='gap-0 py-0'>
        <CardHeader className='border-b py-3'>
          <CardTitle className='text-sm'>{t('Monitored models')}</CardTitle>
          {!readOnly ? (
            <CardAction>
              <Button
                type='button'
                size='sm'
                variant='outline'
                onClick={() => setEditor({ index: null })}
              >
                <Plus aria-hidden='true' />
                {t('Add model')}
              </Button>
            </CardAction>
          ) : null}
        </CardHeader>
        <CardContent className='p-0'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className='pl-4'>{t('Model')}</TableHead>
                <TableHead>{t('Question')}</TableHead>
                <TableHead>{t('Expected answer')}</TableHead>
                <TableHead>{t('Response model check')}</TableHead>
                <TableHead className='text-right'>{t('Retry times')}</TableHead>
                <TableHead className='text-right'>{t('Channels')}</TableHead>
                <TableHead className='pr-4 text-right'>
                  {t('Actions')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {current.models.map((config, index) => {
                const serving =
                  candidates.find((item) => item.model === config.model)
                    ?.channels.length ?? 0
                return (
                  <TableRow key={config.model}>
                    <TableCell className='pl-4'>
                      <code className='text-xs'>{config.model}</code>
                    </TableCell>
                    <TableCell
                      className='max-w-64 truncate'
                      title={config.question}
                    >
                      {config.question}
                    </TableCell>
                    <TableCell>
                      <code className='text-xs'>{config.expected_answer}</code>
                      <span className='text-muted-foreground text-xs'>
                        {' '}
                        · {t(MATCH_MODE_LABELS[config.match_mode])}
                      </span>
                    </TableCell>
                    <TableCell>
                      <Badge variant='outline'>
                        {t(RESPONSE_CHECK_LABELS[config.response_model_check])}
                      </Badge>
                    </TableCell>
                    <TableCell className='text-right tabular-nums'>
                      {config.retry_times}
                    </TableCell>
                    <TableCell className='text-right tabular-nums'>
                      {Math.max(
                        0,
                        serving - config.excluded_channel_ids.length
                      )}{' '}
                      / {serving}
                    </TableCell>
                    <TableCell className='pr-4'>
                      <div className='flex justify-end gap-1'>
                        <Button
                          type='button'
                          variant='ghost'
                          size='sm'
                          onClick={() => setEditor({ index })}
                        >
                          <Pencil aria-hidden='true' />
                          {readOnly ? t('View') : t('Edit')}
                        </Button>
                        {!readOnly ? (
                          <Button
                            type='button'
                            variant='ghost'
                            size='icon-sm'
                            aria-label={t('Delete')}
                            onClick={() => setDeleting(index)}
                          >
                            <Trash2 />
                          </Button>
                        ) : null}
                      </div>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <Card className='gap-0 py-0'>
        <CardHeader className='border-b py-3'>
          <CardTitle className='text-sm'>{t('Action rules')}</CardTitle>
          <CardDescription>
            {t(
              'Actions run in order after each detection. Disabled channels are only recorded.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='p-4'>
          <ActionRules
            settings={current}
            readOnly={readOnly}
            onChange={(key, actions) =>
              update({ [key]: actions } as Partial<DetectionSettings>)
            }
          />
        </CardContent>
      </Card>

      {dirty ? (
        <div className='bg-background/95 supports-[backdrop-filter]:bg-background/80 sticky bottom-0 z-20 -mx-1 flex flex-wrap items-center justify-end gap-2 rounded-lg border px-4 py-3 shadow-md backdrop-blur'>
          <span className='text-muted-foreground mr-auto text-sm'>
            {t('You have unsaved changes.')}
          </span>
          <Button
            type='button'
            variant='outline'
            onClick={() => setDraft(null)}
          >
            {t('Discard changes')}
          </Button>
          <Button
            type='button'
            disabled={save.isPending}
            onClick={() => save.mutate(current)}
          >
            {save.isPending ? t('Saving...') : t('Save settings')}
          </Button>
        </div>
      ) : null}

      {editor ? (
        <ModelEditorDrawer
          key={editor.index ?? 'new'}
          open
          onOpenChange={(open) => !open && setEditor(null)}
          initial={editing}
          takenModels={current.models
            .filter((_, position) => position !== editor.index)
            .map((item) => item.model)}
          candidates={candidates}
          readOnly={readOnly}
          onApply={(config: MonitoredModel) => {
            const models =
              editor.index == null
                ? [...current.models, config]
                : current.models.map((item, position) =>
                    position === editor.index ? config : item
                  )
            update({ models })
            setEditor(null)
          }}
        />
      ) : null}

      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => !open && setDeleting(null)}
        title={t('Remove monitored model')}
        desc={t(
          'Stop monitoring {{model}}? Its past records are kept until they expire.',
          { model: deleting !== null ? current.models[deleting]?.model : '' }
        )}
        confirmText={t('Remove')}
        destructive
        handleConfirm={() => {
          if (deleting === null) return
          update({
            models: current.models.filter(
              (_, position) => position !== deleting
            ),
          })
          setDeleting(null)
        }}
      />
    </div>
  )
}

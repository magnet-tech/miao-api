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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Gauge, Pencil, Plus, RotateCcw, Timer, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  SideDrawerSection,
  SideDrawerSectionHeader,
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { StatusBadge } from '@/components/status-badge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { hasPermission } from '@/lib/admin-permissions'
import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'

import {
  getUserLimits,
  resetUserLimitWindow,
  updateUserLimits,
  type UsageRule,
  type UsageRuleView,
} from '../api'
import { MAX_RULES, ruleTitle } from '../lib/rules'
import { RuleEditor } from './rule-editor'
import { RuleUsage } from './rule-usage'

type DraftRule = UsageRule & { view?: UsageRuleView }

type UserLimitsDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  user: { id: number; username: string }
  onSaved?: () => void
}

const stripView = (rule: DraftRule): UsageRule => ({
  window_seconds: rule.window_seconds,
  mode: rule.mode,
  calendar_period: rule.calendar_period,
  quota_limit: rule.quota_limit,
  enabled: rule.enabled,
  models: rule.models,
})

const sameScope = (a: UsageRule, b: UsageRule) =>
  a.window_seconds === b.window_seconds &&
  a.mode === b.mode &&
  [...a.models].sort().join(',') === [...b.models].sort().join(',')

export function UserLimitsDrawer(props: UserLimitsDrawerProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentUser = useAuthStore((state) => state.auth.user)
  const canWrite = hasPermission(currentUser, 'user_limit', 'write')
  const canReset = hasPermission(currentUser, 'user_limit', 'reset')

  const query = useQuery({
    queryKey: ['user-limits', props.user.id],
    queryFn: async () => {
      const result = await getUserLimits(props.user.id)
      if (!result.success || !result.data) throw new Error(result.message)
      return result.data
    },
    enabled: props.open,
  })

  const [concurrency, setConcurrency] = useState('0')
  const [rules, setRules] = useState<DraftRule[]>([])
  const [editing, setEditing] = useState<number | 'new' | null>(null)
  const [resetTarget, setResetTarget] = useState<DraftRule | null>(null)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!query.data) return
    setConcurrency(String(query.data.max_concurrency))
    setRules(
      query.data.rules.map((rule) => ({ ...stripView(rule), view: rule }))
    )
    setEditing(null)
  }, [query.data])

  const dirty = useMemo(() => {
    if (!query.data) return false
    const saved = JSON.stringify({
      c: query.data.max_concurrency,
      r: query.data.rules.map(stripView),
    })
    return (
      saved !==
      JSON.stringify({ c: Number(concurrency), r: rules.map(stripView) })
    )
  }, [query.data, concurrency, rules])

  const save = async () => {
    const max = Number(concurrency)
    if (!Number.isInteger(max) || max < 0 || max > 1000) {
      toast.error(t('Concurrency limit must be a whole number from 0 to 1000.'))
      return
    }
    setSaving(true)
    try {
      const result = await updateUserLimits(props.user.id, {
        max_concurrency: max,
        rules: rules.map(stripView),
      })
      if (!result.success) {
        handleServerError(result, t('Failed to save usage limits'))
        return
      }
      toast.success(t('Usage limits saved'))
      await queryClient.invalidateQueries({
        queryKey: ['user-limits', props.user.id],
      })
      props.onSaved?.()
    } catch (error) {
      handleServerError(error, t('Failed to save usage limits'))
    } finally {
      setSaving(false)
    }
  }

  const reset = async () => {
    if (!resetTarget) return
    const result = await resetUserLimitWindow(
      props.user.id,
      resetTarget.window_seconds
    )
    setResetTarget(null)
    if (!result.success) {
      handleServerError(result, t('Failed to reset the window'))
      return
    }
    toast.success(t('Window reset'))
    await queryClient.invalidateQueries({
      queryKey: ['user-limits', props.user.id],
    })
    props.onSaved?.()
  }

  const applyRule = (index: number | 'new', rule: UsageRule) => {
    setRules((current) => {
      if (index === 'new') return [...current, rule]
      return current.map((item, i) => {
        if (i !== index) return item
        const keepView =
          item.view && sameScope(item, rule) ? item.view : undefined
        return { ...rule, view: keepView }
      })
    })
    setEditing(null)
  }

  const usedWindows = rules.map((rule) => rule.window_seconds)

  return (
    <>
      <Sheet open={props.open} onOpenChange={props.onOpenChange}>
        <SheetContent
          className={sideDrawerContentClassName('sm:max-w-[640px]')}
        >
          <SheetHeader className={sideDrawerHeaderClassName()}>
            <SheetTitle>{t('Usage Limits')}</SheetTitle>
            <SheetDescription>
              {t(
                '{{username}} · Calendar periods follow the server time zone ({{zone}}).',
                {
                  username: props.user.username,
                  zone: query.data?.server_time_zone ?? 'UTC',
                }
              )}
            </SheetDescription>
          </SheetHeader>

          <div className={sideDrawerFormClassName()}>
            {query.isLoading ? (
              <div className='space-y-3'>
                <Skeleton className='h-20 w-full' />
                <Skeleton className='h-32 w-full' />
              </div>
            ) : (
              <>
                <SideDrawerSection>
                  <SideDrawerSectionHeader
                    icon={<Gauge />}
                    iconTone='primary'
                    title={t('Concurrency')}
                    description={t(
                      'Maximum requests this user can run at the same time across all API keys. 0 means unlimited.'
                    )}
                  />
                  <div className='flex flex-wrap items-center gap-3'>
                    <Input
                      type='number'
                      min={0}
                      max={1000}
                      step={1}
                      className='w-32'
                      value={concurrency}
                      disabled={!canWrite}
                      onChange={(event) => setConcurrency(event.target.value)}
                      aria-label={t('Maximum concurrency')}
                    />
                    <span className='text-muted-foreground text-sm'>
                      {t('{{count}} requests in progress now', {
                        count: query.data?.in_flight ?? 0,
                      })}
                    </span>
                  </div>
                </SideDrawerSection>

                <SideDrawerSection>
                  <SideDrawerSectionHeader
                    icon={<Timer />}
                    iconTone='info'
                    title={t('Usage limit rules')}
                    description={t(
                      'Each rule caps spending in a time window. When a window is used up, matching requests are rejected until it resets.'
                    )}
                  />

                  {rules.length === 0 && editing !== 'new' ? (
                    <p className='text-muted-foreground rounded-lg border border-dashed p-4 text-center text-sm'>
                      {t(
                        'No usage limit rules. This user can spend without a time-window cap.'
                      )}
                    </p>
                  ) : null}

                  {rules.map((rule, index) =>
                    editing === index ? (
                      <RuleEditor
                        key={`edit-${rule.window_seconds}`}
                        userId={props.user.id}
                        initial={rule}
                        usedWindows={usedWindows}
                        onCancel={() => setEditing(null)}
                        onApply={(next) => applyRule(index, next)}
                      />
                    ) : (
                      <div
                        key={rule.window_seconds}
                        className='space-y-3 rounded-lg border p-3'
                      >
                        <div className='flex flex-wrap items-center gap-2'>
                          <span className='text-sm font-medium'>
                            {ruleTitle(t, rule)}
                          </span>
                          <Badge variant='outline'>
                            {rule.mode === 'calendar'
                              ? t('Calendar period')
                              : t('Rolling window')}
                          </Badge>
                          {rule.view?.state === 'exhausted' ? (
                            <StatusBadge
                              label={t('Used up')}
                              variant='danger'
                              size='sm'
                            />
                          ) : null}
                          {rule.view?.state === 'active' ? (
                            <StatusBadge
                              label={t('In progress')}
                              variant='info'
                              size='sm'
                            />
                          ) : null}
                          <div className='ml-auto flex items-center gap-1'>
                            <Switch
                              checked={rule.enabled}
                              disabled={!canWrite}
                              aria-label={t('Enable rule')}
                              onCheckedChange={(checked) =>
                                setRules((current) =>
                                  current.map((item, i) =>
                                    i === index
                                      ? { ...item, enabled: checked }
                                      : item
                                  )
                                )
                              }
                            />
                            <Button
                              type='button'
                              variant='ghost'
                              size='icon-sm'
                              aria-label={t('Edit rule')}
                              disabled={!canWrite}
                              onClick={() => setEditing(index)}
                            >
                              <Pencil />
                            </Button>
                            <Button
                              type='button'
                              variant='ghost'
                              size='icon-sm'
                              aria-label={t('Reset window')}
                              disabled={
                                !canReset ||
                                !rule.view ||
                                rule.view.state === 'idle'
                              }
                              onClick={() => setResetTarget(rule)}
                            >
                              <RotateCcw />
                            </Button>
                            <Button
                              type='button'
                              variant='ghost'
                              size='icon-sm'
                              aria-label={t('Delete rule')}
                              disabled={!canWrite}
                              onClick={() =>
                                setRules((current) =>
                                  current.filter((_, i) => i !== index)
                                )
                              }
                            >
                              <Trash2 />
                            </Button>
                          </div>
                        </div>
                        <div className='flex flex-wrap gap-1'>
                          {rule.models.length === 0 ? (
                            <span className='text-muted-foreground text-xs'>
                              {t('All models')}
                            </span>
                          ) : (
                            rule.models.map((model) => (
                              <Badge
                                key={model}
                                variant='secondary'
                                className='font-mono'
                              >
                                {model}
                              </Badge>
                            ))
                          )}
                        </div>
                        {rule.view ? (
                          <RuleUsage rule={rule.view} />
                        ) : (
                          <p className='text-muted-foreground text-xs'>
                            {t(
                              'New or changed rule. Usage starts counting after you save.'
                            )}
                          </p>
                        )}
                      </div>
                    )
                  )}

                  {editing === 'new' ? (
                    <RuleEditor
                      userId={props.user.id}
                      usedWindows={usedWindows}
                      onCancel={() => setEditing(null)}
                      onApply={(next) => applyRule('new', next)}
                    />
                  ) : (
                    <Button
                      type='button'
                      variant='outline'
                      disabled={!canWrite || rules.length >= MAX_RULES}
                      onClick={() => setEditing('new')}
                    >
                      <Plus />
                      {t('Add rule ({{count}} left)', {
                        count: MAX_RULES - rules.length,
                      })}
                    </Button>
                  )}
                </SideDrawerSection>
              </>
            )}
          </div>

          <SheetFooter className={sideDrawerFooterClassName()}>
            <SheetClose render={<Button variant='outline' />}>
              {t('Close')}
            </SheetClose>
            <Button
              onClick={save}
              disabled={!canWrite || !dirty || saving || editing !== null}
            >
              {saving ? t('Saving...') : t('Save changes')}
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>

      <ConfirmDialog
        open={resetTarget !== null}
        onOpenChange={(open) => !open && setResetTarget(null)}
        title={t('Reset window')}
        desc={t(
          'End the current {{rule}} window for {{username}}? Its used amount returns to 0 now.',
          {
            rule: resetTarget ? ruleTitle(t, resetTarget) : '',
            username: props.user.username,
          }
        )}
        confirmText={t('Reset window')}
        handleConfirm={reset}
      />
    </>
  )
}

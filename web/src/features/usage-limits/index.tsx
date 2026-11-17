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
import {
  ArrowDown,
  ArrowUp,
  ChevronsUpDown,
  RotateCcw,
  Search,
  SlidersHorizontal,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { SectionPageLayout } from '@/components/layout'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useDebounce } from '@/hooks/use-debounce'
import { hasPermission } from '@/lib/admin-permissions'
import { handleServerError } from '@/lib/handle-server-error'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import {
  getLimitOverview,
  resetUserLimitWindow,
  type LimitOverviewFilter,
  type LimitOverviewItem,
} from './api'
import { UserLimitsDrawer } from './components/user-limits-drawer'
import {
  formatDuration,
  progressTone,
  rulePercent,
  ruleTitle,
} from './lib/rules'

type SortKey = 'id' | 'usage'
type SortState = { key: SortKey; desc: boolean }

/** Header button that toggles sorting, styled like the users table header. */
function SortHeader(props: {
  title: string
  sortKey: SortKey
  sort: SortState
  onSort: (sort: SortState) => void
}) {
  const active = props.sort.key === props.sortKey
  let icon = <ChevronsUpDown className='ms-2 h-4 w-4' />
  if (active) {
    icon = props.sort.desc ? (
      <ArrowDown className='ms-2 h-4 w-4' />
    ) : (
      <ArrowUp className='ms-2 h-4 w-4' />
    )
  }
  return (
    <Button
      variant='ghost'
      size='sm'
      className='-ms-3 h-8'
      onClick={() =>
        props.onSort({
          key: props.sortKey,
          desc: active ? !props.sort.desc : true,
        })
      }
    >
      <span>{props.title}</span>
      {icon}
    </Button>
  )
}

export function UsageLimitsOverview() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [keyword, setKeyword] = useState('')
  const debouncedKeyword = useDebounce(keyword.trim(), 300)
  const currentUser = useAuthStore((state) => state.auth.user)
  const canReset = hasPermission(currentUser, 'user_limit', 'reset')
  const [filter, setFilter] = useState<LimitOverviewFilter>('all')
  const [sort, setSort] = useState<SortState>({ key: 'id', desc: true })
  const [managing, setManaging] = useState<LimitOverviewItem | null>(null)
  const [resetting, setResetting] = useState<LimitOverviewItem | null>(null)

  const query = useQuery({
    queryKey: ['usage-limit-overview', debouncedKeyword, filter],
    queryFn: async () => {
      const result = await getLimitOverview({
        keyword: debouncedKeyword,
        filter,
      })
      if (!result.success || !result.data) throw new Error(result.message)
      return result.data
    },
    refetchInterval: 30_000,
  })

  const rows = useMemo(() => {
    const items = [...(query.data?.items ?? [])]
    const value = (item: LimitOverviewItem) =>
      sort.key === 'id' ? item.id : item.highest_percent
    items.sort((a, b) =>
      sort.desc ? value(b) - value(a) : value(a) - value(b)
    )
    return items
  }, [query.data, sort])

  const resetAll = async () => {
    if (!resetting) return
    const result = await resetUserLimitWindow(resetting.id)
    setResetting(null)
    if (!result.success) {
      handleServerError(result, t('Failed to reset the window'))
      return
    }
    toast.success(t('Windows reset'))
    await queryClient.invalidateQueries({ queryKey: ['usage-limit-overview'] })
  }

  const counts = query.data?.counts ?? { all: 0, near: 0, exhausted: 0 }
  const now = Math.floor(Date.now() / 1000)

  const columns: StaticDataTableColumn<LimitOverviewItem>[] = [
    {
      id: 'id',
      header: (
        <SortHeader title='ID' sortKey='id' sort={sort} onSort={setSort} />
      ),
      className: 'w-20',
      cellClassName: 'align-top tabular-nums',
      cell: (item) => item.id,
    },
    {
      id: 'username',
      header: t('Username'),
      className: 'w-48',
      cellClassName: 'align-top',
      cell: (item) => (
        <div>
          <div className='font-medium'>{item.username}</div>
          <div className='text-muted-foreground text-xs'>
            {item.display_name}
          </div>
          {item.status !== 1 ? (
            <StatusBadge
              label={t('Disabled')}
              variant='neutral'
              size='sm'
              className='mt-1'
            />
          ) : null}
        </div>
      ),
    },
    {
      id: 'usage',
      header: (
        <SortHeader
          title={t('Usage by rule')}
          sortKey='usage'
          sort={sort}
          onSort={setSort}
        />
      ),
      cell: (item) => (
        <div className='space-y-2 py-1'>
          {item.rules.map((rule) => {
            const percent = rulePercent(rule)
            let reset = t('Not started')
            if (rule.reset_at > 0) {
              reset = formatDuration(t, rule.reset_at - now)
            }
            return (
              <div
                key={rule.window_seconds}
                className={cn(
                  'grid grid-cols-[140px_1fr_150px] items-center gap-3 text-xs',
                  !rule.enabled && 'opacity-50'
                )}
              >
                <span className='truncate'>
                  {ruleTitle(t, rule)}
                  {rule.models.length > 0
                    ? ` · ${t('{{count}} models', { count: rule.models.length })}`
                    : ''}
                </span>
                <Progress
                  value={Math.min(100, percent)}
                  className={cn(
                    '[&_[data-slot=progress-track]]:h-2',
                    progressTone(rule)
                  )}
                />
                <span
                  className={cn(
                    'tabular-nums',
                    percent >= 100
                      ? 'text-destructive font-medium'
                      : 'text-muted-foreground'
                  )}
                >
                  {Math.round(percent)}% · {reset}
                </span>
              </div>
            )
          })}
        </div>
      ),
    },
    {
      id: 'concurrency',
      header: t('Concurrency'),
      className: 'w-32',
      cellClassName: 'align-top tabular-nums',
      cell: (item) =>
        item.max_concurrency > 0
          ? `${item.in_flight} / ${item.max_concurrency}`
          : t('Unlimited'),
    },
    {
      id: 'actions',
      header: t('Actions'),
      className: 'w-44 text-right',
      cellClassName: 'align-top',
      cell: (item) => (
        <div className='flex justify-end gap-1'>
          <Button
            variant='ghost'
            size='sm'
            disabled={
              !canReset || item.rules.every((rule) => rule.state === 'idle')
            }
            onClick={() => setResetting(item)}
          >
            <RotateCcw />
            {t('Reset')}
          </Button>
          <Button variant='outline' size='sm' onClick={() => setManaging(item)}>
            <SlidersHorizontal />
            {t('Manage')}
          </Button>
        </div>
      ),
    },
  ]

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Usage Limits')}</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <div className='flex flex-col gap-3'>
            <div className='flex flex-wrap items-center gap-2'>
              <div className='relative w-full sm:w-64'>
                <Search
                  className='text-muted-foreground absolute top-2.5 left-2.5 size-4'
                  aria-hidden='true'
                />
                <Input
                  value={keyword}
                  onChange={(event) => setKeyword(event.target.value)}
                  placeholder={t('Filter by username or ID...')}
                  className='pl-8'
                />
              </div>
              <Tabs
                value={filter}
                onValueChange={(value) =>
                  setFilter(value as LimitOverviewFilter)
                }
              >
                <TabsList>
                  <TabsTrigger value='all'>
                    {t('All')} {counts.all}
                  </TabsTrigger>
                  <TabsTrigger value='near'>
                    {t('Near limit')} {counts.near}
                  </TabsTrigger>
                  <TabsTrigger value='exhausted'>
                    {t('Used up')} {counts.exhausted}
                  </TabsTrigger>
                </TabsList>
              </Tabs>
            </div>

            {query.isLoading ? (
              <Skeleton className='h-40 w-full' />
            ) : (
              <StaticDataTable
                columns={columns}
                data={rows}
                getRowKey={(item) => item.id}
                emptyContent={t('No users match this filter.')}
              />
            )}
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      {managing ? (
        <UserLimitsDrawer
          open
          onOpenChange={(open) => !open && setManaging(null)}
          user={{ id: managing.id, username: managing.username }}
          onSaved={() =>
            queryClient.invalidateQueries({
              queryKey: ['usage-limit-overview'],
            })
          }
        />
      ) : null}

      <ConfirmDialog
        open={resetting !== null}
        onOpenChange={(open) => !open && setResetting(null)}
        title={t('Reset all windows')}
        desc={t(
          'End every current usage window for {{username}}? Used amounts return to 0 now.',
          { username: resetting?.username ?? '' }
        )}
        confirmText={t('Reset')}
        handleConfirm={resetAll}
      />
    </>
  )
}

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
import { Gauge } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { IconBadge } from '@/components/ui/icon-badge'

import { getSelfLimits } from '../api'
import { ruleTitle } from '../lib/rules'
import { RuleUsage } from './rule-usage'

/** The signed-in user's own usage limits; hidden when there are none. */
export function WalletUsageLimitsCard() {
  const { t } = useTranslation()
  const { data } = useQuery({
    queryKey: ['self-limits'],
    queryFn: async () => {
      const result = await getSelfLimits()
      return result.success ? (result.data ?? null) : null
    },
    meta: { errorToast: false },
    refetchInterval: 60_000,
  })

  if (!data || (data.rules.length === 0 && data.max_concurrency === 0)) {
    return null
  }

  return (
    <Card data-card-hover='false' className='py-0'>
      <CardContent className='space-y-4 p-3 sm:p-4'>
        <div className='flex min-w-0 items-center gap-2.5'>
          <IconBadge tone='primary'>
            <Gauge />
          </IconBadge>
          <div className='min-w-0 flex-1'>
            <h3 className='truncate text-sm font-semibold'>
              {t('Usage Limits')}
            </h3>
            <p className='text-muted-foreground line-clamp-2 text-xs'>
              {t(
                'Spending caps set by the administrator. When a limit is used up, matching requests pause until it resets.'
              )}
            </p>
          </div>
          {data.max_concurrency > 0 ? (
            <div className='shrink-0 text-right'>
              <div className='text-muted-foreground text-[10px] font-medium tracking-wider uppercase'>
                {t('Concurrent requests')}
              </div>
              <div className='text-sm font-semibold tabular-nums'>
                {data.in_flight} / {data.max_concurrency}
              </div>
            </div>
          ) : null}
        </div>

        <div className='divide-y rounded-lg border'>
          {data.rules.map((rule) => (
            <div
              key={rule.window_seconds}
              className='grid gap-2 px-3 py-3 sm:grid-cols-[minmax(160px,220px)_1fr] sm:items-center sm:gap-4'
            >
              <div className='min-w-0'>
                <div className='flex items-center gap-2 text-sm font-medium'>
                  {ruleTitle(t, rule)}
                  <Badge variant='outline' className='font-normal'>
                    {rule.mode === 'calendar'
                      ? t('Calendar period')
                      : t('Rolling window')}
                  </Badge>
                </div>
                <div className='text-muted-foreground truncate text-xs'>
                  {rule.models.length === 0
                    ? t('All models')
                    : rule.models.join(', ')}
                </div>
              </div>
              <RuleUsage rule={rule} />
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  )
}

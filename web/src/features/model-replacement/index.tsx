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
import { Play } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import dayjs from '@/lib/dayjs'

import { ChannelsTab } from './components/channels-tab'
import { HistoryTab } from './components/history-tab'
import { OverviewTab } from './components/overview-tab'
import { SettingsTab } from './components/settings-tab'
import { useDetectionOverview, useRunDetection } from './lib/hooks'
import { useModelReplacementAccess } from './lib/utils'

export type ModelReplacementTab =
  | 'overview'
  | 'channels'
  | 'history'
  | 'settings'

type ModelReplacementProps = {
  tab: ModelReplacementTab
  lastRuns: number
  edit?: string
  onTabChange: (tab: ModelReplacementTab) => void
  onLastRunsChange: (value: number) => void
}

export function ModelReplacement(props: ModelReplacementProps) {
  const { t } = useTranslation()
  const access = useModelReplacementAccess()
  const overviewQuery = useDetectionOverview()
  const runDetection = useRunDetection()
  const overview = overviewQuery.data
  const active = overview?.active_run ?? null

  let status = t('Off')
  if (overview?.enabled) {
    status = t('On · every {{minutes}} min', {
      minutes: overview.interval_minutes,
    })
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Model Replacement Detection')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <div className='flex flex-wrap items-center gap-3'>
          <StatusBadge
            label={status}
            variant={overview?.enabled ? 'success' : 'neutral'}
            size='sm'
          />
          {overview?.last_run ? (
            <span className='text-muted-foreground hidden text-sm md:inline'>
              {t('Last run {{time}}', {
                time: dayjs(overview.last_run.finished_at * 1000).format(
                  'MM-DD HH:mm'
                ),
              })}
            </span>
          ) : null}
          {access.canOperate ? (
            <Button
              type='button'
              disabled={Boolean(active) || runDetection.isPending}
              onClick={() => runDetection.mutate()}
            >
              <Play aria-hidden='true' />
              {active ? t('Detecting...') : t('Run now')}
            </Button>
          ) : null}
        </div>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <Tabs
          value={props.tab}
          onValueChange={(value) =>
            props.onTabChange(value as ModelReplacementTab)
          }
        >
          <TabsList variant='line' className='mb-2'>
            <TabsTrigger value='overview'>{t('Overview')}</TabsTrigger>
            <TabsTrigger value='channels'>{t('Channels')}</TabsTrigger>
            <TabsTrigger value='history'>{t('History')}</TabsTrigger>
            <TabsTrigger value='settings'>{t('Settings')}</TabsTrigger>
          </TabsList>
          <TabsContent value='overview'>
            <OverviewTab
              overview={overview}
              lastRuns={props.lastRuns}
              onLastRunsChange={props.onLastRunsChange}
            />
          </TabsContent>
          <TabsContent value='channels'>
            <ChannelsTab
              activeChannelId={active?.channel_id ?? null}
              busy={Boolean(active)}
            />
          </TabsContent>
          <TabsContent value='history'>
            <HistoryTab />
          </TabsContent>
          <TabsContent value='settings'>
            <SettingsTab initialEdit={props.edit} />
          </TabsContent>
        </Tabs>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

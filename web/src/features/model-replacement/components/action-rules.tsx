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
import { ArrowDown, ArrowUp, Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

import {
  ACTION_CATEGORIES,
  ACTION_DEFS,
  ACTION_LISTS,
  ACTION_VALUE_PLACEHOLDERS,
} from '../lib/constants'
import { useStableKeys } from '../lib/stable-keys'
import type {
  ActionListKey,
  ActionType,
  DetectionAction,
  DetectionSettings,
} from '../types'

const DEFAULT_VALUES: Partial<Record<ActionType, string>> = {
  increase_priority: '10',
  decrease_priority: '10',
  set_priority: '0',
  increase_weight: '10',
  decrease_weight: '10',
  set_weight: '0',
}

function ActionRow(props: {
  action: DetectionAction
  index: number
  count: number
  readOnly: boolean
  onChange: (action: DetectionAction) => void
  onMove: (offset: number) => void
  onRemove: () => void
}) {
  const { t } = useTranslation()
  const def = ACTION_DEFS[props.action.type]
  const numeric = ['amount', 'integer', 'non_negative'].includes(def.valueKind)
  return (
    <li className='bg-muted/50 flex flex-wrap items-center gap-1.5 rounded-md px-2 py-1.5 text-sm'>
      <span className='text-muted-foreground w-5 text-xs tabular-nums'>
        {props.index + 1}.
      </span>
      <span className='font-medium'>{t(def.labelKey)}</span>
      {def.valueKind !== 'none' ? (
        <Input
          aria-label={t(ACTION_VALUE_PLACEHOLDERS[def.valueKind])}
          placeholder={t(ACTION_VALUE_PLACEHOLDERS[def.valueKind])}
          value={props.action.value}
          inputMode={numeric ? 'numeric' : undefined}
          disabled={props.readOnly}
          onChange={(event) =>
            props.onChange({ ...props.action, value: event.target.value })
          }
          className={cn(
            'h-7 min-w-0 flex-1 bg-background',
            numeric ? 'max-w-24 text-right tabular-nums' : 'max-w-48'
          )}
        />
      ) : (
        <span className='flex-1' />
      )}
      {!props.readOnly ? (
        <span className='ml-auto flex'>
          <Button
            type='button'
            variant='ghost'
            size='icon-sm'
            aria-label={t('Move up')}
            disabled={props.index === 0}
            onClick={() => props.onMove(-1)}
          >
            <ArrowUp />
          </Button>
          <Button
            type='button'
            variant='ghost'
            size='icon-sm'
            aria-label={t('Move down')}
            disabled={props.index === props.count - 1}
            onClick={() => props.onMove(1)}
          >
            <ArrowDown />
          </Button>
          <Button
            type='button'
            variant='ghost'
            size='icon-sm'
            aria-label={t('Delete')}
            onClick={props.onRemove}
          >
            <Trash2 />
          </Button>
        </span>
      ) : null}
    </li>
  )
}

export function ActionRules(props: {
  settings: DetectionSettings
  readOnly: boolean
  onChange: (key: ActionListKey, actions: DetectionAction[]) => void
}) {
  const { t } = useTranslation()
  const stableKeys = useStableKeys()
  return (
    <div className='grid gap-4 lg:grid-cols-3'>
      {ACTION_LISTS.map((list) => {
        const actions = props.settings[list.key]
        const keys = stableKeys.keys(list.key, actions.length)
        const update = (next: DetectionAction[]) =>
          props.onChange(list.key, next)
        return (
          <div
            key={list.key}
            className={cn(
              'flex flex-col gap-2 rounded-lg border border-t-4 p-3',
              list.accent
            )}
          >
            <div>
              <div className='text-sm font-medium'>{t(list.titleKey)}</div>
              <p className='text-muted-foreground text-xs'>
                {t(list.descriptionKey)}
              </p>
            </div>
            <ol className='flex flex-col gap-1.5'>
              {actions.map((action, index) => (
                <ActionRow
                  key={keys[index]}
                  action={action}
                  index={index}
                  count={actions.length}
                  readOnly={props.readOnly}
                  onChange={(next) =>
                    update(
                      actions.map((item, position) =>
                        position === index ? next : item
                      )
                    )
                  }
                  onMove={(offset) => {
                    const next = [...actions]
                    const target = index + offset
                    stableKeys.swap(list.key, index, target)
                    ;[next[index], next[target]] = [next[target], next[index]]
                    update(next)
                  }}
                  onRemove={() => {
                    stableKeys.remove(list.key, index)
                    update(actions.filter((_, position) => position !== index))
                  }}
                />
              ))}
              {actions.length === 0 ? (
                <li className='text-muted-foreground rounded-md border border-dashed px-2 py-3 text-center text-xs'>
                  {t('No actions. The channel stays unchanged.')}
                </li>
              ) : null}
            </ol>
            {!props.readOnly ? (
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      className='mt-auto w-full'
                      disabled={actions.length >= 20}
                    />
                  }
                >
                  <Plus aria-hidden='true' />
                  {t('Add action')}
                </DropdownMenuTrigger>
                <DropdownMenuContent align='start' className='w-56'>
                  {ACTION_CATEGORIES.map((category, categoryIndex) => (
                    <DropdownMenuGroup key={category}>
                      {categoryIndex > 0 ? <DropdownMenuSeparator /> : null}
                      <DropdownMenuLabel>{t(category)}</DropdownMenuLabel>
                      {(Object.keys(ACTION_DEFS) as ActionType[])
                        .filter(
                          (type) => ACTION_DEFS[type].category === category
                        )
                        .map((type) => (
                          <DropdownMenuItem
                            key={type}
                            onClick={() => {
                              stableKeys.add(list.key)
                              update([
                                ...actions,
                                { type, value: DEFAULT_VALUES[type] ?? '' },
                              ])
                            }}
                          >
                            {t(ACTION_DEFS[type].labelKey)}
                          </DropdownMenuItem>
                        ))}
                    </DropdownMenuGroup>
                  ))}
                </DropdownMenuContent>
              </DropdownMenu>
            ) : null}
          </div>
        )
      })}
    </div>
  )
}

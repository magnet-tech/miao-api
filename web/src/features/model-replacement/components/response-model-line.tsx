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
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import { RESPONSE_MODEL_NOTE_LABELS } from '../lib/constants'
import type { ResponseModelNote } from '../types'

export function ResponseModelLine(props: {
  model: string
  responseModel: string
  note: ResponseModelNote
}) {
  const { t } = useTranslation()
  const differs =
    props.responseModel !== '' && props.responseModel !== props.model
  return (
    <div className='flex flex-wrap items-center gap-x-1.5'>
      <span className='text-muted-foreground'>{t('Response model')}: </span>
      <code
        className={cn(
          'rounded px-1',
          differs &&
            props.note === 'mismatch' &&
            'bg-destructive/10 text-destructive',
          differs &&
            props.note === 'loose_accepted' &&
            'bg-warning/10 text-warning'
        )}
      >
        {props.responseModel || t('Not returned')}
      </code>
      <span
        className={cn(
          'text-muted-foreground',
          props.note === 'mismatch' && 'text-destructive'
        )}
      >
        ({t(RESPONSE_MODEL_NOTE_LABELS[props.note])})
      </span>
    </div>
  )
}

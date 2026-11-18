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
import { hasPermission } from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'

import type { DetectionAction } from '../types'
import { ACTION_DEFS } from './constants'

export const MODEL_REPLACEMENT_RESOURCE = 'model_replacement'

export function useModelReplacementAccess() {
  const user = useAuthStore((state) => state.auth.user)
  return {
    canRead: hasPermission(user, MODEL_REPLACEMENT_RESOURCE, 'read'),
    canOperate: hasPermission(user, MODEL_REPLACEMENT_RESOURCE, 'operate'),
    canConfigure: hasPermission(user, MODEL_REPLACEMENT_RESOURCE, 'configure'),
  }
}

type Translate = (key: string, options?: Record<string, unknown>) => string

export function describeAction(t: Translate, action: DetectionAction): string {
  const def = ACTION_DEFS[action.type]
  const label = t(def.labelKey)
  if (def.valueKind === 'none') return label
  if (def.valueKind === 'optional_tag' && !action.value) {
    return t('Remove any tag')
  }
  const numeric = ['amount', 'integer', 'non_negative'].includes(def.valueKind)
  if (numeric && action.amount !== undefined && action.value === '') {
    return `${label} ${action.amount}`
  }
  return `${label} ${action.value}`
}

export function percentLabel(value: number): string {
  return `${Math.round(value)}%`
}

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
import { createFileRoute, redirect } from '@tanstack/react-router'
import z from 'zod'

import {
  ModelReplacement,
  type ModelReplacementTab,
} from '@/features/model-replacement'
import { MODEL_REPLACEMENT_RESOURCE } from '@/features/model-replacement/lib/utils'
import { hasPermission } from '@/lib/admin-permissions'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

const searchSchema = z.object({
  tab: z
    .enum(['overview', 'channels', 'history', 'settings'])
    .optional()
    .catch(undefined),
  last: z.number().int().min(1).max(48).optional().catch(undefined),
  edit: z.string().optional().catch(undefined),
})

const LAST_RUNS_STORAGE_KEY = 'model-replacement:last-runs'

/** The administrator's last choice of how many runs the overview shows. */
function storedLastRuns(): number {
  const value = Number(window.localStorage.getItem(LAST_RUNS_STORAGE_KEY))
  return Number.isInteger(value) && value >= 1 && value <= 48 ? value : 10
}

export const Route = createFileRoute('/_authenticated/model-replacement/')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()
    if (
      !auth.user ||
      auth.user.role < ROLE.ADMIN ||
      !hasPermission(auth.user, MODEL_REPLACEMENT_RESOURCE, 'read')
    ) {
      throw redirect({ to: '/403' })
    }
  },
  validateSearch: searchSchema,
  component: ModelReplacementRoute,
})

function ModelReplacementRoute() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <ModelReplacement
      tab={search.tab ?? 'overview'}
      lastRuns={search.last ?? storedLastRuns()}
      edit={search.edit}
      onTabChange={(tab: ModelReplacementTab) =>
        navigate({
          search: (previous) => ({ ...previous, tab, edit: undefined }),
          replace: true,
        })
      }
      onLastRunsChange={(last) => {
        window.localStorage.setItem(LAST_RUNS_STORAGE_KEY, String(last))
        void navigate({
          search: (previous) => ({ ...previous, last }),
          replace: true,
        })
      }}
    />
  )
}

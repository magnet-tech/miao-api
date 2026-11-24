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
import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  getOverview,
  modelReplacementKeys,
  retestChannel,
  runDetection,
} from '../api'

/** Overview polling; refreshes run data once an active run finishes. */
export function useDetectionOverview() {
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: modelReplacementKeys.overview(),
    queryFn: async () => requireServerSuccess(await getOverview()).data,
    refetchInterval: (state) => (state.state.data?.active_run ? 1000 : 30_000),
  })
  const activeId = query.data?.active_run?.id ?? null
  const previous = useRef<string | null>(null)
  useEffect(() => {
    if (previous.current && !activeId) {
      void queryClient.invalidateQueries({ queryKey: modelReplacementKeys.all })
    }
    previous.current = activeId
  }, [activeId, queryClient])
  return query
}

function useStartMutation<TArgs>(
  run: (args: TArgs) => ReturnType<typeof runDetection>,
  messages: { success: string; failure: string }
) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (args: TArgs) => requireServerSuccess(await run(args)),
    onSuccess: () => {
      toast.success(t(messages.success))
      void queryClient.invalidateQueries({
        queryKey: modelReplacementKeys.overview(),
      })
    },
    onError: (error) => handleServerError(error, t(messages.failure)),
  })
}

export function useRunDetection() {
  return useStartMutation<void>(() => runDetection(), {
    success: 'Detection started',
    failure: 'Failed to start detection',
  })
}

export function useRetestChannel() {
  return useStartMutation<number>((channelId) => retestChannel(channelId), {
    success: 'Retest started',
    failure: 'Failed to start the retest',
  })
}

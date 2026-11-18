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
import { api } from '@/lib/api'

import { ACTION_DEFS } from './lib/constants'
import type {
  ActiveRun,
  ApiResponse,
  CandidateModel,
  CellDetail,
  DetectionAction,
  DetectionChannel,
  DetectionOverview,
  DetectionRun,
  DetectionSettings,
  SettingsResponse,
  TrialRequest,
  TrialResult,
} from './types'

const BASE = '/api/model_replacement'

export const modelReplacementKeys = {
  all: ['model-replacement'] as const,
  overview: () => [...modelReplacementKeys.all, 'overview'] as const,
  settings: () => [...modelReplacementKeys.all, 'settings'] as const,
  channels: () => [...modelReplacementKeys.all, 'channels'] as const,
  candidates: () => [...modelReplacementKeys.all, 'candidates'] as const,
  runs: (limit: number) =>
    [...modelReplacementKeys.all, 'runs', limit] as const,
  run: (id: string) => [...modelReplacementKeys.all, 'run', id] as const,
  cell: (runId: string, channelId: number, model: string) =>
    [...modelReplacementKeys.all, 'cell', runId, channelId, model] as const,
}

export async function getOverview(): Promise<ApiResponse<DetectionOverview>> {
  const res = await api.get(`${BASE}/overview`)
  return res.data
}

const ACTION_LISTS = [
  'pass_actions',
  'fail_actions',
  'request_failure_actions',
] as const

function hasAmount(type: DetectionAction['type']): boolean {
  return ['amount', 'integer', 'non_negative'].includes(
    ACTION_DEFS[type].valueKind
  )
}

/** Settings as the editor holds them: every action parameter as text. */
function editableSettings(settings: DetectionSettings): DetectionSettings {
  const next = { ...settings }
  for (const key of ACTION_LISTS) {
    next[key] = settings[key].map((action) =>
      hasAmount(action.type)
        ? { type: action.type, value: String(action.amount ?? 0) }
        : { type: action.type, value: action.value }
    )
  }
  return next
}

/** Settings as the server stores them: priority and weight in amount. */
function storedSettings(settings: DetectionSettings): DetectionSettings {
  const next = { ...settings }
  for (const key of ACTION_LISTS) {
    next[key] = settings[key].map((action) =>
      hasAmount(action.type)
        ? { type: action.type, value: '', amount: Number(action.value) }
        : { type: action.type, value: action.value.trim(), amount: 0 }
    )
  }
  return next
}

function withEditableSettings(
  response: ApiResponse<SettingsResponse>
): ApiResponse<SettingsResponse> {
  if (!response.data) return response
  return {
    ...response,
    data: {
      ...response.data,
      settings: editableSettings(response.data.settings),
    },
  }
}

export async function getSettings(): Promise<ApiResponse<SettingsResponse>> {
  const res = await api.get(`${BASE}/settings`)
  return withEditableSettings(res.data)
}

export async function updateSettings(
  settings: DetectionSettings
): Promise<ApiResponse<SettingsResponse>> {
  const res = await api.put(`${BASE}/settings`, storedSettings(settings))
  return withEditableSettings(res.data)
}

export async function getChannels(): Promise<ApiResponse<DetectionChannel[]>> {
  const res = await api.get(`${BASE}/channels`)
  return res.data
}

export async function getCandidates(): Promise<ApiResponse<CandidateModel[]>> {
  const res = await api.get(`${BASE}/candidates`)
  return res.data
}

export async function getRuns(
  limit: number
): Promise<ApiResponse<{ runs: DetectionRun[]; total: number }>> {
  const res = await api.get(`${BASE}/runs`, { params: { limit } })
  return res.data
}

export async function getRun(id: string): Promise<ApiResponse<DetectionRun>> {
  const res = await api.get(`${BASE}/runs/${encodeURIComponent(id)}`)
  return res.data
}

export async function getCellDetail(
  runId: string,
  channelId: number,
  model: string
): Promise<ApiResponse<CellDetail>> {
  const res = await api.get(`${BASE}/runs/${encodeURIComponent(runId)}/cell`, {
    params: { channel_id: channelId, model },
  })
  return res.data
}

export async function runDetection(): Promise<ApiResponse<ActiveRun>> {
  const res = await api.post(`${BASE}/run`)
  return res.data
}

export async function retestChannel(
  channelId: number
): Promise<ApiResponse<ActiveRun>> {
  const res = await api.post(`${BASE}/channels/${channelId}/run`)
  return res.data
}

export async function trialRun(
  request: TrialRequest
): Promise<ApiResponse<TrialResult>> {
  const res = await api.post(`${BASE}/trial`, request)
  return res.data
}

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

export type UsageRuleMode = 'window' | 'calendar'
export type CalendarPeriod = 'day' | 'week' | 'month' | 'quarter' | 'year'
export type UsageWindowState = 'idle' | 'active' | 'exhausted'

export interface UsageRule {
  window_seconds: number
  mode: UsageRuleMode
  calendar_period: CalendarPeriod | ''
  quota_limit: number
  enabled: boolean
  models: string[]
}

export interface UsageRuleView extends UsageRule {
  state: UsageWindowState
  used_quota: number
  window_start: number
  reset_at: number
}

export interface UserLimits {
  max_concurrency: number
  in_flight: number
  server_time_zone: string
  rules: UsageRuleView[]
}

export interface LimitModelGroup {
  vendor: string
  models: string[]
}

export interface LimitOverviewItem extends UserLimits {
  id: number
  username: string
  display_name: string
  status: number
  highest_percent: number
}

export interface LimitOverview {
  items: LimitOverviewItem[]
  counts: { all: number; near: number; exhausted: number }
}

export type LimitOverviewFilter = 'all' | 'near' | 'exhausted'

type ApiResponse<T> = { success: boolean; message?: string; data?: T }

export async function getUserLimits(userId: number) {
  const res = await api.get<ApiResponse<UserLimits>>(
    `/api/user/${userId}/limits`
  )
  return res.data
}

export async function updateUserLimits(
  userId: number,
  payload: { max_concurrency: number; rules: UsageRule[] }
) {
  const res = await api.put<ApiResponse<UserLimits>>(
    `/api/user/${userId}/limits`,
    payload
  )
  return res.data
}

export async function resetUserLimitWindow(
  userId: number,
  windowSeconds?: number
) {
  const res = await api.post<ApiResponse<UserLimits>>(
    `/api/user/${userId}/limits/reset`,
    { window_seconds: windowSeconds ?? 0 }
  )
  return res.data
}

export async function getLimitModels(userId: number) {
  const res = await api.get<ApiResponse<LimitModelGroup[]>>(
    `/api/user/${userId}/limits/models`
  )
  return res.data
}

export async function getSelfLimits() {
  const res = await api.get<ApiResponse<UserLimits>>('/api/user/self/limits')
  return res.data
}

export async function getLimitOverview(params: {
  keyword: string
  filter: LimitOverviewFilter
}) {
  const res = await api.get<ApiResponse<LimitOverview>>(
    '/api/usage_limits/users',
    { params }
  )
  return res.data
}

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

import type {
  ApiKey,
  ApiResponse,
  GetApiKeysParams,
  GetApiKeysResponse,
  SearchApiKeysParams,
  ApiKeyFormData,
  TokenAutoGroupsConfig,
} from './types'

// ============================================================================
// API Key Management
// ============================================================================

/**
 * Whose keys a request works on. An administrator managing another user's
 * keys sends requests under /api/user/:id/tokens; changes there carry the
 * management session, full keys a verification for exactly those keys.
 */
export type TokenScope = {
  ownerId: number
  /** The management session for changes; null while read-only. */
  managementToken: string | null
}

function tokenBase(scope?: TokenScope | null): string {
  return scope ? `/api/user/${scope.ownerId}/tokens` : '/api/token'
}

/** Request options for a scope; none for one's own keys. */
function tokenConfig(
  scope?: TokenScope | null,
  proofToken?: string
): [] | [{ headers: Record<string, string> }] {
  const headers: Record<string, string> = {}
  if (scope?.managementToken) {
    headers['X-Key-Management-Session'] = scope.managementToken
  }
  if (proofToken) headers['X-Security-Proof'] = proofToken
  return Object.keys(headers).length > 0 ? [{ headers }] : []
}

// Get paginated API keys list
export async function getApiKeys(
  params: GetApiKeysParams = {},
  scope?: TokenScope | null
): Promise<GetApiKeysResponse> {
  const { p = 1, size = 10 } = params
  const res = await api.get(`${tokenBase(scope)}/?p=${p}&size=${size}`)
  return res.data
}

// Search API keys by keyword or token (with pagination)
export async function searchApiKeys(
  params: SearchApiKeysParams,
  scope?: TokenScope | null
): Promise<GetApiKeysResponse> {
  const { keyword = '', token = '', p, size } = params
  const queryParams = new URLSearchParams()
  if (keyword) queryParams.set('keyword', keyword)
  if (token) queryParams.set('token', token)
  if (p != null) queryParams.set('p', String(p))
  if (size != null) queryParams.set('size', String(size))
  const res = await api.get(
    `${tokenBase(scope)}/search?${queryParams.toString()}`
  )
  return res.data
}

// Get single API key by ID
export async function getApiKey(
  id: number,
  scope?: TokenScope | null
): Promise<ApiResponse<ApiKey>> {
  const res = await api.get(`${tokenBase(scope)}/${id}`)
  return res.data
}

// Get the user's global Auto order and the per-token selection limit.
export async function getTokenAutoGroups(
  scope?: TokenScope | null
): Promise<ApiResponse<TokenAutoGroupsConfig>> {
  const res = await api.get(`${tokenBase(scope)}/auto-groups`)
  return res.data
}

// Create a new API key
export async function createApiKey(
  data: ApiKeyFormData,
  scope?: TokenScope | null
): Promise<ApiResponse<ApiKey>> {
  const res = await api.post(
    `${tokenBase(scope)}/`,
    data,
    ...tokenConfig(scope)
  )
  return res.data
}

// Update an existing API key
export async function updateApiKey(
  data: ApiKeyFormData & { id: number },
  scope?: TokenScope | null
): Promise<ApiResponse<ApiKey>> {
  const res = await api.put(`${tokenBase(scope)}/`, data, ...tokenConfig(scope))
  return res.data
}

// Delete a single API key
export async function deleteApiKey(
  id: number,
  scope?: TokenScope | null
): Promise<ApiResponse> {
  const path = scope ? `${tokenBase(scope)}/${id}` : `/api/token/${id}/`
  const res = await api.delete(path, ...tokenConfig(scope))
  return res.data
}

// Batch delete multiple API keys
export async function batchDeleteApiKeys(
  ids: number[],
  scope?: TokenScope | null
): Promise<ApiResponse<number>> {
  const res = await api.post(
    `${tokenBase(scope)}/batch`,
    { ids },
    ...tokenConfig(scope)
  )
  return res.data
}

// Update API key status (enable/disable)
export async function updateApiKeyStatus(
  id: number,
  status: number,
  scope?: TokenScope | null
): Promise<ApiResponse<ApiKey>> {
  const res = await api.put(
    `${tokenBase(scope)}/?status_only=true`,
    { id, status },
    ...tokenConfig(scope)
  )
  return res.data
}

// Fetch the real (unmasked) key for a token by ID
export async function fetchTokenKey(
  id: number,
  scope?: TokenScope | null,
  proofToken?: string
): Promise<{ success: boolean; message?: string; data?: { key: string } }> {
  const config = tokenConfig(scope, proofToken)
  const res = await (config.length > 0
    ? api.post(`${tokenBase(scope)}/${id}/key`, undefined, ...config)
    : api.post(`${tokenBase(scope)}/${id}/key`))
  return res.data
}

// Batch fetch real (unmasked) keys for multiple tokens
export async function fetchTokenKeysBatch(
  ids: number[],
  scope?: TokenScope | null,
  proofToken?: string
): Promise<{
  success: boolean
  message?: string
  data?: { keys: Record<number, string> }
}> {
  const res = await api.post(
    `${tokenBase(scope)}/batch/keys`,
    { ids },
    ...tokenConfig(scope, proofToken)
  )
  return res.data
}

// Open a 30-minute management session for another user's keys.
export async function startKeyManagement(
  ownerId: number,
  proofToken: string
): Promise<ApiResponse<{ management_token: string; expires_at: number }>> {
  const res = await api.post(
    `/api/user/${ownerId}/tokens/management-session`,
    undefined,
    { headers: { 'X-Security-Proof': proofToken } }
  )
  return res.data
}

// End a management session early.
export async function endKeyManagement(scope: TokenScope): Promise<void> {
  await api.delete(
    `${tokenBase(scope)}/management-session`,
    ...tokenConfig(scope)
  )
}

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
import React, { useState, useCallback, useRef, useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'
import useDialogState from '@/hooks/use-dialog'
import { handleServerError } from '@/lib/handle-server-error'
import { AuthOperationError } from '@/lib/secure-verification'

import {
  endKeyManagement,
  fetchTokenKey,
  fetchTokenKeysBatch,
  startKeyManagement,
  type TokenScope,
} from '../api'
import { ERROR_MESSAGES } from '../constants'
import type { ApiKey, ApiKeysDialogType } from '../types'

type ApiKeysContextType = {
  open: ApiKeysDialogType | null
  setOpen: (str: ApiKeysDialogType | null) => void
  currentRow: ApiKey | null
  setCurrentRow: React.Dispatch<React.SetStateAction<ApiKey | null>>
  refreshTrigger: number
  triggerRefresh: () => void
  resolvedKey: string
  setResolvedKey: React.Dispatch<React.SetStateAction<string>>
  resolveRealKey: (id: number) => Promise<string | null>
  resolveRealKeysBatch: (ids: number[]) => Promise<Record<number, string>>
  resolvedKeys: Record<number, string>
  loadingKeys: Record<number, boolean>
  copiedKeyId: number | null
  markKeyCopied: (id: number) => void
  /** The user whose keys an administrator manages; null for one's own. */
  owner: KeyOwner | null
  /** Passed to every keys API call. */
  tokenScope: TokenScope | null
  /** False while managing another user's keys without a session. */
  canChange: boolean
  /** When the management session ends, in Unix seconds. */
  managementExpiresAt: number | null
  startManagement: () => Promise<void>
  endManagement: () => void
}

export type KeyOwner = { id: number; username: string }

const ApiKeysContext = React.createContext<ApiKeysContextType | null>(null)

export function ApiKeysProvider(props: {
  children: React.ReactNode
  owner?: KeyOwner | null
}) {
  const { t } = useTranslation()
  const owner = props.owner ?? null
  const verification = useSecureVerification()
  const [management, setManagement] = useState<{
    token: string
    expiresAt: number
  } | null>(null)
  const tokenScope = useMemo<TokenScope | null>(
    () =>
      owner
        ? { ownerId: owner.id, managementToken: management?.token ?? null }
        : null,
    [owner, management]
  )
  const [open, setOpen] = useDialogState<ApiKeysDialogType>(null)
  const [currentRow, setCurrentRow] = useState<ApiKey | null>(null)
  const [refreshTrigger, setRefreshTrigger] = useState(0)
  const [resolvedKey, setResolvedKey] = useState('')

  const [resolvedKeys, setResolvedKeys] = useState<Record<number, string>>({})
  const [loadingKeys, setLoadingKeys] = useState<Record<number, boolean>>({})
  const pendingRequests = useRef<Record<number, Promise<string | null>>>({})

  const [copiedKeyId, setCopiedKeyId] = useState<number | null>(null)
  const copiedTimerRef = useRef<ReturnType<typeof setTimeout>>(undefined)

  useEffect(() => {
    return () => clearTimeout(copiedTimerRef.current)
  }, [])

  const markKeyCopied = useCallback((id: number) => {
    setCopiedKeyId(id)
    clearTimeout(copiedTimerRef.current)
    copiedTimerRef.current = setTimeout(() => setCopiedKeyId(null), 2000)
  }, [])

  const triggerRefresh = useCallback(() => {
    setRefreshTrigger((prev) => prev + 1)
  }, [])

  const { requestVerification } = verification
  /**
   * Another user's full keys need a verification for exactly those keys each
   * time: undefined for one's own keys, null when the operator cancels.
   */
  const revealProof = useCallback(
    async (ids: number[]): Promise<string | undefined | null> => {
      if (!owner) return undefined
      try {
        const proof = await requestVerification({
          scope: 'admin.user.tokens',
          context: { user_id: owner.id, action: 'reveal', token_ids: ids },
          title: t('Verify to view full API keys'),
          description: t(
            'Confirm your identity to view the full API keys of {{username}}.',
            { username: owner.username }
          ),
        })
        return proof?.proof_token ?? null
      } catch (error) {
        handleServerError(
          AuthOperationError.from(error),
          t('Verification failed')
        )
        return null
      }
    },
    [owner, requestVerification, t]
  )

  const startManagement = useCallback(async () => {
    if (!owner) return
    try {
      const proof = await requestVerification({
        scope: 'admin.user.tokens',
        context: { user_id: owner.id, action: 'manage' },
        title: t("Verify to manage this user's API keys"),
        description: t(
          'After verifying, you can change the API keys of {{username}} for 30 minutes without being asked again.',
          { username: owner.username }
        ),
      })
      if (!proof) return
      const result = await startKeyManagement(owner.id, proof.proof_token)
      if (!result.success || !result.data) {
        handleServerError(result, t('Verification failed'))
        return
      }
      setManagement({
        token: result.data.management_token,
        expiresAt: result.data.expires_at,
      })
    } catch (error) {
      handleServerError(
        AuthOperationError.from(error),
        t('Verification failed')
      )
    }
  }, [owner, requestVerification, t])

  // The session ends when its time is up, when the operator ends it and when
  // the page is left.
  const endManagement = useCallback(() => {
    setManagement(null)
    if (tokenScope?.managementToken) {
      void endKeyManagement(tokenScope).catch(() => undefined)
    }
  }, [tokenScope])

  const endOnLeave = useRef(endManagement)
  useEffect(() => {
    endOnLeave.current = endManagement
  }, [endManagement])
  useEffect(() => () => endOnLeave.current(), [])

  useEffect(() => {
    if (!management) return
    const remaining = management.expiresAt * 1000 - Date.now()
    const timer = setTimeout(() => setManagement(null), Math.max(remaining, 0))
    return () => clearTimeout(timer)
  }, [management])

  const resolveRealKey = useCallback(
    async (id: number): Promise<string | null> => {
      if (resolvedKeys[id]) return resolvedKeys[id]
      if (id in pendingRequests.current) return pendingRequests.current[id]

      const request = (async () => {
        setLoadingKeys((prev) => ({ ...prev, [id]: true }))
        try {
          const proof = await revealProof([id])
          if (proof === null) return null
          const res = await fetchTokenKey(id, tokenScope, proof)
          if (res.success && res.data?.key) {
            const fullKey = `sk-${res.data.key}`
            setResolvedKeys((prev) => ({ ...prev, [id]: fullKey }))
            return fullKey
          }
          handleServerError(res, t(ERROR_MESSAGES.UNEXPECTED))
          return null
        } catch (error) {
          handleServerError(error, t(ERROR_MESSAGES.UNEXPECTED))
          return null
        } finally {
          delete pendingRequests.current[id]
          setLoadingKeys((prev) => {
            const next = { ...prev }
            delete next[id]
            return next
          })
        }
      })()

      pendingRequests.current[id] = request
      return request
    },
    [resolvedKeys, revealProof, t, tokenScope]
  )

  const resolveRealKeysBatch = useCallback(
    async (ids: number[]): Promise<Record<number, string>> => {
      const uncachedIds = ids.filter((id) => !resolvedKeys[id])
      if (uncachedIds.length === 0) {
        const result: Record<number, string> = {}
        for (const id of ids) result[id] = resolvedKeys[id]
        return result
      }

      for (const id of uncachedIds) {
        setLoadingKeys((prev) => ({ ...prev, [id]: true }))
      }

      try {
        const proof = await revealProof(uncachedIds)
        if (proof === null) return {}
        const res = await fetchTokenKeysBatch(uncachedIds, tokenScope, proof)
        if (res.success && res.data?.keys) {
          const newKeys: Record<number, string> = {}
          for (const [idStr, key] of Object.entries(res.data.keys)) {
            newKeys[Number(idStr)] = `sk-${key}`
          }
          setResolvedKeys((prev) => ({ ...prev, ...newKeys }))

          const result: Record<number, string> = { ...newKeys }
          for (const id of ids) {
            if (resolvedKeys[id]) result[id] = resolvedKeys[id]
          }
          return result
        }
        handleServerError(res, t(ERROR_MESSAGES.UNEXPECTED))
        return {}
      } catch (error) {
        handleServerError(error, t(ERROR_MESSAGES.UNEXPECTED))
        return {}
      } finally {
        for (const id of uncachedIds) {
          setLoadingKeys((prev) => {
            const next = { ...prev }
            delete next[id]
            return next
          })
        }
      }
    },
    [resolvedKeys, revealProof, t, tokenScope]
  )

  return (
    <ApiKeysContext
      value={{
        open,
        setOpen,
        currentRow,
        setCurrentRow,
        refreshTrigger,
        triggerRefresh,
        resolvedKey,
        setResolvedKey,
        resolveRealKey,
        resolveRealKeysBatch,
        resolvedKeys,
        loadingKeys,
        copiedKeyId,
        markKeyCopied,
        owner,
        tokenScope,
        canChange: !owner || management !== null,
        managementExpiresAt: management?.expiresAt ?? null,
        startManagement,
        endManagement,
      }}
    >
      {props.children}
      <SecureVerificationDialog {...verification.dialogProps} />
    </ApiKeysContext>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export const useApiKeys = () => {
  const apiKeysContext = React.useContext(ApiKeysContext)

  if (!apiKeysContext) {
    throw new Error('useApiKeys has to be used within <ApiKeysContext>')
  }

  return apiKeysContext
}

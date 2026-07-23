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
import {
  type ImportResult,
  type PoolAccountUsage,
  type PoolAuthFile,
  type PoolInfo,
  type PoolUsageData,
  type PoolUsageJobSnapshot,
  type ProbeResult,
  type ProbeJobSnapshot,
  deriveAuthFileName,
} from '@/features/pool/api'
import { api } from '@/lib/api'

type ApiEnvelope<T> = {
  success: boolean
  message?: string
  data?: T
}

export type PrivatePoolState = {
  status: 'none' | 'provisioning' | 'ready' | 'error'
  provision_enabled: boolean
  pool_id?: string
  label?: string
  error?: string
  pool?: PoolInfo & {
    owner_user_id?: number
    kind?: 'private'
    group_key?: string
    created_at?: number
    auto_clean_enabled?: boolean
    auto_clean_hours?: number
    usage_auto_refresh_enabled?: boolean
    usage_auto_reset_enabled?: boolean
  }
}

export type PrivatePoolSettings = {
  auto_clean_enabled: boolean
  auto_clean_hours: number
  usage_auto_refresh_enabled: boolean
  usage_auto_reset_enabled: boolean
}

export type PrivatePoolOAuthSession = {
  session_id: string
  status: 'starting' | 'waiting_callback' | 'exchanging'
  url: string
  expires_in: number
  expires_at: number
}

export type PrivatePoolOAuthStatus = {
  status: 'waiting_callback' | 'exchanging' | 'ok' | 'error'
  error?: string
}

function requireData<T>(response: ApiEnvelope<T>, fallback: string): T {
  if (!response.success || response.data === undefined) {
    throw new Error(response.message || fallback)
  }
  return response.data
}

function normalizeList(data: unknown): PoolAuthFile[] {
  if (Array.isArray(data)) return data as PoolAuthFile[]
  if (data && typeof data === 'object') {
    const object = data as Record<string, unknown>
    for (const key of ['files', 'items', 'auth_files', 'data']) {
      if (Array.isArray(object[key])) return object[key] as PoolAuthFile[]
    }
  }
  return []
}

export async function getPrivatePool(): Promise<PrivatePoolState> {
  const response =
    await api.get<ApiEnvelope<PrivatePoolState>>('/api/private-pool')
  return requireData(response.data, 'Failed to load private pool')
}

export async function createPrivatePool(): Promise<void> {
  const response = await api.post<ApiEnvelope<unknown>>('/api/private-pool', {})
  if (!response.data.success) {
    throw new Error(response.data.message || 'Failed to create private pool')
  }
}

export async function listPrivatePoolAccounts(): Promise<PoolAuthFile[]> {
  const response = await api.get<ApiEnvelope<unknown>>(
    '/api/private-pool/auth-files'
  )
  if (!response.data.success) {
    throw new Error(response.data.message || 'Failed to load pool accounts')
  }
  return normalizeList(response.data.data)
}

export async function addPrivatePoolAccounts(
  content: string
): Promise<ImportResult> {
  const response = await api.post<ApiEnvelope<ImportResult>>(
    '/api/private-pool/auth-files',
    { name: deriveAuthFileName(content), content }
  )
  return requireData(response.data, 'Failed to add pool account')
}

export async function importPrivatePoolAccounts(
  file: File
): Promise<ImportResult> {
  const form = new FormData()
  form.append('file', file)
  const response = await api.post<ApiEnvelope<ImportResult>>(
    '/api/private-pool/auth-files/import',
    form
  )
  return requireData(response.data, 'Failed to import pool accounts')
}

export async function cleanPrivatePoolAccounts(): Promise<number> {
  const response = await api.post<ApiEnvelope<{ disabled?: number }>>(
    '/api/private-pool/auth-files/clean'
  )
  const data = requireData(response.data, 'Failed to clean the pool')
  return data.disabled ?? 0
}

export async function deletePrivatePoolAccount(name: string): Promise<void> {
  const response = await api.delete<ApiEnvelope<unknown>>(
    '/api/private-pool/auth-files',
    { params: { name } }
  )
  if (!response.data.success) {
    throw new Error(response.data.message || 'Failed to delete pool account')
  }
}

export async function setPrivatePoolAccountDisabled(
  name: string,
  disabled: boolean
): Promise<void> {
  const response = await api.patch<ApiEnvelope<unknown>>(
    '/api/private-pool/auth-files/status',
    { name, disabled }
  )
  if (!response.data.success) {
    throw new Error(response.data.message || 'Failed to update pool account')
  }
}

export async function verifyPrivatePoolAccount(
  name: string,
  heavy = false
): Promise<ProbeResult> {
  const response = await api.post<ApiEnvelope<ProbeResult>>(
    '/api/private-pool/auth-files/verify',
    { name, heavy }
  )
  return requireData(response.data, 'Failed to verify pool account')
}

export async function startPrivatePoolVerifyAll(options: {
  heavy: boolean
  autoDisable: boolean
}): Promise<ProbeJobSnapshot> {
  const response = await api.post<ApiEnvelope<ProbeJobSnapshot>>(
    '/api/private-pool/auth-files/verify-all',
    { heavy: options.heavy, auto_disable: options.autoDisable }
  )
  return requireData(response.data, 'Failed to start verification')
}

export async function getPrivatePoolVerifyProgress(): Promise<ProbeJobSnapshot | null> {
  const response = await api.get<ApiEnvelope<ProbeJobSnapshot | null>>(
    '/api/private-pool/auth-files/verify-all/progress'
  )
  if (!response.data.success) {
    throw new Error(
      response.data.message || 'Failed to load verification progress'
    )
  }
  return response.data.data ?? null
}

export async function getPrivatePoolUsage(): Promise<PoolUsageData> {
  const response = await api.get<ApiEnvelope<PoolUsageData>>(
    '/api/private-pool/auth-files/usage'
  )
  return requireData(response.data, 'Failed to load pool usage')
}

export async function refreshPrivatePoolAccount(
  name: string
): Promise<PoolAccountUsage> {
  const response = await api.post<ApiEnvelope<PoolAccountUsage>>(
    '/api/private-pool/auth-files/usage/refresh',
    { name }
  )
  return requireData(response.data, 'Failed to refresh account usage')
}

export async function refreshAllPrivatePoolAccounts(): Promise<PoolUsageJobSnapshot> {
  const response = await api.post<ApiEnvelope<PoolUsageJobSnapshot>>(
    '/api/private-pool/auth-files/usage/refresh',
    {}
  )
  return requireData(response.data, 'Failed to refresh pool usage')
}

export async function resetPrivatePoolAccountQuota(
  name: string
): Promise<PoolAccountUsage> {
  const response = await api.post<ApiEnvelope<PoolAccountUsage>>(
    '/api/private-pool/auth-files/usage/reset',
    { name }
  )
  return requireData(response.data, 'Failed to reset account quota')
}

export async function getPrivatePoolSettings(): Promise<PrivatePoolSettings> {
  const response = await api.get<ApiEnvelope<PrivatePoolSettings>>(
    '/api/private-pool/settings'
  )
  return requireData(response.data, 'Failed to load private pool settings')
}

export async function patchPrivatePoolSettings(
  patch: Partial<PrivatePoolSettings>
): Promise<PrivatePoolSettings> {
  const response = await api.patch<ApiEnvelope<PrivatePoolSettings>>(
    '/api/private-pool/settings',
    patch
  )
  return requireData(response.data, 'Failed to update private pool settings')
}

export async function startPrivatePoolCodexLogin(): Promise<PrivatePoolOAuthSession> {
  const response = await api.post<ApiEnvelope<PrivatePoolOAuthSession>>(
    '/api/private-pool/oauth/codex/start'
  )
  return requireData(response.data, 'Failed to start Codex login')
}

export async function submitPrivatePoolCodexCallback(
  sessionId: string,
  redirectUrl: string
): Promise<PrivatePoolOAuthStatus> {
  const response = await api.post<ApiEnvelope<PrivatePoolOAuthStatus>>(
    '/api/private-pool/oauth/codex/callback',
    { session_id: sessionId, redirect_url: redirectUrl }
  )
  return requireData(response.data, 'Failed to submit OAuth callback')
}

export async function getPrivatePoolCodexLoginStatus(
  sessionId: string
): Promise<PrivatePoolOAuthStatus> {
  const response = await api.get<ApiEnvelope<PrivatePoolOAuthStatus>>(
    '/api/private-pool/oauth/codex/status',
    { params: { session_id: sessionId } }
  )
  return requireData(response.data, 'Failed to check Codex login status')
}

export async function cancelPrivatePoolCodexLogin(
  sessionId: string
): Promise<void> {
  const response = await api.delete<ApiEnvelope<unknown>>(
    '/api/private-pool/oauth/codex/session',
    { params: { session_id: sessionId } }
  )
  if (!response.data.success) {
    throw new Error(response.data.message || 'Failed to cancel Codex login')
  }
}

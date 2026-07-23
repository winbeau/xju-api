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
  }
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
  name: string
): Promise<ProbeResult> {
  const response = await api.post<ApiEnvelope<ProbeResult>>(
    '/api/private-pool/auth-files/verify',
    { name, heavy: false }
  )
  return requireData(response.data, 'Failed to verify pool account')
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

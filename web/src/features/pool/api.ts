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

/**
 * xju-api:new — account-pool auth files, proxied by new-api's root-only /api/pool endpoints
 * to the CLIProxyAPI management API. Adding an account here is the same as
 * dropping an auth JSON into the pool — the pool hot-reloads, no restart.
 */

/** One 10-minute bucket of the account's recent request outcomes. */
type RecentRequestBucket = {
  time: string
  success: number
  failed: number
}

export type PoolAuthFile = {
  name: string
  email?: string
  account?: string
  account_type?: string
  disabled?: boolean
  unavailable?: boolean
  failed?: number
  success?: number
  status_message?: string
  last_refresh?: string
  updated_at?: string
  next_retry_after?: string
  // auth_index is the stable runtime id the management api-call pins a probe to.
  auth_index?: string
  recent_requests?: RecentRequestBucket[]
  // Codex accounts carry their ChatGPT subscription window + plan in the id_token.
  id_token?: {
    chatgpt_subscription_active_until?: string
    plan_type?: string
    chatgpt_account_id?: string
  }
  // The management API returns yet more metadata; keep it open.
  [key: string]: unknown
}

type ApiEnvelope<T> = {
  success: boolean
  message?: string
  data?: T
}

/**
 * The management API's list payload isn't a bare array — normalize the couple
 * of shapes it can take (array, or {files:[...]}/{items:[...]}) into a list.
 */
function normalizeList(data: unknown): PoolAuthFile[] {
  if (Array.isArray(data)) return data as PoolAuthFile[]
  if (data && typeof data === 'object') {
    const obj = data as Record<string, unknown>
    for (const key of ['files', 'items', 'auth_files', 'data']) {
      if (Array.isArray(obj[key])) return obj[key] as PoolAuthFile[]
    }
  }
  return []
}

export type PoolInfo = { id: string; label: string }

export type ImportResult = {
  imported: number
  skipped: { name: string; reason: string }[]
  failed: { name: string; error: string }[]
}

/**
 * xju-api runs isolated pools (default + k12). Every management call carries the
 * target pool as `?pool=`; the backend routes it to that pool's CLIProxyAPI
 * management API. An empty pool means the primary (default) pool.
 */
function poolQuery(pool: string): string {
  return pool ? `?pool=${encodeURIComponent(pool)}` : ''
}

export async function listPools(): Promise<PoolInfo[]> {
  const res = await api.get<ApiEnvelope<PoolInfo[]>>('/api/pool/pools')
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to load pools')
  }
  return Array.isArray(res.data.data) ? res.data.data : []
}

export async function listPoolAuthFiles(pool: string): Promise<PoolAuthFile[]> {
  const res = await api.get<ApiEnvelope<unknown>>(
    `/api/pool/auth-files${poolQuery(pool)}`
  )
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to load pool auth files')
  }
  return normalizeList(res.data.data)
}

export async function addPoolAuthFile(
  pool: string,
  args: { name: string; content: string }
): Promise<void> {
  const res = await api.post<ApiEnvelope<unknown>>(
    `/api/pool/auth-files${poolQuery(pool)}`,
    args
  )
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to add pool auth file')
  }
}

export async function importPoolAuthFiles(
  pool: string,
  file: File
): Promise<ImportResult> {
  const form = new FormData()
  form.append('file', file)
  const res = await api.post<ApiEnvelope<ImportResult>>(
    `/api/pool/auth-files/import${poolQuery(pool)}`,
    form
  )
  if (!res.data.success || !res.data.data) {
    throw new Error(res.data.message || 'Failed to import accounts')
  }
  return res.data.data
}

export async function deletePoolAuthFile(
  pool: string,
  name: string
): Promise<void> {
  const res = await api.delete<ApiEnvelope<unknown>>('/api/pool/auth-files', {
    params: { name, pool },
  })
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to delete pool auth file')
  }
}

export async function setPoolAuthFileDisabled(
  pool: string,
  name: string,
  disabled: boolean
): Promise<void> {
  const res = await api.patch<ApiEnvelope<unknown>>(
    `/api/pool/auth-files/status${poolQuery(pool)}`,
    { name, disabled }
  )
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to update account status')
  }
}

export async function cleanPoolAuthFilesNow(pool: string): Promise<number> {
  const res = await api.post<ApiEnvelope<{ disabled?: number }>>(
    `/api/pool/auth-files/clean${poolQuery(pool)}`
  )
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to clean the pool')
  }
  return res.data.data?.disabled ?? 0
}

/**
 * Derive a stable, human-legible filename from a pasted codex auth JSON. Codex
 * files carry an email/account id; use it so the pool list is readable and
 * re-pasting the same account overwrites rather than duplicating.
 */
export function deriveAuthFileName(content: string): string {
  try {
    const parsed = JSON.parse(content) as Record<string, unknown>
    const candidate =
      (typeof parsed.email === 'string' && parsed.email) ||
      (typeof parsed.account === 'string' && parsed.account) ||
      (typeof parsed.name === 'string' && parsed.name) ||
      ''
    if (candidate) {
      const slug = candidate
        .toLowerCase()
        .replaceAll(/[^a-z0-9]+/g, '-')
        .replaceAll(/^-+|-+$/g, '')
        .slice(0, 48)
      if (slug) return `codex-${slug}.json`
    }
  } catch {
    /* fall through to a timestamped default */
  }
  return `codex-${Date.now()}.json`
}

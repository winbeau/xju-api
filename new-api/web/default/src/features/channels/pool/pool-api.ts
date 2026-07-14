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
 * Account-pool auth files, proxied by new-api's root-only /api/pool endpoints
 * to the CLIProxyAPI management API. Adding an account here is the same as
 * dropping an auth JSON into the pool — the pool hot-reloads, no restart.
 */

export type PoolAuthFile = {
  name: string
  // The management API returns extra metadata (status, type, last-seen, ...);
  // keep it open so the list view can surface whatever it sends.
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

export async function listPoolAuthFiles(): Promise<PoolAuthFile[]> {
  const res = await api.get<ApiEnvelope<unknown>>('/api/pool/auth-files')
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to load pool auth files')
  }
  return normalizeList(res.data.data)
}

export async function addPoolAuthFile(args: {
  name: string
  content: string
}): Promise<void> {
  const res = await api.post<ApiEnvelope<unknown>>('/api/pool/auth-files', args)
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to add pool auth file')
  }
}

export async function deletePoolAuthFile(name: string): Promise<void> {
  const res = await api.delete<ApiEnvelope<unknown>>('/api/pool/auth-files', {
    params: { name },
  })
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to delete pool auth file')
  }
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

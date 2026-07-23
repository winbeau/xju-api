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
// xju-api:new — shared account-state, verification, activity, and quota
// presentation helpers used by the admin pool and owner-scoped private pool.
import type {
  PoolAuthFile,
  ProbeResult,
  ProbeVerdict,
} from '@/features/pool/api'
import {
  isSubscriptionExpired,
  subscriptionUntil,
} from '@/features/pool/subscription'

export type AccountState = 'ok' | 'disabled' | 'expired' | 'unavailable'

export function accountState(file: PoolAuthFile): AccountState {
  if (file.disabled) return 'disabled'
  if (isSubscriptionExpired(file)) return 'expired'
  if (file.unavailable) return 'unavailable'
  return 'ok'
}

export const STATE_META: Record<
  AccountState,
  { labelKey: string; variant: 'success' | 'neutral' | 'danger' | 'warning' }
> = {
  ok: { labelKey: 'Active', variant: 'success' },
  disabled: { labelKey: 'Disabled', variant: 'neutral' },
  expired: { labelKey: 'Subscription expired', variant: 'danger' },
  unavailable: { labelKey: 'Unavailable', variant: 'warning' },
}

export const VERDICT_META: Record<
  ProbeVerdict,
  { labelKey: string; variant: 'success' | 'neutral' | 'danger' | 'warning' }
> = {
  online: { labelKey: 'Online', variant: 'success' },
  credential_dead: { labelKey: 'Credential dead', variant: 'danger' },
  subscription_expired: { labelKey: 'Subscription expired', variant: 'danger' },
  quota_exhausted: { labelKey: 'Quota exhausted', variant: 'warning' },
  rate_limited: { labelKey: 'Rate limited', variant: 'warning' },
  unknown: { labelKey: 'Unknown', variant: 'neutral' },
}

export function poolStats(
  files: PoolAuthFile[],
  verdicts: Record<string, ProbeResult>
): { total: number; enabled: number; online: number } {
  let enabled = 0
  let online = 0
  for (const file of files) {
    if (!file.disabled) enabled++
    const verdict = verdicts[file.name]?.verdict
    if (verdict ? verdict === 'online' : accountState(file) === 'ok') online++
  }
  return { total: files.length, enabled, online }
}

const VERDICT_ORDER: ProbeVerdict[] = [
  'online',
  'credential_dead',
  'subscription_expired',
  'quota_exhausted',
  'rate_limited',
  'unknown',
]

export function verdictBreakdown(
  results: ProbeResult[]
): [ProbeVerdict, number][] {
  const counts = new Map<ProbeVerdict, number>()
  for (const result of results) {
    counts.set(result.verdict, (counts.get(result.verdict) ?? 0) + 1)
  }
  return VERDICT_ORDER.filter((verdict) => counts.has(verdict)).map(
    (verdict) => [verdict, counts.get(verdict) ?? 0]
  )
}

export function recentActivity(
  file: PoolAuthFile
): { total: number; rate: number } | null {
  const buckets = file.recent_requests
  if (!Array.isArray(buckets) || buckets.length === 0) return null
  let ok = 0
  let failed = 0
  for (const bucket of buckets) {
    ok += bucket.success || 0
    failed += bucket.failed || 0
  }
  const total = ok + failed
  return total === 0 ? null : { total, rate: Math.round((ok / total) * 100) }
}

export function quotaPercentClass(percent: number): string {
  if (percent >= 95) return 'text-destructive'
  if (percent >= 80) return 'text-warning'
  return ''
}

export function cooldownLabel(file: PoolAuthFile): string | null {
  const raw = file.next_retry_after
  if (!raw) return null
  const parsed = new Date(raw)
  if (Number.isNaN(parsed.getTime())) return null
  const minutes = Math.ceil((parsed.getTime() - Date.now()) / 60000)
  if (minutes <= 0) return null
  if (minutes < 60) return `${minutes}m`
  const hours = Math.floor(minutes / 60)
  const remainder = minutes % 60
  return remainder > 0 ? `${hours}h ${remainder}m` : `${hours}h`
}

export { subscriptionUntil }

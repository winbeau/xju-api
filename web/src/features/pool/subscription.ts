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
import type { PoolAuthFile } from './api'

// subscriptionUntil parses the ChatGPT subscription window off a codex account.
// OpenAI emits an ISO-8601 string today, but the Go claim field is `any` and is
// passed through unnormalized, so guard a numeric Unix value: a bare
// `new Date(seconds)` would be read as milliseconds and render as Jan-1970,
// which isSubscriptionExpired would then flag as an expired (valid) account.
export function subscriptionUntil(file: PoolAuthFile): Date | null {
  const raw = file.id_token?.chatgpt_subscription_active_until
  if (raw === undefined || raw === null || raw === '') return null
  let parsed: Date
  if (typeof raw === 'number') {
    // < 1e12 ⇒ a 10-digit epoch in seconds; otherwise already milliseconds.
    parsed = new Date(raw < 1e12 ? raw * 1000 : raw)
  } else {
    parsed = new Date(raw)
  }
  return Number.isNaN(parsed.getTime()) ? null : parsed
}

export function isSubscriptionExpired(file: PoolAuthFile): boolean {
  const until = subscriptionUntil(file)
  return until !== null && until.getTime() < Date.now()
}

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

/**
 * The public base URL of this deployment, for snippets a user copies out
 * (Codex config, connection strings, chat links).
 *
 * The current origin is the source of truth: it's literally the address the
 * user reached this page on, so it is always correct. The admin-set
 * `status.server_address` is a fallback only for embedded/proxied renders
 * where `window.location` isn't the public host — and even then only when it
 * isn't the stock `http://localhost:3000` default, which is exactly what made
 * earlier copies emit `localhost` for everyone.
 *
 * Returns a URL with no trailing slash, or '' if nothing usable is available.
 */
export function getPublicServerAddress(): string {
  if (typeof window !== 'undefined' && window.location?.origin) {
    return window.location.origin
  }
  try {
    const raw =
      typeof localStorage !== 'undefined' ? localStorage.getItem('status') : null
    if (raw) {
      const configured = String(JSON.parse(raw).server_address ?? '').trim()
      if (configured && !isLocalhost(configured)) {
        return configured.replace(/\/+$/, '')
      }
    }
  } catch {
    /* no usable configured address */
  }
  return ''
}

function isLocalhost(url: string): boolean {
  return /^https?:\/\/localhost(:\d+)?\/?$/i.test(url)
}

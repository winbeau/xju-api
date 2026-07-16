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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import type { PoolAuthFile } from './api'
import { isSubscriptionExpired, subscriptionUntil } from './subscription'

const withUntil = (v: unknown): PoolAuthFile =>
  ({ name: 'x', id_token: { chatgpt_subscription_active_until: v } }) as PoolAuthFile

describe('subscriptionUntil', () => {
  test('T5.1 ISO-8601 string parses', () => {
    const d = subscriptionUntil(withUntil('2027-06-01T00:00:00Z'))
    assert.equal(d?.getUTCFullYear(), 2027)
  })
  test('T5.2 numeric Unix seconds parse as seconds, not 1970', () => {
    const d = subscriptionUntil(withUntil(1811808000)) // 2027-06-01 in seconds
    assert.equal(d?.getUTCFullYear(), 2027)
  })
  test('T5.3 numeric milliseconds parse as ms', () => {
    const d = subscriptionUntil(withUntil(1811808000000))
    assert.equal(d?.getUTCFullYear(), 2027)
  })
  test('T5.4 missing / empty / invalid → null', () => {
    assert.equal(subscriptionUntil(withUntil(undefined)), null)
    assert.equal(subscriptionUntil(withUntil('')), null)
    assert.equal(subscriptionUntil(withUntil('not-a-date')), null)
    assert.equal(subscriptionUntil({ name: 'x' } as PoolAuthFile), null)
  })
})

describe('isSubscriptionExpired', () => {
  test('T5.5 past true, future false, no-date false', () => {
    assert.equal(isSubscriptionExpired(withUntil('2000-01-01T00:00:00Z')), true)
    assert.equal(isSubscriptionExpired(withUntil('2099-01-01T00:00:00Z')), false)
    assert.equal(isSubscriptionExpired({ name: 'x' } as PoolAuthFile), false)
  })
})

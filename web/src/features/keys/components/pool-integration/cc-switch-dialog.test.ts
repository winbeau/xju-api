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
import { describe, expect, test } from 'bun:test'

import { buildCCSwitchURL, buildClaudeConfig } from './cc-switch-config'

describe('XJU Claude defaults', () => {
  test('maps Sol, Terra and Luna to the expected Claude roles', () => {
    expect(buildClaudeConfig('sk-test', {})).toEqual({
      env: {
        ANTHROPIC_BASE_URL: 'https://api.selab.top',
        ANTHROPIC_AUTH_TOKEN: 'sk-test',
        ANTHROPIC_MODEL: 'gpt-5.6-sol',
        ANTHROPIC_DEFAULT_HAIKU_MODEL: 'gpt-5.6-luna',
        ANTHROPIC_DEFAULT_SONNET_MODEL: 'gpt-5.6-terra',
        ANTHROPIC_DEFAULT_OPUS_MODEL: 'gpt-5.6-sol',
      },
    })
  })

  test('keeps the same role mapping in the CC Switch Deep Link', () => {
    const url = new URL(
      buildCCSwitchURL('claude', 'XJU API - Claude', {}, 'sk-test')
    )
    expect(url.protocol).toBe('ccswitch:')
    expect(url.searchParams.get('endpoint')).toBe('https://api.selab.top')
    expect(url.searchParams.get('model')).toBe('gpt-5.6-sol')
    expect(url.searchParams.get('haikuModel')).toBe('gpt-5.6-luna')
    expect(url.searchParams.get('sonnetModel')).toBe('gpt-5.6-terra')
    expect(url.searchParams.get('opusModel')).toBe('gpt-5.6-sol')
  })
})

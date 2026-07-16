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
/// <reference types="@rsbuild/core/types" />

declare module '@visactor/react-vchart' {
  export const VChart: React.ComponentType<Record<string, unknown>>
}

declare module '@visactor/vchart-semi-theme' {
  export const initVChartSemiTheme: (opts?: Record<string, unknown>) => void
}

// Minimal surface of bun's test runner used by *.test.ts files that need
// module mocking (node:test has no mock.module under bun). The project only
// types "node", so declare just what the tests consume instead of pulling in
// the full bun-types package.
declare module 'bun:test' {
  export function describe(name: string, fn: () => void): void
  export function test(name: string, fn: () => void | Promise<void>): void
  export function expect(actual: unknown): {
    toEqual(expected: unknown): void
    toBe(expected: unknown): void
  }
  export const mock: {
    module(specifier: string, factory: () => unknown): void
  }
}

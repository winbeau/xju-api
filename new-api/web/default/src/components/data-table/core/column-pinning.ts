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
import { cn } from '@/lib/utils'

import type { DataTableColumnClassName, DataTablePinnedColumn } from './types'

export function getResolvedColumnClassName(
  getColumnClassName?: DataTableColumnClassName,
  pinnedColumns?: DataTablePinnedColumn[]
): DataTableColumnClassName {
  return getResolvedColumnClassNameFromMap(
    getColumnClassName,
    getPinnedColumnMap(pinnedColumns)
  )
}

export function getResolvedColumnClassNameFromMap(
  getColumnClassName?: DataTableColumnClassName,
  pinnedColumnById?: Map<string, DataTablePinnedColumn>
): DataTableColumnClassName {
  return (columnId, kind) => {
    const customClassName = getColumnClassName?.(columnId, kind)
    const pinnedColumn = pinnedColumnById?.get(columnId)

    if (!pinnedColumn) {
      return customClassName
    }

    return cn(customClassName, getPinnedColumnClassName(pinnedColumn, kind))
  }
}

export function getPinnedColumnMap(pinnedColumns?: DataTablePinnedColumn[]) {
  if (!pinnedColumns?.length) {
    return undefined
  }

  return new Map(pinnedColumns.map((column) => [column.columnId, column]))
}

function getPinnedColumnClassName(
  pinnedColumn: DataTablePinnedColumn,
  kind: 'header' | 'cell'
) {
  // A pinned column needs to read as *detached* from the scrolling ones, and
  // the usual way — an edge shadow — is out of the question in this design
  // system. Instead the column casts a short gradient onto the cells it
  // overlaps: content scrolling underneath dissolves into the column's own
  // background rather than being sliced off by a hard line.
  //
  // `to-background` deliberately targets the *cell* background; the header row
  // sits on a different surface, so it gets its own stop below.
  const edgeClassName =
    pinnedColumn.side === 'left'
      ? 'before:pointer-events-none before:absolute before:inset-y-0 before:-right-6 before:w-6 before:bg-linear-to-l'
      : 'before:pointer-events-none before:absolute before:inset-y-0 before:-left-6 before:w-6 before:bg-linear-to-r'

  const edgeGradientStops =
    kind === 'header'
      ? 'before:from-transparent before:to-[var(--table-header-bg,var(--table-header))]'
      : 'before:from-transparent before:to-background'

  return cn(
    'sticky whitespace-nowrap',
    pinnedColumn.side === 'left' ? 'left-0' : 'right-0',
    edgeClassName,
    edgeGradientStops,
    kind === 'header'
      ? '[background-color:var(--table-header-bg,var(--table-header))] group-hover:[background-color:var(--table-header-hover)] z-30'
      : 'bg-background z-10 group-hover:[background-color:color-mix(in_oklch,var(--muted)_50%,var(--background))] group-data-[state=selected]:bg-muted',
    pinnedColumn.className,
    kind === 'header'
      ? pinnedColumn.headerClassName
      : pinnedColumn.cellClassName
  )
}

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
import { useRef } from 'react'

/**
 * Stable React keys for editable lists whose items have no identity, so inline
 * inputs keep focus while their own value changes. Keys are regenerated when a
 * list is replaced from outside (for example after discarding changes).
 */
export function useStableKeys() {
  const next = useRef(1)
  const lists = useRef<Record<string, number[]>>({})
  const allocate = () => next.current++
  return {
    keys(list: string, length: number): number[] {
      let keys = lists.current[list]
      if (!keys || keys.length !== length) {
        keys = Array.from({ length }, allocate)
        lists.current[list] = keys
      }
      return keys
    },
    add(list: string) {
      lists.current[list] = [...(lists.current[list] ?? []), allocate()]
    },
    remove(list: string, index: number) {
      lists.current[list] = (lists.current[list] ?? []).filter(
        (_, position) => position !== index
      )
    },
    swap(list: string, a: number, b: number) {
      const keys = [...(lists.current[list] ?? [])]
      ;[keys[a], keys[b]] = [keys[b], keys[a]]
      lists.current[list] = keys
    },
  }
}

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

export const INVITE_CODE_STATUS = {
  enabled: 1, // unused / valid
  disabled: 2,
  used: 3,
} as const

export type InviteCode = {
  id: number
  code: string
  status: number
  creator_id: number
  used_user_id: number
  created_time: number
  used_time: number
  expired_time: number // 0 = never expires
}

export type InviteCodeListResponse = {
  items: InviteCode[]
  total: number
  page: number
  page_size: number
}

type Envelope<T> = { success: boolean; message?: string; data?: T }

export async function generateInviteCodes(params: {
  count: number
  valid_days: number
}): Promise<string[]> {
  const res = await api.post('/api/invite_code/generate', params)
  const body = res.data as Envelope<string[]>
  if (!body?.success) throw new Error(body?.message || 'Failed to generate invite codes')
  return body.data ?? []
}

export async function listInviteCodes(params: {
  p?: number
  page_size?: number
  keyword?: string
  status?: string
}): Promise<InviteCodeListResponse> {
  const { p = 1, page_size = 20, keyword = '', status = '' } = params
  const qs = new URLSearchParams({ p: String(p), page_size: String(page_size) })
  if (keyword) qs.set('keyword', keyword)
  if (status) qs.set('status', status)
  const res = await api.get(`/api/invite_code/?${qs.toString()}`)
  const body = res.data as Envelope<InviteCodeListResponse>
  const data = body?.data
  return {
    items: data?.items ?? [],
    total: data?.total ?? 0,
    page: data?.page ?? p,
    page_size: data?.page_size ?? page_size,
  }
}

export async function setInviteCodeStatus(id: number, status: number): Promise<void> {
  const res = await api.put('/api/invite_code/status', { id, status })
  const body = res.data as Envelope<unknown>
  if (!body?.success) throw new Error(body?.message || 'Failed to update invite code')
}

export async function deleteInviteCode(id: number): Promise<void> {
  const res = await api.delete(`/api/invite_code/${id}`)
  const body = res.data as Envelope<unknown>
  if (!body?.success) throw new Error(body?.message || 'Failed to delete invite code')
}

export async function deleteInvalidInviteCodes(): Promise<number> {
  const res = await api.delete('/api/invite_code/invalid')
  const body = res.data as Envelope<number>
  if (!body?.success) throw new Error(body?.message || 'Failed to prune invite codes')
  return body.data ?? 0
}

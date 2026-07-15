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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Copy,
  Loader2,
  Power,
  RefreshCw,
  Sparkles,
  Trash2,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { copyToClipboard } from '@/lib/copy-to-clipboard'

import {
  INVITE_CODE_STATUS,
  type InviteCode,
  deleteInviteCode,
  deleteInvalidInviteCodes,
  generateInviteCodes,
  listInviteCodes,
  setInviteCodeStatus,
} from './api'

type InviteCodeDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

const PAGE_SIZE = 20

type Derived = { labelKey: string; variant: 'success' | 'neutral' | 'danger' }

function deriveStatus(code: InviteCode): Derived {
  if (code.status === INVITE_CODE_STATUS.used) {
    return { labelKey: 'Used', variant: 'neutral' }
  }
  if (code.status === INVITE_CODE_STATUS.disabled) {
    return { labelKey: 'Disabled', variant: 'neutral' }
  }
  if (code.expired_time !== 0 && code.expired_time * 1000 < Date.now()) {
    return { labelKey: 'Expired', variant: 'danger' }
  }
  return { labelKey: 'Unused', variant: 'success' }
}

export function InviteCodeDialog({ open, onOpenChange }: InviteCodeDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const [count, setCount] = useState(1)
  const [validDays, setValidDays] = useState(0)
  const [generated, setGenerated] = useState<string[]>([])
  const [statusFilter, setStatusFilter] = useState('')
  const [page, setPage] = useState(1)

  const listQuery = useQuery({
    queryKey: ['invite-codes', page, statusFilter],
    queryFn: () =>
      listInviteCodes({ p: page, page_size: PAGE_SIZE, status: statusFilter }),
    enabled: open,
    staleTime: 5_000,
  })

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ['invite-codes'] })

  const generateMutation = useMutation({
    mutationFn: () =>
      generateInviteCodes({ count: Math.max(1, count), valid_days: Math.max(0, validDays) }),
    onSuccess: async (codes) => {
      setGenerated(codes)
      toast.success(t('Generated {{count}} invite code(s)', { count: codes.length }))
      setPage(1)
      await invalidate()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const statusMutation = useMutation({
    mutationFn: (args: { id: number; status: number }) =>
      setInviteCodeStatus(args.id, args.status),
    onSuccess: async () => await invalidate(),
    onError: (error: Error) => toast.error(error.message),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: number) => deleteInviteCode(id),
    onSuccess: async () => {
      toast.success(t('Invite code deleted'))
      await invalidate()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const pruneMutation = useMutation({
    mutationFn: deleteInvalidInviteCodes,
    onSuccess: async (n) => {
      toast.success(t('Pruned {{count}} invalid invite code(s)', { count: n }))
      await invalidate()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const copy = async (text: string, label: string) => {
    const ok = await copyToClipboard(text)
    if (ok) toast.success(label)
    else toast.error(t('Copy failed'))
  }

  const items = listQuery.data?.items ?? []
  const total = listQuery.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  const FILTERS: Array<{ key: string; labelKey: string }> = [
    { key: '', labelKey: 'All' },
    { key: String(INVITE_CODE_STATUS.enabled), labelKey: 'Unused' },
    { key: String(INVITE_CODE_STATUS.used), labelKey: 'Used' },
    { key: String(INVITE_CODE_STATUS.disabled), labelKey: 'Disabled' },
    { key: 'expired', labelKey: 'Expired' },
  ]

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Invite Codes')}
      description={t(
        'Generate single-use invite codes and manage their validity. Share a code with the person you invite — they enter it on the sign-up page (or use a /register?aff=CODE link).'
      )}
      contentClassName='max-w-2xl'
      bodyClassName='space-y-4'
    >
      {/* Generate */}
      <div className='border-border grid gap-3 rounded-md border p-3'>
        <div className='flex flex-wrap items-end gap-3'>
          <div className='grid gap-1'>
            <label className='text-muted-foreground text-xs'>
              {t('Quantity')}
            </label>
            <Input
              type='number'
              min={1}
              max={200}
              value={count}
              onChange={(e) => setCount(Number(e.target.value))}
              className='h-9 w-24'
            />
          </div>
          <div className='grid gap-1'>
            <label className='text-muted-foreground text-xs'>
              {t('Valid days (0 = never expires)')}
            </label>
            <Input
              type='number'
              min={0}
              value={validDays}
              onChange={(e) => setValidDays(Number(e.target.value))}
              className='h-9 w-40'
            />
          </div>
          <Button
            type='button'
            onClick={() => generateMutation.mutate()}
            disabled={generateMutation.isPending}
          >
            {generateMutation.isPending ? (
              <Loader2 className='animate-spin' />
            ) : (
              <Sparkles />
            )}
            {t('Generate')}
          </Button>
        </div>

        {generated.length > 0 && (
          <div className='bg-muted grid gap-2 rounded-md p-2'>
            <div className='flex items-center justify-between'>
              <span className='text-xs font-medium'>
                {t('New codes ({{count}})', { count: generated.length })}
              </span>
              <Button
                type='button'
                variant='ghost'
                size='sm'
                onClick={() => copy(generated.join('\n'), t('All codes copied'))}
              >
                <Copy className='size-3.5' />
                {t('Copy all')}
              </Button>
            </div>
            <div className='max-h-28 overflow-auto font-mono text-xs'>
              {generated.map((c) => (
                <div key={c} className='truncate'>
                  {c}
                </div>
              ))}
            </div>
          </div>
        )}
      </div>

      {/* Manage */}
      <div className='flex flex-wrap items-center gap-1.5'>
        {FILTERS.map((f) => (
          <Button
            key={f.key || 'all'}
            type='button'
            size='sm'
            variant={statusFilter === f.key ? 'default' : 'outline'}
            onClick={() => {
              setStatusFilter(f.key)
              setPage(1)
            }}
          >
            {t(f.labelKey)}
          </Button>
        ))}
        <div className='ml-auto flex items-center gap-1.5'>
          <Button
            type='button'
            size='sm'
            variant='outline'
            onClick={() => invalidate()}
            disabled={listQuery.isFetching}
          >
            <RefreshCw className={listQuery.isFetching ? 'animate-spin' : ''} />
            {t('Refresh')}
          </Button>
          <Button
            type='button'
            size='sm'
            variant='outline'
            onClick={() => pruneMutation.mutate()}
            disabled={pruneMutation.isPending}
          >
            <Trash2 />
            {t('Prune invalid')}
          </Button>
        </div>
      </div>

      <div className='border-border overflow-hidden rounded-md border'>
        {listQuery.isLoading && (
          <div className='text-muted-foreground flex items-center gap-2 p-4 text-sm'>
            <Loader2 className='size-4 animate-spin' />
            {t('Loading...')}
          </div>
        )}
        {!listQuery.isLoading && items.length === 0 && (
          <div className='text-muted-foreground p-4 text-sm'>
            {t('No invite codes yet.')}
          </div>
        )}
        {!listQuery.isLoading && items.length > 0 && (
          <ul className='divide-border divide-y'>
            {items.map((code) => {
              const meta = deriveStatus(code)
              const isUnused = meta.labelKey === 'Unused'
              return (
                <li
                  key={code.id}
                  className='hover:bg-muted flex items-center justify-between gap-3 px-3 py-2 transition-colors'
                >
                  <div className='min-w-0'>
                    <div className='flex items-center gap-2'>
                      <span className='truncate font-mono text-xs'>
                        {code.code}
                      </span>
                      <StatusBadge
                        label={t(meta.labelKey)}
                        variant={meta.variant}
                        copyable={false}
                      />
                    </div>
                    <p className='text-muted-foreground text-xs'>
                      {code.expired_time === 0
                        ? t('Never expires')
                        : t('Expires {{date}}', {
                            date: new Date(
                              code.expired_time * 1000
                            ).toLocaleString(),
                          })}
                    </p>
                  </div>
                  <div className='flex shrink-0 items-center gap-1'>
                    <Button
                      type='button'
                      variant='ghost'
                      size='icon-sm'
                      title={t('Copy')}
                      aria-label={t('Copy')}
                      onClick={() => copy(code.code, t('Code copied'))}
                    >
                      <Copy className='size-4' />
                    </Button>
                    {code.status !== INVITE_CODE_STATUS.used && (
                      <Button
                        type='button'
                        variant='ghost'
                        size='icon-sm'
                        title={isUnused ? t('Disable') : t('Enable')}
                        aria-label={isUnused ? t('Disable') : t('Enable')}
                        className={isUnused ? '' : 'text-success'}
                        onClick={() =>
                          statusMutation.mutate({
                            id: code.id,
                            status: isUnused
                              ? INVITE_CODE_STATUS.disabled
                              : INVITE_CODE_STATUS.enabled,
                          })
                        }
                        disabled={statusMutation.isPending}
                      >
                        <Power className='size-4' />
                      </Button>
                    )}
                    <Button
                      type='button'
                      variant='ghost'
                      size='icon-sm'
                      className='text-destructive hover:text-destructive'
                      title={t('Delete')}
                      aria-label={t('Delete')}
                      onClick={() => deleteMutation.mutate(code.id)}
                      disabled={deleteMutation.isPending}
                    >
                      <Trash2 className='size-4' />
                    </Button>
                  </div>
                </li>
              )
            })}
          </ul>
        )}
      </div>

      <div className='text-muted-foreground flex items-center justify-between text-xs'>
        <span>{t('Total: {{count}}', { count: total })}</span>
        <div className='flex items-center gap-2'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={page <= 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
          >
            {t('Previous')}
          </Button>
          <span>
            {page} / {totalPages}
          </span>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={page >= totalPages}
            onClick={() => setPage((p) => p + 1)}
          >
            {t('Next')}
          </Button>
        </div>
      </div>
    </Dialog>
  )
}

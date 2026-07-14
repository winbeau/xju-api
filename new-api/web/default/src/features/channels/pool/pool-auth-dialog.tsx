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
import { ClipboardPaste, Loader2, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Textarea } from '@/components/ui/textarea'

import {
  addPoolAuthFile,
  deletePoolAuthFile,
  deriveAuthFileName,
  listPoolAuthFiles,
  type PoolAuthFile,
} from './pool-api'

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

function poolFileStatus(file: PoolAuthFile): string | null {
  const status = file.status ?? file.state
  return typeof status === 'string' ? status : null
}

export function PoolAuthDialog({ open, onOpenChange }: Props) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [content, setContent] = useState('')

  const listQuery = useQuery({
    queryKey: ['pool', 'auth-files'],
    queryFn: listPoolAuthFiles,
    enabled: open,
    staleTime: 10_000,
  })

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ['pool', 'auth-files'] })

  const addMutation = useMutation({
    mutationFn: async () => {
      const trimmed = content.trim()
      if (!trimmed) throw new Error(t('Paste an auth JSON first'))
      // Fail fast on a bad paste, with the cursor still on the textarea.
      try {
        JSON.parse(trimmed)
      } catch {
        throw new Error(t('That is not valid JSON'))
      }
      await addPoolAuthFile({
        name: deriveAuthFileName(trimmed),
        content: trimmed,
      })
    },
    onSuccess: async () => {
      toast.success(t('Account added to the pool'))
      setContent('')
      await invalidate()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const deleteMutation = useMutation({
    mutationFn: (name: string) => deletePoolAuthFile(name),
    onSuccess: async () => {
      toast.success(t('Account removed'))
      await invalidate()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const handlePaste = async () => {
    try {
      const text = await navigator.clipboard.readText()
      if (text.trim()) setContent(text)
    } catch {
      toast.error(t('Clipboard not available — paste manually'))
    }
  }

  const files = listQuery.data ?? []

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>{t('Account Pool')}</DialogTitle>
          <DialogDescription>
            {t(
              'Paste a codex auth JSON to add an upstream account. The pool reloads instantly — no restart, no channel edits.'
            )}
          </DialogDescription>
        </DialogHeader>

        <div className='grid gap-5'>
          {/* Paste + add */}
          <div className='grid gap-2'>
            <div className='flex items-center justify-between'>
              <span className='text-sm font-medium'>{t('Add account')}</span>
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={handlePaste}
              >
                <ClipboardPaste />
                {t('Paste')}
              </Button>
            </div>
            <Textarea
              value={content}
              onChange={(event) => setContent(event.target.value)}
              placeholder='{ "email": "...", "OPENAI_API_KEY": "..." }'
              className='h-40 font-mono text-xs'
              spellCheck={false}
            />
            <Button
              type='button'
              onClick={() => addMutation.mutate()}
              disabled={addMutation.isPending || !content.trim()}
              className='justify-self-start'
            >
              {addMutation.isPending ? <Loader2 className='animate-spin' /> : <Plus />}
              {t('Add to pool')}
            </Button>
          </div>

          {/* Current accounts */}
          <div className='grid gap-2'>
            <span className='text-sm font-medium'>
              {t('Accounts in pool')}
              {files.length > 0 ? ` (${files.length})` : ''}
            </span>
            <div className='border-border overflow-hidden rounded-md border'>
              {listQuery.isLoading && (
                <div className='text-muted-foreground flex items-center gap-2 p-4 text-sm'>
                  <Loader2 className='size-4 animate-spin' />
                  {t('Loading...')}
                </div>
              )}
              {!listQuery.isLoading && listQuery.isError && (
                <div className='text-destructive p-4 text-sm'>
                  {(listQuery.error as Error).message}
                </div>
              )}
              {!listQuery.isLoading && !listQuery.isError && files.length === 0 && (
                <div className='text-muted-foreground p-4 text-sm'>
                  {t('No accounts yet.')}
                </div>
              )}
              {!listQuery.isLoading && !listQuery.isError && files.length > 0 && (
                <ul className='divide-border divide-y'>
                  {files.map((file) => {
                    const status = poolFileStatus(file)
                    return (
                      <li
                        key={file.name}
                        className='hover:bg-muted flex items-center justify-between gap-3 px-3 py-2 transition-colors'
                      >
                        <div className='min-w-0'>
                          <p className='truncate font-mono text-xs'>
                            {file.name}
                          </p>
                          {status && (
                            <p className='text-muted-foreground text-xs'>
                              {status}
                            </p>
                          )}
                        </div>
                        <Button
                          type='button'
                          variant='ghost'
                          size='icon-sm'
                          className='text-destructive hover:text-destructive shrink-0'
                          onClick={() => deleteMutation.mutate(file.name)}
                          disabled={deleteMutation.isPending}
                          aria-label={t('Remove')}
                        >
                          <Trash2 className='size-4' />
                        </Button>
                      </li>
                    )
                  })}
                </ul>
              )}
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}

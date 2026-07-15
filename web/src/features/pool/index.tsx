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
  ClipboardPaste,
  FileArchive,
  Loader2,
  Play,
  Plus,
  Power,
  RefreshCw,
  Sparkles,
  Trash2,
  Upload,
} from 'lucide-react'
import { useRef, useState, type ChangeEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { StatusBadge } from '@/components/status-badge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import {
  addPoolAuthFile,
  cleanPoolAuthFilesNow,
  deletePoolAuthFile,
  deriveAuthFileName,
  importPoolAuthFiles,
  listPoolAuthFiles,
  listPools,
  setPoolAuthFileDisabled,
  type ImportResult,
  type PoolAuthFile,
  type PoolInfo,
} from '@/features/channels/pool/pool-api'
import { useStatus } from '@/hooks/use-status'
import { api } from '@/lib/api'

type AccountState = 'ok' | 'disabled' | 'unavailable'

function accountState(file: PoolAuthFile): AccountState {
  if (file.disabled) return 'disabled'
  if (file.unavailable) return 'unavailable'
  return 'ok'
}

const STATE_META: Record<
  AccountState,
  { labelKey: string; variant: 'success' | 'neutral' | 'danger' }
> = {
  ok: { labelKey: 'Active', variant: 'success' },
  disabled: { labelKey: 'Disabled', variant: 'neutral' },
  unavailable: { labelKey: 'Unavailable', variant: 'danger' },
}

export function Pool() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { status } = useStatus()
  const [content, setContent] = useState('')
  const [pool, setPool] = useState('default')
  const [importResult, setImportResult] = useState<ImportResult | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const zipInputRef = useRef<HTMLInputElement>(null)

  const autoCleanEnabled = Boolean(status?.pool_auto_clean_enabled)
  const autoCleanHours = Number(status?.pool_auto_clean_hours ?? 24)

  const poolsQuery = useQuery({
    queryKey: ['pool', 'pools'],
    queryFn: listPools,
    staleTime: 60_000,
  })
  const pools: PoolInfo[] = poolsQuery.data ?? [
    { id: 'default', label: 'Default' },
  ]

  const listQuery = useQuery({
    queryKey: ['pool', 'auth-files', pool],
    queryFn: () => listPoolAuthFiles(pool),
    staleTime: 10_000,
  })

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ['pool', 'auth-files', pool] })

  const addMutation = useMutation({
    mutationFn: async () => {
      const trimmed = content.trim()
      if (!trimmed) throw new Error(t('Paste an auth JSON first'))
      try {
        JSON.parse(trimmed)
      } catch {
        throw new Error(t('That is not valid JSON'))
      }
      await addPoolAuthFile(pool, {
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
    mutationFn: (name: string) => deletePoolAuthFile(pool, name),
    onSuccess: async () => {
      toast.success(t('Account removed'))
      await invalidate()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const toggleMutation = useMutation({
    mutationFn: (args: { name: string; disabled: boolean }) =>
      setPoolAuthFileDisabled(pool, args.name, args.disabled),
    onSuccess: async () => await invalidate(),
    onError: (error: Error) => toast.error(error.message),
  })

  const importMutation = useMutation({
    mutationFn: (file: File) => importPoolAuthFiles(pool, file),
    onSuccess: async (result) => {
      setImportResult(result)
      toast.success(
        t('Imported {{imported}} · skipped {{skipped}} · failed {{failed}}', {
          imported: result.imported,
          skipped: result.skipped.length,
          failed: result.failed.length,
        })
      )
      await invalidate()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const cleanMutation = useMutation({
    mutationFn: () => cleanPoolAuthFilesNow(pool),
    onSuccess: async (disabled) => {
      toast.success(
        disabled > 0
          ? t('Disabled {{count}} stale account(s)', { count: disabled })
          : t('Nothing to clean — all accounts are healthy')
      )
      await invalidate()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const autoCleanMutation = useMutation({
    mutationFn: (enabled: boolean) =>
      api.put('/api/option/', {
        key: 'PoolAutoCleanEnabled',
        value: String(enabled),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['status'] })
      toast.success(t('Saved successfully'))
    },
    onError: () => toast.error(t('Save failed')),
  })

  const handlePaste = async () => {
    try {
      const text = await navigator.clipboard.readText()
      if (text.trim()) setContent(text)
    } catch {
      toast.error(t('Clipboard not available — paste manually'))
    }
  }

  const handleFileUpload = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    // Reset so selecting the same file again still fires onChange.
    event.target.value = ''
    if (!file) return
    try {
      const text = await file.text()
      if (text.trim()) setContent(text)
    } catch {
      toast.error(t('Could not read that file'))
    }
  }

  const handleZipImport = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    // Reset so selecting the same file again still fires onChange.
    event.target.value = ''
    if (!file) return
    setImportResult(null)
    importMutation.mutate(file)
  }

  const files = listQuery.data ?? []
  const listReady = !listQuery.isLoading && !listQuery.isError
  const activeCount = files.filter((f) => accountState(f) === 'ok').length

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        <span className='inline-flex min-w-0 items-center gap-2'>
          <span className='truncate'>{t('Account Pool')}</span>
          <Badge variant='outline' className='shrink-0'>
            Root
          </Badge>
        </span>
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        {pools.length > 1 && (
          <Tabs
            value={pool}
            onValueChange={(value) => {
              setPool(String(value))
              setImportResult(null)
            }}
            className='mb-4'
          >
            <TabsList>
              {pools.map((p) => (
                <TabsTrigger key={p.id} value={p.id}>
                  {p.label}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
        )}
        <div className='grid gap-4 xl:grid-cols-[minmax(0,1fr)_22rem]'>
          {/* Accounts list */}
          <Card data-card-hover='false'>
            <CardHeader className='flex-row items-center justify-between gap-3 space-y-0'>
              <div className='min-w-0'>
                <CardTitle className='text-base'>
                  {t('Accounts in pool')}
                  {files.length > 0
                    ? ` · ${activeCount}/${files.length} ${t('active')}`
                    : ''}
                </CardTitle>
                <CardDescription>
                  {t('Upstream codex accounts behind the shared pool.')}
                </CardDescription>
              </div>
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={() => invalidate()}
                disabled={listQuery.isFetching}
              >
                <RefreshCw
                  className={listQuery.isFetching ? 'animate-spin' : ''}
                />
                {t('Refresh')}
              </Button>
            </CardHeader>
            <CardContent>
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
                {listReady && files.length === 0 && (
                  <div className='text-muted-foreground p-4 text-sm'>
                    {t('No accounts yet.')}
                  </div>
                )}
                {listReady && files.length > 0 && (
                  <ul className='divide-border divide-y'>
                    {files.map((file) => {
                      const state = accountState(file)
                      const meta = STATE_META[state]
                      const label = file.email || file.account || file.name
                      return (
                        <li
                          key={file.name}
                          className='hover:bg-muted flex items-center justify-between gap-3 px-3 py-2.5 transition-colors'
                        >
                          <div className='min-w-0'>
                            <div className='flex items-center gap-2'>
                              <span className='truncate text-sm font-medium'>
                                {label}
                              </span>
                              <StatusBadge
                                label={t(meta.labelKey)}
                                variant={meta.variant}
                                copyable={false}
                              />
                            </div>
                            <p className='text-muted-foreground truncate font-mono text-xs'>
                              {file.name}
                              {typeof file.failed === 'number' && file.failed > 0
                                ? ` · ${t('{{count}} recent failures', { count: file.failed })}`
                                : ''}
                            </p>
                          </div>
                          <div className='flex shrink-0 items-center gap-1'>
                            <Button
                              type='button'
                              variant='ghost'
                              size='icon-sm'
                              aria-label={
                                file.disabled ? t('Enable') : t('Disable')
                              }
                              title={file.disabled ? t('Enable') : t('Disable')}
                              className={file.disabled ? 'text-success' : ''}
                              onClick={() =>
                                toggleMutation.mutate({
                                  name: file.name,
                                  disabled: !file.disabled,
                                })
                              }
                              disabled={toggleMutation.isPending}
                            >
                              <Power className='size-4' />
                            </Button>
                            <Button
                              type='button'
                              variant='ghost'
                              size='icon-sm'
                              className='text-destructive hover:text-destructive'
                              aria-label={t('Remove')}
                              title={t('Remove')}
                              onClick={() => deleteMutation.mutate(file.name)}
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
            </CardContent>
          </Card>

          {/* Right column: add + auto-clean */}
          <div className='grid content-start gap-4'>
            <Card data-card-hover='false'>
              <CardHeader>
                <CardTitle className='text-base'>{t('Add account')}</CardTitle>
                <CardDescription>
                  {t(
                    'Paste a codex auth JSON, or import a .zip of many accounts. The pool reloads instantly.'
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent className='grid gap-2'>
                <div className='flex justify-end gap-2'>
                  <input
                    ref={zipInputRef}
                    type='file'
                    accept='.zip'
                    className='hidden'
                    onChange={handleZipImport}
                  />
                  <input
                    ref={fileInputRef}
                    type='file'
                    accept='.json,application/json'
                    className='hidden'
                    onChange={handleFileUpload}
                  />
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={() => zipInputRef.current?.click()}
                    disabled={importMutation.isPending}
                  >
                    {importMutation.isPending ? (
                      <Loader2 className='animate-spin' />
                    ) : (
                      <FileArchive />
                    )}
                    {t('Import .zip')}
                  </Button>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={() => fileInputRef.current?.click()}
                  >
                    <Upload />
                    {t('Upload')}
                  </Button>
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
                  className='h-36 font-mono text-xs'
                  spellCheck={false}
                />
                <Button
                  type='button'
                  onClick={() => addMutation.mutate()}
                  disabled={addMutation.isPending || !content.trim()}
                >
                  {addMutation.isPending ? (
                    <Loader2 className='animate-spin' />
                  ) : (
                    <Plus />
                  )}
                  {t('Add to pool')}
                </Button>
                {importResult && (
                  <div className='border-border mt-1 rounded-md border p-2 text-xs'>
                    <p className='font-medium'>
                      {t(
                        'Imported {{imported}} · skipped {{skipped}} · failed {{failed}}',
                        {
                          imported: importResult.imported,
                          skipped: importResult.skipped.length,
                          failed: importResult.failed.length,
                        }
                      )}
                    </p>
                    {importResult.failed.length > 0 && (
                      <ul className='text-destructive mt-1 max-h-24 overflow-auto'>
                        {importResult.failed.map((f) => (
                          <li key={f.name} className='truncate font-mono'>
                            {f.name}: {f.error}
                          </li>
                        ))}
                      </ul>
                    )}
                    {importResult.skipped.length > 0 && (
                      <ul className='text-muted-foreground mt-1 max-h-24 overflow-auto'>
                        {importResult.skipped.map((s) => (
                          <li key={s.name} className='truncate font-mono'>
                            {s.name}: {s.reason}
                          </li>
                        ))}
                      </ul>
                    )}
                  </div>
                )}
              </CardContent>
            </Card>

            <Card data-card-hover='false'>
              <CardHeader>
                <CardTitle className='flex items-center gap-1.5 text-base'>
                  <Sparkles className='size-4' />
                  {t('Auto-clean')}
                </CardTitle>
                <CardDescription>
                  {t(
                    'Hourly: disable accounts that stay unavailable past {{hours}}h.',
                    { hours: autoCleanHours }
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent className='grid gap-3'>
                <div className='flex items-center justify-between'>
                  <span className='text-sm font-medium'>
                    {t('Enable auto-clean')}
                  </span>
                  <Switch
                    checked={autoCleanEnabled}
                    disabled={autoCleanMutation.isPending}
                    onCheckedChange={(v) => autoCleanMutation.mutate(v)}
                  />
                </div>
                <Button
                  type='button'
                  variant='outline'
                  onClick={() => cleanMutation.mutate()}
                  disabled={cleanMutation.isPending}
                >
                  {cleanMutation.isPending ? (
                    <Loader2 className='animate-spin' />
                  ) : (
                    <Play />
                  )}
                  {t('Clean now')}
                </Button>
              </CardContent>
            </Card>
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

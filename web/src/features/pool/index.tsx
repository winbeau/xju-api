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
  Activity,
  ClipboardPaste,
  FileArchive,
  Loader2,
  Play,
  Plus,
  Power,
  RefreshCw,
  ShieldCheck,
  Sparkles,
  Trash2,
  Upload,
} from 'lucide-react'
import { useEffect, useRef, useState, type ChangeEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { SectionPageLayout } from '@/components/layout'
import { StatusBadge } from '@/components/status-badge'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
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
  createPool,
  deletePool,
  deletePoolAuthFile,
  deriveAuthFileName,
  getPoolCreateStatus,
  getVerifyProgress,
  importPoolAuthFiles,
  isDynamicPool,
  listPoolAuthFiles,
  listPools,
  setPoolAuthFileDisabled,
  startVerifyAll,
  verifyPoolAuthFile,
  type ImportResult,
  type PoolAuthFile,
  type PoolInfo,
  type ProbeResult,
  type ProbeVerdict,
} from '@/features/pool/api'
import { useStatus } from '@/hooks/use-status'
import { api } from '@/lib/api'

type AccountState = 'ok' | 'disabled' | 'expired' | 'unavailable'

// xju-api:new — parse the codex subscription window carried in id_token. An
// expired subscription is a certain death (no cooldown will bring it back),
// so it ranks above the transient `unavailable` cooldown state.
function subscriptionUntil(file: PoolAuthFile): Date | null {
  const raw = file.id_token?.chatgpt_subscription_active_until
  if (!raw) return null
  const parsed = new Date(raw)
  return Number.isNaN(parsed.getTime()) ? null : parsed
}

function isSubscriptionExpired(file: PoolAuthFile): boolean {
  const until = subscriptionUntil(file)
  return until !== null && until.getTime() < Date.now()
}

function accountState(file: PoolAuthFile): AccountState {
  if (file.disabled) return 'disabled'
  if (isSubscriptionExpired(file)) return 'expired'
  if (file.unavailable) return 'unavailable'
  return 'ok'
}

const STATE_META: Record<
  AccountState,
  { labelKey: string; variant: 'success' | 'neutral' | 'danger' | 'warning' }
> = {
  ok: { labelKey: 'Active', variant: 'success' },
  disabled: { labelKey: 'Disabled', variant: 'neutral' },
  expired: { labelKey: 'Subscription expired', variant: 'danger' },
  // Transient cooldown that self-heals when NextRetryAfter passes — amber, not red.
  unavailable: { labelKey: 'Unavailable', variant: 'warning' },
}

// xju-api:new — verdict → badge style/label for active verification results.
const VERDICT_META: Record<
  ProbeVerdict,
  { labelKey: string; variant: 'success' | 'neutral' | 'danger' | 'warning' }
> = {
  online: { labelKey: 'Online', variant: 'success' },
  credential_dead: { labelKey: 'Credential dead', variant: 'danger' },
  subscription_expired: { labelKey: 'Subscription expired', variant: 'danger' },
  quota_exhausted: { labelKey: 'Quota exhausted', variant: 'warning' },
  rate_limited: { labelKey: 'Rate limited', variant: 'warning' },
  unknown: { labelKey: 'Unknown', variant: 'neutral' },
}

// xju-api:new — pool stats split into two orthogonal dimensions so "enabled"
// (operator toggle) and "online" (health) never conflate. Total counts every
// account; enabled counts those the operator hasn't disabled; online prefers
// the live verify verdict and falls back to the passive health proxy.
function poolStats(
  files: PoolAuthFile[],
  verdicts: Record<string, ProbeResult>
): { total: number; enabled: number; online: number } {
  let enabled = 0
  let online = 0
  for (const f of files) {
    if (!f.disabled) enabled++
    const verdict = verdicts[f.name]?.verdict
    const isOnline = verdict
      ? verdict === 'online'
      : accountState(f) === 'ok'
    if (isOnline) online++
  }
  return { total: files.length, enabled, online }
}

// xju-api:new — count verify results by verdict, in a stable display order, for
// the verify-all summary breakdown.
const VERDICT_ORDER: ProbeVerdict[] = [
  'online',
  'credential_dead',
  'subscription_expired',
  'quota_exhausted',
  'rate_limited',
  'unknown',
]

function verdictBreakdown(results: ProbeResult[]): [ProbeVerdict, number][] {
  const counts = new Map<ProbeVerdict, number>()
  for (const r of results) counts.set(r.verdict, (counts.get(r.verdict) ?? 0) + 1)
  return VERDICT_ORDER.filter((v) => counts.has(v)).map((v) => [
    v,
    counts.get(v) ?? 0,
  ])
}

// xju-api:new — aggregate the 10-minute recent-request buckets into a success
// rate + total, so the operator sees "is this account actually being used and
// succeeding" without opening logs. Returns null when there was no activity.
function recentActivity(
  file: PoolAuthFile
): { total: number; rate: number } | null {
  const buckets = file.recent_requests
  if (!Array.isArray(buckets) || buckets.length === 0) return null
  let ok = 0
  let bad = 0
  for (const b of buckets) {
    ok += b.success || 0
    bad += b.failed || 0
  }
  const total = ok + bad
  if (total === 0) return null
  return { total, rate: Math.round((ok / total) * 100) }
}

// xju-api:new — human-readable time left on the account's cooldown
// (NextRetryAfter), e.g. "8m" or "2h 5m". Null when there is no active
// cooldown. Refreshed each time the list refetches. Codex cooldowns run up to
// 12h (404), so minutes alone read poorly — roll into hours past 60 minutes.
function cooldownLabel(file: PoolAuthFile): string | null {
  const raw = file.next_retry_after
  if (!raw) return null
  const parsed = new Date(raw)
  if (Number.isNaN(parsed.getTime())) return null
  const min = Math.ceil((parsed.getTime() - Date.now()) / 60000)
  if (min <= 0) return null
  if (min < 60) return `${min}m`
  const h = Math.floor(min / 60)
  const m = min % 60
  return m > 0 ? `${h}h ${m}m` : `${h}h`
}

export function Pool() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { status } = useStatus()
  const [content, setContent] = useState('')
  const [pool, setPool] = useState('default')
  const [importResult, setImportResult] = useState<ImportResult | null>(null)
  // xju-api:new — per-account verify verdicts (keyed by file name) + verify-all options.
  const [verdicts, setVerdicts] = useState<Record<string, ProbeResult>>({})
  const [heavyProbe, setHeavyProbe] = useState(false)
  const [autoDisable, setAutoDisable] = useState(false)
  // xju-api:new — one-click pool creation + deletion (#4 Phase D).
  const [createOpen, setCreateOpen] = useState(false)
  const [newLabel, setNewLabel] = useState('')
  const [creatingId, setCreatingId] = useState<string | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<PoolInfo | null>(null)
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

  // xju-api:new — single-account verify (report-only, never changes state).
  const verifyMutation = useMutation({
    mutationFn: (name: string) => verifyPoolAuthFile(pool, name, heavyProbe),
    onSuccess: (result) =>
      setVerdicts((prev) => ({ ...prev, [result.name]: result })),
    onError: (error: Error) => toast.error(error.message),
  })

  // xju-api:new — verify-all: kick off the background job, then poll progress.
  const verifyAllMutation = useMutation({
    mutationFn: () => startVerifyAll(pool, { heavy: heavyProbe, autoDisable }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['pool', 'verify', pool] })
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const progressQuery = useQuery({
    queryKey: ['pool', 'verify', pool],
    queryFn: () => getVerifyProgress(pool),
    // Poll while a run is in flight; otherwise leave the last snapshot in place.
    refetchInterval: (query) =>
      query.state.data?.running ? 2000 : false,
  })
  const progress = progressQuery.data ?? null

  // Fold the job's per-account results into the same verdict map the row badges
  // read, and refresh the list once when a run finishes (auto-disable may have
  // changed account state).
  const jobRunning = progress?.running ?? false
  const jobResultCount = progress?.results?.length ?? 0
  const prevRunning = useRef(false)
  useEffect(() => {
    if (!progress?.results?.length) return
    setVerdicts((prev) => {
      const next = { ...prev }
      for (const r of progress.results) next[r.name] = r
      return next
    })
  }, [progress?.results, jobResultCount])
  useEffect(() => {
    if (prevRunning.current && !jobRunning) {
      queryClient.invalidateQueries({
        queryKey: ['pool', 'auth-files', pool],
      })
    }
    prevRunning.current = jobRunning
  }, [jobRunning, pool, queryClient])

  // xju-api:new — create a pool, then poll provisioning until it's ready and
  // switch to its tab.
  const createMutation = useMutation({
    mutationFn: () => createPool(newLabel.trim()),
    onSuccess: (res) => setCreatingId(res.pool_id),
    onError: (error: Error) => toast.error(error.message),
  })
  const createStatusQuery = useQuery({
    queryKey: ['pool', 'create-status', creatingId],
    queryFn: () => getPoolCreateStatus(creatingId as string),
    enabled: !!creatingId,
    refetchInterval: (query) =>
      query.state.data?.status === 'provisioning' ? 2000 : false,
  })
  const createStatus = createStatusQuery.data?.status
  const createError = createStatusQuery.data?.error
  useEffect(() => {
    if (!creatingId || !createStatus) return
    if (createStatus === 'ready') {
      const id = creatingId
      setCreatingId(null)
      setCreateOpen(false)
      setNewLabel('')
      queryClient.invalidateQueries({ queryKey: ['pool', 'pools'] })
      setPool(id)
      toast.success(t('Pool created — now import accounts into it'))
    } else if (createStatus === 'error') {
      setCreatingId(null)
      toast.error(createError || t('Pool provisioning failed'))
    }
  }, [createStatus, createError, creatingId, queryClient, t])

  const deletePoolMutation = useMutation({
    mutationFn: (id: string) => deletePool(id),
    onSuccess: async (_data, id) => {
      setDeleteTarget(null)
      if (pool === id) setPool('default')
      await queryClient.invalidateQueries({ queryKey: ['pool', 'pools'] })
      toast.success(t('Pool deleted'))
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
  const stats = poolStats(files, verdicts)
  const verifyingName = verifyMutation.isPending
    ? verifyMutation.variables
    : null

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
      {/* xju-api:edit — pool tab nav + create action share the title row, so
          switching/creating pools reads as a top-level action. */}
      <SectionPageLayout.Actions>
        {pools.length > 1 && (
          <Tabs
            value={pool}
            onValueChange={(value) => {
              setPool(String(value))
              setImportResult(null)
            }}
          >
            <TabsList className='h-auto gap-1 p-1'>
              {pools.map((p) => (
                <TabsTrigger
                  key={p.id}
                  value={p.id}
                  className='px-3 py-1 text-sm font-medium'
                >
                  {p.label}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
        )}
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={() => setCreateOpen(true)}
        >
          <Plus className='size-4' />
          {t('New pool')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='grid gap-4 xl:grid-cols-[minmax(0,1fr)_22rem]'>
          {/* Accounts list */}
          <Card data-card-hover='false'>
            <CardHeader className='flex flex-row items-start justify-between gap-3 space-y-0'>
              <div className='min-w-0'>
                <CardTitle className='text-base'>
                  {t('Accounts in pool')}
                </CardTitle>
                <CardDescription>
                  {t('Upstream codex accounts behind the shared pool.')}
                </CardDescription>
              </div>
              {/* xju-api:edit — stats + actions live on the card's right,
                  balancing the title/description on the left. Stats are three
                  orthogonal dimensions: total / enabled (operator toggle) /
                  online (health), no longer conflated. */}
              <div className='flex shrink-0 flex-col items-end gap-2'>
                {files.length > 0 && (
                  <div className='flex flex-wrap items-center justify-end gap-x-3 gap-y-0.5 text-xs'>
                    <span className='text-muted-foreground'>
                      <span className='text-foreground font-semibold'>
                        {stats.total}
                      </span>{' '}
                      {t('Total')}
                    </span>
                    <span className='text-muted-foreground'>
                      <span className='text-foreground font-semibold'>
                        {stats.enabled}
                      </span>{' '}
                      {t('Enabled')}
                    </span>
                    <span className='text-success'>
                      <span className='font-semibold'>{stats.online}</span>{' '}
                      {t('Online')}
                    </span>
                  </div>
                )}
                <div className='flex items-center gap-2'>
                  {/* xju-api:new — delete a dynamically-created pool; the
                      env-seeded default/k12 pools cannot be removed here. */}
                  {isDynamicPool(pool) && (
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      className='text-destructive hover:text-destructive'
                      onClick={() =>
                        setDeleteTarget(
                          pools.find((p) => p.id === pool) ?? {
                            id: pool,
                            label: pool,
                          }
                        )
                      }
                    >
                      <Trash2 className='size-4' />
                      {t('Delete pool')}
                    </Button>
                  )}
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
                </div>
              </div>
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
                      const plan = file.id_token?.plan_type
                      const subUntil = subscriptionUntil(file)
                      const activity = recentActivity(file)
                      const cooldown = cooldownLabel(file)
                      const verdict = verdicts[file.name]
                      const verdictMeta = verdict
                        ? VERDICT_META[verdict.verdict]
                        : null
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
                              {plan && (
                                <Badge
                                  variant='outline'
                                  className='shrink-0 uppercase'
                                >
                                  {plan}
                                </Badge>
                              )}
                              {verdictMeta && (
                                <StatusBadge
                                  label={`✓ ${t(verdictMeta.labelKey)}`}
                                  variant={verdictMeta.variant}
                                  copyable={false}
                                />
                              )}
                            </div>
                            <p className='text-muted-foreground truncate font-mono text-xs'>
                              {file.name}
                            </p>
                            <div className='text-muted-foreground mt-0.5 flex flex-wrap items-center gap-x-2.5 gap-y-0.5 text-xs'>
                              {subUntil && (
                                <span
                                  className={
                                    state === 'expired' ? 'text-destructive' : ''
                                  }
                                >
                                  {state === 'expired'
                                    ? t('Expired {{date}}', {
                                        date: subUntil.toLocaleDateString(),
                                      })
                                    : t('Subscription until {{date}}', {
                                        date: subUntil.toLocaleDateString(),
                                      })}
                                </span>
                              )}
                              {activity && (
                                <span
                                  className={
                                    activity.rate < 100 ? 'text-warning' : ''
                                  }
                                >
                                  {t('{{rate}}% ok · {{total}} recent', {
                                    rate: activity.rate,
                                    total: activity.total,
                                  })}
                                </span>
                              )}
                              {cooldown !== null && (
                                <span className='text-warning'>
                                  {t('cooldown · retries in {{time}}', {
                                    time: cooldown,
                                  })}
                                </span>
                              )}
                              {file.status_message && (
                                <span
                                  className='truncate'
                                  title={file.status_message}
                                >
                                  {file.status_message}
                                </span>
                              )}
                            </div>
                          </div>
                          <div className='flex shrink-0 items-center gap-1'>
                            <Button
                              type='button'
                              variant='ghost'
                              size='icon-sm'
                              aria-label={t('Verify')}
                              title={t('Verify now')}
                              onClick={() => verifyMutation.mutate(file.name)}
                              disabled={verifyMutation.isPending}
                            >
                              {verifyingName === file.name ? (
                                <Loader2 className='size-4 animate-spin' />
                              ) : (
                                <Activity className='size-4' />
                              )}
                            </Button>
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

            {/* xju-api:new — active verification (号池验活 Part A) */}
            <Card data-card-hover='false'>
              <CardHeader>
                <CardTitle className='flex items-center gap-1.5 text-base'>
                  <ShieldCheck className='size-4' />
                  {t('Verify accounts')}
                </CardTitle>
                <CardDescription>
                  {t(
                    'Probe each account live to confirm it is actually online, instead of trusting the passive status.'
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent className='grid gap-3'>
                <div className='flex items-start justify-between gap-3'>
                  <div className='min-w-0'>
                    <span className='text-sm font-medium'>
                      {t('Deep probe')}
                    </span>
                    <p className='text-muted-foreground text-xs'>
                      {t(
                        'Also run a tiny inference to catch quota-exhausted accounts (uses a little quota).'
                      )}
                    </p>
                  </div>
                  <Switch
                    checked={heavyProbe}
                    onCheckedChange={setHeavyProbe}
                    disabled={jobRunning}
                  />
                </div>
                <div className='flex items-start justify-between gap-3'>
                  <div className='min-w-0'>
                    <span className='text-sm font-medium'>
                      {t('Auto-disable dead')}
                    </span>
                    <p className='text-muted-foreground text-xs'>
                      {t(
                        'Disable accounts found credential-dead or subscription-expired.'
                      )}
                    </p>
                  </div>
                  <Switch
                    checked={autoDisable}
                    onCheckedChange={setAutoDisable}
                    disabled={jobRunning}
                  />
                </div>
                <Button
                  type='button'
                  onClick={() => verifyAllMutation.mutate()}
                  disabled={verifyAllMutation.isPending || jobRunning}
                >
                  {jobRunning ? (
                    <Loader2 className='animate-spin' />
                  ) : (
                    <ShieldCheck />
                  )}
                  {t('Verify all')}
                </Button>
                {progress && (progress.running || progress.done > 0) && (
                  <div className='border-border rounded-md border p-2 text-xs'>
                    {progress.running ? (
                      <p>
                        {t('Verifying {{done}}/{{total}}...', {
                          done: progress.done,
                          total: progress.total,
                        })}
                      </p>
                    ) : (
                      <p className='font-medium'>
                        {t('Verified {{total}} · disabled {{disabled}}', {
                          total: progress.total,
                          disabled: progress.disabled,
                        })}
                      </p>
                    )}
                    <div className='text-muted-foreground mt-1 flex flex-wrap gap-x-2.5 gap-y-0.5'>
                      {verdictBreakdown(progress.results ?? []).map(
                        ([verdict, count]) => (
                          <span key={verdict}>
                            {t(VERDICT_META[verdict].labelKey)}: {count}
                          </span>
                        )
                      )}
                    </div>
                  </div>
                )}
              </CardContent>
            </Card>
          </div>
        </div>

        {/* xju-api:new — create pool dialog (#4 Phase D). Only a name is needed;
            everything else clones the existing pools. Provisioning runs on the
            host, so the dialog polls until the new pool is ready. */}
        <Dialog
          open={createOpen}
          onOpenChange={(o) => {
            if (!creatingId) {
              setCreateOpen(o)
              if (!o) setNewLabel('')
            }
          }}
          title={t('New pool')}
          description={t(
            'Spin up a new isolated account pool. Only a name is required — it gets its own upstream instance and routing channel, then you import accounts into it.'
          )}
          contentClassName='max-w-md'
          bodyClassName='space-y-3'
        >
          <div className='grid gap-1'>
            <label className='text-muted-foreground text-xs'>{t('Name')}</label>
            <Input
              autoFocus
              value={newLabel}
              placeholder={t('e.g. Campus, Trial, Team B')}
              disabled={!!creatingId}
              onChange={(e) => setNewLabel(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && newLabel.trim() && !creatingId) {
                  createMutation.mutate()
                }
              }}
            />
          </div>
          {creatingId && (
            <div className='text-muted-foreground flex items-center gap-2 text-sm'>
              <Loader2 className='size-4 animate-spin' />
              {t('Provisioning {{id}}...', { id: creatingId })}
            </div>
          )}
          <div className='flex justify-end gap-2'>
            <Button
              type='button'
              onClick={() => createMutation.mutate()}
              disabled={
                !newLabel.trim() || createMutation.isPending || !!creatingId
              }
            >
              {createMutation.isPending || creatingId ? (
                <Loader2 className='animate-spin' />
              ) : (
                <Plus />
              )}
              {t('Create')}
            </Button>
          </div>
        </Dialog>

        {/* xju-api:new — delete pool confirm (#4 Phase D). */}
        <AlertDialog
          open={!!deleteTarget}
          onOpenChange={(o) => {
            if (!o) setDeleteTarget(null)
          }}
        >
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>
                {t('Delete pool "{{label}}"?', {
                  label: deleteTarget?.label ?? '',
                })}
              </AlertDialogTitle>
              <AlertDialogDescription>
                {t(
                  'This stops and removes the pool’s upstream instance and its routing channel. The account files are kept on the server. This cannot be undone from here.'
                )}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel disabled={deletePoolMutation.isPending}>
                {t('Cancel')}
              </AlertDialogCancel>
              <AlertDialogAction
                className='bg-destructive text-white hover:bg-destructive/90'
                disabled={deletePoolMutation.isPending}
                onClick={(e) => {
                  e.preventDefault()
                  if (deleteTarget) deletePoolMutation.mutate(deleteTarget.id)
                }}
              >
                {deletePoolMutation.isPending && (
                  <Loader2 className='animate-spin' />
                )}
                {t('Delete pool')}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

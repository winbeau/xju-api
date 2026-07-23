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
import { Link } from '@tanstack/react-router'
import {
  Activity,
  ArrowRight,
  CheckCircle2,
  CircleAlert,
  ClipboardPaste,
  ExternalLink,
  FileArchive,
  Gauge,
  KeyRound,
  Loader2,
  LogIn,
  Play,
  Plus,
  Power,
  RefreshCw,
  RotateCcw,
  ShieldCheck,
  Sparkles,
  Trash2,
  Upload,
  Users,
} from 'lucide-react'
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ChangeEvent,
} from 'react'
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
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import type {
  ImportResult,
  PoolAuthFile,
  ProbeResult,
} from '@/features/pool/api'
import {
  accountState,
  cooldownLabel,
  poolStats,
  quotaPercentClass,
  recentActivity,
  STATE_META,
  subscriptionUntil,
  VERDICT_META,
  verdictBreakdown,
} from '@/features/pool/workbench-utils'
import {
  addPrivatePoolAccounts,
  cancelPrivatePoolCodexLogin,
  cleanPrivatePoolAccounts,
  createPrivatePool,
  deletePrivatePoolAccount,
  getPrivatePool,
  getPrivatePoolCodexLoginStatus,
  getPrivatePoolSettings,
  getPrivatePoolUsage,
  getPrivatePoolVerifyProgress,
  importPrivatePoolAccounts,
  listPrivatePoolAccounts,
  patchPrivatePoolSettings,
  refreshAllPrivatePoolAccounts,
  refreshPrivatePoolAccount,
  resetPrivatePoolAccountQuota,
  setPrivatePoolAccountDisabled,
  startPrivatePoolCodexLogin,
  startPrivatePoolVerifyAll,
  submitPrivatePoolCodexCallback,
  verifyPrivatePoolAccount,
  type PrivatePoolOAuthSession,
} from '@/features/private-pool/api'

const STEPS = [
  {
    icon: CheckCircle2,
    title: 'Create pool',
    description: 'Provision your isolated account pool.',
  },
  {
    icon: Upload,
    title: 'Add accounts',
    description: 'Login, upload, paste JSON, or import a ZIP.',
  },
  {
    icon: ShieldCheck,
    title: 'Verify accounts',
    description: 'Confirm at least one account is online.',
  },
  {
    icon: KeyRound,
    title: 'Create API key',
    description: 'New keys route only to your private pool.',
  },
]

export function PrivatePool() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const zipInputRef = useRef<HTMLInputElement>(null)
  const [content, setContent] = useState('')
  const [importResult, setImportResult] = useState<ImportResult | null>(null)
  const [verdicts, setVerdicts] = useState<Record<string, ProbeResult>>({})
  const [heavyProbe, setHeavyProbe] = useState(false)
  const [autoDisable, setAutoDisable] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<PoolAuthFile | null>(null)
  const [resetTarget, setResetTarget] = useState<PoolAuthFile | null>(null)
  const [oauthOpen, setOAuthOpen] = useState(false)
  const [oauthSession, setOAuthSession] =
    useState<PrivatePoolOAuthSession | null>(null)
  const [oauthCallbackURL, setOAuthCallbackURL] = useState('')
  const [oauthCompleted, setOAuthCompleted] = useState(false)
  const oauthOpenRef = useRef(false)

  const stateQuery = useQuery({
    queryKey: ['private-pool'],
    queryFn: getPrivatePool,
    refetchInterval: (query) =>
      query.state.data?.status === 'provisioning' ? 2000 : false,
  })
  const state = stateQuery.data
  const ready = state?.status === 'ready'

  const accountsQuery = useQuery({
    queryKey: ['private-pool', 'accounts'],
    queryFn: listPrivatePoolAccounts,
    enabled: ready,
    staleTime: 10_000,
  })
  const usageQuery = useQuery({
    queryKey: ['private-pool', 'usage'],
    queryFn: getPrivatePoolUsage,
    enabled: ready,
    refetchInterval: (query) => (query.state.data?.job?.running ? 2500 : false),
  })
  const settingsQuery = useQuery({
    queryKey: ['private-pool', 'settings'],
    queryFn: getPrivatePoolSettings,
    enabled: ready,
  })
  const progressQuery = useQuery({
    queryKey: ['private-pool', 'verify'],
    queryFn: getPrivatePoolVerifyProgress,
    enabled: ready,
    refetchInterval: (query) => (query.state.data?.running ? 2000 : false),
  })
  const oauthStatusQuery = useQuery({
    queryKey: ['private-pool', 'oauth', oauthSession?.session_id],
    queryFn: () => {
      if (!oauthSession) throw new Error(t('Login session expired'))
      return getPrivatePoolCodexLoginStatus(oauthSession.session_id)
    },
    enabled: Boolean(oauthSession && !oauthCompleted),
    refetchInterval: (query) => {
      const status = query.state.data?.status
      return status === 'ok' || status === 'error' ? false : 2000
    },
    retry: false,
  })

  const accounts = accountsQuery.data ?? []
  const usage = usageQuery.data?.accounts ?? {}
  const usageJob = usageQuery.data?.job ?? null
  const settings = settingsQuery.data
  const progress = progressQuery.data ?? null
  const stats = poolStats(accounts, verdicts)

  const invalidateAccounts = useCallback(async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['private-pool', 'accounts'] }),
      queryClient.invalidateQueries({ queryKey: ['private-pool', 'usage'] }),
    ])
  }, [queryClient])

  const createMutation = useMutation({
    mutationFn: createPrivatePool,
    onSuccess: async () => {
      toast.success(t('Private pool provisioning started'))
      await queryClient.invalidateQueries({ queryKey: ['private-pool'] })
    },
    onError: (error: Error) => toast.error(error.message),
  })
  const addMutation = useMutation({
    mutationFn: async () => {
      const value = content.trim()
      if (!value) throw new Error(t('Paste an auth JSON first'))
      JSON.parse(value)
      return addPrivatePoolAccounts(value)
    },
    onSuccess: async (result) => {
      setContent('')
      setImportResult(result)
      toast.success(
        t('Imported {{imported}} · skipped {{skipped}} · failed {{failed}}', {
          imported: result.imported,
          skipped: result.skipped.length,
          failed: result.failed.length,
        })
      )
      await invalidateAccounts()
    },
    onError: (error: Error) => toast.error(error.message),
  })
  const importMutation = useMutation({
    mutationFn: importPrivatePoolAccounts,
    onSuccess: async (result) => {
      setImportResult(result)
      toast.success(
        t('Imported {{imported}} · skipped {{skipped}} · failed {{failed}}', {
          imported: result.imported,
          skipped: result.skipped.length,
          failed: result.failed.length,
        })
      )
      await invalidateAccounts()
    },
    onError: (error: Error) => toast.error(error.message),
  })
  const toggleMutation = useMutation({
    mutationFn: ({ name, disabled }: { name: string; disabled: boolean }) =>
      setPrivatePoolAccountDisabled(name, disabled),
    onSuccess: invalidateAccounts,
    onError: (error: Error) => toast.error(error.message),
  })
  const verifyMutation = useMutation({
    mutationFn: (name: string) => verifyPrivatePoolAccount(name, heavyProbe),
    onSuccess: (result) =>
      setVerdicts((current) => ({ ...current, [result.name]: result })),
    onError: (error: Error) => toast.error(error.message),
  })
  const verifyAllMutation = useMutation({
    mutationFn: () =>
      startPrivatePoolVerifyAll({ heavy: heavyProbe, autoDisable }),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ['private-pool', 'verify'] }),
    onError: (error: Error) => toast.error(error.message),
  })
  const refreshMutation = useMutation({
    mutationFn: refreshPrivatePoolAccount,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ['private-pool', 'usage'] }),
    onError: (error: Error) => toast.error(error.message),
  })
  const refreshAllMutation = useMutation({
    mutationFn: refreshAllPrivatePoolAccounts,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ['private-pool', 'usage'] }),
    onError: (error: Error) => toast.error(error.message),
  })
  const deleteMutation = useMutation({
    mutationFn: deletePrivatePoolAccount,
    onSuccess: async () => {
      setDeleteTarget(null)
      toast.success(t('Account removed'))
      await invalidateAccounts()
    },
    onError: (error: Error) => toast.error(error.message),
  })
  const resetMutation = useMutation({
    mutationFn: resetPrivatePoolAccountQuota,
    onSuccess: async () => {
      setResetTarget(null)
      toast.success(t('Quota reset — usage windows renewed'))
      await queryClient.invalidateQueries({
        queryKey: ['private-pool', 'usage'],
      })
    },
    onError: (error: Error) => toast.error(error.message),
  })
  const cleanMutation = useMutation({
    mutationFn: cleanPrivatePoolAccounts,
    onSuccess: async (disabled) => {
      toast.success(
        disabled > 0
          ? t('Disabled {{count}} stale account(s)', { count: disabled })
          : t('Nothing to clean — all accounts are healthy')
      )
      await invalidateAccounts()
    },
    onError: (error: Error) => toast.error(error.message),
  })
  const settingsMutation = useMutation({
    mutationFn: patchPrivatePoolSettings,
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: ['private-pool', 'settings'],
      })
      toast.success(t('Saved successfully'))
    },
    onError: (error: Error) => toast.error(error.message),
  })
  const oauthStartMutation = useMutation({
    mutationFn: startPrivatePoolCodexLogin,
    onSuccess: (session) => {
      if (!oauthOpenRef.current) {
        void cancelPrivatePoolCodexLogin(session.session_id)
        return
      }
      setOAuthSession(session)
    },
    onError: (error: Error) => toast.error(error.message),
  })
  const oauthCallbackMutation = useMutation({
    mutationFn: () => {
      if (!oauthSession) throw new Error(t('Login session expired'))
      return submitPrivatePoolCodexCallback(
        oauthSession.session_id,
        oauthCallbackURL.trim()
      )
    },
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: ['private-pool', 'oauth', oauthSession?.session_id],
      }),
    onError: (error: Error) => toast.error(error.message),
  })

  const jobRunning = progress?.running ?? false
  const prevRunning = useRef(false)
  useEffect(() => {
    if (!progress?.results?.length) return
    setVerdicts((current) => {
      const next = { ...current }
      for (const result of progress.results) next[result.name] = result
      return next
    })
  }, [progress])
  useEffect(() => {
    if (prevRunning.current && !jobRunning) void invalidateAccounts()
    prevRunning.current = jobRunning
  }, [invalidateAccounts, jobRunning])
  useEffect(() => {
    const result = oauthStatusQuery.data
    if (!result || oauthCompleted) return
    if (result.status === 'ok') {
      setOAuthCompleted(true)
      toast.success(t('Codex account added to your private pool'))
      void invalidateAccounts()
    } else if (result.status === 'error') {
      setOAuthCompleted(true)
      toast.error(result.error || t('Authentication failed'))
    }
  }, [invalidateAccounts, oauthCompleted, oauthStatusQuery.data, t])

  const handleFileUpload = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    try {
      setContent(await file.text())
    } catch {
      toast.error(t('Could not read that file'))
    }
  }
  const handleZipImport = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    setImportResult(null)
    importMutation.mutate(file)
  }
  const handlePaste = async () => {
    try {
      const text = await navigator.clipboard.readText()
      if (text.trim()) setContent(text)
    } catch {
      toast.error(t('Clipboard not available — paste manually'))
    }
  }
  const openOAuthDialog = () => {
    oauthOpenRef.current = true
    setOAuthOpen(true)
    setOAuthSession(null)
    setOAuthCallbackURL('')
    setOAuthCompleted(false)
    oauthStartMutation.mutate()
  }
  const closeOAuthDialog = () => {
    oauthOpenRef.current = false
    if (oauthSession && !oauthCompleted) {
      void cancelPrivatePoolCodexLogin(oauthSession.session_id)
    }
    setOAuthOpen(false)
    setOAuthSession(null)
    setOAuthCallbackURL('')
    setOAuthCompleted(false)
  }

  let completedSteps = 0
  if (ready) {
    completedSteps = accounts.length === 0 ? 1 : 2
    if (stats.online > 0 || Object.keys(verdicts).length > 0) completedSteps = 3
  }

  let oauthStatusMessage = t('Waiting for the localhost callback URL...')
  if (oauthCallbackMutation.isSuccess) {
    oauthStatusMessage = t('Exchanging the authorization code...')
  }
  if (oauthCompleted) {
    oauthStatusMessage =
      oauthStatusQuery.data?.status === 'error'
        ? oauthStatusQuery.data.error || t('Authentication failed')
        : t('Account added. The account list has been refreshed.')
  }
  const oauthPollError = oauthStatusQuery.isError
    ? oauthStatusQuery.error.message
    : ''
  if (oauthPollError) oauthStatusMessage = oauthPollError
  const oauthFailed =
    (oauthCompleted && oauthStatusQuery.data?.status === 'error') ||
    Boolean(oauthPollError)
  let oauthStatusIcon = <Loader2 className='size-4 animate-spin' />
  if (oauthCompleted || oauthPollError) {
    oauthStatusIcon = oauthFailed ? (
      <CircleAlert className='text-destructive size-4' />
    ) : (
      <CheckCircle2 className='text-success size-4' />
    )
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('My Pool')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto flex w-full max-w-[96rem] flex-col gap-5 pb-8'>
          <div className='grid gap-3 md:grid-cols-4'>
            {STEPS.map((step, index) => {
              const Icon = step.icon
              const complete = index < completedSteps
              return (
                <Card
                  key={step.title}
                  className={complete ? 'border-success/40' : ''}
                >
                  <CardContent className='flex gap-3 p-4'>
                    <div className='bg-muted flex size-9 shrink-0 items-center justify-center rounded-lg'>
                      {complete ? (
                        <CheckCircle2 className='text-success size-5' />
                      ) : (
                        <Icon className='size-5' />
                      )}
                    </div>
                    <div className='min-w-0'>
                      <p className='text-sm font-medium'>
                        {index + 1}. {t(step.title)}
                      </p>
                      <p className='text-muted-foreground mt-1 text-xs'>
                        {t(step.description)}
                      </p>
                    </div>
                  </CardContent>
                </Card>
              )
            })}
          </div>

          {stateQuery.isLoading && (
            <Card>
              <CardContent className='flex items-center gap-3 p-6'>
                <Loader2 className='size-5 animate-spin' />
                {t('Loading...')}
              </CardContent>
            </Card>
          )}

          {state?.status === 'none' && (
            <Card>
              <CardHeader>
                <CardTitle>{t('Create your private account pool')}</CardTitle>
                <CardDescription>
                  {t(
                    'Your upstream accounts, routing channel, and API keys will be isolated from every other user.'
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <Button
                  onClick={() => createMutation.mutate()}
                  disabled={
                    createMutation.isPending || !state.provision_enabled
                  }
                >
                  {createMutation.isPending ? (
                    <Loader2 className='size-4 animate-spin' />
                  ) : (
                    <Plus className='size-4' />
                  )}
                  {t('Create pool')}
                </Button>
              </CardContent>
            </Card>
          )}

          {state?.status === 'provisioning' && (
            <Card>
              <CardContent className='flex items-start gap-4 p-6'>
                <Loader2 className='text-primary mt-0.5 size-6 animate-spin' />
                <div>
                  <p className='font-medium'>
                    {t('Creating your isolated pool')}
                  </p>
                  <p className='text-muted-foreground mt-1 text-sm'>
                    {t(
                      'The container, management channel, and private routing group are being prepared. This page refreshes automatically.'
                    )}
                  </p>
                </div>
              </CardContent>
            </Card>
          )}

          {state?.status === 'error' && (
            <Card className='border-destructive/40'>
              <CardHeader>
                <CardTitle>{t('Pool creation failed')}</CardTitle>
                <CardDescription>
                  {state.error ||
                    t('Please retry or contact the administrator.')}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <Button
                  onClick={() => createMutation.mutate()}
                  disabled={createMutation.isPending}
                >
                  {t('Retry')}
                </Button>
              </CardContent>
            </Card>
          )}

          {ready && (
            <>
              <Card>
                <CardContent className='flex flex-col gap-4 p-5 sm:flex-row sm:items-center sm:justify-between'>
                  <div>
                    <div className='flex items-center gap-2'>
                      <CardTitle className='text-lg'>
                        {state?.pool?.label || state?.label || t('My Pool')}
                      </CardTitle>
                      <StatusBadge
                        label={t('Ready')}
                        variant='success'
                        copyable={false}
                      />
                    </div>
                    <p className='text-muted-foreground mt-1 text-sm'>
                      {t(
                        'API Keys you create are locked to this pool and never fall back to another user or the shared pool.'
                      )}
                    </p>
                  </div>
                  <Button
                    variant='outline'
                    render={<Link to='/keys' search={{ create: true }} />}
                  >
                    <KeyRound className='size-4' />
                    {t('Create API Key')}
                    <ArrowRight className='size-4' />
                  </Button>
                </CardContent>
              </Card>

              <div className='grid gap-4 xl:grid-cols-[minmax(0,1fr)_22rem]'>
                <Card data-card-hover='false'>
                  <CardHeader className='flex flex-row items-start justify-between gap-3 space-y-0'>
                    <div className='min-w-0'>
                      <CardTitle className='text-base'>
                        {t('Accounts in pool')}
                      </CardTitle>
                      <CardDescription>
                        {t('Codex accounts available only to your API keys.')}
                      </CardDescription>
                    </div>
                    <div className='flex shrink-0 flex-col items-end gap-2'>
                      {accounts.length > 0 && (
                        <div className='flex flex-wrap items-center justify-end gap-x-3 text-xs'>
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
                            <span className='font-semibold'>
                              {stats.online}
                            </span>{' '}
                            {t('Online')}
                          </span>
                        </div>
                      )}
                      <Button
                        type='button'
                        variant='outline'
                        size='sm'
                        onClick={() => void invalidateAccounts()}
                        disabled={accountsQuery.isFetching}
                      >
                        <RefreshCw
                          className={
                            accountsQuery.isFetching ? 'animate-spin' : ''
                          }
                        />
                        {t('Refresh')}
                      </Button>
                    </div>
                  </CardHeader>
                  <CardContent>
                    <div className='border-border overflow-hidden rounded-md border'>
                      {accountsQuery.isLoading && (
                        <div className='text-muted-foreground flex items-center gap-2 p-4 text-sm'>
                          <Loader2 className='size-4 animate-spin' />
                          {t('Loading...')}
                        </div>
                      )}
                      {accountsQuery.isError && (
                        <div className='text-destructive p-4 text-sm'>
                          {(accountsQuery.error as Error).message}
                        </div>
                      )}
                      {!accountsQuery.isLoading && accounts.length === 0 && (
                        <div className='text-muted-foreground p-8 text-center text-sm'>
                          <Users className='mx-auto mb-3 size-8' />
                          {t('No accounts yet.')}
                        </div>
                      )}
                      {accounts.length > 0 && (
                        <ul className='divide-border divide-y'>
                          {accounts.map((file) => {
                            const accountStatus = accountState(file)
                            const meta = STATE_META[accountStatus]
                            const label =
                              file.email || file.account || file.name
                            const plan = file.id_token?.plan_type
                            const subUntil = subscriptionUntil(file)
                            const activity = recentActivity(file)
                            const cooldown = cooldownLabel(file)
                            const accountUsage = usage[file.name]
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
                                  <div className='flex flex-wrap items-center gap-2'>
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
                                          accountStatus === 'expired'
                                            ? 'text-destructive'
                                            : ''
                                        }
                                      >
                                        {accountStatus === 'expired'
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
                                          activity.rate < 100
                                            ? 'text-warning'
                                            : ''
                                        }
                                      >
                                        {t('{{rate}}% ok · {{total}} recent', {
                                          rate: activity.rate,
                                          total: activity.total,
                                        })}
                                      </span>
                                    )}
                                    {cooldown && (
                                      <span className='text-warning'>
                                        {t('cooldown · retries in {{time}}', {
                                          time: cooldown,
                                        })}
                                      </span>
                                    )}
                                    {accountUsage && !accountUsage.error && (
                                      <>
                                        {accountUsage.five_hour_used_percent !=
                                          null && (
                                          <span
                                            className={quotaPercentClass(
                                              accountUsage.five_hour_used_percent
                                            )}
                                          >
                                            {t('5h {{percent}}%', {
                                              percent: Math.round(
                                                accountUsage.five_hour_used_percent
                                              ),
                                            })}
                                          </span>
                                        )}
                                        {accountUsage.weekly_used_percent !=
                                          null && (
                                          <span
                                            className={quotaPercentClass(
                                              accountUsage.weekly_used_percent
                                            )}
                                          >
                                            {t('Wk {{percent}}%', {
                                              percent: Math.round(
                                                accountUsage.weekly_used_percent
                                              ),
                                            })}
                                          </span>
                                        )}
                                        {accountUsage.limit_reached && (
                                          <span className='text-destructive'>
                                            {t('Quota exhausted')}
                                          </span>
                                        )}
                                        {(accountUsage.reset_credits ?? 0) >
                                          0 && (
                                          <span className='text-info'>
                                            {t('Reset credits: {{count}}', {
                                              count: accountUsage.reset_credits,
                                            })}
                                          </span>
                                        )}
                                      </>
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
                                    title={t('Verify now')}
                                    onClick={() =>
                                      verifyMutation.mutate(file.name)
                                    }
                                    disabled={verifyMutation.isPending}
                                  >
                                    {verifyMutation.isPending &&
                                    verifyMutation.variables === file.name ? (
                                      <Loader2 className='size-4 animate-spin' />
                                    ) : (
                                      <Activity className='size-4' />
                                    )}
                                  </Button>
                                  <Button
                                    type='button'
                                    variant='ghost'
                                    size='icon-sm'
                                    title={t('Refresh quota')}
                                    onClick={() =>
                                      refreshMutation.mutate(file.name)
                                    }
                                    disabled={refreshMutation.isPending}
                                  >
                                    {refreshMutation.isPending &&
                                    refreshMutation.variables === file.name ? (
                                      <Loader2 className='size-4 animate-spin' />
                                    ) : (
                                      <Gauge className='size-4' />
                                    )}
                                  </Button>
                                  {(accountUsage?.reset_credits ?? 0) > 0 && (
                                    <Button
                                      type='button'
                                      variant='ghost'
                                      size='icon-sm'
                                      title={t('Reset quota')}
                                      onClick={() => setResetTarget(file)}
                                    >
                                      <RotateCcw className='size-4' />
                                    </Button>
                                  )}
                                  <Button
                                    type='button'
                                    variant='ghost'
                                    size='icon-sm'
                                    title={
                                      file.disabled ? t('Enable') : t('Disable')
                                    }
                                    className={
                                      file.disabled ? 'text-success' : ''
                                    }
                                    onClick={() =>
                                      toggleMutation.mutate({
                                        name: file.name,
                                        disabled: !file.disabled,
                                      })
                                    }
                                  >
                                    <Power className='size-4' />
                                  </Button>
                                  <Button
                                    type='button'
                                    variant='ghost'
                                    size='icon-sm'
                                    className='text-destructive hover:text-destructive'
                                    title={t('Remove')}
                                    onClick={() => setDeleteTarget(file)}
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

                <div className='grid content-start gap-4'>
                  <Card data-card-hover='false'>
                    <CardHeader>
                      <CardTitle className='text-base'>
                        {t('Add account')}
                      </CardTitle>
                      <CardDescription>
                        {t(
                          'Login with OpenAI, import a ZIP, upload JSON, or paste credentials. Up to 20 accounts.'
                        )}
                      </CardDescription>
                    </CardHeader>
                    <CardContent className='grid gap-2'>
                      <input
                        ref={zipInputRef}
                        type='file'
                        accept='.zip,application/zip'
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
                      <div className='flex flex-wrap justify-end gap-2'>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          onClick={openOAuthDialog}
                          disabled={accounts.length >= 20}
                        >
                          <LogIn />
                          {t('Login')}
                        </Button>
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
                          {importResult.failed.map((failure) => (
                            <p
                              key={failure.name}
                              className='text-destructive truncate font-mono'
                            >
                              {failure.name}: {failure.error}
                            </p>
                          ))}
                          {importResult.skipped.map((skipped) => (
                            <p
                              key={skipped.name}
                              className='text-muted-foreground truncate font-mono'
                            >
                              {skipped.name}: {skipped.reason}
                            </p>
                          ))}
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
                          { hours: settings?.auto_clean_hours ?? 24 }
                        )}
                      </CardDescription>
                    </CardHeader>
                    <CardContent className='grid gap-3'>
                      <div className='flex items-center justify-between'>
                        <span className='text-sm font-medium'>
                          {t('Enable auto-clean')}
                        </span>
                        <Switch
                          checked={settings?.auto_clean_enabled ?? false}
                          disabled={!settings || settingsMutation.isPending}
                          onCheckedChange={(value) =>
                            settingsMutation.mutate({
                              auto_clean_enabled: value,
                            })
                          }
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

                  <Card data-card-hover='false'>
                    <CardHeader>
                      <CardTitle className='flex items-center gap-1.5 text-base'>
                        <Gauge className='size-4' />
                        {t('Account quota')}
                      </CardTitle>
                      <CardDescription>
                        {t(
                          'Per-account ChatGPT usage windows (5h / weekly) and reset credits.'
                        )}
                      </CardDescription>
                    </CardHeader>
                    <CardContent className='grid gap-3'>
                      <div className='flex items-start justify-between gap-3'>
                        <div>
                          <span className='text-sm font-medium'>
                            {t('Auto refresh hourly')}
                          </span>
                          <p className='text-muted-foreground text-xs'>
                            {t(
                              'Fetch every account’s quota in the background, once an hour.'
                            )}
                          </p>
                        </div>
                        <Switch
                          checked={
                            settings?.usage_auto_refresh_enabled ?? false
                          }
                          disabled={!settings || settingsMutation.isPending}
                          onCheckedChange={(value) =>
                            settingsMutation.mutate({
                              usage_auto_refresh_enabled: value,
                            })
                          }
                        />
                      </div>
                      <div className='flex items-start justify-between gap-3'>
                        <div>
                          <span className='text-sm font-medium'>
                            {t('Auto reset when exhausted')}
                          </span>
                          <p className='text-muted-foreground text-xs'>
                            {t(
                              'Spend one reset credit automatically when an account runs out of quota.'
                            )}
                          </p>
                        </div>
                        <Switch
                          checked={settings?.usage_auto_reset_enabled ?? false}
                          disabled={!settings || settingsMutation.isPending}
                          onCheckedChange={(value) =>
                            settingsMutation.mutate({
                              usage_auto_reset_enabled: value,
                            })
                          }
                        />
                      </div>
                      <Button
                        type='button'
                        onClick={() => refreshAllMutation.mutate()}
                        disabled={
                          refreshAllMutation.isPending ||
                          Boolean(usageJob?.running)
                        }
                      >
                        {usageJob?.running ? (
                          <Loader2 className='animate-spin' />
                        ) : (
                          <Gauge />
                        )}
                        {t('Refresh all quota')}
                      </Button>
                      {usageJob && (usageJob.running || usageJob.done > 0) && (
                        <div className='border-border rounded-md border p-2 text-xs'>
                          {usageJob.running
                            ? t('Refreshing quota {{done}}/{{total}}...', {
                                done: usageJob.done,
                                total: usageJob.total,
                              })
                            : t(
                                'Quota refreshed {{total}} · skipped {{skipped}} · auto-reset {{resets}} · failed {{errors}}',
                                {
                                  total: usageJob.total,
                                  skipped: usageJob.skipped,
                                  resets: usageJob.resets,
                                  errors: usageJob.errors,
                                }
                              )}
                        </div>
                      )}
                    </CardContent>
                  </Card>

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
                        <div>
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
                        <div>
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
                          <p className='font-medium'>
                            {progress.running
                              ? t('Verifying {{done}}/{{total}}...', {
                                  done: progress.done,
                                  total: progress.total,
                                })
                              : t(
                                  'Verified {{total}} · disabled {{disabled}}',
                                  {
                                    total: progress.total,
                                    disabled: progress.disabled,
                                  }
                                )}
                          </p>
                          <div className='text-muted-foreground mt-1 flex flex-wrap gap-x-2.5'>
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
            </>
          )}
        </div>

        <Dialog
          open={oauthOpen}
          onOpenChange={(open) => {
            if (!open) closeOAuthDialog()
          }}
          title={t('Login with OpenAI')}
          description={t(
            'Credentials and MFA stay on OpenAI. This page only receives the one-time localhost callback.'
          )}
          contentClassName='max-w-xl'
          bodyClassName='space-y-4'
        >
          {oauthStartMutation.isPending && (
            <div className='text-muted-foreground flex items-center gap-2 text-sm'>
              <Loader2 className='size-4 animate-spin' />
              {t('Preparing a secure login session...')}
            </div>
          )}
          {oauthStartMutation.isError && !oauthSession && (
            <div className='flex justify-end'>
              <Button type='button' onClick={closeOAuthDialog}>
                {t('Close')}
              </Button>
            </div>
          )}
          {oauthSession && (
            <>
              <div className='border-border rounded-md border p-3'>
                <p className='text-sm font-medium'>
                  1. {t('Open the OpenAI login page')}
                </p>
                <p className='text-muted-foreground mt-1 text-xs'>
                  {t(
                    'Complete login in the new tab. It will automatically redirect to localhost:1455.'
                  )}
                </p>
                <Button
                  className='mt-3'
                  type='button'
                  variant='outline'
                  onClick={() =>
                    window.open(
                      oauthSession.url,
                      '_blank',
                      'noopener,noreferrer'
                    )
                  }
                >
                  <ExternalLink />
                  {t('Open OpenAI login')}
                </Button>
              </div>
              <div className='border-border rounded-md border p-3'>
                <p className='text-sm font-medium'>
                  2. {t('Copy the localhost callback URL')}
                </p>
                <p className='text-muted-foreground mt-1 text-xs'>
                  {t(
                    'A “site cannot be reached” page is expected. Press Ctrl+L, Ctrl+C and paste the complete address below.'
                  )}
                </p>
                <Textarea
                  className='mt-3 min-h-24 font-mono text-xs'
                  value={oauthCallbackURL}
                  onChange={(event) => setOAuthCallbackURL(event.target.value)}
                  placeholder='http://localhost:1455/auth/callback?code=...&state=...'
                  spellCheck={false}
                />
                <div className='mt-2 flex flex-wrap gap-2'>
                  <Button
                    type='button'
                    variant='outline'
                    onClick={async () => {
                      try {
                        setOAuthCallbackURL(
                          await navigator.clipboard.readText()
                        )
                      } catch {
                        toast.error(
                          t('Clipboard not available — paste manually')
                        )
                      }
                    }}
                  >
                    <ClipboardPaste />
                    {t('Paste')}
                  </Button>
                  <Button
                    type='button'
                    onClick={() => oauthCallbackMutation.mutate()}
                    disabled={
                      !oauthCallbackURL.trim() ||
                      oauthCallbackMutation.isPending ||
                      oauthCompleted
                    }
                  >
                    {oauthCallbackMutation.isPending ? (
                      <Loader2 className='animate-spin' />
                    ) : (
                      <LogIn />
                    )}
                    {t('Complete login')}
                  </Button>
                </div>
              </div>
              <div className='border-border rounded-md border p-3 text-sm'>
                <p className='font-medium'>3. {t('Import status')}</p>
                <div className='text-muted-foreground mt-2 flex items-center gap-2'>
                  {oauthStatusIcon}
                  {oauthStatusMessage}
                </div>
              </div>
              <div className='flex justify-end'>
                <Button type='button' onClick={closeOAuthDialog}>
                  {oauthCompleted ? t('Done') : t('Cancel')}
                </Button>
              </div>
            </>
          )}
        </Dialog>

        <AlertDialog
          open={Boolean(deleteTarget)}
          onOpenChange={(open) => !open && setDeleteTarget(null)}
        >
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>{t('Remove this account?')}</AlertDialogTitle>
              <AlertDialogDescription>
                {deleteTarget?.email || deleteTarget?.name}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
              <AlertDialogAction
                className='bg-destructive hover:bg-destructive/90 text-white'
                onClick={(event) => {
                  event.preventDefault()
                  if (deleteTarget) deleteMutation.mutate(deleteTarget.name)
                }}
              >
                {deleteMutation.isPending && (
                  <Loader2 className='size-4 animate-spin' />
                )}
                {t('Remove')}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>

        <AlertDialog
          open={Boolean(resetTarget)}
          onOpenChange={(open) => !open && setResetTarget(null)}
        >
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>
                {t('Reset quota for "{{label}}"?', {
                  label: resetTarget?.email || resetTarget?.name || '',
                })}
              </AlertDialogTitle>
              <AlertDialogDescription>
                {t(
                  'This consumes one of the account’s reset credits to renew its usage windows. A used credit cannot be restored.'
                )}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
              <AlertDialogAction
                onClick={(event) => {
                  event.preventDefault()
                  if (resetTarget) resetMutation.mutate(resetTarget.name)
                }}
              >
                {resetMutation.isPending && (
                  <Loader2 className='size-4 animate-spin' />
                )}
                {t('Reset quota')}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

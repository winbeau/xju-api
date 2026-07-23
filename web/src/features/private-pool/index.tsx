/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import {
  ArrowRight,
  CheckCircle2,
  FileArchive,
  Gauge,
  KeyRound,
  Loader2,
  Plus,
  Power,
  RefreshCw,
  RotateCcw,
  ShieldCheck,
  Trash2,
  Upload,
  Users,
} from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

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
import { Textarea } from '@/components/ui/textarea'
import type { PoolAuthFile, ProbeResult } from '@/features/pool/api'

import {
  addPrivatePoolAccounts,
  createPrivatePool,
  deletePrivatePoolAccount,
  getPrivatePool,
  getPrivatePoolUsage,
  importPrivatePoolAccounts,
  listPrivatePoolAccounts,
  refreshAllPrivatePoolAccounts,
  refreshPrivatePoolAccount,
  resetPrivatePoolAccountQuota,
  setPrivatePoolAccountDisabled,
  verifyPrivatePoolAccount,
} from './api'

const STEPS = [
  {
    icon: Plus,
    title: 'Create pool',
    description: 'Provision your isolated account pool.',
  },
  {
    icon: Upload,
    title: 'Add accounts',
    description: 'Paste auth JSON or import a ZIP.',
  },
  {
    icon: ShieldCheck,
    title: 'Verify accounts',
    description: 'Confirm at least one account is online.',
  },
  {
    icon: KeyRound,
    title: 'Create API Key',
    description: 'New keys route only to your pool.',
  },
]

function accountStatus(file: PoolAuthFile): {
  label: string
  variant: 'success' | 'neutral' | 'warning'
} {
  if (file.disabled) return { label: 'Disabled', variant: 'neutral' }
  if (file.unavailable) return { label: 'Unavailable', variant: 'warning' }
  return { label: 'Active', variant: 'success' }
}

function percent(value?: number): string {
  return typeof value === 'number' ? `${Math.round(value)}%` : '—'
}

export function PrivatePool() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [content, setContent] = useState('')
  const [verdicts, setVerdicts] = useState<Record<string, ProbeResult>>({})
  const [deleteTarget, setDeleteTarget] = useState<PoolAuthFile | null>(null)
  const [resetTarget, setResetTarget] = useState<PoolAuthFile | null>(null)

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
  const accounts = accountsQuery.data ?? []
  const usage = usageQuery.data?.accounts ?? {}

  const invalidateAccounts = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['private-pool', 'accounts'] }),
      queryClient.invalidateQueries({ queryKey: ['private-pool', 'usage'] }),
    ])
  }

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
      toast.success(
        t('Imported {{imported}} · skipped {{skipped}} · failed {{failed}}', {
          imported: result.imported,
          skipped: result.skipped.length,
          failed: result.failed.length,
        })
      )
      if (fileInputRef.current) fileInputRef.current.value = ''
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
    mutationFn: verifyPrivatePoolAccount,
    onSuccess: (result) =>
      setVerdicts((current) => ({ ...current, [result.name]: result })),
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

  let completedSteps = 0
  if (ready) {
    completedSteps = accounts.length === 0 ? 1 : 2
    if (Object.keys(verdicts).length > 0) {
      completedSteps = 3
    }
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('My Pool')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto flex w-full max-w-6xl flex-col gap-5 pb-8'>
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
                    'Your accounts, routing channel, and API Keys are isolated from every other user.'
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent className='space-y-4'>
                <div className='bg-muted/50 rounded-lg p-4 text-sm'>
                  <p className='font-medium'>{t('Before you start')}</p>
                  <p className='text-muted-foreground mt-1'>
                    {t(
                      'An auth JSON is an account credential. Only upload accounts you own and never share the file with others.'
                    )}
                  </p>
                </div>
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
                  {t('Create my private pool')}
                </Button>
                {!state.provision_enabled && (
                  <p className='text-destructive text-sm'>
                    {t(
                      'Pool provisioning is not enabled. Contact the administrator.'
                    )}
                  </p>
                )}
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

              <Card>
                <CardHeader>
                  <CardTitle>{t('Add upstream accounts')}</CardTitle>
                  <CardDescription>
                    {t(
                      'Paste a Codex auth JSON or import a ZIP. Up to 20 accounts; ZIP uploads are limited to 8 MiB.'
                    )}
                  </CardDescription>
                </CardHeader>
                <CardContent className='space-y-3'>
                  <Textarea
                    value={content}
                    onChange={(event) => setContent(event.target.value)}
                    className='min-h-36 font-mono text-xs'
                    placeholder={t('Paste a codex auth JSON here')}
                  />
                  <div className='flex flex-wrap gap-2'>
                    <Button
                      onClick={() => addMutation.mutate()}
                      disabled={addMutation.isPending}
                    >
                      {addMutation.isPending ? (
                        <Loader2 className='size-4 animate-spin' />
                      ) : (
                        <Plus className='size-4' />
                      )}
                      {t('Add account')}
                    </Button>
                    <input
                      ref={fileInputRef}
                      type='file'
                      accept='.zip,application/zip'
                      className='hidden'
                      onChange={(event) => {
                        const file = event.target.files?.[0]
                        if (file) importMutation.mutate(file)
                      }}
                    />
                    <Button
                      variant='outline'
                      onClick={() => fileInputRef.current?.click()}
                      disabled={importMutation.isPending}
                    >
                      {importMutation.isPending ? (
                        <Loader2 className='size-4 animate-spin' />
                      ) : (
                        <FileArchive className='size-4' />
                      )}
                      {t('Import .zip')}
                    </Button>
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardHeader className='flex-row items-center justify-between space-y-0'>
                  <div>
                    <CardTitle>{t('Accounts in pool')}</CardTitle>
                    <CardDescription>
                      {t('{{count}} / 20 accounts in your private pool', {
                        count: accounts.length,
                      })}
                    </CardDescription>
                  </div>
                  <Button
                    size='sm'
                    variant='outline'
                    onClick={() => refreshAllMutation.mutate()}
                    disabled={
                      accounts.length === 0 || refreshAllMutation.isPending
                    }
                  >
                    <RefreshCw
                      className={
                        refreshAllMutation.isPending
                          ? 'size-4 animate-spin'
                          : 'size-4'
                      }
                    />
                    {t('Refresh usage')}
                  </Button>
                </CardHeader>
                <CardContent>
                  {(() => {
                    if (accountsQuery.isLoading) {
                      return (
                        <div className='flex items-center gap-2 py-8'>
                          <Loader2 className='size-4 animate-spin' />
                          {t('Loading...')}
                        </div>
                      )
                    }
                    if (accounts.length === 0) {
                      return (
                        <div className='border-border bg-muted/20 rounded-lg border border-dashed p-8 text-center'>
                          <Users className='text-muted-foreground mx-auto size-8' />
                          <p className='mt-3 font-medium'>
                            {t('No accounts yet.')}
                          </p>
                          <p className='text-muted-foreground mt-1 text-sm'>
                            {t(
                              'Add at least one account, then verify it before creating an API Key.'
                            )}
                          </p>
                        </div>
                      )
                    }
                    return (
                      <div className='divide-border divide-y'>
                        {accounts.map((file) => {
                          const status = accountStatus(file)
                          const verdict = verdicts[file.name]
                          const quota = usage[file.name]
                          return (
                            <div
                              key={file.name}
                              className='flex flex-col gap-3 py-4 first:pt-0 last:pb-0 lg:flex-row lg:items-center'
                            >
                              <div className='min-w-0 flex-1'>
                                <div className='flex flex-wrap items-center gap-2'>
                                  <p className='truncate font-medium'>
                                    {file.email || file.account || file.name}
                                  </p>
                                  <StatusBadge
                                    label={t(status.label)}
                                    variant={status.variant}
                                    copyable={false}
                                  />
                                  {verdict && (
                                    <Badge variant='outline'>
                                      {t(verdict.verdict)}
                                    </Badge>
                                  )}
                                </div>
                                <p className='text-muted-foreground mt-1 truncate text-xs'>
                                  {file.name}
                                </p>
                              </div>
                              <div className='grid grid-cols-2 gap-3 text-xs sm:grid-cols-4 lg:w-[360px]'>
                                <div>
                                  <p className='text-muted-foreground'>
                                    {t('Plan')}
                                  </p>
                                  <p className='mt-1 font-medium'>
                                    {quota?.plan || file.account_type || '—'}
                                  </p>
                                </div>
                                <div>
                                  <p className='text-muted-foreground'>
                                    {t('5h usage')}
                                  </p>
                                  <p className='mt-1 font-medium'>
                                    {percent(quota?.five_hour_used_percent)}
                                  </p>
                                </div>
                                <div>
                                  <p className='text-muted-foreground'>
                                    {t('Weekly usage')}
                                  </p>
                                  <p className='mt-1 font-medium'>
                                    {percent(quota?.weekly_used_percent)}
                                  </p>
                                </div>
                                <div>
                                  <p className='text-muted-foreground'>
                                    {t('Reset credits')}
                                  </p>
                                  <p className='mt-1 font-medium'>
                                    {quota?.reset_credits ?? '—'}
                                  </p>
                                </div>
                              </div>
                              <div className='flex flex-wrap gap-1 lg:justify-end'>
                                <Button
                                  size='icon-sm'
                                  variant='ghost'
                                  title={t('Verify')}
                                  onClick={() =>
                                    verifyMutation.mutate(file.name)
                                  }
                                  disabled={
                                    verifyMutation.isPending &&
                                    verifyMutation.variables === file.name
                                  }
                                >
                                  {verifyMutation.isPending &&
                                  verifyMutation.variables === file.name ? (
                                    <Loader2 className='size-4 animate-spin' />
                                  ) : (
                                    <ShieldCheck className='size-4' />
                                  )}
                                </Button>
                                <Button
                                  size='icon-sm'
                                  variant='ghost'
                                  title={t('Refresh usage')}
                                  onClick={() =>
                                    refreshMutation.mutate(file.name)
                                  }
                                  disabled={
                                    refreshMutation.isPending &&
                                    refreshMutation.variables === file.name
                                  }
                                >
                                  <Gauge className='size-4' />
                                </Button>
                                <Button
                                  size='icon-sm'
                                  variant='ghost'
                                  title={
                                    file.disabled ? t('Enable') : t('Disable')
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
                                  size='icon-sm'
                                  variant='ghost'
                                  title={t('Reset quota')}
                                  onClick={() => setResetTarget(file)}
                                >
                                  <RotateCcw className='size-4' />
                                </Button>
                                <Button
                                  size='icon-sm'
                                  variant='ghost'
                                  className='text-destructive'
                                  title={t('Delete')}
                                  onClick={() => setDeleteTarget(file)}
                                >
                                  <Trash2 className='size-4' />
                                </Button>
                              </div>
                            </div>
                          )
                        })}
                      </div>
                    )
                  })()}
                </CardContent>
              </Card>
            </>
          )}
        </div>
      </SectionPageLayout.Content>

      <AlertDialog
        open={!!deleteTarget}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Delete account?')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'This removes the credential from your private pool. This action cannot be undone.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
            <AlertDialogAction
              onClick={() =>
                deleteTarget && deleteMutation.mutate(deleteTarget.name)
              }
            >
              {t('Delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <AlertDialog
        open={!!resetTarget}
        onOpenChange={(open) => !open && setResetTarget(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('Consume one reset credit?')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'This manually resets the account usage window and consumes one available ChatGPT reset credit.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
            <AlertDialogAction
              onClick={() =>
                resetTarget && resetMutation.mutate(resetTarget.name)
              }
            >
              {t('Reset quota')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </SectionPageLayout>
  )
}

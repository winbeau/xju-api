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
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { AlertTriangle, Boxes, Loader2 } from 'lucide-react'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import {
  getPrivatePool,
  listPrivatePoolAccounts,
} from '@/features/private-pool/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { ApiKeysDialogs } from './components/api-keys-dialogs'
import { ApiKeysPrimaryButtons } from './components/api-keys-primary-buttons'
import { ApiKeysProvider, useApiKeys } from './components/api-keys-provider'
import { ApiKeysTable } from './components/api-keys-table'

function PrivatePoolStatusBanner() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const isRoot = (user?.role ?? 0) >= ROLE.SUPER_ADMIN
  const stateQuery = useQuery({
    queryKey: ['private-pool'],
    queryFn: getPrivatePool,
    enabled: !isRoot,
    refetchInterval: (query) =>
      query.state.data?.status === 'provisioning' ? 2000 : false,
  })
  const accountsQuery = useQuery({
    queryKey: ['private-pool', 'accounts'],
    queryFn: listPrivatePoolAccounts,
    enabled: !isRoot && stateQuery.data?.status === 'ready',
  })

  if (isRoot || stateQuery.isLoading) return null
  const state = stateQuery.data
  const accounts = accountsQuery.data ?? []
  if (state?.status === 'ready' && accounts.length > 0) return null

  const provisioning = state?.status === 'provisioning'
  const failed = state?.status === 'error'
  let title: string
  let description: string
  let StatusIcon = Boxes
  let statusIconClass = 'text-primary mt-0.5 size-5 shrink-0'
  if (provisioning) {
    title = t('Your private pool is being created')
    description = t(
      'API Key creation will be available as soon as the private routing channel is ready.'
    )
    StatusIcon = Loader2
    statusIconClass += ' animate-spin'
  } else if (failed) {
    title = t('Your private pool needs attention')
    description = state.error || t('Open My Pool to retry provisioning.')
    StatusIcon = AlertTriangle
    statusIconClass = 'text-destructive mt-0.5 size-5 shrink-0'
  } else if (state?.status === 'ready') {
    title = t('Add an account before creating an API Key')
    description = t('Your pool is ready but has no upstream accounts yet.')
  } else {
    title = t('Create your private pool before creating an API Key')
    description = t(
      'Every API Key you create will be locked to your own isolated account pool.'
    )
  }

  return (
    <div className='border-border bg-muted/30 mb-4 flex flex-col gap-3 rounded-lg border p-4 sm:flex-row sm:items-center sm:justify-between'>
      <div className='flex gap-3'>
        <StatusIcon className={statusIconClass} />
        <div>
          <p className='text-sm font-medium'>{title}</p>
          <p className='text-muted-foreground mt-1 text-xs'>{description}</p>
        </div>
      </div>
      {!provisioning && (
        <Button size='sm' variant='outline' render={<Link to='/my-pool' />}>
          {t('Open My Pool')}
        </Button>
      )}
    </div>
  )
}

function ApiKeysContent({ initialCreate }: { initialCreate: boolean }) {
  const { t } = useTranslation()
  const { setOpen } = useApiKeys()

  useEffect(() => {
    if (initialCreate) setOpen('create')
  }, [initialCreate, setOpen])

  return (
    <>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>{t('API Keys')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <ApiKeysPrimaryButtons />
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <PrivatePoolStatusBanner />
          <ApiKeysTable />
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <ApiKeysDialogs />
    </>
  )
}

export function ApiKeys({
  initialCreate = false,
}: {
  initialCreate?: boolean
}) {
  return (
    <ApiKeysProvider>
      <ApiKeysContent initialCreate={initialCreate} />
    </ApiKeysProvider>
  )
}

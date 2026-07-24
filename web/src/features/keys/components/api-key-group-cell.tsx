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
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StatusBadge } from '@/components/status-badge'

import { updateApiKeyGroup } from '../api'
import { ERROR_MESSAGES, SUCCESS_MESSAGES } from '../constants'
import type { ApiKeyGroupOption } from '../lib'
import type { ApiKey } from '../types'
import { ApiKeyGroupCombobox } from './api-key-group-combobox'
import { useApiKeys } from './api-keys-provider'

export function ApiKeyGroupCell({
  apiKey,
  options,
  isLoading,
}: {
  apiKey: ApiKey
  options: ApiKeyGroupOption[]
  isLoading: boolean
}) {
  const { t } = useTranslation()
  const { triggerRefresh } = useApiKeys()
  const mutation = useMutation({
    mutationFn: async (group: string) => {
      const result = await updateApiKeyGroup(apiKey.id, group)
      if (!result.success) {
        throw new Error(result.message || t(ERROR_MESSAGES.UPDATE_FAILED))
      }
      return result
    },
    onSuccess: () => {
      toast.success(t(SUCCESS_MESSAGES.API_KEY_UPDATED))
      triggerRefresh()
    },
    onError: (error: Error) => {
      toast.error(error.message || t(ERROR_MESSAGES.UPDATE_FAILED))
    },
  })

  return (
    <div className='flex min-w-0 items-center gap-1.5'>
      <div className='min-w-0 flex-1'>
        <ApiKeyGroupCombobox
          compact
          options={options}
          value={apiKey.group || ''}
          onValueChange={(group) => {
            if (group !== apiKey.group) mutation.mutate(group)
          }}
          placeholder={apiKey.group || '-'}
          disabled={isLoading || mutation.isPending || options.length === 0}
        />
      </div>
      {apiKey.cross_group_retry && (
        <StatusBadge label={t('Cross-group')} variant='info' copyable={false} />
      )}
    </div>
  )
}

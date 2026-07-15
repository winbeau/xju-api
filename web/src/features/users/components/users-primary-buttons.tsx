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
import { Plus, Ticket } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { InviteCodeDialog } from '@/features/invite-codes/invite-code-dialog'

import { useUsers } from './users-provider'

export function UsersPrimaryButtons() {
  const { t } = useTranslation()
  const { setOpen, setCurrentRow } = useUsers()
  const [inviteOpen, setInviteOpen] = useState(false)

  const handleCreate = () => {
    setCurrentRow(null)
    setOpen('create')
  }

  return (
    <div className='flex gap-2'>
      {/* xju-api:inject — 一次性邀请码生成入口(features/invite-codes/),
          放用户页是因为运营视角"发邀请码"与"管用户"同属一个动作面。 */}
      <Button
        size='sm'
        variant='outline'
        onClick={() => setInviteOpen(true)}
      >
        <Ticket className='h-4 w-4' />
        {t('Generate invite code')}
      </Button>
      <Button size='sm' onClick={handleCreate}>
        <Plus className='h-4 w-4' />
        {t('Add User')}
      </Button>
      <InviteCodeDialog open={inviteOpen} onOpenChange={setInviteOpen} />
    </div>
  )
}

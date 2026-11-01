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
import { Edit, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'

export function AppManagedBadge() {
  const { t } = useTranslation()
  return (
    <StatusBadge label={t('App-managed')} variant='info' copyable={false} />
  )
}

export function ManagedKeyValue() {
  const { t } = useTranslation()
  return (
    <span className='text-muted-foreground'>{t('Managed by GC Work')}</span>
  )
}

export function ManagedKeyActions() {
  const { t } = useTranslation()
  return (
    <div className='-ml-1.5 flex items-center gap-1'>
      <Button variant='ghost' size='icon-sm' disabled aria-label={t('Edit')}>
        <Edit />
      </Button>
      <Button variant='ghost' size='icon-sm' disabled aria-label={t('Delete')}>
        <Trash2 />
      </Button>
    </div>
  )
}

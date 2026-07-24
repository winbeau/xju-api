/*
Copyright (C) 2026 xju-api contributors

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { Bell, Clock3, RefreshCw } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { AnnouncementsSection } from '@/features/system-settings/content/announcements-section'
import { useSystemOptions } from '@/features/system-settings/hooks/use-system-options'
import { NoticeSection } from '@/features/system-settings/maintenance/notice-section'

function isEnabled(value: string | undefined, fallback: boolean): boolean {
  if (value === undefined) return fallback
  return value === 'true' || value === '1'
}

export function AnnouncementPublishing() {
  const { t } = useTranslation()
  const { data, isLoading, isError, refetch, isFetching } = useSystemOptions()

  const settings = useMemo(() => {
    const optionMap = new Map(
      (data?.data ?? []).map((option) => [option.key, option.value])
    )

    return {
      notice: optionMap.get('Notice') ?? '',
      timeline:
        optionMap.get('console_setting.announcements') ??
        optionMap.get('Announcements') ??
        '[]',
      timelineEnabled: isEnabled(
        optionMap.get('console_setting.announcements_enabled'),
        true
      ),
    }
  }, [data?.data])

  const failed = isError || data?.success === false
  const renderEditor = () => {
    if (isLoading) {
      return (
        <div className='text-muted-foreground flex min-h-40 items-center justify-center text-sm'>
          {t('Loading...')}
        </div>
      )
    }

    if (failed) {
      return (
        <div className='flex min-h-40 flex-col items-center justify-center gap-3 rounded-xl border'>
          <p className='text-muted-foreground text-sm'>
            {data?.message || t('Failed to load')}
          </p>
          <Button
            type='button'
            size='sm'
            variant='outline'
            onClick={() => void refetch()}
            disabled={isFetching}
          >
            <RefreshCw data-icon='inline-start' />
            {t('Retry')}
          </Button>
        </div>
      )
    }

    return (
      <Tabs defaultValue='notice' className='gap-4'>
        <TabsList>
          <TabsTrigger value='notice'>
            <Bell data-icon='inline-start' />
            {t('Notification Announcement')}
          </TabsTrigger>
          <TabsTrigger value='timeline'>
            <Clock3 data-icon='inline-start' />
            {t('Timeline Announcement')}
          </TabsTrigger>
        </TabsList>

        <TabsContent value='notice'>
          <Card>
            <CardContent>
              <NoticeSection
                defaultValue={settings.notice}
                inlineActions
                title={t('Notification Announcement')}
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value='timeline'>
          <Card>
            <CardContent>
              <AnnouncementsSection
                enabled={settings.timelineEnabled}
                data={settings.timeline}
                title={t('Timeline Announcement')}
              />
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    )
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Announcement Publishing')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto flex w-full max-w-6xl flex-col gap-4'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Manage the notice and timeline shown in the header notification center. Markdown and sanitized HTML are supported with no content length limit.'
            )}
          </p>

          {renderEditor()}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

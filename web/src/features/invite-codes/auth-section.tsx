/*
Copyright (C) 2026 xju-api contributors

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormLabel,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '@/features/system-settings/components/settings-form-layout'
import { SettingsPageFormActions } from '@/features/system-settings/components/settings-page-context'
import { SettingsSection } from '@/features/system-settings/components/settings-section'
import { useResetForm } from '@/features/system-settings/hooks/use-reset-form'
import { useUpdateOption } from '@/features/system-settings/hooks/use-update-option'

// xju-api:new — 邀请码横切收口(REFACTOR-PLAN §5.1):InviteCodeRequired 开关
// 从上游 BasicAuthSection 摘出,作为独立 section 走上游 section-registry 扩展点
// 注册进「系统设置 → 认证」;上游认证表单不再被就地插字段。

const inviteCodeAuthSchema = z.object({
  InviteCodeRequired: z.boolean(),
})

type InviteCodeAuthFormValues = z.infer<typeof inviteCodeAuthSchema>

type InviteCodeAuthSectionProps = {
  defaultValues: InviteCodeAuthFormValues
}

export function InviteCodeAuthSection({
  defaultValues,
}: InviteCodeAuthSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const form = useForm<InviteCodeAuthFormValues>({
    resolver: zodResolver(inviteCodeAuthSchema),
    defaultValues,
  })

  useResetForm(form, defaultValues)

  const onSubmit = async (data: InviteCodeAuthFormValues) => {
    if (data.InviteCodeRequired !== defaultValues.InviteCodeRequired) {
      await updateOption.mutateAsync({
        key: 'InviteCodeRequired',
        value: data.InviteCodeRequired,
      })
    }
  }

  return (
    <SettingsSection title={t('Invite-only Registration')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
          />
          <FormField
            control={form.control}
            name='InviteCodeRequired'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Invite-only Registration')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Require a valid invite code (an existing user’s code) to register'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

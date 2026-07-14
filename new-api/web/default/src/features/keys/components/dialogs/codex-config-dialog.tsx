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
import { Check, Copy } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { copyToClipboard } from '@/lib/copy-to-clipboard'

import { getPublicServerAddress } from '../../lib/server-address'

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** The unmasked key, resolved by the caller before the dialog opens. */
  tokenKey: string
}


function buildConfigToml(baseUrl: string): string {
  return `model_provider = "OpenAI"
model = "gpt-5.5"
review_model = "gpt-5.5"
model_reasoning_effort = "xhigh"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true

[model_providers.OpenAI]
name = "OpenAI"
base_url = "${baseUrl}/v1"
wire_api = "responses"
requires_openai_auth = true

[features]
goals = true`
}

function buildAuthJson(tokenKey: string): string {
  // The resolver already prefixes `sk-` (see api-keys-provider), so only add
  // it when it is somehow missing — never blindly, or the key comes out `sk-sk-`.
  const key = tokenKey.startsWith('sk-') ? tokenKey : `sk-${tokenKey}`
  return JSON.stringify({ OPENAI_API_KEY: key }, null, 2)
}

function CopyBlock(props: { title: string; hint: string; content: string }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)

  const handleCopy = async () => {
    const ok = await copyToClipboard(props.content)
    if (!ok) return
    setCopied(true)
    window.setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className='min-w-0'>
      <div className='mb-2 flex items-center justify-between gap-2'>
        <div className='min-w-0'>
          <p className='text-sm font-medium'>{props.title}</p>
          <p className='text-muted-foreground text-xs'>{props.hint}</p>
        </div>
        <Button
          variant='outline'
          size='sm'
          onClick={handleCopy}
          className='shrink-0'
        >
          {copied ? <Check /> : <Copy />}
          {copied ? t('Copied') : t('Copy')}
        </Button>
      </div>
      <pre className='border-border bg-muted text-foreground max-h-64 overflow-auto rounded-md border p-3 font-mono text-xs leading-relaxed'>
        {props.content}
      </pre>
    </div>
  )
}

/**
 * Hands the user a ready-to-paste Codex CLI configuration for this key.
 *
 * The base URL is read from the deployment rather than hard-coded, so the
 * snippet is correct wherever this instance is hosted.
 */
export function CodexConfigDialog(props: Props) {
  const { t } = useTranslation()
  const baseUrl = getPublicServerAddress()

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>{t('Codex Config')}</DialogTitle>
          <DialogDescription>
            {t(
              'Works for both the Codex CLI and the Codex GUI — the config is the same. The API key below is this key, in full — treat it as a secret.'
            )}
          </DialogDescription>
        </DialogHeader>

        <div className='grid gap-5'>
          <CopyBlock
            title={t('config.toml')}
            hint={t('Paste at the top of ~/.codex/config.toml')}
            content={buildConfigToml(baseUrl)}
          />
          <CopyBlock
            title={t('auth.json')}
            hint={t('Save as ~/.codex/auth.json')}
            content={buildAuthJson(props.tokenKey)}
          />
        </div>
      </DialogContent>
    </Dialog>
  )
}

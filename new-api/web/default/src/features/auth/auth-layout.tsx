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
import { useTranslation } from 'react-i18next'

import { IconCodex } from '@/assets/custom/icon-codex'
import { Skeleton } from '@/components/ui/skeleton'
import { useSystemConfig } from '@/hooks/use-system-config'

type AuthLayoutProps = {
  children: React.ReactNode
}

/**
 * Two-column auth shell: a calm brand panel on the left (dot-grid texture, no
 * decorative glows), the form on the right. Copy is deliberately plain — what
 * the service is and how you use it, no marketing slogans.
 *
 * The panel collapses below `lg` — on a phone the form is the whole point and
 * the decoration would just push it below the fold.
 */
export function AuthLayout({ children }: AuthLayoutProps) {
  const { t } = useTranslation()
  const { systemName, logo, loading } = useSystemConfig()

  return (
    <div className='grid h-svh lg:grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)]'>
      {/* ── Brand panel ─────────────────────────────────────────────── */}
      <aside className='bg-muted border-border relative hidden overflow-hidden border-r lg:flex lg:flex-col lg:justify-center lg:px-16 xl:px-24'>
        {/* Dot grid — the texture that keeps a flat field from reading as an
         * empty div. Kept at 6% ink so it registers without pattern. */}
        <div
          aria-hidden
          className='pointer-events-none absolute inset-0 [background-image:radial-gradient(color-mix(in_oklch,var(--foreground)_6%,transparent)_1px,transparent_1px)] [background-size:20px_20px]'
        />

        <div className='relative z-10 max-w-lg'>
          <div className='mb-10 flex items-center gap-2.5'>
            {loading ? (
              <Skeleton className='size-7 rounded-md' />
            ) : (
              <img
                src={logo}
                alt={t('Logo')}
                className='size-7 rounded-md object-cover'
              />
            )}
            {loading ? (
              <Skeleton className='h-5 w-24' />
            ) : (
              <span className='text-[17px] font-semibold tracking-tight'>
                {systemName}
              </span>
            )}
          </div>

          <h1 className='text-[clamp(1.6rem,2.4vw,2.1rem)] leading-[1.25] font-semibold tracking-tight text-balance'>
            {t('An OpenAI-compatible AI API relay')}
          </h1>
          <p className='text-muted-foreground mt-4 max-w-md text-[15px] leading-relaxed'>
            {t(
              'Buy a day, three-day, or week card, get an API key, and use it in clients like Codex and Cherry Studio — the same OpenAI API you already use.'
            )}
          </p>

          {/* Supported clients — plain logos, no numbers, no slogans. */}
          <div className='mt-10'>
            <p className='text-muted-foreground/70 mb-3 text-[11px] font-medium tracking-[0.08em] uppercase'>
              {t('Works with')}
            </p>
            <div className='flex flex-wrap items-center gap-2'>
              <span className='border-border/60 bg-background/40 text-foreground/80 inline-flex items-center gap-2 rounded-full border px-3.5 py-1.5 text-[13px] font-medium'>
                <IconCodex className='size-4' />
                Codex
              </span>
              <span className='border-border/60 bg-background/40 text-foreground/80 inline-flex items-center gap-2 rounded-full border px-3.5 py-1.5 text-[13px] font-medium'>
                <img
                  src='https://cherry-ai.com/favicon.ico'
                  alt=''
                  aria-hidden
                  className='size-4 rounded object-contain'
                  onError={(e) => {
                    e.currentTarget.style.display = 'none'
                  }}
                />
                Cherry Studio
              </span>
              <span className='border-border/60 bg-background/40 text-foreground/80 inline-flex items-center gap-2 rounded-full border px-3.5 py-1.5 text-[13px] font-medium'>
                <img
                  src='https://ccswitch.io/favicon.png'
                  alt=''
                  aria-hidden
                  className='size-4 rounded object-contain'
                  onError={(e) => {
                    e.currentTarget.style.display = 'none'
                  }}
                />
                CC Switch
              </span>
              <span className='text-muted-foreground/70 inline-flex items-center rounded-full px-2 py-1.5 text-[13px]'>
                {t('and other OpenAI-compatible clients')}
              </span>
            </div>
          </div>
        </div>
      </aside>

      {/* ── Form column ─────────────────────────────────────────────── */}
      <main className='flex items-center justify-center overflow-y-auto px-4 py-10'>
        <div className='flex w-full max-w-[400px] flex-col justify-center'>
          {/* Brand repeats here only on small screens, where the panel is gone. */}
          <div className='mb-8 flex items-center gap-2.5 lg:hidden'>
            {loading ? (
              <Skeleton className='size-6 rounded-md' />
            ) : (
              <img
                src={logo}
                alt={t('Logo')}
                className='size-6 rounded-md object-cover'
              />
            )}
            {loading ? (
              <Skeleton className='h-5 w-24' />
            ) : (
              <span className='text-[15px] font-semibold tracking-tight'>
                {systemName}
              </span>
            )}
          </div>
          {children}
        </div>
      </main>
    </div>
  )
}

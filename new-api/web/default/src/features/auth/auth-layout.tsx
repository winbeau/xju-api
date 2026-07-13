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

import { Skeleton } from '@/components/ui/skeleton'
import { useSystemConfig } from '@/hooks/use-system-config'

type AuthLayoutProps = {
  children: React.ReactNode
}

/**
 * Two-column auth shell, mirroring xju-feiyue's login: a brand panel on the
 * left (beige canvas, dot grid, warm/cool glows, editorial serif headline)
 * and the form on the right against plain white.
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
        {/* Dot grid — the texture that keeps a flat beige field from reading
         * as an empty div. Kept at 6% ink so it registers without pattern. */}
        <div
          aria-hidden
          className='pointer-events-none absolute inset-0 [background-image:radial-gradient(color-mix(in_oklch,var(--foreground)_6%,transparent)_1px,transparent_1px)] [background-size:20px_20px]'
        />
        {/* Two soft glows (warm + cool) at low alpha. This is the one place
         * the system permits a gradient: it never touches text or controls. */}
        <div
          aria-hidden
          className='pointer-events-none absolute inset-0'
          style={{
            background: [
              'radial-gradient(ellipse 55% 45% at 15% 20%, color-mix(in oklch, var(--warning) 7%, transparent) 0%, transparent 70%)',
              'radial-gradient(ellipse 50% 45% at 85% 80%, color-mix(in oklch, var(--info) 6%, transparent) 0%, transparent 70%)',
            ].join(', '),
          }}
        />

        <div className='relative z-10 max-w-lg'>
          <div className='mb-10 flex items-center gap-2.5'>
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

          <h1 className='font-serif text-[clamp(2rem,3.2vw,2.75rem)] leading-[1.2] font-semibold tracking-tight text-balance'>
            {t('One key.')}
            <br />
            {t('Every model.')}
          </h1>
          <p className='text-muted-foreground mt-5 max-w-md text-[15px] leading-relaxed'>
            {t(
              'A time-based key to a shared pool of frontier models. Bring your own client — the gateway speaks the API you already use.'
            )}
          </p>

          {/* Placeholder for the value props / stats you want to land here.
           * Three hairline-separated rows, no cards, no shadows. */}
          <dl className='divide-border border-border mt-12 divide-y border-t'>
            {[
              {
                term: t('Unified endpoint'),
                desc: t('OpenAI-compatible. Swap the base URL, nothing else.'),
              },
              {
                term: t('Time-based keys'),
                desc: t('Day, three-day and week cards. No metering surprises.'),
              },
              {
                term: t('Pooled upstreams'),
                desc: t('Frontier models behind one rotating account pool.'),
              },
            ].map((item) => (
              <div key={item.term} className='py-3.5'>
                <dt className='text-[13px] font-medium'>{item.term}</dt>
                <dd className='text-muted-foreground mt-0.5 text-[13px]'>
                  {item.desc}
                </dd>
              </div>
            ))}
          </dl>
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

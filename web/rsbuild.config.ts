import { readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import { defineConfig, loadEnv, type RsbuildConfig } from '@rsbuild/core'
import { pluginReact } from '@rsbuild/plugin-react'
import { pluginTailwindcss } from '@rsbuild/plugin-tailwindcss'
import { tanstackRouter } from '@tanstack/router-plugin/rspack'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

export default defineConfig(({ envMode }): RsbuildConfig => {
  const env = loadEnv({ mode: envMode, prefixes: ['VITE_'] })
  const serverUrl =
    process.env.VITE_REACT_APP_SERVER_URL ||
    env.rawPublicVars.VITE_REACT_APP_SERVER_URL ||
    'http://localhost:3000'

  const isProd = envMode === 'production'
  const devProxy = Object.fromEntries(
    (['/api', '/mj', '/pg'] as const).map((key) => [
      key,
      { target: serverUrl, changeOrigin: true },
    ])
  ) as Record<string, { target: string; changeOrigin: boolean }>

  return {
    plugins: [pluginReact(), pluginTailwindcss({ optimize: false })],
    // Rsbuild 2: replaces deprecated `performance.chunkSplit` (RSPack 2 aligned)
    splitChunks: {
      preset: 'default',
      cacheGroups: {
        'vendor-react': {
          test: /node_modules[\\/](react|react-dom)[\\/]/,
          name: 'vendor-react',
          chunks: 'all',
          priority: 0,
          enforce: true,
        },
        'vendor-ui-primitives': {
          test: /node_modules[\\/](@base-ui|@radix-ui)[\\/]/,
          name: 'vendor-ui-primitives',
          chunks: 'all',
          priority: 0,
          enforce: true,
        },
        'vendor-tanstack': {
          test: /node_modules[\\/]@tanstack[\\/]/,
          name: 'vendor-tanstack',
          chunks: 'all',
          priority: 0,
          enforce: true,
        },
      },
    },
    source: {
      entry: {
        index: './src/main.tsx',
      },
    },
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
      },
    },
    html: {
      template: './index.html',
      // Don't auto-inject an unversioned <link rel="icon" href="/favicon.ico">.
      // index.html already declares versioned favicon links (?v=…) so browsers
      // re-fetch the icon instead of serving a hard-cached old one.
      // @ts-expect-error — rsbuild 2.1.4 的 ChainedHtmlOption<string> 未收录 false,
      // 但运行期支持 false(跳过 favicon 注入);值保持不变。
      favicon: false,
    },
    server: {
      host: '0.0.0.0',
      strictPort: false,
      proxy: devProxy,
      // Opt-in HTTPS for the dev server. The backend issues its session cookie
      // with `Secure` (main.go: sessions.Options{Secure: ...}), and a browser
      // silently drops a Secure cookie on an http:// origin — so signing in
      // against a real backend through the dev proxy appears to "succeed" (the
      // login call returns 200) and then bounces straight back to /sign-in,
      // because the session was never stored. Point these at a self-signed pair
      // to reproduce the authenticated app locally:
      //   DEV_HTTPS_KEY=… DEV_HTTPS_CERT=… bun run dev
      ...(process.env.DEV_HTTPS_KEY && process.env.DEV_HTTPS_CERT
        ? {
            https: {
              key: readFileSync(process.env.DEV_HTTPS_KEY),
              cert: readFileSync(process.env.DEV_HTTPS_CERT),
            },
          }
        : {}),
    },
    output: {
      // Production optimizations
      minify: isProd,
      target: 'web',
      distPath: {
        root: 'dist',
      },
      // Rely on Rsbuild default legalComments ("linked" → per-chunk *.LICENSE.txt) in all modes.
      // Do not set "none" in production: that strips minifier-preserved third-party notices and
      // extracted license files, which some distributions require for open-source compliance.
    },
    performance: {
      // Remove console in production
      removeConsole: isProd ? ['log'] : false,
      buildCache: false,
    },
    tools: {
      rspack: {
        plugins: [
          tanstackRouter({
            target: 'react',
            // Dev: avoid per-route async chunks (reduces white flash on navigation + faster HMR feedback).
            // Prod: keep route-based code splitting.
            autoCodeSplitting: isProd,
          }),
        ],
      },
    },
  }
})

import { defineConfig } from 'vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { sentrySvelteKit } from '@sentry/sveltekit';
import tailwindcss from '@tailwindcss/vite';
import { SvelteKitPWA } from '@vite-pwa/sveltekit';

export default defineConfig({
  // Strip Sentry's tracing/debug code at build time. We run errors-only
  // (tracesSampleRate:0, no replay), but Sentry's tracing/web-vitals paths sit
  // behind runtime `if (__SENTRY_TRACING__)` checks that ship regardless. Defining
  // these Sentry build flags to `false` turns those into dead `if (false)` branches
  // Rollup drops — ~22KB gzip off the client entry chunk, no behaviour change.
  define: {
    __SENTRY_DEBUG__: false,
    __SENTRY_TRACING__: false,
  },
  build: {
    // Keep flag-icons' ~500 flag SVGs as external files instead of inlining them
    // as data-URIs into the global CSS. They're each under Vite's 4KB inline
    // threshold, so the default would embed the whole sheet (~100KB gzip) into the
    // always-loaded stylesheet even though most pages show no flags. Externalizing
    // lets the browser fetch only the handful of flags a page actually renders.
    assetsInlineLimit: (filePath) => (filePath.includes('flag-icons/flags/') ? false : undefined),
  },
  // SvelteKit owns routing/SSR; it provides the $lib alias, so the manual alias
  // is gone. sentrySvelteKit() must precede sveltekit(); tailwindcss() too.
  //
  // Source-map upload is inert without SENTRY_AUTH_TOKEN — the build still
  // succeeds, only readable minified stack traces in Sentry are skipped. When
  // enabled in ops, org/project/token come from the environment (freehire-ops),
  // never from code.
  plugins: [
    sentrySvelteKit({
      sourceMapsUploadOptions: {
        org: process.env.SENTRY_ORG,
        project: process.env.SENTRY_PROJECT,
        authToken: process.env.SENTRY_AUTH_TOKEN,
      },
    }),
    tailwindcss(),
    sveltekit(),
    // Installable PWA: precaches the built app shell (JS/CSS/fonts, manifest
    // icons) for fast repeat loads. Deliberately carries NO runtime-caching
    // entry for /api/* or navigations — every job listing, filter result and
    // /me/* response is personalized or live (and some are SSE streams), so the
    // only safe policy is "never served from cache", which for generateSW means
    // not registering a handler for them at all rather than trying to tune a
    // NetworkFirst TTL. `injectRegister: null` because svelte.config.js's CSP
    // has no 'unsafe-inline' script-src — the SW is registered from a real
    // module import in +layout.svelte instead of an auto-injected inline script.
    SvelteKitPWA({
      strategies: 'generateSW',
      registerType: 'autoUpdate',
      injectRegister: null,
      manifest: {
        // Name/description echo the WebSite JSON-LD copy in src/lib/seo.ts
        // (SITE_DESCRIPTION) — same product description, not re-derived from it,
        // since a build-config file has no access to the $lib alias.
        name: 'freehire — the open-source search engine for every job',
        short_name: 'freehire',
        description:
          'An open-source search engine for tech jobs: millions of openings indexed straight from company career boards, deduplicated and tagged with stack, seniority and location.',
        start_url: '/',
        display: 'standalone',
        background_color: '#ffffff',
        theme_color: '#0a0a0a',
        icons: [
          { src: 'pwa-192x192.png', sizes: '192x192', type: 'image/png' },
          { src: 'pwa-512x512.png', sizes: '512x512', type: 'image/png' },
          {
            src: 'pwa-maskable-512x512.png',
            sizes: '512x512',
            type: 'image/png',
            purpose: 'maskable',
          },
        ],
      },
    }),
  ],
  server: {
    port: 5173,
    strictPort: false,
    // Bind to all interfaces (not just localhost) so the dev server is
    // reachable from other devices (e.g. a phone) on the same LAN.
    host: true,
    // Proxy the API so the browser (and server-side `load` via event.fetch) only
    // ever talks to this origin. That makes dev match the same-origin production
    // deployment, so the SameSite=Lax auth cookie is sent and no CORS is needed.
    // Target overridable via VITE_API_URL. /health is proxied too, mirroring the
    // prod nginx config (design D2) so dev and prod route identically.
    proxy: {
      '/api': {
        target: process.env.VITE_API_URL ?? 'http://localhost:8080',
        changeOrigin: true,
        // `/api/v1/tools/ws` (the browser-tool wire) upgrades to a WebSocket;
        // without this the dev proxy would answer the handshake with the SPA.
        ws: true,
      },
      '/health': {
        target: process.env.VITE_API_URL ?? 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
});

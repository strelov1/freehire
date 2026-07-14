import { defineConfig } from 'vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { sentrySvelteKit } from '@sentry/sveltekit';
import tailwindcss from '@tailwindcss/vite';

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
  ],
  server: {
    port: 5173,
    strictPort: false,
    // Proxy the API so the browser (and server-side `load` via event.fetch) only
    // ever talks to this origin. That makes dev match the same-origin production
    // deployment, so the SameSite=Lax auth cookie is sent and no CORS is needed.
    // Target overridable via VITE_API_URL. /health is proxied too, mirroring the
    // prod nginx config (design D2) so dev and prod route identically.
    proxy: {
      '/api': {
        target: process.env.VITE_API_URL ?? 'http://localhost:8080',
        changeOrigin: true,
      },
      '/health': {
        target: process.env.VITE_API_URL ?? 'http://localhost:8080',
        changeOrigin: true,
      },
      // The freehire-agent backend (roy management on :8079), reached same-origin
      // so the assistant chat's httpOnly cookie is sent and the `/ws` upgrade has
      // no CORS. `ws:true` forwards the WebSocket; the rewrite strips the prefix
      // so `/assistant-api/ws` → backend `/ws`, `/assistant-api/auth/login` →
      // `/auth/login`. Prod wiring (an nginx location) is a later seam.
      '/assistant-api': {
        target: process.env.VITE_ASSISTANT_API_URL ?? 'http://127.0.0.1:8079',
        changeOrigin: true,
        ws: true,
        rewrite: (path) => path.replace(/^\/assistant-api/, ''),
      },
    },
  },
});

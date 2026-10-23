import adapter from '@sveltejs/adapter-node';
import { defineConfig } from 'vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { sentrySvelteKit } from '@sentry/sveltekit/vite';
import tailwindcss from '@tailwindcss/vite';
import { svelteOptions } from './svelte.options.ts';

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
  // SvelteKit owns routing/SSR; `#lib` is a package.json subpath import, so no manual
  // alias is needed. sentrySvelteKit() must precede sveltekit(); tailwindcss() too.
  //
  // Source-map upload is inert without SENTRY_AUTH_TOKEN — the build still
  // succeeds, only readable minified stack traces in Sentry are skipped. When
  // enabled in ops, org/project/token come from the environment (freehire-ops),
  // never from code.
  //
  // That is the ABSENT-token case, and for a long time it was the only one written
  // down while production was in the other one. A token Sentry REJECTS behaves the
  // same from out here — warning printed, build exits 0 — so **this build's exit
  // status is not evidence that anything was uploaded**, and no option set on the
  // plugin below changes that. Measured 2026-09-16: uploads stopped landing on
  // 2026-09-14 and every deploy in between was green.
  //
  // What asks instead is scripts/sentry-credential-check.mjs, run before this build
  // by release.sh (which lives in the private freehire-ops repository). Its header is
  // the canonical explanation — of the mechanism, of that date, and of the
  // release-`fileCount` measurement that looked like evidence and was not. Read it
  // there rather than trusting a second copy here to stay true; a second copy is
  // exactly what went stale last time.
  plugins: [
    sentrySvelteKit({
      // v11 flattened these onto the plugin options directly; the nested
      // `sourceMapsUploadOptions` wrapper from v10 no longer exists.
      org: process.env.SENTRY_ORG,
      project: process.env.SENTRY_PROJECT,
      authToken: process.env.SENTRY_AUTH_TOKEN,
    }),
    tailwindcss(),
    // Kit 3 reads no svelte.config.js: what used to live there is passed here.
    sveltekit({
      ...svelteOptions,
      // The frontend ships as a long-lived Node server (see design D1/D2): nginx
      // fronts it and proxies /api + /health to the Go backend, keeping the SPA
      // and API same-origin for the SameSite=Lax auth cookie.
      adapter: adapter(),

      // Poll _app/version.json so `updated` can tell an open tab that the build it
      // was served has been replaced. Without this Kit never checks, `updated.current`
      // stays false forever, and the first the tab hears of a deploy is a dynamic
      // import 404ing for a chunk the release deleted — which reaches the reader as
      // the 500 page. Five minutes: a deploy runs every few hours, and the file is a few bytes.
      version: { pollInterval: 300_000 },

      // Absolute asset paths. Relative paths only buy anything when the app can be
      // served from an unknown prefix; this one is adapter-node on its own domain.
      // (They were also how the since-removed service worker registered at
      // /jobs/<slug>/sw.js and 404ed from every page but '/'.)
      //
      // `origin` is the public site URL every canonical, sitemap and JSON-LD URL is built
      // from (they read `page.url.origin`). adapter-node used to take it from ORIGIN at
      // RUNTIME; SvelteKit 3 removed that, so it is read from ORIGIN at BUILD time and
      // baked in. Unset, the server derives the origin per request from the Host header
      // (and PROTOCOL_HEADER/HOST_HEADER when set) — correct behind our nginx, but a
      // canonical URL is then only as trustworthy as whatever Host reached it.
      paths: { relative: false, origin: process.env.ORIGIN || undefined },

      // Content-Security-Policy: defence-in-depth against stored/reflected XSS. Only
      // same-origin scripts run; SvelteKit auto-adds a per-response nonce (mode 'auto')
      // to the inline scripts IT injects (the hydration bootstrap). Inline JSON-LD
      // (<script type="application/ld+json">) is non-executable and unaffected.
      // style-src is left unset (no default-src), so styles and fonts are unrestricted.
      //
      // The anti-FOUC script in app.html is author-written, so SvelteKit does NOT
      // nonce it — it is allowed by the SHA-256 of its exact contents below. WARNING:
      // editing that <script> in app.html changes its hash and will silently break
      // EVERY no-flash pass it carries (the theme, the CLI strip, the onboarding
      // nudge): the browser blocks the whole block, nothing errors, and the only
      // symptom is a flash of the wrong theme. Recompute whenever you touch it —
      //
      //   python3 -c "import re,hashlib,base64;b=re.findall(r'<script>(.*?)</script>',
      //     open('src/app.html').read(),re.S)[0];
      //     print('sha256-'+base64.b64encode(hashlib.sha256(b.encode()).digest()).decode())"
      //
      // — and verify in a real browser: a CSP block shows up only in the console.
      // src/antiFoucCsp.test.ts fails when the two drift apart.
      csp: {
        mode: 'auto',
        directives: {
          'script-src': [
            'self',
            // Anti-FOUC script in app.html — theme + CLI promo strip + onboarding
            // nudge (see WARNING above).
            'sha256-rBKxNqi22dqRkzLy61OvvLRma7co8OUnpbQ8vSxSJQc=',
            // Google Analytics: the gtag.js host. GA now loads from the same-origin
            // bundle (#lib/analytics, consent-gated), so no inline-script hash is
            // needed — only the external host it injects.
            'https://www.googletagmanager.com',
          ],
          // Cheap defence-in-depth: pin the document base (no <base> injection) and
          // forbid legacy plugin/embed vectors.
          'base-uri': ['self'],
          'object-src': ['none'],
          // img-src is the layer under the assistant's markdown sanitizer. The chat
          // renders model output with {@html}, and model output is untrusted — it reads
          // job descriptions, browsed pages and other attacker-controlled text — so an
          // image it can be talked into writing is a no-click GET carrying whatever the
          // model held. The sanitizer (lib/markdown.ts) is the primary control;
          // this pins where an image could go if that ever regresses.
          //
          // The list is exhaustive by inspection, not by habit: the only browser-side
          // <img> sinks are CompanyLogo.svelte (the logo proxy), TemplateGallery.svelte
          // (same-origin /cv-previews/*.svg) and the /contributors pages (GitHub avatars).
          // OG cards render server-side and are never fetched under our own CSP, and job
          // descriptions carry no images at all — the ingest sanitizer strips them
          // (internal/sources/sanitize.go). `data:` is here for the handful of Tailwind
          // utilities that inline an SVG.
          // api.producthunt.com serves the Product Hunt "featured" badge embedded in
          // the footer (Footer.svelte) — a single static SVG per theme.
          // avatars.githubusercontent.com is GitHub's avatar CDN, read by the contributor
          // showcase and profile pages. Adding a host here widens what the assistant's
          // markdown could point an <img> at if the sanitizer ever regressed, so it is
          // worth naming why this one is acceptable: it is a static image CDN on a path
          // shape we do not control and it accepts no query we could smuggle data through,
          // which is the same argument the logo proxy and the badge host already sit on.
          // Left off, every avatar on those pages silently falls back to initials — the
          // page still renders, so nothing fails loudly.
          'img-src': [
            'self',
            'data:',
            'https://logo.freehire.me',
            'https://api.producthunt.com',
            'https://avatars.githubusercontent.com',
          ],
          // Pinning img-src deliberately stopped there: connect-src was considered as
          // part of the same hardening and left out, because getting it wrong fails
          // silently — error reporting and analytics simply stop — and that deserves its
          // own change with its own verification rather than riding along on this one.
          //
          // Sentry: NO connect-src is set here on purpose. With no default-src, the
          // browser does not restrict fetch/beacon, so the Sentry SDK reaches its
          // ingest host (https://*.ingest.de.sentry.io — EU region) unblocked. The
          // client SDK ships in the same-origin bundle and injects no external script,
          // so script-src needs nothing either. If a connect-src is ever introduced,
          // it MUST include the Sentry ingest host above (and GA's collect host).
          //
          // PostHog: needs no CSP entry either. Ingestion and the lazily-loaded
          // session-replay recorder both go through the same-origin /ingest reverse
          // proxy (nginx → eu.i.posthog.com; see the add-posthog-analytics design),
          // so 'self' already covers script-src and no external host is contacted. A
          // future connect-src must include /ingest (same origin).
        },
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

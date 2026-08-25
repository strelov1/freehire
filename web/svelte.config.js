import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { mdsvex } from 'mdsvex';

/** @type {import('@sveltejs/kit').Config} */
export default {
  // `.svelte` compiles as usual; `.svx` blog posts (web/src/posts/*.svx) go through
  // mdsvex, which compiles markdown + frontmatter to Svelte components at build time.
  // mdsvex output is bundled same-origin, so the strict script-src CSP below is unaffected.
  extensions: ['.svelte', '.svx'],
  preprocess: [vitePreprocess(), mdsvex({ extensions: ['.svx'] })],
  kit: {
    // The frontend ships as a long-lived Node server (see design D1/D2): nginx
    // fronts it and proxies /api + /health to the Go backend, keeping the SPA
    // and API same-origin for the SameSite=Lax auth cookie.
    adapter: adapter(),

    // Poll _app/version.json so `updated` can tell an open tab that the build it
    // was served has been replaced. Without this Kit never checks, `updated.current`
    // stays false forever, and the first the tab hears of a deploy is a dynamic
    // import 404ing for a chunk the release deleted — which reaches the reader as
    // the 500 page. The service-worker fix below removed one route to that failure;
    // this closes the other, for a tab that simply sat open across a release.
    // Five minutes: a deploy runs every few hours, and the file is a few bytes.
    version: { pollInterval: 300_000 },

    // Absolute asset paths. Kit defaults `relative` to true, which sets Vite's
    // `base` to './' — and @vite-pwa/sveltekit reads that base directly
    // (`base = viteOptions.base ?? "/"`) to build the service-worker registration.
    // The result shipped as `new Workbox('./sw.js', { scope: './' })`, which the
    // browser resolves against the CURRENT url: /sw.js from the home page, but
    // /jobs/<slug>/sw.js or /collections/<slug>/sw.js from anywhere else — 404,
    // ~5.7k a day in the SSR log. So the worker only ever registered for someone
    // who arrived at '/', and an already-installed one (scope '/', so it controls
    // the whole site) never re-registered for anyone landing deeper from search.
    // It kept serving a precached app shell naming _app/immutable chunks that the
    // next deploy had deleted, and the failed dynamic import surfaces to the user
    // as the 500 error page. Mostly seen on phones, where a session outlives a
    // deploy. nginx never logged any of it: /_app/immutable/ has access_log off,
    // and the 500 is drawn client-side over an HTTP 200.
    //
    // Relative paths only buy anything when the app can be served from an unknown
    // prefix; this one is adapter-node on its own domain.
    paths: { relative: false },

    // Content-Security-Policy: defence-in-depth against stored/reflected XSS. Only
    // same-origin scripts run; SvelteKit auto-adds a per-response nonce (mode 'auto')
    // to the inline scripts IT injects (the hydration bootstrap). Inline JSON-LD
    // (<script type="application/ld+json">) is non-executable and unaffected.
    // style-src is left unset (no default-src), so styles and fonts are unrestricted.
    //
    // The anti-FOUC script in app.html is author-written, so SvelteKit does NOT
    // nonce it — it is allowed by the SHA-256 of its exact contents below. WARNING:
    // editing that <script> in app.html changes its hash and will silently break
    // BOTH no-flash passes it carries (the theme and the Product Hunt strip): the
    // browser blocks the whole block, nothing errors, and the only symptom is a
    // flash of the wrong theme. Recompute whenever you touch it —
    //
    //   python3 -c "import re,hashlib,base64;b=re.findall(r'<script>(.*?)</script>',
    //     open('src/app.html').read(),re.S)[0];
    //     print('sha256-'+base64.b64encode(hashlib.sha256(b.encode()).digest()).decode())"
    //
    // — and verify in a real browser: a CSP block shows up only in the console.
    csp: {
      mode: 'auto',
      directives: {
        'script-src': [
          'self',
          // Anti-FOUC script in app.html — theme + Product Hunt strip + onboarding
          // nudge (see WARNING above).
          'sha256-u3FGDCCLNrppO+D5gI/BmV8qq0wTVlA/OoPesWqF1Ts=',
          // Google Analytics: the gtag.js host. GA now loads from the same-origin
          // bundle ($lib/analytics, consent-gated), so no inline-script hash is
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
        // <img> sinks are CompanyLogo.svelte (the logo proxy) and TemplateGallery.svelte
        // (same-origin /cv-previews/*.svg). OG cards render server-side and are never
        // fetched under our own CSP, and job descriptions carry no images at all — the
        // ingest sanitizer strips them (internal/sources/sanitize.go). `data:` is here
        // for the handful of Tailwind utilities that inline an SVG.
        // api.producthunt.com serves the Product Hunt "featured" badge embedded in
        // the footer (Footer.svelte) — a single static SVG per theme.
        'img-src': [
          'self',
          'data:',
          'https://logo.freehire.me',
          'https://api.producthunt.com',
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
  },
};

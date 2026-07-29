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

    // Content-Security-Policy: defence-in-depth against stored/reflected XSS. Only
    // same-origin scripts run; SvelteKit auto-adds a per-response nonce (mode 'auto')
    // to the inline scripts IT injects (the hydration bootstrap). Inline JSON-LD
    // (<script type="application/ld+json">) is non-executable and unaffected.
    // style-src is left unset (no default-src), so styles and fonts are unrestricted.
    //
    // The anti-FOUC theme script in app.html is author-written, so SvelteKit does NOT
    // nonce it — it is allowed by the SHA-256 of its exact contents below. WARNING:
    // editing that <script> in app.html changes its hash and will silently break the
    // no-flash theme load; recompute and update this hash when you touch it.
    csp: {
      mode: 'auto',
      directives: {
        'script-src': [
          'self',
          // Anti-FOUC theme script in app.html (see WARNING above).
          'sha256-qvzE1AlG+fDQlxleonlMQaOrsjjgE6qfHfnkE0pD/bo=',
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
        // model held. The sanitizer (lib/assistant/markdown.ts) is the primary control;
        // this pins where an image could go if that ever regresses.
        //
        // The list is exhaustive by inspection, not by habit: the only browser-side
        // <img> sinks are CompanyLogo.svelte (the logo proxy) and TemplateGallery.svelte
        // (same-origin /cv-previews/*.svg). OG cards render server-side and are never
        // fetched under our own CSP, and job descriptions carry no images at all — the
        // ingest sanitizer strips them (internal/sources/sanitize.go). `data:` is here
        // for the handful of Tailwind utilities that inline an SVG.
        'img-src': ['self', 'data:', 'https://logo.freehire.me'],
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

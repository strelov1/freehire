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
    // style-src/img-src are left unset (no default-src), so styles/fonts/logo.dev
    // images are unrestricted.
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
          // Google Analytics: SHA-256 of the inline gtag bootstrap in app.html,
          // plus the gtag.js host it injects. Same hash caveat as the theme
          // script — editing that <script> changes this hash and silently
          // breaks GA data collection.
          'sha256-tf1xyFn3XkSa3dEPltHXXh1gwP9PXgX19twcwLYVszU=',
          'https://www.googletagmanager.com',
        ],
        // Cheap defence-in-depth: pin the document base (no <base> injection) and
        // forbid legacy plugin/embed vectors.
        'base-uri': ['self'],
        'object-src': ['none'],
        // Sentry: NO connect-src is set here on purpose. With no default-src, the
        // browser does not restrict fetch/beacon, so the Sentry SDK reaches its
        // ingest host (https://*.ingest.de.sentry.io — EU region) unblocked. The
        // client SDK ships in the same-origin bundle and injects no external script,
        // so script-src needs nothing either. If a connect-src is ever introduced,
        // it MUST include the Sentry ingest host above (and GA's collect host).
      },
    },
  },
};

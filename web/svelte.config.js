import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
export default {
  preprocess: vitePreprocess(),
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
        'script-src': ['self', 'sha256-qvzE1AlG+fDQlxleonlMQaOrsjjgE6qfHfnkE0pD/bo='],
        // Cheap defence-in-depth: pin the document base (no <base> injection) and
        // forbid legacy plugin/embed vectors.
        'base-uri': ['self'],
        'object-src': ['none'],
      },
    },
  },
};

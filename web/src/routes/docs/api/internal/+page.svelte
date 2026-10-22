<script lang="ts">
  // The internal (session-cookie-only) API reference — the counterpart of
  // ../+page.svelte for endpoints a personal API key cannot reach: saved
  // searches, notifications, moderator tools, the browser extension. Not
  // promoted from the footer or any agent-facing landing page (an agent can
  // never call anything documented here), but not secret either — noindex only,
  // via Seo's `robots` prop, so it stays reachable by direct link without
  // competing with /docs/api in search results.
  import ScalarReference from '#lib/components/ScalarReference.svelte';
  import Seo from '#lib/components/Seo.svelte';
  import { scalarConfigFromUrl, SCALAR_INTERNAL_SPEC_URL } from '#lib/docs/scalarConfig.js';

  let { data } = $props();
</script>

<Seo
  title="freehire API — internal reference"
  description="The freehire endpoints that require a signed-in browser session: saved searches, notifications, moderator tools, and the browser extension. For the endpoints an API key can call, see the public API reference."
  robots="noindex"
/>

<ScalarReference scalarHtml={data.scalarHtml} config={() => scalarConfigFromUrl(SCALAR_INTERNAL_SPEC_URL)} />

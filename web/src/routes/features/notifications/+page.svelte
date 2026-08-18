<script lang="ts">
  import { page } from '$app/state';
  import Seo from '$lib/components/Seo.svelte';
  import NotificationsLandingView from '$lib/components/NotificationsLandingView.svelte';
  import { faqPageJsonLd, jsonLdScript } from '$lib/seo';
  import { NOTIFICATIONS_FAQ } from '$lib/notificationsFaq';

  const canonical = $derived(`${page.url.origin}/features/notifications`);
  // The FAQ block and this payload render from the same NOTIFICATIONS_FAQ array, so the
  // structured data can never disagree with what the page shows.
  const jsonLd = $derived(jsonLdScript([faqPageJsonLd(NOTIFICATIONS_FAQ)]));
</script>

<Seo
  title="Notifications — job alerts and tracking nudges, on your channel | freehire"
  description="Save a search and get told about a new match instantly or as a daily digest, over email, Telegram or push. The same settings carry your tracking nudges — a saved job you haven't applied to, an application gone quiet, an interview coming up — with quiet hours to keep it out of your evening."
  {canonical}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <NotificationsLandingView />
</div>

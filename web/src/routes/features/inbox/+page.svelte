<script lang="ts">
  import { page } from '$app/state';
  import InboxLandingView from '$lib/components/InboxLandingView.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import { INBOX_FAQ } from '$lib/inboxFaq';
  import { faqPageJsonLd, jsonLdScript } from '$lib/seo';

  const canonical = $derived(`${page.url.origin}/features/inbox`);
  // The FAQ block and this payload render from the same INBOX_FAQ array, so the
  // structured data can never disagree with what the page shows.
  const jsonLd = $derived(jsonLdScript([faqPageJsonLd(INBOX_FAQ)]));
</script>

<Seo
  title="Inbox — recruiter replies, sorted onto your board | freehire"
  description="freehire reads your job mail and tags what it says — received, rejected, interview, information requested — then attaches each reply to the application it belongs to and moves the card forward. Connect Gmail read-only, or claim a freehire address and apply with it."
  {canonical}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <InboxLandingView />
</div>

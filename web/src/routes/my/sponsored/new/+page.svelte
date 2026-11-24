<script lang="ts">
  import { page } from '$app/state';
  import { messages } from '#lib/components/Sponsored.messages.js';
  import PromoteJobForm from '#lib/components/PromoteJobForm.svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';

  const s = $derived(t(messages, locale()));
</script>

<svelte:head><title>{s.promote} — freehire</title></svelte:head>

<!-- Under /my so the account shell's sign-in redirect (with return to this URL) applies. -->
<PromoteJobForm slug={page.url.searchParams.get('job')} />

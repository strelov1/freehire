<script lang="ts">
  import { messages } from '$lib/components/ReferralsView.messages';
  import ReferralsView from '$lib/components/ReferralsView.svelte';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t } from '$lib/i18n/t';

  // The title and heading share the view's catalog rather than getting a second
  // one — every other string on this page is the view's.
  const s = $derived(t(messages, locale()));
</script>

<svelte:head><title>{s.headTitle}</title></svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex. -->
<div class="max-w-3xl">
  <h1 class="mb-4 text-lg font-semibold tracking-tight">{s.title}</h1>
  <ReferralsView />
</div>
